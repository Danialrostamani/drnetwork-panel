package sub

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"

	"github.com/gin-gonic/gin"
)

func TestBrowsersGetTheAccountPage(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "sub.db")); err != nil {
		t.Fatal(err)
	}
	c := model.Client{Name: "alice", Enable: false, Volume: 1 << 30, Up: 1 << 30, Expiry: time.Now().Add(48 * time.Hour).Unix(), Inbounds: json.RawMessage(`[]`), Links: json.RawMessage(`[]`), Config: json.RawMessage(`{}`)}
	if err := database.GetDB().Create(&c).Error; err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	NewSubHandler(r.Group("/sub"))
	get := func(ua, accept, lang string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/sub/alice", nil)
		req.Header.Set("User-Agent", ua)
		req.Header.Set("Accept", accept)
		req.Header.Set("Accept-Language", lang)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	w := get("Mozilla/5.0 (Linux; Android 14)", "text/html,application/xhtml+xml", "fa-IR,fa;q=0.9")
	body := w.Body.String()
	if w.Code != 200 || !strings.Contains(body, "وضعیت اشتراک") || !strings.Contains(body, "subscription-userinfo") || strings.Contains(body, "__CF$cv$params") || strings.Contains(body, "ZgotmplZ") {
		t.Fatalf("page %d (%d bytes)", w.Code, len(body))
	}
	if w.Header().Get("Content-Type") != "text/html; charset=utf-8" {
		t.Fatal("not html")
	}
	// The page then fetches the same URL with Accept */*; that must stay the
	// subscription, not another copy of the HTML.
	if w := get("Mozilla/5.0", "*/*", ""); strings.Contains(w.Header().Get("Content-Type"), "text/html") {
		t.Fatal("fetch got html")
	}
	// An app gets what it always got: a disabled client has no links.
	if w := get("v2rayNG/1.9", "*/*", ""); w.Code != 400 {
		t.Fatalf("app got %d", w.Code)
	}
	head := httptest.NewRequest(http.MethodHead, "/sub/alice", nil)
	head.Header.Set("User-Agent", "Mozilla/5.0")
	hw := httptest.NewRecorder()
	r.ServeHTTP(hw, head)
	if strings.Contains(hw.Header().Get("Content-Type"), "text/html") {
		t.Fatal("head got html")
	}
}
