package tgbot

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
)

func TestPanelManagementCanLeaveTheBot(t *testing.T) {
	b, got := ownerBot(t, shopConfig{})
	ctx := context.Background()
	node := model.Node{Name: "n1", Enable: false, BaseUrl: "https://127.0.0.1:1", WebPath: "/app/", Token: "x"}
	if err := database.GetDB().Create(&node).Error; err != nil {
		t.Fatal(err)
	}
	b.cfg.Panel = false
	ob := b.as(boss)
	menu := callbackData(ob.menuTree())
	for _, gone := range []string{"o:in:ls:0", "o:out:ls:0", "o:ep:ls:0", "o:sv:ls:0", "o:tl:ls:0", "g:rl:ls:0", "g:dn:ls:0", "g:basics:ls:0", "s:ls", "m:logs:info", "x:ls"} {
		if indexOf(menu, gone) >= 0 {
			t.Fatalf("menu still has %s: %v", gone, menu)
		}
	}
	for _, kept := range []string{"h:home", "c:ls:a:0", "o:nd:ls:0", "t:d1", "a:ls", "m:backup", "m:maint", "m:restart", "m:sync", "q:home"} {
		if indexOf(menu, kept) < 0 {
			t.Fatalf("menu lost %s: %v", kept, menu)
		}
	}
	if hasData(ob.homeKeyboard(), "m:logs:info") {
		t.Fatal("home still offers the logs")
	}
	// The node card watches, it does not edit.
	_, kb := ob.objCard(kindByCode("nd"), node.Id)
	id := fmt.Sprint(node.Id)
	if hasData(kb, "o:nd:e:"+id) || hasData(kb, "o:nd:d:"+id) || hasData(kb, "o:nd:j:"+id) || !hasData(kb, "o:nd:otg:"+id) {
		t.Fatalf("node card: %v", callbackData(kb))
	}
	// Asked for anyway, they are refused.
	for _, data := range []string{"o:in:ls:0", "o:out:lnk:0", "g:rl:ls:0", "s:ls", "o:nd:e:" + id, "o:nd:dy:" + id} {
		before := len(got())
		b.handle(ctx, callbackFrom(boss, data))
		if b.pend.get(boss) != nil || !strings.Contains(strings.Join(answers(got()[before:]), "|")+strings.Join(texts(got()[before:], "sendMessage"), "|"), "access") {
			t.Fatalf("%s was not refused: %+v", data, got()[before:])
		}
	}
	var n int64
	database.GetDB().Model(model.Node{}).Count(&n)
	if n != 1 {
		t.Fatal("a node was deleted")
	}
	before := len(got())
	b.handle(ctx, privateMessage(boss, "/inbounds"))
	if m := sentTo(got()[before:], boss); len(m) == 0 || strings.Contains(m[len(m)-1].Text, "Inbounds (") {
		t.Fatalf("/inbounds answered: %+v", m)
	}
	before = len(got())
	b.handle(ctx, privateMessage(boss, "/help"))
	if h := lastTo(t, got()[before:], boss); strings.Contains(h, "/logs") || strings.Contains(h, "/settings") || strings.Contains(h, "/inbounds") || !strings.Contains(h, "/add") {
		t.Fatalf("help: %s", h)
	}
	// The owner still restarts the panel from the owner panel.
	if !ob.allowed([]string{"m", "prest"}) {
		t.Fatal("panel restart refused to the owner")
	}
	// Switched on, it all comes back.
	b.cfg.Panel = true
	if menu = callbackData(b.as(boss).menuTree()); indexOf(menu, "o:in:ls:0") < 0 || indexOf(menu, "s:ls") < 0 {
		t.Fatalf("panel on: %v", menu)
	}
}
