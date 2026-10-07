package tgbot

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
)

func TestDepletionAlertFollowsTheWarningAndTheTurnOff(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "bot.db")); err != nil {
		t.Fatal(err)
	}
	db := database.GetDB()
	mk := func(name string, enable bool, up, expiry int64) model.Client {
		c := model.Client{Name: name, Enable: enable, Volume: 100, Up: up, Expiry: expiry, Inbounds: json.RawMessage(`[]`), Links: json.RawMessage(`[]`)}
		if err := db.Create(&c).Error; err != nil {
			t.Fatal(err)
		}
		return c
	}
	near := mk("near", true, 95, 0)
	mk("olddead", false, 150, 0)
	mk("oldexp", false, 0, time.Now().Add(-time.Hour).Unix())
	b, got := fakeTelegram(t)
	ctx := context.Background()
	reported := map[string]bool{}
	text := func(from int) string {
		var s string
		for _, m := range got()[from:] {
			s += m.Text + "\n"
		}
		return s
	}

	b.checkClients(ctx, reported)
	first := text(0)
	if !strings.Contains(first, "near") || strings.Contains(first, "olddead") || strings.Contains(first, "oldexp") {
		t.Fatalf("first check:\n%s", first)
	}

	// The panel turns the client off the moment it runs out: the depletion
	// alert still comes, after the 90% warning.
	near.Up, near.Enable = 120, false
	if err := db.Save(&near).Error; err != nil {
		t.Fatal(err)
	}
	mk("newexp", false, 0, time.Now().Add(-time.Minute).Unix())
	n := len(got())
	b.checkClients(ctx, reported)
	second := text(n)
	if !strings.Contains(second, "near") || !strings.Contains(second, b.t("alertDepleted")) || !strings.Contains(second, "newexp") || !strings.Contains(second, b.t("alertExpired")) {
		t.Fatalf("second check:\n%s", second)
	}
	if strings.Contains(second, "olddead") || strings.Contains(second, "oldexp") {
		t.Fatalf("old clients announced again:\n%s", second)
	}

	// Nothing new, nothing sent.
	n = len(got())
	b.checkClients(ctx, reported)
	if len(got()) != n {
		t.Fatalf("repeated alert:\n%s", text(n))
	}
}

func TestCallWaitsOutFloodControl(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			_, _ = w.Write([]byte(`{"ok":false,"error_code":429,"description":"Too Many Requests: retry after 1","parameters":{"retry_after":1}}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	}))
	defer srv.Close()
	previous := apiBase
	apiBase = srv.URL
	defer func() { apiBase = previous }()
	b := newBot(botConfig{Token: "1:abc", Admins: []int64{42}, Lang: "en"})
	if err := b.call(context.Background(), "sendMessage", map[string]any{"chat_id": 42, "text": "x"}, nil); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatalf("calls = %d, want 2", calls.Load())
	}
}

func TestBrokenHTMLIsSentAsPlainText(t *testing.T) {
	var plain atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var m map[string]any
		_ = json.NewDecoder(r.Body).Decode(&m)
		if m["parse_mode"] == "HTML" {
			_, _ = w.Write([]byte(`{"ok":false,"error_code":400,"description":"Bad Request: can't parse entities: unclosed start tag"}`))
			return
		}
		plain.Store(m["text"])
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	}))
	defer srv.Close()
	previous := apiBase
	apiBase = srv.URL
	defer func() { apiBase = previous }()
	b := newBot(botConfig{Token: "1:abc", Admins: []int64{42}, Lang: "en"})
	b.send(context.Background(), 42, "<b>a &amp; b</b> <code>c")
	if got, _ := plain.Load().(string); got != "a & b c" {
		t.Fatalf("plain text = %q", got)
	}
}
