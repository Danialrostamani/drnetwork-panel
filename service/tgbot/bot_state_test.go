package tgbot

import "testing"

// A copy of the bot (the per-update copy that as returns) must carry its own
// identity but share the conversation stores and the access state with the bot
// it came from.
func TestBotCopyKeepsIdentityApartAndSharesStores(t *testing.T) {
	b := &bot{stores: stores{pend: &pendingStore{}}}
	c := *b
	c.owner, c.self, c.scope = true, 42, "g"
	if b.owner || b.self != 0 || b.scope != "" {
		t.Fatal("identity set on a copy leaked into the original bot")
	}
	if c.pend != b.pend {
		t.Fatal("a copy must share the pending store with the original bot")
	}
}
