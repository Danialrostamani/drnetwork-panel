package tgbot

import (
	"testing"
	"time"
)

// The client card offers a typed volume and time instead of fixed steps,
// drops the rarely used tools, and lists IPs only for an IP-limited client.
func TestClientCardButtons(t *testing.T) {
	e := newAccessEnv(t)
	c := seedAccount(t, "cardbtn", nil)
	id := itoa(int64(c.Id))

	n := len(e.got())
	e.press(fullAdmin, "c:view:"+id)
	kb := keyboardOf(e.since(n))
	for _, d := range []string{"c:ask:cvol:", "c:ask:cdays:", "c:dinf:", "c:ips:", "c:edit:", "c:xl:", "c:sub:", "c:del:"} {
		if !hasButtonData(kb, d+id) {
			t.Errorf("card lacks %s: %v", d, kb)
		}
	}
	for _, d := range []string{"c:gb10:", "c:gb50:", "c:d30:", "c:cfg:", "c:inb:", "c:lnk:", "c:kick:", "c:json:"} {
		if hasButtonData(kb, d+id) {
			t.Errorf("card still has %s", d)
		}
	}

	// Volume: a plain number is the total, +N adds.
	e.press(fullAdmin, "c:ask:cvol:"+id)
	e.say(fullAdmin, "+5")
	if got := loadByName(t, "cardbtn").Volume; got != 70*gib {
		t.Fatalf("+5 GB gave %d", got)
	}
	e.press(fullAdmin, "c:ask:cvol:"+id)
	e.say(fullAdmin, "20")
	if got := loadByName(t, "cardbtn").Volume; got != 20*gib {
		t.Fatalf("20 GB gave %d", got)
	}

	// Time: +N extends the expiry, a plain number counts from now.
	before := loadByName(t, "cardbtn").Expiry
	e.press(fullAdmin, "c:ask:cdays:"+id)
	e.say(fullAdmin, "+10")
	if got := loadByName(t, "cardbtn").Expiry; got < before+10*86400-5 || got > before+10*86400+5 {
		t.Fatalf("+10 days: %d -> %d", before, got)
	}
	e.press(fullAdmin, "c:ask:cdays:"+id)
	e.say(fullAdmin, "7")
	want := time.Now().Add(7 * 24 * time.Hour).Unix()
	if got := loadByName(t, "cardbtn").Expiry; got < want-5 || got > want+5 {
		t.Fatalf("7 days gave %d, want ~%d", got, want)
	}

	// Unlimited time in one tap; the button then goes away.
	e.press(fullAdmin, "c:dinf:"+id)
	if got := loadByName(t, "cardbtn").Expiry; got != 0 {
		t.Fatalf("unlimited time left expiry %d", got)
	}
	e.press(fullAdmin, "c:ask:ip:"+id)
	e.say(fullAdmin, "0")
	n = len(e.got())
	e.press(fullAdmin, "c:view:"+id)
	kb = keyboardOf(e.since(n))
	if hasButtonData(kb, "c:dinf:"+id) || hasButtonData(kb, "c:ips:"+id) {
		t.Errorf("unlimited, IP-unlimited card still offers ♾ or IPs: %v", kb)
	}
	// +N on unlimited time is refused, nothing changes.
	e.press(fullAdmin, "c:ask:cdays:"+id)
	e.say(fullAdmin, "+3")
	if got := loadByName(t, "cardbtn").Expiry; got != 0 {
		t.Fatalf("+3 on unlimited set expiry %d", got)
	}
}
