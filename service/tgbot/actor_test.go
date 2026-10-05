package tgbot

import (
	"strconv"
	"strings"
	"testing"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
)

func lastActor(t *testing.T, key string) string {
	t.Helper()
	var c model.Changes
	if err := database.GetDB().Where("`key` = ?", key).Order("id desc").First(&c).Error; err != nil {
		t.Fatalf("no change was recorded for %s: %v", key, err)
	}
	return c.Actor
}

func countChanges(t *testing.T, actor string) int64 {
	t.Helper()
	var n int64
	database.GetDB().Model(model.Changes{}).Where("actor = ?", actor).Count(&n)
	return n
}

// Whatever an administrator changes from the bot, the change history names them
// by Telegram ID, so the panel shows who did it, whichever part of the bot the
// change came from.
func TestChangeHistoryNamesTheAdministratorWhoActed(t *testing.T) {
	e := newAccessEnv(t)
	who := func(id int64) string { return "telegram:" + strconv.FormatInt(id, 10) }

	// Clients.
	e.say(fullAdmin, "/volume o1 5")
	if got := lastActor(t, "clients"); got != who(fullAdmin) {
		t.Fatalf("a full administrator's client edit: actor %q", got)
	}
	e.say(salesAdmin, "/volume s1 7")
	if got := lastActor(t, "clients"); got != who(salesAdmin) {
		t.Fatalf("a group-limited administrator's client edit: actor %q", got)
	}
	e.say(ownerID, "/disable n1")
	if got := lastActor(t, "clients"); got != who(ownerID) {
		t.Fatalf("the owner's client edit: actor %q", got)
	}
	// A button counts like a command.
	e.press(salesAdmin, "c:tog:"+e.cid("s2"))
	if got := lastActor(t, "clients"); got != who(salesAdmin) {
		t.Fatalf("a button press: actor %q", got)
	}

	// Settings, from the settings screens and from the owner's Admins screen.
	e.press(fullAdmin, "s:tg:subEncode")
	if got := lastActor(t, "settings"); got != who(fullAdmin) {
		t.Fatalf("a setting: actor %q", got)
	}
	e.press(ownerID, "a:add")
	e.say(ownerID, "900")
	if got := lastActor(t, "settings"); got != who(ownerID) {
		t.Fatalf("adding an administrator: actor %q", got)
	}

	// The core's configuration, and objects.
	e.press(fullAdmin, "g:log:lv:warn")
	if got := lastActor(t, "config"); got != who(fullAdmin) {
		t.Fatalf("the core configuration: actor %q", got)
	}
	e.press(fullAdmin, "o:out:n:0")
	e.say(fullAdmin, "```json\n"+templateJSON("out", "direct")+"\n```")
	if got := lastActor(t, "outbounds"); got != who(fullAdmin) {
		t.Fatalf("an outbound: actor %q", got)
	}

	// Nothing was recorded under the bare name, and every record names somebody.
	if n := countChanges(t, "telegram"); n != 0 {
		t.Fatalf("%d changes were recorded without saying who made them", n)
	}
	if n := countChanges(t, who(salesAdmin)); n != 2 {
		t.Fatalf("the group-limited administrator made %d changes, want 2", n)
	}

	// The bot's own history screen says who too.
	n := len(e.got())
	e.say(fullAdmin, "/changes")
	text := lastText(e.since(n), "sendMessage")
	for _, want := range []string{who(fullAdmin), who(salesAdmin), who(ownerID)} {
		if !strings.Contains(text, want) {
			t.Errorf("the history screen lacks %s:\n%s", want, text)
		}
	}
}

func TestActorOfABotWithoutAnAdministrator(t *testing.T) {
	b, _ := testBot(t)
	if got := b.actor(); got != "telegram" {
		t.Fatalf("actor = %q", got)
	}
	if got := b.as(42).actor(); got != "telegram:42" {
		t.Fatalf("actor of 42 = %q", got)
	}
	// Somebody who is not an administrator is still never anonymous: the ID is
	// what the history keeps. (They cannot change anything anyway.)
	if got := b.as(9).actor(); got != "telegram:9" {
		t.Fatalf("actor of 9 = %q", got)
	}
}
