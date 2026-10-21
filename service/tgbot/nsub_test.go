package tgbot

import (
	"strings"
	"testing"
)

// The token switch on the card turns the token link on; then the new-link
// button asks first, moves the link and sends the new one.
func TestTokenLinkFromTheCard(t *testing.T) {
	e := newAccessEnv(t)
	setSetting(t, "subURI", cardSubURI)
	c := seedAccount(t, "leaked", nil)
	id := itoa(int64(c.Id))

	n := len(e.got())
	e.press(fullAdmin, "c:view:"+id)
	if kb := keyboardOf(e.since(n)); !hasButtonData(kb, "c:tok:"+id) || hasButtonData(kb, "c:nsub:"+id) {
		t.Fatalf("buttons without the token link: %v", kb)
	}
	e.press(fullAdmin, "c:nsub:"+id)
	if loadByName(t, "leaked").SubToken != c.SubToken {
		t.Fatal("a new token without the token link")
	}

	n = len(e.got())
	e.press(fullAdmin, "c:tok:"+id)
	if !loadByName(t, "leaked").UseSubToken {
		t.Fatal("the switch did not turn the token link on")
	}
	if text := lastText(e.since(n), "editMessageText"); !strings.Contains(text, "/sub/"+c.SubToken) {
		t.Fatalf("card after switching on:\n%s", text)
	}

	n = len(e.got())
	e.press(fullAdmin, "c:nsub:"+id)
	if loadByName(t, "leaked").SubToken != c.SubToken {
		t.Fatal("the link moved before the confirmation")
	}
	if kb := keyboardOf(e.since(n)); !hasButtonData(kb, "c:nsuby:"+id) {
		t.Fatalf("no confirmation: %v", kb)
	}
	n = len(e.got())
	e.press(fullAdmin, "c:nsuby:"+id)
	fresh := loadByName(t, "leaked").SubToken
	if fresh == c.SubToken || len(fresh) != len(c.SubToken) {
		t.Fatalf("token %q -> %q", c.SubToken, fresh)
	}
	text, _ := subMessages(t, e.since(n))
	if !strings.Contains(text, "/sub/"+fresh) || strings.Contains(text, c.SubToken) {
		t.Errorf("sent:\n%s", text)
	}

	e.press(fullAdmin, "c:tok:"+id)
	if loadByName(t, "leaked").UseSubToken {
		t.Fatal("the switch did not turn the token link off")
	}
}

// /add and /addbulk take "token" for clients with the token link.
func TestAddCommandsTakeTheTokenWord(t *testing.T) {
	e := newAccessEnv(t)
	e.say(fullAdmin, "/add tk1 10 30 token")
	e.say(fullAdmin, "/add plain1 10 30")
	e.say(fullAdmin, "/addbulk tkb 2 10 30 0 token")
	for name, want := range map[string]bool{"tk1": true, "plain1": false, "tkb1": true, "tkb2": true} {
		if got := loadByName(t, name).UseSubToken; got != want {
			t.Errorf("%s: token link %v, want %v", name, got, want)
		}
	}
}
