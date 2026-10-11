package tgbot

import (
	"context"
	"strings"
	"testing"

	"github.com/Danialrostamani/drnetwork-panel/service"
)

// bottomMenu is the bottom menu of the last message that set one, as
// callback data.
func bottomMenu(msgs []sent) []string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if m := msgs[i].Markup; m != nil && len(m.Reply) > 0 {
			var out []string
			for _, row := range m.Reply {
				for _, k := range row {
					out = append(out, menuData(k.Text))
				}
			}
			if !m.Persistent {
				return nil
			}
			return out
		}
	}
	return nil
}

func has(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func inlineData(msgs []sent) []string {
	var out []string
	for _, m := range msgs {
		if m.Markup == nil {
			continue
		}
		for _, row := range m.Markup.Keyboard {
			for _, k := range row {
				out = append(out, k.Data)
			}
		}
	}
	return out
}

func TestBottomMenu(t *testing.T) {
	e, _ := shopEnv(t)
	b := e.b

	// A customer's /start pins the shop's menu, trial and referral included.
	n := len(e.got())
	e.say(buyer, "/start")
	menu := bottomMenu(e.since(n))
	for _, d := range []string{"p:plans:0", "p:my", "p:renew", "p:wallet", "p:trial", "p:ref", "p:guide", "p:support"} {
		if !has(menu, d) {
			t.Fatalf("customer menu %v lacks %s", menu, d)
		}
	}
	if has(menu, "k:admin") || has(menu, "o:nd:ls:0") {
		t.Fatalf("customer menu %v has administrator items", menu)
	}

	// Its buttons answer with new messages.
	n = len(e.got())
	e.say(buyer, b.menuLabel("p:wallet"))
	if got := e.since(n); len(got) == 0 || got[len(got)-1].Method != "sendMessage" || !has(inlineData(got), "p:topup") {
		t.Fatalf("wallet button: %+v", got)
	}
	n = len(e.got())
	e.say(buyer, b.menuLabel("p:guide"))
	if !has(inlineData(e.since(n)), "p:guide:ios") {
		t.Fatalf("guide button: %+v", e.since(n))
	}
	n = len(e.got())
	e.press(buyer, "p:guide:ios")
	if txt := lastText(e.since(n), "editMessageText"); !strings.Contains(txt, "Streisand") {
		t.Fatalf("iOS guide: %q", txt)
	}
	n = len(e.got())
	e.say(buyer, b.menuLabel("p:my"))
	if !has(inlineData(e.since(n)), "p:plans:0") {
		t.Fatalf("no services: %+v", e.since(n))
	}

	// An administrator's label typed by a customer opens nothing.
	n = len(e.got())
	e.say(buyer, b.menuLabel("o:nd:ls:0"))
	for _, d := range inlineData(e.since(n)) {
		if strings.HasPrefix(d, "o:") {
			t.Fatalf("customer reached %s", d)
		}
	}

	// The administrator gets the panel's menu, and a customer view.
	n = len(e.got())
	e.say(42, "/start")
	menu = bottomMenu(e.since(n))
	if !has(menu, "o:nd:ls:0") || !has(menu, "q:menu") || !has(menu, "k:shop") {
		t.Fatalf("admin menu %v", menu)
	}
	n = len(e.got())
	e.say(42, b.menuLabel("o:nd:ls:0"))
	if got := e.since(n); len(got) == 0 || got[len(got)-1].Method != "sendMessage" {
		t.Fatalf("nodes button: %+v", got)
	}
	n = len(e.got())
	e.say(42, b.menuLabel("k:shop"))
	if menu = bottomMenu(e.since(n)); !has(menu, "k:admin") || !has(menu, "p:plans:0") {
		t.Fatalf("customer view menu %v", menu)
	}
	n = len(e.got())
	e.say(42, b.menuLabel("k:admin"))
	if menu = bottomMenu(e.since(n)); !has(menu, "m:menu") {
		t.Fatalf("back to admin menu %v", menu)
	}

	// A limited administrator's menu holds only what their role opens.
	n = len(e.got())
	e.say(salesAdmin, "/start")
	for _, d := range bottomMenu(e.since(n)) {
		if strings.HasPrefix(d, "k:") {
			continue
		}
		if !e.b.as(salesAdmin).allowed(strings.Split(d, ":")) {
			t.Fatalf("limited admin menu has %s", d)
		}
	}

	// A service of the customer has its quick renewal.
	e.press(buyer, "p:trial")
	bound := boundClients(buyer)
	if len(bound) != 1 {
		t.Fatalf("%d services after the trial", len(bound))
	}
	n = len(e.got())
	e.say(buyer, b.menuLabel("p:my"))
	if !has(inlineData(e.since(n)), "p:plans:"+itoa(int64(bound[0].Id))) {
		t.Fatalf("no quick renewal: %+v", e.since(n))
	}

	// A closed shop keeps the guide and says it is closed.
	_ = (&service.ShopService{}).SetSetting("shopEnable", "false")
	n = len(e.got())
	e.say(buyer, b.menuLabel("p:plans:0"))
	if txt := lastText(e.since(n), "sendMessage"); !strings.Contains(txt, b.tr("فروشگاه فعال نیست", "The shop is closed")) {
		t.Fatalf("closed shop: %q", txt)
	}
	n = len(e.got())
	e.say(buyer, b.menuLabel("p:guide"))
	if !has(inlineData(e.since(n)), "p:guide:android") {
		t.Fatal("closed shop lost the guide")
	}
}

// The bottom menu reaches the people who started the bot before it existed:
// the administrators with the start message and the first /menu, the
// customers once with their next message.
func TestBottomMenuReachesExistingChats(t *testing.T) {
	e, _ := shopEnv(t)
	b := e.b

	b.announce(context.Background())
	var told []int64
	for _, m := range e.got() {
		if strings.Contains(m.Text, b.t("started")) && has(bottomMenu([]sent{m}), "m:menu") {
			told = append(told, m.ChatID)
		}
	}
	if len(told) != 2 {
		t.Fatalf("started with a menu went to %v", told)
	}

	// The start message carried it, so /menu sends only the menu; after a
	// restart the first /menu brings it again.
	n := len(e.got())
	e.say(fullAdmin, "/menu")
	if out := e.since(n); len(out) != 1 {
		t.Fatalf("/menu after the start message: %d messages", len(out))
	}
	b.menus = &menuState{}
	n = len(e.got())
	e.say(fullAdmin, "/menu")
	e.say(fullAdmin, "/menu")
	if out := e.since(n); len(out) != 3 || !has(bottomMenu(out[:1]), "m:menu") {
		t.Fatalf("/menu after a restart: %d messages", len(out))
	}

	// A customer from before the menu gets it with their next press, once.
	if _, err := (&service.ShopService{}).Touch(buyer, "Old", "", 0); err != nil {
		t.Fatal(err)
	}
	n = len(e.got())
	e.press(buyer, "p:wallet")
	out := e.since(n)
	if !has(bottomMenu(out), "p:plans:0") {
		t.Fatalf("no menu for an old customer: %+v", out)
	}
	n = len(e.got())
	b.menus = &menuState{}
	e.press(buyer, "p:wallet")
	if bottomMenu(e.since(n)) != nil {
		t.Fatal("the menu was sent twice")
	}
	// A new customer gets it from /start and not again.
	const fresh = int64(6000)
	n = len(e.got())
	e.say(fresh, "/start")
	e.press(fresh, "p:wallet")
	menus := 0
	for _, m := range e.since(n) {
		if m.Markup != nil && len(m.Markup.Reply) > 0 {
			menus++
		}
	}
	if menus != 1 {
		t.Fatalf("a new customer got %d menus", menus)
	}
}
