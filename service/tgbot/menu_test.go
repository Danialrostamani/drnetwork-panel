package tgbot

import (
	"context"
	"strings"
	"testing"
)

// The main menu keeps the everyday sections and opens the rest in groups.
func TestMainMenuIsGrouped(t *testing.T) {
	b, got := testBot(t)
	b.cfg.Panel = true
	menu := callbackData(b.mainMenu())
	for _, d := range []string{"h:home", "c:ls:a:0", "m:sell", "t:d1", "m:tech", "m:care", "a:ls"} {
		if indexOf(menu, d) < 0 {
			t.Fatalf("the main menu lacks %s: %v", d, menu)
		}
	}
	for _, d := range []string{"o:in:ls:0", "s:ls", "m:backup", "m:restart", "k:ordsP"} {
		if indexOf(menu, d) >= 0 {
			t.Fatalf("%s should be in a group, not on the main menu: %v", d, menu)
		}
	}
	if n := len(menu); n > 9 {
		t.Fatalf("the main menu has %d buttons", n)
	}
	ctx := context.Background()
	for _, g := range []string{"m:tech", "m:care", "m:sell"} {
		before := len(got())
		b.handle(ctx, callbackFrom(42, g))
		msgs := got()[before:]
		if len(msgs) == 0 || strings.Contains(msgs[len(msgs)-1].Text, "❌") {
			t.Fatalf("%s: %+v", g, msgs)
		}
	}
	rows := b.adminMenu()
	flat := strings.Join(rows[0], ",") + "," + strings.Join(rows[1], ",")
	if flat != "c:ls:a:0,m:sell,t:d1,m:menu" {
		t.Fatalf("bottom menu: %v", rows)
	}
	// Old bottom-menu labels still work.
	for _, old := range []string{"📋 منوی اصلی", "🏠 خانه", "📋 سفارش‌ها"} {
		if menuData(old) == "" {
			t.Fatalf("%q is no longer a button", old)
		}
	}
}
