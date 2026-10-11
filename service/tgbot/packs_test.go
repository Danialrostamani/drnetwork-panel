package tgbot

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/service"
)

func TestPlanCategoriesAndExtraPacks(t *testing.T) {
	e, gold := shopEnv(t)
	s := &service.ShopService{}
	vol := &model.ShopPlan{Name: "Vol10", Volume: 10 * gib, Days: 30, Price: 500, Enable: true, Category: "Volume"}
	unl := &model.ShopPlan{Name: "Unl", Days: 30, Price: 900, Enable: true, Category: "Unlimited"}
	pack := &model.ShopPlan{Name: "Extra5", Volume: 5 * gib, Price: 300, Enable: true, Addon: true}
	for _, p := range []*model.ShopPlan{vol, unl, pack} {
		if err := s.SavePlan(p); err != nil {
			t.Fatal(err)
		}
	}

	// Categories first, the uncategorised plan under "other", no pack.
	n := len(e.got())
	e.press(buyer, "p:plans:0")
	data := inlineData(e.since(n))
	for _, d := range []string{"p:cat:0:0", "p:cat:1:0", "p:cat:2:0"} {
		if !has(data, d) {
			t.Fatalf("categories %v lack %s", data, d)
		}
	}
	n = len(e.got())
	e.press(buyer, "p:cat:0:0")
	data = inlineData(e.since(n))
	if !has(data, fmt.Sprintf("p:plan:%d:0", vol.Id)) || has(data, fmt.Sprintf("p:plan:%d:0", gold.Id)) {
		t.Fatalf("category Volume: %v", data)
	}
	n = len(e.got())
	e.press(buyer, "p:cat:2:0")
	if data = inlineData(e.since(n)); !has(data, fmt.Sprintf("p:plan:%d:0", gold.Id)) || has(data, fmt.Sprintf("p:plan:%d:0", pack.Id)) {
		t.Fatalf("category other: %v", data)
	}

	// A pack is for a service the customer has.
	n = len(e.got())
	e.press(buyer, fmt.Sprintf("p:plan:%d:0", pack.Id))
	if txt := lastText(e.since(n), "editMessageText"); !strings.HasPrefix(txt, "❌") {
		t.Fatalf("pack without a service: %q", txt)
	}

	// The trial ran out of volume; a pack adds to it and keeps the usage
	// and the expiry.
	e.press(buyer, "p:trial")
	c := boundClients(buyer)[0]
	if err := database.GetDB().Model(&model.Client{}).Where("id = ?", c.Id).Updates(map[string]any{"up": c.Volume, "down": 0, "enable": false}).Error; err != nil {
		t.Fatal(err)
	}
	n = len(e.got())
	e.say(buyer, e.b.menuLabel("p:my"))
	if !has(inlineData(e.since(n)), fmt.Sprintf("p:addon:%d", c.Id)) {
		t.Fatalf("no extra volume button: %+v", e.since(n))
	}
	n = len(e.got())
	e.press(buyer, fmt.Sprintf("p:addon:%d", c.Id))
	if data = inlineData(e.since(n)); !has(data, fmt.Sprintf("p:plan:%d:%d", pack.Id, c.Id)) || has(data, fmt.Sprintf("p:plan:%d:%d", gold.Id, c.Id)) {
		t.Fatalf("packs: %v", data)
	}
	_, _ = s.AddFunds(nil, buyer, 1000, "admin", 0, 0)
	e.press(buyer, fmt.Sprintf("p:walY:%d:%d", pack.Id, c.Id))
	o := lastOrder(t)
	after := e.b.findClientByName(c.Name)
	if o.Kind != model.OrderAddon || o.Status != model.OrderApproved || after == nil {
		t.Fatalf("order %+v client %+v", o, after)
	}
	if after.Volume != c.Volume+5*gib || after.Up != c.Volume || after.Expiry != c.Expiry || !after.Enable {
		t.Fatalf("after the pack: volume %d up %d expiry %d/%d enable %v", after.Volume, after.Up, after.Expiry, c.Expiry, after.Enable)
	}

	// Auto-renewal never picks a pack.
	if p, err := s.RenewPlanOf(c.Id); err == nil && p.Addon {
		t.Fatalf("auto-renewal with the pack %+v", p)
	}
}

func TestFixedAmountDiscount(t *testing.T) {
	e, gold := shopEnv(t)
	s := &service.ShopService{}
	d, err := service.ParseDiscountLine("OFF300 300 10")
	if err != nil || d.Amount != 300 || d.Percent != 0 {
		t.Fatalf("parsed %+v, %v", d, err)
	}
	if err := s.SaveDiscount(d); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveDiscount(&model.ShopDiscount{Code: "BOTH", Percent: 10, Amount: 5}); err == nil {
		t.Fatal("a code with a percent and an amount was saved")
	}
	if _, paid := service.PriceWith(1000, 0, 0, 300); paid != 700 {
		t.Fatalf("paid %d", paid)
	}
	if _, paid := service.PriceWith(200, 0, 0, 300); paid != 0 {
		t.Fatalf("below zero: %d", paid)
	}
	// Typed in the shop, it takes 300 off Gold's 1000.
	e.press(buyer, fmt.Sprintf("p:code:%d:0", gold.Id))
	n := len(e.got())
	e.say(buyer, "OFF300")
	if txt := lastText(e.since(n), "sendMessage"); !strings.Contains(txt, e.b.money(700)) {
		t.Fatalf("with the code: %q", txt)
	}
}
