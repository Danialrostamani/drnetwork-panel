package tgbot

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/logger"

	"github.com/op/go-logging"
)

func TestMain(m *testing.M) {
	logger.InitLogger(logging.ERROR)
	m.Run()
}

func TestParseAdminsAcceptsMixedSeparatorsAndDropsJunk(t *testing.T) {
	got := parseAdmins("123, 456;789\n123 abc 0 -5")
	want := []int64{123, 456, 789, -5}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseAdmins = %v, want %v", got, want)
	}
}

func TestParseCommand(t *testing.T) {
	cmd, arg := parseCommand("/Clients@MyBot  alice smith ")
	if cmd != "clients" || arg != "alice smith" {
		t.Fatalf("got %q %q", cmd, arg)
	}
	if cmd, _ := parseCommand("hello"); cmd != "" {
		t.Fatalf("plain text parsed as command %q", cmd)
	}
}

func TestSplitMessageKeepsLinesAndRespectsLimit(t *testing.T) {
	text := strings.Repeat("abcdefghi\n", 10)
	parts := splitMessage(text, 25)
	if len(parts) < 4 {
		t.Fatalf("expected several parts, got %d", len(parts))
	}
	for _, p := range parts {
		if len([]rune(p)) > 25 {
			t.Fatalf("part too long: %d", len([]rune(p)))
		}
	}
	if joined := strings.Join(parts, "\n"); strings.Count(joined, "abcdefghi") != 10 {
		t.Fatal("content lost while splitting")
	}
}

func TestScrubHidesTokenFromErrors(t *testing.T) {
	b := &bot{cfg: botConfig{Token: "123:SECRET"}}
	if got := b.scrub(`Post "https://api.telegram.org/bot123:SECRET/getMe": EOF`); strings.Contains(got, "SECRET") {
		t.Fatalf("token leaked: %s", got)
	}
}

type sent struct {
	ChatID int64  `json:"chat_id"`
	Text   string `json:"text"`
}

func fakeTelegram(t *testing.T) (*bot, func() []sent) {
	t.Helper()
	var mu sync.Mutex
	var messages []sent
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/sendMessage") {
			var m sent
			_ = json.NewDecoder(r.Body).Decode(&m)
			mu.Lock()
			messages = append(messages, m)
			mu.Unlock()
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	}))
	t.Cleanup(srv.Close)
	previous := apiBase
	apiBase = srv.URL
	t.Cleanup(func() { apiBase = previous })
	b := newBot(botConfig{Token: "1:abc", Admins: []int64{42}, Lang: "en"})
	return b, func() []sent {
		mu.Lock()
		defer mu.Unlock()
		return append([]sent(nil), messages...)
	}
}

func privateMessage(from int64, text string) update {
	var u update
	raw := `{"update_id":1,"message":{"text":` + mustJSON(text) + `,"chat":{"id":` + itoa(from) + `,"type":"private"},"from":{"id":` + itoa(from) + `}}}`
	_ = json.Unmarshal([]byte(raw), &u)
	return u
}

func mustJSON(s string) string { b, _ := json.Marshal(s); return string(b) }
func itoa(n int64) string      { b, _ := json.Marshal(n); return string(b) }

func TestStrangersOnlyGetTheirIDAndAdminsGetData(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "bot.db")); err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().Create(&model.Client{Name: "alice", Enable: true, Volume: 1000, Up: 500, Down: 450, Inbounds: json.RawMessage(`[]`), Links: json.RawMessage(`[]`)}).Error; err != nil {
		t.Fatal(err)
	}
	b, got := fakeTelegram(t)
	ctx := context.Background()

	b.handle(ctx, privateMessage(7, "/clients"))
	b.handle(ctx, privateMessage(7, "/id"))
	b.handle(ctx, privateMessage(42, "/clients"))
	b.handle(ctx, privateMessage(42, "/clients alice"))

	msgs := got()
	if len(msgs) != 4 {
		t.Fatalf("messages = %+v", msgs)
	}
	if msgs[0].ChatID != 7 || strings.Contains(msgs[0].Text, "alice") {
		t.Fatalf("stranger was served: %+v", msgs[0])
	}
	if !strings.Contains(msgs[1].Text, "7") {
		t.Fatalf("/id did not echo the ID: %+v", msgs[1])
	}
	if !strings.Contains(msgs[2].Text, "alice") || !strings.Contains(msgs[2].Text, "95%") {
		t.Fatalf("near-limit list = %q", msgs[2].Text)
	}
	if !strings.Contains(msgs[3].Text, "alice") || !strings.Contains(msgs[3].Text, "1000 B") {
		t.Fatalf("client detail = %q", msgs[3].Text)
	}
}

func TestGroupChatsAreIgnored(t *testing.T) {
	b, got := fakeTelegram(t)
	var u update
	_ = json.Unmarshal([]byte(`{"update_id":1,"message":{"text":"/help","chat":{"id":-100,"type":"group"},"from":{"id":42}}}`), &u)
	b.handle(context.Background(), u)
	if len(got()) != 0 {
		t.Fatal("bot answered in a group chat")
	}
}
