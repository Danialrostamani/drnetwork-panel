package tgbot

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/Danialrostamani/drnetwork-panel/core"
	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/service"
)

// The panel adds sellers, changes what they may do, gives them a limit and
// terms, and takes them away again; the owner is out of reach of all of it.
func TestPanelManagesSellers(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "bot.db")); err != nil {
		t.Fatal(err)
	}
	ss := &service.SettingService{}
	if err := ss.SetMaintenance(true); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]string{"tgBotOwner": "1000"})
	if err := ss.Save(database.GetDB(), raw); err != nil {
		t.Fatal(err)
	}
	cs := service.NewConfigService(core.NewCore())
	ctx := context.Background()

	group, err := PanelAddAdmin(ctx, cs, "admin", "", 42, false, "")
	if err != nil || group != "admin-42" {
		t.Fatalf("add = %q, %v", group, err)
	}
	if _, err := PanelAddAdmin(ctx, cs, "admin", "", 43, true, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := PanelAddAdmin(ctx, cs, "admin", "", 42, true, ""); err == nil {
		t.Fatal("added the same seller twice")
	}
	if _, err := PanelAddAdmin(ctx, cs, "admin", "", 1000, true, ""); err == nil {
		t.Fatal("added the owner as a seller")
	}
	if admins, scopes, _ := accessSettings(t); admins != "42, 43" || scopes != "42=admin-42" {
		t.Fatalf("admins %q scopes %q", admins, scopes)
	}
	if err := PanelSetAdminRole(cs, "admin", "", 43, "sections", "", []string{"clients", "stats", "bogus"}); err != nil {
		t.Fatal(err)
	}
	if _, _, perms := accessSettings(t); perms != "43=clients,stats" {
		t.Fatalf("perms %q", perms)
	}
	if err := PanelSetAdminRole(cs, "admin", "", 1000, "group", "x", nil); err == nil {
		t.Fatal("limited the owner")
	}

	if err := PanelSetQuota("admin", 42, "set", 100); err != nil {
		t.Fatal(err)
	}
	if err := PanelSetQuota("admin", 42, "add", 50); err != nil {
		t.Fatal(err)
	}
	if err := PanelSetQuota("admin", 1000, "set", 10); err == nil {
		t.Fatal("limited the owner's volume")
	}
	if err := PanelSetSellerTerms(42, PanelSellerTerms{RequireOwner: true, RatePerGB: 2000, MinPerGB: -5}); err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().Create(&model.BotOrder{TgId: 600, Topic: "buy", Status: orderApproved, AdminId: 42, PlanSet: true, PlanGB: 10, Amount: 90000}).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().Create(&model.BotOrder{TgId: 601, Topic: "buy", Status: orderPending, AdminId: 42}).Error; err != nil {
		t.Fatal(err)
	}

	view, err := PanelAdmins(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var seller *PanelAdmin
	for i := range view.Admins {
		if view.Admins[i].Id == 42 {
			seller = &view.Admins[i]
		}
	}
	if seller == nil || seller.Quota == nil || seller.Quota.Total != 150*gib || !seller.RequireOwner || seller.RatePerGB != 2000 || seller.MinPerGB != 0 || seller.Owed != 20000 || len(view.Sections) != len(sectionDefs) {
		t.Fatalf("seller = %+v", seller)
	}
	dash, err := PanelBotDashboard()
	if err != nil {
		t.Fatal(err)
	}
	if dash.Pending != 1 || len(dash.Orders) != 1 || dash.Sellers != 2 || dash.Owed != 20000 || dash.Owner != 1000 {
		t.Fatalf("dashboard = %+v", dash)
	}
	if _, err := PanelSettle(42); err != nil {
		t.Fatal(err)
	}
	if _, _, owed := unsettled(42); owed != 0 {
		t.Fatalf("owed after settling: %d", owed)
	}

	if err := PanelRemoveAdmin(cs, "admin", "", 42); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := loadQuota(42); ok {
		t.Fatal("the limit outlived the seller")
	}
	if admins, scopes, _ := accessSettings(t); admins != "43" || scopes != "" {
		t.Fatalf("after removal: admins %q scopes %q", admins, scopes)
	}
	if err := PanelRemoveAdmin(cs, "admin", "", 1000); err == nil {
		t.Fatal("removed the owner")
	}
}
