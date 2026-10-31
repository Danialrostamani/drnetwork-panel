package tgbot

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/service"
)

func TestPrivateBotKeepsEveryToolForOwnerAndPartners(t *testing.T) {
	e := newAccessEnv(t)
	e.b.cfg.Private = true
	e.b.cfg.Panel = false
	for _, id := range []int64{ownerID, fullAdmin} {
		b := e.b.as(id)
		for _, section := range sectionDefs {
			if !b.can(section.key) {
				t.Errorf("%d lost %s", id, section.key)
			}
		}
		for _, data := range []string{"o:in:ls:0", "s:ls", "o:nd:n", "m:backup", "m:restart", "a:add", "q:home"} {
			if !b.allowed(strings.Split(data, ":")) {
				t.Errorf("%d denied %s", id, data)
			}
		}
		if !b.pendingAllowed(&pending{kind: "obj.new", key: "nd"}) {
			t.Fatal("partner node wizard blocked")
		}
	}
}
func TestPrivateBotCannotBeBypassedWithCommandsCallbacksOrPending(t *testing.T) {
	e := newAccessEnv(t)
	e.b.cfg.Private = true
	e.b.cfg.Panel = true
	e.configure(map[string]string{"tgBotPerms": "55=clients,stats,backup,nodes,core,settings,admins,home,logs"})
	b := e.b.as(clientsOnly)
	if !b.allowedCommand("clients") {
		t.Fatal("necessary client tools lost")
	}
	for _, cmd := range []string{"settings", "nodes", "backup", "logs", "restart", "stats", "home"} {
		if b.allowedCommand(cmd) {
			t.Errorf("private command leaked: %s", cmd)
		}
	}
	for _, data := range []string{"s:ls", "m:backup", "t:d1", "h:home", "a:ls", "a:add", "q:home", "o:nd:ls:0"} {
		if b.allowed(strings.Split(data, ":")) {
			t.Errorf("private callback leaked: %s", data)
		}
	}
	for _, p := range []*pending{{kind: "obj.new", key: "nd"}, {kind: "set"}, {kind: "ad.add"}} {
		if b.pendingAllowed(p) {
			t.Errorf("private prompt leaked: %+v", p)
		}
	}
	if len(b.techMenu()) != 0 {
		t.Fatal("nonpartner sees technical menu")
	}
	if !e.b.as(salesAdmin).allowedCommand("clients") {
		t.Fatal("scoped seller lost client tools")
	}
}
func TestPrivateBotSettingDefaultsSecurely(t *testing.T) {
	_, _ = testBot(t)
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Private {
		t.Fatal("fresh configuration is not private")
	}
}
func TestGrantPartnerRetainsSellerGroupShopAndOwner(t *testing.T) {
	e := newAccessEnv(t)
	raw := `{"extra":"keep","desks":{"77":{"payCard":"card","trialGB":2,"salesGroup":"wrong","future":{"x":1}},"42":{"payCard":"other"}}}`
	e.configure(map[string]string{"tgBotShop": raw})
	if err := PanelGrantPartner(e.b.configService, "web-owner", "", salesAdmin); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Owner != ownerID || !cfg.access().roleOf(salesAdmin).full() {
		t.Fatal("grant changed owner or lost full role")
	}
	shop, err := (&service.SettingService{}).GetTgBotShop()
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]json.RawMessage
	if err = json.Unmarshal([]byte(shop), &got); err != nil {
		t.Fatal(err)
	}
	if string(got["extra"]) != `"keep"` || !strings.Contains(shop, `"future":{"x":1}`) {
		t.Fatalf("unknown shop settings lost: %s", shop)
	}
	b := newBot(cfg)
	d := b.loadShopRaw().Desks["77"]
	if d.SalesGroup != salesGroup || d.PayCard != "card" || d.TrialGB != 2 {
		t.Fatalf("shop changed: %+v", d)
	}
	if err := PanelGrantPartner(e.b.configService, "web-owner", "", salesAdmin); err != nil {
		t.Fatal("idempotent grant failed", err)
	}
	if _, err := PanelAdmins(context.Background()); err != nil {
		t.Fatal(err)
	}
}
func TestGrantPartnerFailsClosed(t *testing.T) {
	e := newAccessEnv(t)
	for _, id := range []int64{-1, 0, ownerID} {
		if err := PanelGrantPartner(e.b.configService, "owner", "", id); err == nil {
			t.Fatalf("accepted %d", id)
		}
	}
	e.configure(map[string]string{"tgBotShop": "{broken"})
	if err := PanelGrantPartner(e.b.configService, "owner", "", salesAdmin); err == nil {
		t.Fatal("discarded malformed shop")
	}
	cfg, _ := loadConfig()
	if cfg.access().roleOf(salesAdmin).group != salesGroup {
		t.Fatal("partially granted on failure")
	}
	e.configure(map[string]string{"tgBotAdmins": "42,800"})
	if err := PanelSetSellerTerms(800, PanelSellerTerms{Suspended: true}); err != nil {
		t.Fatal(err)
	}
	if err := PanelGrantPartner(e.b.configService, "owner", "", 800); err == nil {
		t.Fatal("silently reactivated suspended person")
	}
	e.configure(map[string]string{"tgBotOwner": ""})
	if err := PanelGrantPartner(e.b.configService, "owner", "", 900); err == nil {
		t.Fatal("grant without owner")
	}
	var n int64
	database.GetDB().Table("settings").Where("key = ? AND value LIKE ?", "tgBotAdmins", "%900%").Count(&n)
	if n != 0 {
		t.Fatal("failed grant persisted")
	}
}

func TestPrivateBotBroadcastsOnlyToOwnerAndPartners(t *testing.T) {
	e := newAccessEnv(t)
	e.b.cfg.Private = true
	e.configure(map[string]string{"tgBotPerms": "55=stats,backup,home,nodes,core\n57=nodes\n58=core"})
	for _, section := range []string{"nodes", "home", "core", "stats", "backup"} {
		n := len(e.got())
		e.b.broadcast(context.Background(), "private notice", section)
		got := e.since(n)
		if len(got) != 2 {
			t.Fatalf("%s: expected only owner and partner, got %+v", section, got)
		}
		for _, m := range got {
			if m.ChatID != ownerID && m.ChatID != fullAdmin {
				t.Fatalf("%s leaked to %d", section, m.ChatID)
			}
		}
	}
}

func TestPrivateBotScheduledReportHasNoDelegatedRecipients(t *testing.T) {
	e := newAccessEnv(t)
	e.b.cfg.Private = true
	e.configure(map[string]string{"tgBotPerms": "55=stats,backup,home\n57=home\n58=backup"})
	e.b.cfg.ReportBackup = true
	n := len(e.got())
	e.b.sendReport(context.Background())
	recipients := map[int64]bool{}
	for _, m := range e.since(n) {
		if m.ChatID != ownerID && m.ChatID != fullAdmin {
			t.Fatalf("report or backup leaked to %d", m.ChatID)
		}
		recipients[m.ChatID] = true
	}
	if !recipients[ownerID] || !recipients[fullAdmin] {
		t.Fatalf("missing management reports: %v", recipients)
	}
}
