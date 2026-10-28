package tgbot

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/service"
)

func TestFullPartnerOwnsSalesGroupWithoutRestrictingAccess(t *testing.T) {
	e := newAccessEnv(t)
	setShop(t, shopConfig{Desks: map[string]shopDesk{
		"42": {SalesGroup: "Partners", PayCard: "partner-card", Support: "support"},
		"77": {SalesGroup: "Other"}, // Cannot override a seller's enforced scope.
	}})
	seedClient(t, "partner-client", "Partners", gib, 0)
	partner := e.b.as(fullAdmin)
	if partner.owner || partner.restricted() || !partner.canManageAdmins() || !partner.can("nodes") {
		t.Fatal("personal group changed a full partner's authority or ownership")
	}
	if len(partner.loadClients()) != len(allClients()) || partner.clientByID(e.id["o1"]) == nil {
		t.Fatal("partner lost access to another group's client")
	}
	if e.b.sellerGroup(fullAdmin) != "Partners" || e.b.sellerGroup(salesAdmin) != salesGroup {
		t.Fatal("sales ownership was confused with enforced access scope")
	}
	s := partner.loadShop()
	s.PayCard = "updated-card"
	if err := partner.saveShop(s); err != nil {
		t.Fatal(err)
	}
	d := e.b.loadShopRaw().Desks["42"]
	if d.SalesGroup != "Partners" || d.Support != "support" || d.PayCard != "updated-card" {
		t.Fatalf("editing the personal shop lost its ownership: %+v", d)
	}
	view, err := PanelAdmins(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range view.Admins {
		if a.Id == fullAdmin {
			if a.Role != "full" || a.Group != "" || a.SalesGroup != "Partners" || a.Clients != 1 {
				t.Fatalf("partner card = %+v", a)
			}
			return
		}
	}
	t.Fatal("partner was missing from the admin list")
}

func TestFullPartnerCanManageAdminsWithoutTakingOwnership(t *testing.T) {
	e := newAccessEnv(t)
	e.press(fullAdmin, "a:add")
	e.say(fullAdmin, "900")
	if !e.b.access().isMember(900) {
		t.Fatal("full partner could not add an administrator")
	}
	cfg, err := loadConfig()
	if err != nil || cfg.Owner != ownerID || e.b.as(fullAdmin).owner {
		t.Fatalf("partner took ownership: owner=%d, err=%v", cfg.Owner, err)
	}
	if err := e.b.as(fullAdmin).editAccess(func(d *accessDoc) error { return d.remove(ownerID) }); !errors.Is(err, errIsOwner) {
		t.Fatalf("owner protection failed: %v", err)
	}
	giveQuota(t, clientsOnly, 10)
	if err := e.b.as(fullAdmin).quotaAdd(clientsOnly, 5*gib); err != nil {
		t.Fatalf("partner cannot manage a seller's quota: %v", err)
	}
	var edits int64
	if err := database.GetDB().Model(&model.Changes{}).
		Where("actor = ? AND `key` = ?", "telegram:42", "settings").Count(&edits).Error; err != nil {
		t.Fatal(err)
	}
	if edits == 0 {
		t.Fatal("administrator change was not attributed to the partner")
	}
}

func TestStalePartnerCannotCompleteAnAdministratorEdit(t *testing.T) {
	e := newAccessEnv(t)
	stale := e.b.as(fullAdmin)
	// Deliberately do not reload the bot snapshot: editAccess must recheck
	// the settings read inside its critical section.
	raw, _ := json.Marshal(map[string]string{"tgBotPerms": "42=clients\n55=clients,stats\n56=\n57=nodes\n58=core"})
	if err := (&service.SettingService{}).Save(database.GetDB(), raw); err != nil {
		t.Fatal(err)
	}
	if err := stale.editAccess(func(d *accessDoc) error { return d.add(901) }); !errors.Is(err, errNotOwner) {
		t.Fatalf("revoked partner completed an administrator edit: %v", err)
	}
}

func TestCustomSectionsAndSuspensionCannotGrantPartnerAuthority(t *testing.T) {
	e := newAccessEnv(t)
	useAccess(e.b, botConfig{Owner: ownerID, Admins: []int64{fullAdmin}, Perms: map[int64]sectionSet{fullAdmin: allSections()}})
	if b := e.b.as(fullAdmin); b.canManageAdmins() || b.canApprove() {
		t.Fatal("a custom section checklist implicitly became a trusted partner")
	}
	useAccess(e.b, botConfig{Owner: ownerID, Admins: []int64{fullAdmin}})
	if _, err := patchAdminMeta(fullAdmin, func(m *model.BotAdminMeta) { m.Suspended = true }); err != nil {
		t.Fatal(err)
	}
	if b := e.b.as(fullAdmin); b.canManageAdmins() || b.canApprove() || b.allowed([]string{"q", "home"}) {
		t.Fatal("suspended partner retained global management rights")
	}
}

func TestPartnerShopSalesAndTrialsUseOwnGroup(t *testing.T) {
	for _, topic := range []string{"buy", "test"} {
		t.Run(topic, func(t *testing.T) {
			e := newAccessEnv(t)
			setShop(t, shopConfig{
				Plans: []shopPlan{{ID: "1", Name: "Gold", Days: 30, GB: 10, Group: otherGroup}},
				Desks: map[string]shopDesk{"42": {SalesGroup: "Partners"}},
			})
			o := &model.BotOrder{TgId: 5001, Topic: topic, PlanID: "1", AdminId: fullAdmin, Status: orderPending, Hint: "partner-sale"}
			if err := e.b.saveOrder(o); err != nil {
				t.Fatal(err)
			}
			if err := e.b.approveOrder(o); err != nil {
				t.Fatal(err)
			}
			c, err := rawFullClient(o.ClientId)
			if err != nil || c.Group != "Partners" || o.AdminId != fullAdmin {
				t.Fatalf("sale lost its owner/group: client=%+v err=%v", c, err)
			}
			if e.b.as(fullAdmin).restricted() {
				t.Fatal("selling into a personal group restricted the partner")
			}
		})
	}
}

func TestPartnerGlobalShopAndReportsAreSeparateFromPersonalShop(t *testing.T) {
	e := newAccessEnv(t)
	setShop(t, shopConfig{PayCard: "global-card", Desks: map[string]shopDesk{"42": {PayCard: "personal-card", SalesGroup: "Partners"}}})
	e.press(fullAdmin, "q:gshop")
	global := e.b.as(fullAdmin).deskView(fullAdmin)
	if global.self != fullAdmin || global.owner || !global.globalShop || global.loadShop().PayCard != "global-card" {
		t.Fatal("global shop changed the acting identity or opened the personal shop")
	}
	s := global.loadShop()
	s.PayCard = "new-global-card"
	if err := global.saveShop(s); err != nil {
		t.Fatal(err)
	}
	if raw := e.b.loadShopRaw(); raw.PayCard != "new-global-card" || raw.Desks["42"].PayCard != "personal-card" {
		t.Fatal("global shop editing changed a personal shop")
	}
	if all, id := global.orderAdminFilter(fullAdmin); !all || id != 0 {
		t.Fatal("global reports were scoped to the partner's own sales")
	}
	e.press(fullAdmin, "q:myshop")
	personal := e.b.as(fullAdmin).deskView(fullAdmin)
	if personal.globalShop || personal.loadShop().PayCard != "personal-card" {
		t.Fatal("leaving global mode did not restore the personal shop")
	}
	if all, id := personal.orderAdminFilter(fullAdmin); all || id != fullAdmin {
		t.Fatal("personal reports included everyone else's sales")
	}
}

func TestUnusableSalesGroupDoesNotChangeAuthorization(t *testing.T) {
	e := newAccessEnv(t)
	for _, group := range []string{service.ClusterGroup, "bad\nline", strings.Repeat("x", maxGroupRunes+1)} {
		setShop(t, shopConfig{Desks: map[string]shopDesk{"42": {SalesGroup: group}, "77": {SalesGroup: group}}})
		if e.b.sellerGroup(fullAdmin) != "" || !e.b.as(fullAdmin).canManageAdmins() {
			t.Fatalf("unusable personal sales group changed authorization: %q", group)
		}
		if e.b.sellerGroup(salesAdmin) != salesGroup || e.b.as(salesAdmin).inScope(otherGroup) {
			t.Fatal("a desk setting widened a scoped seller's access")
		}
		o := &model.BotOrder{TgId: 5001, Topic: "buy", AdminId: fullAdmin, PlanSet: true, PlanGB: 1, PlanDays: 1}
		before := clientCount(t)
		if err := e.b.approveOrder(o); !errors.Is(err, errBadGroup) || clientCount(t) != before {
			t.Fatalf("invalid group did not fail the sale closed: %v", err)
		}
	}
}

func TestPartnerSetsPersonalGroupFromTelegramWithoutChangingAccess(t *testing.T) {
	e := newAccessEnv(t)
	beforeAdmins, beforeScopes, beforePerms := accessSettings(t)
	e.press(fullAdmin, "k:sgrp")
	if p := e.b.pend.get(fullAdmin); p == nil || p.kind != "shop.salesgroup" {
		t.Fatal("full partner was not offered their personal group editor")
	}
	e.say(fullAdmin, "Partners")
	if e.b.sellerGroup(fullAdmin) != "Partners" || e.b.as(fullAdmin).restricted() {
		t.Fatal("personal group was not saved independently of access")
	}
	if admins, scopes, perms := accessSettings(t); admins != beforeAdmins || scopes != beforeScopes || perms != beforePerms {
		t.Fatal("editing the sales group changed access settings")
	}
	for _, id := range []int64{salesAdmin, clientsOnly, noSections} {
		if err := e.b.as(id).setSalesGroup("Other"); !errors.Is(err, errNotOwner) {
			t.Fatalf("restricted administrator %d changed a personal sales group: %v", id, err)
		}
	}
	var edits int64
	if err := database.GetDB().Model(&model.Changes{}).
		Where("actor = ? AND `key` = ?", "telegram:42", "settings").Count(&edits).Error; err != nil {
		t.Fatal(err)
	}
	if edits == 0 {
		t.Fatal("personal group edit was not attributed to its actor")
	}
	if err := e.b.as(fullAdmin).setSalesGroup(service.ClusterGroup); !errors.Is(err, errBadGroup) {
		t.Fatalf("reserved group accepted: %v", err)
	}
	e.press(fullAdmin, "k:sgrp")
	e.say(fullAdmin, "-")
	if e.b.sellerGroup(fullAdmin) != "" || !e.b.as(fullAdmin).canManageAdmins() {
		t.Fatal("clearing the personal group changed administrative access")
	}
}

func TestRevokedPartnerCannotWriteAnOpenGlobalShop(t *testing.T) {
	e := newAccessEnv(t)
	setShop(t, shopConfig{PayCard: "global-card", Desks: map[string]shopDesk{"42": {PayCard: "personal-card"}}})
	e.press(fullAdmin, "q:gshop")
	stale := e.b.as(fullAdmin).deskView(fullAdmin)
	s := stale.loadShop()
	e.configure(map[string]string{"tgBotPerms": "42=clients\n55=clients,stats\n56=\n57=nodes\n58=core"})
	s.PayCard = "unauthorized"
	if err := stale.saveShop(s); !errors.Is(err, errNotOwner) {
		t.Fatalf("revoked partner wrote an open global view: %v", err)
	}
	e.press(fullAdmin, "k:pcard")
	if e.b.deskTarget(fullAdmin) != 0 || e.b.pend.get(fullAdmin) != nil {
		t.Fatal("revoked global shop context was retained")
	}
	raw := e.b.loadShopRaw()
	if raw.PayCard != "global-card" || raw.Desks["42"].PayCard != "personal-card" {
		t.Fatal("revocation redirected an edit into the global or personal shop")
	}
}

func TestFullPartnerCanApproveAnotherSellersOrder(t *testing.T) {
	e := newAccessEnv(t)
	setShop(t, shopConfig{Plans: []shopPlan{{ID: "1", Name: "Gold", Days: 30, GB: 10}}})
	if _, err := patchAdminMeta(salesAdmin, func(m *model.BotAdminMeta) { m.RequireOwner = true }); err != nil {
		t.Fatal(err)
	}
	o := &model.BotOrder{TgId: 5001, Topic: "buy", PlanID: "1", AdminId: salesAdmin, Status: orderPending, Hint: "seller-order"}
	if err := e.b.saveOrder(o); err != nil {
		t.Fatal(err)
	}
	e.press(fullAdmin, "j:ok:"+itoa(int64(o.Id)))
	done := lastOrder(t, o.TgId)
	if done.Status != orderApproved || done.AdminId != salesAdmin {
		t.Fatalf("trusted partner could not manage the order without taking its sale: %+v", done)
	}
	c, err := rawFullClient(done.ClientId)
	if err != nil || c.Group != salesGroup {
		t.Fatalf("global approval changed the seller's client group: %+v, %v", c, err)
	}
	var decision model.Changes
	if err := database.GetDB().Where("actor = ? AND `key` = ? AND action = ?", "telegram:42", "orders", "approve").
		Last(&decision).Error; err != nil {
		t.Fatalf("decision was not attributed to the actual partner: %v", err)
	}
	var detail map[string]interface{}
	if err := json.Unmarshal(decision.Obj, &detail); err != nil || detail["sellerId"] != float64(salesAdmin) || detail["clientId"] != float64(done.ClientId) {
		t.Fatalf("decision lost the original seller or target client: %s, %v", decision.Obj, err)
	}
}

func TestPartnerSalesQuotaDoesNotBlockGlobalAdministration(t *testing.T) {
	e := newAccessEnv(t)
	giveQuota(t, fullAdmin, 0)
	partner := e.b.as(fullAdmin)
	if err := partner.setVolume(e.id["o1"], 2*gib); err != nil {
		t.Fatalf("an exhausted personal sales quota blocked global administration: %v", err)
	}
	setShop(t, shopConfig{
		Plans: []shopPlan{{ID: "1", Name: "Gold", Days: 30, GB: 10}},
		Desks: map[string]shopDesk{"42": {SalesGroup: "Partners"}},
	})
	o := &model.BotOrder{TgId: 5001, Topic: "buy", PlanID: "1", AdminId: fullAdmin, Status: orderPending}
	before := clientCount(t)
	if err := partner.approveOrder(o); err == nil || clientCount(t) != before {
		t.Fatalf("personal shop delivery bypassed its sales quota: %v", err)
	}
}
