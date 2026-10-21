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

// A client with the token link is found by its token only; one without, by
// its name only.
func TestSubLinksFindTheClientByTokenOrName(t *testing.T) {
	s, alice := tokenEnv(t)
	bob := model.Client{Name: "bob", Enable: true, UseSubToken: true, Inbounds: json.RawMessage(`[]`), Links: json.RawMessage(`[]`), Config: json.RawMessage(`{}`)}
	if err := database.GetDB().Create(&bob).Error; err != nil {
		t.Fatal(err)
	}
	ctx := func() *gin.Context { return subCtx("203.0.113.5", "") }
	if name, ok := s.resolveSub(ctx(), "alice"); !ok || name != "alice" {
		t.Fatalf("name link: %q, %v", name, ok)
	}
	if _, ok := s.resolveSub(ctx(), alice.SubToken); ok {
		t.Fatal("the token of a client without the token link worked")
	}
	if name, ok := s.resolveSub(ctx(), bob.SubToken); !ok || name != "bob" {
		t.Fatalf("token link: %q, %v", name, ok)
	}
	if _, ok := s.resolveSub(ctx(), "bob"); ok {
		t.Fatal("the name of a client with the token link worked")
	}
	if _, ok := s.resolveSub(ctx(), "abcdefghijklmnopqrstuvwx"); ok {
		t.Fatal("an unknown token worked")
	}
}

// An address that keeps guessing names is cut off from names for a while --
// not from tokens, and not the other addresses.
func TestGuessingNamesBlocksTheAddress(t *testing.T) {
	s, _ := tokenEnv(t)
	c := model.Client{Name: "carol", Enable: true, UseSubToken: true, Inbounds: json.RawMessage(`[]`), Links: json.RawMessage(`[]`), Config: json.RawMessage(`{}`)}
	if err := database.GetDB().Create(&c).Error; err != nil {
		t.Fatal(err)
	}
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
