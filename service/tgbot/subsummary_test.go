package tgbot

import (
	"strings"
	"testing"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
)

const boundUser = int64(4242) // a client's own Telegram account

// seedAccount makes a client that has everything the summary talks about -- and
// a few things it must keep to itself.
func seedAccount(t *testing.T, name string, fields map[string]interface{}) model.Client {
	t.Helper()
	c := seedClient(t, name, "beni", 65*gib, 0)
	base := map[string]interface{}{
		"up": gib, "down": 40 * gib,
		"expiry":     time.Now().Add(55*24*time.Hour + time.Hour).Unix(),
		"limit_ip":   2,
		"created_at": time.Date(2026, 9, 25, 21, 55, 0, 0, time.UTC).Unix(),
		"online_at":  time.Date(2026, 10, 6, 10, 16, 0, 0, time.UTC).Unix(),
		"desc":       "NL-note",
		"tg_id":      boundUser,
	}
	for k, v := range fields {
		base[k] = v
	}
	if err := database.GetDB().Model(model.Client{}).Where("id = ?", c.Id).Updates(base).Error; err != nil {
		t.Fatal(err)
	}
	return loadByName(t, name)
}

// subMessages is what the bot sent for a request for the subscription: the
// text and how many QR pictures came with it.
func subMessages(t *testing.T, msgs []sent) (text string, photos int) {
	t.Helper()
	all := texts(msgs, "sendMessage")
	if len(all) != 1 {
		t.Fatalf("want one text message, got %d: %q", len(all), all)
	}
	for _, m := range msgs {
		if m.Method == "sendPhoto" {
			photos++
		}
	}
	return all[0], photos
}

func mustContain(t *testing.T, text string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
}

func mustNotContain(t *testing.T, text string, banned ...string) {
	t.Helper()
	for _, no := range banned {
		if strings.Contains(text, no) {
			t.Errorf("must not contain %q:\n%s", no, text)
		}
	}
}

// The subscription message begins with the client's account: what was used and
// what is left, until when, the limits, when they were last online and created
// -- so it can be forwarded to them as it is.
func TestSubscriptionMessageStartsWithTheAccountSummary(t *testing.T) {
	e := newAccessEnv(t)
	c := seedAccount(t, "EgoNgNB9", nil)
	link, err := e.b.subLink(c.Name)
	if err != nil {
		t.Fatal(err)
	}

	n := len(e.got())
	e.press(fullAdmin, "c:sub:"+itoa(int64(c.Id)))
	text, photos := subMessages(t, e.since(n))
	if photos != 1 {
		t.Fatalf("%d QR pictures", photos)
	}
	mustContain(t, text,
		"👤 <b>EgoNgNB9</b>  🟢 enabled",
		"📊 <b>Usage</b> 🟨🟨🟨🟨🟨🟨⬜⬜⬜⬜ 63%",
		"41.00 GiB / 65.00 GiB  (↑ 1.00 GiB ↓ 40.00 GiB)",
		"📦 <b>Remaining</b>: 24.00 GiB",
		"⏳ <b>Expiry</b>: "+e.b.stamp(c.Expiry)+" (55 days left)",
		"📱 IP limit: 2",
		"🕒 Last online: "+e.b.stamp(c.OnlineAt),
		"📅 Created: "+e.b.stamp(c.CreatedAt),
		"🔗 Subscription link of <b>EgoNgNB9</b>:\n<code>"+link+"</code>",
	)
	// The account comes first and the link last, so the link is what is copied.
	if strings.Index(text, "👤") > strings.Index(text, "📊") || strings.Index(text, "📅") > strings.Index(text, "🔗") || !strings.HasSuffix(text, "</code>") {
		t.Errorf("wrong order:\n%s", text)
	}
	// What is for the administrator only stays out of a message meant to be passed on.
	mustNotContain(t, text, "NL-note", "beni", "Lifetime", "🔔", "4242", "📡", "🏷", "📝")
}

// No volume, no time limit, never online: the summary says so instead of
// leaving lines out or inventing numbers.
func TestSubscriptionSummaryOfAnUnlimitedClient(t *testing.T) {
	e := newAccessEnv(t)
	c := seedAccount(t, "free", map[string]interface{}{
		"volume": 0, "expiry": 0, "limit_ip": 0, "online_at": 0, "up": 3 * gib, "down": 0, "desc": "", "tg_id": 0,
	})
	n := len(e.got())
	e.say(fullAdmin, "/sub free")
	text, _ := subMessages(t, e.since(n))
	mustContain(t, text,
		"📊 <b>Usage</b>: 3.00 GiB ∞",
		"⏳ <b>Expiry</b>: unlimited",
		"🕒 Last online: never",
		"📅 Created: "+e.b.stamp(c.CreatedAt),
	)
	mustNotContain(t, text, "Remaining", "IP limit", "%")
}

// A disabled client who used everything and ran out of time says exactly that.
func TestSubscriptionSummaryOfADepletedClient(t *testing.T) {
	e := newAccessEnv(t)
	seedAccount(t, "gone", map[string]interface{}{
		"enable": false, "up": 70 * gib, "down": 0, "expiry": time.Now().Add(-48 * time.Hour).Unix(),
	})
	n := len(e.got())
	e.say(fullAdmin, "/sub gone")
	text, _ := subMessages(t, e.since(n))
	mustContain(t, text, "🔴 disabled", "🟥🟥🟥🟥🟥🟥🟥🟥🟥🟥 107%", "📦 <b>Remaining</b>: 0 B", "⏳ <b>Expiry</b>: expired")
}

// Whoever asks gets the same account: the administrator's button and command,
// and the client themselves through /sub and the button of their /usage.
func TestSubscriptionSummaryIsTheSameForEveryoneWhoAsks(t *testing.T) {
	e := newAccessEnv(t)
	c := seedAccount(t, "ego", nil)
	id := itoa(int64(c.Id))
	ask := map[string]func(){
		"administrator's button":  func() { e.press(fullAdmin, "c:sub:"+id) },
		"administrator's command": func() { e.say(fullAdmin, "/sub ego") },
		"client's command":        func() { e.say(boundUser, "/sub") },
		"client's button":         func() { e.press(boundUser, "u:sub:"+id) },
	}
	var first string
	for who, do := range ask {
		n := len(e.got())
		do()
		text, photos := subMessages(t, e.since(n))
		if photos != 1 {
			t.Errorf("%s: %d QR pictures", who, photos)
		}
		mustContain(t, text, "📊 <b>Usage</b>", "📦 <b>Remaining</b>: 24.00 GiB", "(55 days left)", "🕒 Last online:", "📅 Created:", "🔗 Subscription link of <b>ego</b>:")
		mustNotContain(t, text, "NL-note", "beni", "4242")
		if first == "" {
			first = text
		} else if text != first {
			t.Errorf("%s got a different message:\n%s\n--- instead of ---\n%s", who, text, first)
		}
	}
}

// A client who is online right now is marked, as on the card.
func TestSubscriptionSummaryMarksAnOnlineClient(t *testing.T) {
	e := newAccessEnv(t)
	c := seedAccount(t, "live", nil)
	previous := allOnlineUsers
	allOnlineUsers = func() []string { return []string{"live"} }
	t.Cleanup(func() { allOnlineUsers = previous })
	n := len(e.got())
	e.press(fullAdmin, "c:sub:"+itoa(int64(c.Id)))
	text, _ := subMessages(t, e.since(n))
	mustContain(t, text, "👤 <b>live</b>  🟢 enabled  ⚡️ online")
}

func TestSubscriptionSummaryInPersian(t *testing.T) {
	e := newAccessEnv(t)
	e.b.cfg.Lang = "fa"
	t.Cleanup(func() { e.b.cfg.Lang = "en" })
	c := seedAccount(t, "fa-user", nil)
	n := len(e.got())
	e.press(fullAdmin, "c:sub:"+itoa(int64(c.Id)))
	text, _ := subMessages(t, e.since(n))
	mustContain(t, text, "فعال", "<b>مصرف</b>", "<b>حجم باقی‌مانده</b>: 24.00 GiB", "<b>انقضا</b>", "55 روز مانده", "محدودیت IP: 2", "آخرین اتصال:", "ساخته‌شده:", "لینک اشتراک <b>fa-user</b>")
}

// The summary is only for clients the asker may see: a group-limited
// administrator gets nothing about somebody else's client.
func TestSubscriptionSummaryStaysInsideTheAdministratorsGroup(t *testing.T) {
	e := newAccessEnv(t)
	n := len(e.got())
	e.press(salesAdmin, "c:sub:"+e.cid("o1"))
	e.say(salesAdmin, "/sub o1")
	for _, m := range e.since(n) {
		if m.Method == "sendPhoto" || strings.Contains(m.Text, "Usage") || strings.Contains(m.Text, "Subscription link") {
			t.Fatalf("an administrator got another group's subscription: %+v", m)
		}
	}
	// Their own client works.
	n = len(e.got())
	e.press(salesAdmin, "c:sub:"+e.cid("s1"))
	text, _ := subMessages(t, e.since(n))
	mustContain(t, text, "👤 <b>s1</b>", "📊 <b>Usage</b>", "🔗 Subscription link of <b>s1</b>:")
}
