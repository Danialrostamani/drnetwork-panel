package sub

import (
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"

	"github.com/gin-gonic/gin"
)

func tokenEnv(t *testing.T) (*SubHandler, model.Client) {
	t.Helper()
	if err := database.InitDB(filepath.Join(t.TempDir(), "sub.db")); err != nil {
		t.Fatal(err)
	}
	c := model.Client{Name: "alice", Enable: true, Inbounds: json.RawMessage(`[]`), Links: json.RawMessage(`[]`), Config: json.RawMessage(`{}`)}
	if err := database.GetDB().Create(&c).Error; err != nil {
		t.Fatal(err)
	}
	if len(c.SubToken) != model.SubTokenLength {
		t.Fatalf("a new client got the token %q", c.SubToken)
	}
	old := subFails
	subFails = &subFailTable{entries: map[string]*subFailEntry{}, now: time.Now}
	t.Cleanup(func() { subFails = old })
	return &SubHandler{}, c
}

func subCtx(remote, forwarded string) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/sub/x", nil)
	c.Request.RemoteAddr = remote + ":5000"
	if forwarded != "" {
		c.Request.Header.Set("X-Forwarded-For", forwarded)
	}
	return c
}

func setNameLinks(t *testing.T, on string) {
	t.Helper()
	db := database.GetDB()
	db.Where("key = ?", "subNameLinks").Delete(model.Setting{})
	if err := db.Create(&model.Setting{Key: "subNameLinks", Value: on}).Error; err != nil {
		t.Fatal(err)
	}
}

// The token always finds its client; the name only while the old links are on.
func TestSubLinksFindTheClientByToken(t *testing.T) {
	s, c := tokenEnv(t)
	if name, ok := s.resolveSub(subCtx("203.0.113.5", ""), c.SubToken); !ok || name != "alice" {
		t.Fatalf("token: %q, %v", name, ok)
	}
	if name, ok := s.resolveSub(subCtx("203.0.113.5", ""), "alice"); !ok || name != "alice" {
		t.Fatalf("old link by name while allowed: %q, %v", name, ok)
	}
	setNameLinks(t, "false")
	if _, ok := s.resolveSub(subCtx("203.0.113.5", ""), "alice"); ok {
		t.Fatal("a link by name worked with the old links off")
	}
	if name, ok := s.resolveSub(subCtx("203.0.113.5", ""), c.SubToken); !ok || name != "alice" {
		t.Fatalf("token with the old links off: %q, %v", name, ok)
	}
	if _, ok := s.resolveSub(subCtx("203.0.113.5", ""), "abcdefghijklmnopqrstuvwx"); ok {
		t.Fatal("an unknown token worked")
	}
}

// An address that keeps guessing names is cut off from names for a while --
// not from tokens, and not the other addresses.
func TestGuessingNamesBlocksTheAddress(t *testing.T) {
	s, c := tokenEnv(t)
	now := time.Unix(1_000_000, 0)
	subFails.now = func() time.Time { return now }
	for i := 0; i < subFailLimit; i++ {
		if _, ok := s.resolveSub(subCtx("203.0.113.5", ""), "guess"+string(rune('a'+i%26))); ok {
			t.Fatal("a guess worked")
		}
	}
	if _, ok := s.resolveSub(subCtx("203.0.113.5", ""), "alice"); ok {
		t.Fatal("a blocked address still gets links by name")
	}
	if _, ok := s.resolveSub(subCtx("203.0.113.5", ""), c.SubToken); !ok {
		t.Fatal("a blocked address lost its token link")
	}
	if _, ok := s.resolveSub(subCtx("198.51.100.7", ""), "alice"); !ok {
		t.Fatal("another address was blocked too")
	}
	now = now.Add(subFailBlock + time.Second)
	if _, ok := s.resolveSub(subCtx("203.0.113.5", ""), "alice"); !ok {
		t.Fatal("the block did not end")
	}
}

// A forwarded address counts only behind a local proxy; from anywhere else the
// header could say anything.
func TestSubRequestIPTrustsOnlyLocalProxies(t *testing.T) {
	if got := subRequestIP(subCtx("127.0.0.1", "203.0.113.9")); got != "203.0.113.9" {
		t.Errorf("behind a local proxy: %q", got)
	}
	if got := subRequestIP(subCtx("8.8.8.8", "203.0.113.9")); got != "8.8.8.8" {
		t.Errorf("forwarded header from outside: %q", got)
	}
}

// The table of failures does not grow without end.
func TestSubFailTableIsBounded(t *testing.T) {
	tb := &subFailTable{entries: map[string]*subFailEntry{}, now: time.Now}
	for i := 0; i < subFailMaxIPs+50; i++ {
		tb.fail("10." + strconv.Itoa(i/65536) + "." + strconv.Itoa(i/256%256) + "." + strconv.Itoa(i%256))
	}
	if len(tb.entries) > subFailMaxIPs {
		t.Fatalf("%d addresses kept", len(tb.entries))
	}
}
