package tgbot

import (
	"strings"
	"testing"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
)

// setSetting changes one panel setting in this test's own database.
func setSetting(t *testing.T, key, value string) {
	t.Helper()
	res := database.GetDB().Model(&model.Setting{}).Where("key = ?", key).Update("value", value)
	if res.Error != nil || res.RowsAffected != 1 {
		t.Fatalf("setting %s: %d rows, %v", key, res.RowsAffected, res.Error)
	}
}

const cardSubURI = "https://sub.example.org:2096/sub/"

func hasButtonData(kb [][]button, data string) bool {
	for _, row := range kb {
		for _, b := range row {
			if b.Data == data {
				return true
			}
		}
	}
	return false
}

// The client card ends with the subscription link, the same one the
// Subscription / QR button sends, in a code span so that a tap copies it.
func TestClientCardEndsWithTheSubscriptionLink(t *testing.T) {
	e := newAccessEnv(t)
	setSetting(t, "subURI", cardSubURI)
	c := seedAccount(t, "EgoNgNB9", nil)
	id := itoa(int64(c.Id))
	want := "🔗 Subscription:\n<code>https://sub.example.org:2096/sub/EgoNgNB9</code>"

	check := func(how, text string, kb [][]button) {
		t.Helper()
		mustContain(t, text, "👤 <b>EgoNgNB9</b>", "📊 <b>Usage</b>", "📅 Created:", want)
		if !strings.HasSuffix(text, want) {
			t.Errorf("%s: the link is not the last thing on the card:\n%s", how, text)
		}
		if !hasButtonData(kb, "c:sub:"+id) || !hasButtonData(kb, "c:tog:"+id) {
			t.Errorf("%s: the card lost its buttons: %v", how, kb)
		}
	}
	n := len(e.got())
	e.press(fullAdmin, "c:view:"+id)
	msgs := e.since(n)
	check("card from a button", lastText(msgs, "editMessageText"), keyboardOf(msgs))

	n = len(e.got())
	e.say(fullAdmin, "/clients EgoNgNB9")
	msgs = e.since(n)
	check("card from a search", lastText(msgs, "sendMessage"), keyboardOf(msgs))

	// The button under the card still sends the same link, with the QR code.
	n = len(e.got())
	e.press(fullAdmin, "c:sub:"+id)
	text, photos := subMessages(t, e.since(n))
	mustContain(t, text, "<code>https://sub.example.org:2096/sub/EgoNgNB9</code>")
	if photos != 1 {
		t.Errorf("%d QR pictures", photos)
	}
}

// Without a subscription address in the settings the card shows the link the
// button would send -- never a different one.
func TestClientCardLinkIsTheOneTheButtonSends(t *testing.T) {
	e := newAccessEnv(t)
	c := seedAccount(t, "plain", nil)
	link, err := e.b.subLink("plain")
	if err != nil || !strings.HasSuffix(link, "/plain") {
		t.Fatalf("link %q, %v", link, err)
	}
	n := len(e.got())
	e.press(fullAdmin, "c:view:"+itoa(int64(c.Id)))
	text := lastText(e.since(n), "editMessageText")
	if !strings.HasSuffix(text, "🔗 Subscription:\n<code>"+link+"</code>") {
		t.Errorf("card:\n%s\nwant it to end with the link %s", text, link)
	}
}

func TestClientCardSubscriptionLineInPersian(t *testing.T) {
	e := newAccessEnv(t)
	setSetting(t, "subURI", cardSubURI)
	e.b.cfg.Lang = "fa"
	t.Cleanup(func() { e.b.cfg.Lang = "en" })
	c := seedAccount(t, "fa-card", nil)
	n := len(e.got())
	e.press(fullAdmin, "c:view:"+itoa(int64(c.Id)))
	text := lastText(e.since(n), "editMessageText")
	if !strings.HasSuffix(text, "🔗 لینک اشتراک:\n<code>https://sub.example.org:2096/sub/fa-card</code>") {
		t.Errorf("card:\n%s", text)
	}
}

// An address with characters Telegram's HTML would take for markup must not
// break the card.
func TestClientCardEscapesTheLink(t *testing.T) {
	e := newAccessEnv(t)
	setSetting(t, "subURI", "https://sub.example.org/s?a=1&b=<2>/")
	c := seedAccount(t, "esc", nil)
	n := len(e.got())
	e.press(fullAdmin, "c:view:"+itoa(int64(c.Id)))
	text := lastText(e.since(n), "editMessageText")
	if !strings.HasSuffix(text, "<code>https://sub.example.org/s?a=1&amp;b=&lt;2&gt;/esc</code>") {
		t.Errorf("card:\n%s", text)
	}
}

// The client's own /usage shows their link too, with their two buttons only.
func TestBoundClientsOwnCardHasTheLink(t *testing.T) {
	e := newAccessEnv(t)
	setSetting(t, "subURI", cardSubURI)
	c := seedAccount(t, "mine", nil)
	id := itoa(int64(c.Id))
	n := len(e.got())
	e.say(boundUser, "/usage")
	msgs := e.since(n)
	text := lastText(msgs, "sendMessage")
	mustContain(t, text, "👤 <b>mine</b>", "🔗 Subscription:\n<code>https://sub.example.org:2096/sub/mine</code>")
	mustNotContain(t, text, "NL-note")
	kb := keyboardOf(msgs)
	if !hasButtonData(kb, "u:sub:"+id) || hasButtonData(kb, "c:tog:"+id) {
		t.Errorf("buttons: %v", kb)
	}
}

// A group-limited administrator's card of their own client has the link; the
// links of other groups' clients stay out of reach, as the cards themselves do.
func TestGroupLimitedAdminSeesTheLinkOfTheirOwnClientsOnly(t *testing.T) {
	e := newAccessEnv(t)
	setSetting(t, "subURI", cardSubURI)
	n := len(e.got())
	e.press(salesAdmin, "c:view:"+e.cid("s1"))
	text := lastText(e.since(n), "editMessageText")
	mustContain(t, text, "👤 <b>s1</b>", "<code>https://sub.example.org:2096/sub/s1</code>")

	n = len(e.got())
	e.press(salesAdmin, "c:view:"+e.cid("o1"))
	e.say(salesAdmin, "/clients o1")
	for _, m := range e.since(n) {
		if strings.Contains(m.Text, "/sub/o1") {
			t.Fatalf("the link of another group's client leaked: %+v", m)
		}
	}
}
