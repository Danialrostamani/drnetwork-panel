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
	if w.Code != 200 || !strings.Contains(body, "alice") || !strings.Contains(body, "حجم تمام شده") || !strings.Contains(body, "v2rayng://install-config?url=") || strings.Contains(body, "ZgotmplZ") {
		t.Fatalf("page %d:\n%s", w.Code, body)
	}
	// An app gets what it always got: a disabled client has no links.
	if w := get("v2rayNG/1.9", "*/*", ""); w.Code != 400 {
		t.Fatalf("app got %d", w.Code)
	}
	if w := get("Mozilla/5.0", "text/html", ""); !strings.Contains(w.Body.String(), "Out of volume") {
		t.Fatal("english page missing")
	}
	if w := get("Mozilla/5.0", "text/html", ""); w.Header().Get("Content-Type") != "text/html; charset=utf-8" {
		t.Fatal("not html")
	}
}
