package sub

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/service"

	"github.com/gin-gonic/gin"
)

func pageServer(t *testing.T) *gin.Engine {
	t.Helper()
	if err := database.InitDB(filepath.Join(t.TempDir(), "sub.db")); err != nil {
		t.Fatal(err)
	}
	c := model.Client{Name: "alice", Enable: true, CreatedAt: 1_700_000_000, OnlineAt: 1_700_000_500, Inbounds: json.RawMessage(`[]`), Links: json.RawMessage(`[]`), Config: json.RawMessage(`{}`)}
	if err := database.GetDB().Create(&c).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := (&service.SettingService{}).GetAllSetting(); err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	NewSubHandler(r.Group("/sub"))
	return r
}

func fetch(r *gin.Engine, method, path, accept string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 14)")
	req.Header.Set("Accept", accept)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// The page loads its font, QR code library, icons, manifest and worker from
// the panel itself.
func TestThePageServesItsOwnFiles(t *testing.T) {
	r := pageServer(t)
	page := fetch(r, http.MethodGet, "/sub/alice", "text/html").Body.String()
	if strings.Contains(page, "cdn.jsdelivr.net") || !strings.Contains(page, `src="sui-qr.js"`) || !strings.Contains(page, `url("sui-font.woff2")`) {
		t.Fatal("the page still loads from a CDN")
	}
	cases := []struct{ path, kind, body string }{
		{"/sub/sui-qr.js", "application/javascript", "qrcode"},
		{"/sub/sui-font.woff2", "font/woff2", "wOF2"},
		{"/sub/sui-icon-192.png", "image/png", "\x89PNG"},
		{"/sub/sui-icon-512.png", "image/png", "\x89PNG"},
		{"/sub/sw.js", "application/javascript", "cache-self"},
		{"/sub/manifest.json?id=alice", "application/manifest+json", `"start_url":"./alice"`},
	}
	for _, tc := range cases {
		w := fetch(r, http.MethodGet, tc.path, "*/*")
		if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), tc.kind) || !strings.Contains(w.Body.String(), tc.body) {
			t.Fatalf("%s: %d %q", tc.path, w.Code, w.Header().Get("Content-Type"))
		}
		if h := fetch(r, http.MethodHead, tc.path, "*/*"); h.Code != 200 {
			t.Fatalf("HEAD %s: %d", tc.path, h.Code)
		}
	}
	if w := fetch(r, http.MethodGet, "/sub/manifest.json?id=../x", "*/*"); !strings.Contains(w.Body.String(), `"start_url":"./"`) {
		t.Fatal("a path got into the manifest")
	}
}

// The panel's page settings reach the page, safely.
func TestThePageTakesThePanelSettings(t *testing.T) {
	r := pageServer(t)
	conf := `{"name":"Dr </script><b>","notice":"Hi","accent":"#112233","support":"@help","hideQr":true}`
	if err := (&service.SettingService{}).Save(database.GetDB(), json.RawMessage(`{"subPageConf":`+mustQuote(conf)+`}`)); err != nil {
		t.Fatal(err)
	}
	page := fetch(r, http.MethodGet, "/sub/alice", "text/html").Body.String()
	i := strings.Index(page, "const SERVER_CONFIG = ")
	if i < 0 || strings.Contains(page, "const SERVER_CONFIG = /*SUB_CONFIG*/null") {
		t.Fatal("no config in the page")
	}
	line := page[i : i+strings.Index(page[i:], "\n")]
	for _, want := range []string{`"panelName":"Dr \u003c/script\u003e\u003cb\u003e"`, `"supportLink":"https://t.me/help"`, `"renewTarget":""`, `"hideQr":true`, `"accent":"#112233"`, `"statusSelf":true`} {
		if !strings.Contains(line, want) {
			t.Fatalf("config %s misses %s", line, want)
		}
	}
	if strings.Contains(line, "</script>") {
		t.Fatal("the config can close the script")
	}
	if err := (&service.SettingService{}).Save(database.GetDB(), json.RawMessage(`{"subPageConf":"{\"support\":\"javascript:x\"}"}`)); err == nil {
		t.Fatal("a script link was saved")
	}
}

func mustQuote(s string) string { b, _ := json.Marshal(s); return string(b) }

// The link answers ?format=status with what the page's live card shows.
func TestTheLinkTellsItsStatus(t *testing.T) {
	r := pageServer(t)
	w := fetch(r, http.MethodGet, "/sub/alice?format=status", "*/*")
	var got pageStatus
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &got) != nil || !got.OK || got.Client.CreatedAt != 1_700_000_000 || got.Client.LastSeen != 1_700_000_500 || got.Client.Online {
		t.Fatalf("status %d %s", w.Code, w.Body.String())
	}
	if w := fetch(r, http.MethodGet, "/sub/nobody?format=status", "*/*"); w.Code != 400 {
		t.Fatalf("unknown client: %d", w.Code)
	}
	statusMu.Lock()
	statusCache = map[string]statusEntry{}
	statusMu.Unlock()
	if err := (&service.SettingService{}).Save(database.GetDB(), json.RawMessage(`{"subPageConf":"{\"hideLive\":true}"}`)); err != nil {
		t.Fatal(err)
	}
	if w := fetch(r, http.MethodGet, "/sub/alice?format=status", "*/*"); w.Code != 404 {
		t.Fatalf("hidden status: %d", w.Code)
	}
}
