package tgbot

import (
	"strings"
	"testing"
)

// The new-link button asks first, then moves the link and sends the new one.
func TestNewSubscriptionLinkFromTheCard(t *testing.T) {
	e := newAccessEnv(t)
	setSetting(t, "subURI", cardSubURI)
	c := seedAccount(t, "leaked", nil)
	id := itoa(int64(c.Id))

	n := len(e.got())
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
}
