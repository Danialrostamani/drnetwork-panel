package middleware

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestDecoyServesOnlyItsOwnFiles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	base := t.TempDir()
	site := filepath.Join(base, "site")
	write := func(p, s string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(site, "index.html"), "home page")
	write(filepath.Join(site, "404.html"), "not here")
	write(filepath.Join(site, "blog", "index.html"), "blog")
	write(filepath.Join(site, ".git", "config"), "secret")
	write(filepath.Join(base, "outside.txt"), "outside")
	if err := os.Symlink(filepath.Join(base, "outside.txt"), filepath.Join(site, "link.txt")); err != nil {
		t.Skip("no symlinks: ", err)
	}
	if err := os.Symlink(base, filepath.Join(site, "up")); err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.NoRoute(Decoy(site))
	get := func(method, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "/", nil)
		req.URL.Path = path
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}
	cases := []struct {
		method, path string
		code         int
		body         string
	}{
		{"GET", "/", 200, "home page"},
		{"HEAD", "/", 200, ""},
		{"GET", "/index.html", 200, "home page"},
		{"GET", "/blog/", 200, "blog"},
		{"GET", "/missing", 404, "not here"},
		{"POST", "/", 404, "not here"},
		{"GET", "/.git/config", 404, "not here"},
		{"GET", "/blog/../.git/config", 404, "not here"},
		{"GET", "/../outside.txt", 404, "not here"},
		{"GET", "/link.txt", 404, "not here"},
		{"GET", "/up/outside.txt", 404, "not here"},
	}
	for _, c := range cases {
		rec := get(c.method, c.path)
		if rec.Code != c.code || !strings.Contains(rec.Body.String(), c.body) || strings.Contains(rec.Body.String(), "secret") || strings.Contains(rec.Body.String(), "outside") {
			t.Errorf("%s %s = %d %q, want %d %q", c.method, c.path, rec.Code, rec.Body.String(), c.code, c.body)
		}
	}
	if rec := get("GET", "/blog"); rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != "/blog/" {
		t.Errorf("folder without slash: %d %q", rec.Code, rec.Header().Get("Location"))
	}
	// A folder that does not exist answers the bare 404.
	r2 := gin.New()
	r2.NoRoute(Decoy(filepath.Join(base, "nope")))
	rec := httptest.NewRecorder()
	r2.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != 404 || rec.Body.Len() != 0 {
		t.Errorf("missing site: %d %q", rec.Code, rec.Body.String())
	}
}
