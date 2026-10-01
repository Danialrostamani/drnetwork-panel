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

	"github.com/Danialrostamani/drnetwork-panel/core"
	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/logger"
	"github.com/Danialrostamani/drnetwork-panel/service"

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
	Method string `json:"-"`
}

func fakeTelegram(t *testing.T) (*bot, func() []sent) {
	t.Helper()
	var mu sync.Mutex
	var messages []sent
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		switch method {
		case "sendMessage", "editMessageText", "answerCallbackQuery":
			var m sent
			_ = json.NewDecoder(r.Body).Decode(&m)
			m.Method = method
			mu.Lock()
			messages = append(messages, m)
			mu.Unlock()
		case "sendPhoto", "sendDocument":
			mu.Lock()
			messages = append(messages, sent{Method: method})
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

func callbackFrom(from int64, data string) update {
	var u update
	raw := `{"update_id":2,"callback_query":{"id":"cb1","data":` + mustJSON(data) + `,"from":{"id":` + itoa(from) + `},"message":{"message_id":9,"chat":{"id":` + itoa(from) + `,"type":"private"}}}}`
	_ = json.Unmarshal([]byte(raw), &u)
	return u
}

func texts(msgs []sent, method string) []string {
	var out []string
	for _, m := range msgs {
		if m.Method == method {
			out = append(out, m.Text)
		}
	}
	return out
}

func loadByName(t *testing.T, name string) model.Client {
	t.Helper()
	var c model.Client
	if err := database.GetDB().Where("name = ?", name).First(&c).Error; err != nil {
		t.Fatalf("client %s: %v", name, err)
	}
	return c
}

// The management commands go through ConfigService.Save, the same path as the
// web panel, so this drives a client through its whole life from the bot.
func TestManagementLifecycleThroughCommandsAndButtons(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "bot.db")); err != nil {
		t.Fatal(err)
	}
	// Maintenance keeps Save from trying to start a real core afterwards.
	cs := service.NewConfigService(core.NewCore())
	if err := (&service.SettingService{}).SetMaintenance(true); err != nil {
		t.Fatal(err)
	}
	b, got := fakeTelegram(t)
	b.configService = cs
	ctx := context.Background()

	b.handle(ctx, privateMessage(42, "/add bob 5 30 2"))
	bob := loadByName(t, "bob")
	if !bob.Enable || bob.Volume != 5*gib || bob.LimitIp != 2 || bob.Expiry == 0 || !strings.Contains(string(bob.Config), "xtls-rprx-vision") {
		t.Fatalf("created client = %+v", bob)
	}
	b.handle(ctx, privateMessage(42, "/add bob 5 30"))
	b.handle(ctx, privateMessage(42, "/add bad/name 5 30"))
	var count int64
	database.GetDB().Model(model.Client{}).Count(&count)
	if count != 1 {
		t.Fatalf("duplicate or invalid names were created: %d clients", count)
	}

	id := itoa(int64(bob.Id))
	b.handle(ctx, callbackFrom(42, "c:tog:"+id))
	if loadByName(t, "bob").Enable {
		t.Fatal("toggle did not disable the client")
	}
	b.handle(ctx, privateMessage(42, "/enable bob"))
	b.handle(ctx, callbackFrom(42, "c:gb10:"+id))
	b.handle(ctx, callbackFrom(42, "c:d30:"+id))
	after := loadByName(t, "bob")
	if !after.Enable || after.Volume != 15*gib || after.Expiry < bob.Expiry+29*86400 {
		t.Fatalf("after edits = %+v", after)
	}

	b.handle(ctx, privateMessage(42, "/volume bob 2"))
	b.handle(ctx, privateMessage(42, "/limitip bob 0"))
	b.handle(ctx, privateMessage(42, "/expiry bob 0"))
	after = loadByName(t, "bob")
	if after.Volume != 2*gib || after.LimitIp != 0 || after.Expiry != 0 {
		t.Fatalf("after /volume /limitip /expiry = %+v", after)
	}
	// An unlimited client cannot have time added to it.
	b.handle(ctx, callbackFrom(42, "c:d30:"+id))
	if loadByName(t, "bob").Expiry != 0 {
		t.Fatal("added days to an unlimited client")
	}

	// Reset needs a confirmation press and keeps lifetime totals.
	database.GetDB().Model(model.Client{}).Where("id = ?", bob.Id).Updates(map[string]interface{}{"up": 100, "down": 200})
	b.handle(ctx, callbackFrom(42, "c:rst:"+id))
	if loadByName(t, "bob").Up != 100 {
		t.Fatal("reset happened before it was confirmed")
	}
	b.handle(ctx, callbackFrom(42, "c:rsty:"+id))
	after = loadByName(t, "bob")
	if after.Up != 0 || after.Down != 0 || after.TotalUp != 100 || after.TotalDown != 200 {
		t.Fatalf("after reset = %+v", after)
	}

	// Binding lets the owner look at their own client, and survives panel edits.
	b.handle(ctx, privateMessage(42, "/bind bob 99"))
	if loadByName(t, "bob").TgId != 99 {
		t.Fatal("bind did not store the Telegram ID")
	}
	b.handle(ctx, privateMessage(42, "/volume bob 3"))
	if loadByName(t, "bob").TgId != 99 {
		t.Fatal("an edit erased the Telegram binding")
	}
	before := len(got())
	b.handle(ctx, privateMessage(99, "/usage"))
	b.handle(ctx, privateMessage(99, "/sub"))
	b.handle(ctx, privateMessage(99, "/add eve 1 1"))
	b.handle(ctx, callbackFrom(99, "c:dely:"+id))
	msgs := got()[before:]
	if len(msgs) < 4 || !strings.Contains(msgs[0].Text, "bob") || strings.Contains(msgs[0].Text, "xtls") {
		t.Fatalf("user view = %+v", msgs)
	}
	if loadByName(t, "bob").Id != bob.Id {
		t.Fatal("a bound user deleted their client")
	}
	database.GetDB().Model(model.Client{}).Count(&count)
	if count != 1 {
		t.Fatal("a bound user created a client")
	}
	var photos int
	for _, m := range msgs {
		if m.Method == "sendPhoto" {
			photos++
		}
	}
	if photos != 1 {
		t.Fatalf("QR photos sent = %d", photos)
	}

	b.handle(ctx, privateMessage(42, "/unbind bob"))
	if loadByName(t, "bob").TgId != 0 {
		t.Fatal("unbind failed")
	}
	b.handle(ctx, callbackFrom(42, "c:dely:"+id))
	database.GetDB().Model(model.Client{}).Count(&count)
	if count != 0 {
		t.Fatal("delete did not remove the client")
	}
	if len(texts(got(), "editMessageText")) == 0 {
		t.Fatal("button presses did not edit the message")
	}
}

func TestStrangerButtonPressesAreRefused(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "bot.db")); err != nil {
		t.Fatal(err)
	}
	c := model.Client{Name: "carol", Enable: true, Inbounds: json.RawMessage(`[]`), Links: json.RawMessage(`[]`)}
	database.GetDB().Create(&c)
	b, got := fakeTelegram(t)
	b.handle(context.Background(), callbackFrom(7, "c:tog:"+itoa(int64(c.Id))))
	b.handle(context.Background(), callbackFrom(7, "m:restarty"))
	if !loadByName(t, "carol").Enable {
		t.Fatal("a stranger toggled a client")
	}
	for _, m := range got() {
		if m.Method != "answerCallbackQuery" {
			t.Fatalf("stranger got %s", m.Method)
		}
	}
}

func TestNewClientConfigCoversEveryProtocol(t *testing.T) {
	var cfg map[string]map[string]interface{}
	if err := json.Unmarshal(newClientConfig("zed"), &cfg); err != nil {
		t.Fatal(err)
	}
	if len(cfg) != 15 || cfg["vless"]["name"] != "zed" || cfg["mixed"]["username"] != "zed" {
		t.Fatalf("config = %v", cfg)
	}
	if id, _ := cfg["vmess"]["uuid"].(string); len(id) != 36 || id[14] != '4' {
		t.Fatalf("uuid = %q", id)
	}
	if !clientNameRe.MatchString("a.b-c_d@e") || clientNameRe.MatchString("a b") {
		t.Fatal("name validation")
	}
}
