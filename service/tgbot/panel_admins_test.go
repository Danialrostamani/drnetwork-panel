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

func TestAutoGroupName(t *testing.T) {
	for in, want := range map[string]string{"Ali 😀 Reza": "Ali Reza", "": "admin-5", "🔥🔥": "admin-5", "علی_فروش": "علی_فروش"} {
		if got := autoGroupName(in, 5); got != want {
			t.Errorf("autoGroupName(%q) = %q, want %q", in, got, want)
		}
	}
}

// The panel gives every administrator but the owner a group of their own and
// moves the clients of their customers and sales into it; the owner cannot be
// limited and keeps every client.
func TestPanelAdminGroups(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "bot.db")); err != nil {
		t.Fatal(err)
	}
	ss := &service.SettingService{}
	if err := ss.SetMaintenance(true); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]string{"tgBotOwner": "1000", "tgBotAdmins": "42, 43"})
	if err := ss.Save(database.GetDB(), raw); err != nil {
		t.Fatal(err)
	}
	cs := service.NewConfigService(core.NewCore())
	add := func(name, group string, tg int64) uint {
		c := model.Client{Enable: true, Name: name, Config: newClientConfig(name), Inbounds: json.RawMessage(`[]`), Links: json.RawMessage(`[]`), Group: group, TgId: tg}
		data, _ := json.Marshal(c)
		if _, err := cs.Save("clients", "new", data, "", "admin", ""); err != nil {
			t.Fatal(err)
		}
		var got model.Client
		if err := database.GetDB().Where("name = ?", name).First(&got).Error; err != nil {
			t.Fatal(err)
		}
		return got.Id
	}
	add("bound", "", 500)
	sold := add("sold", "plans", 0)
	add("other", "", 0)
	db := database.GetDB()
	if err := db.Create(&model.BotCustomer{TgId: 500, AdminId: 42, FirstSeen: 1}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.BotOrder{TgId: 600, Topic: "buy", Status: orderApproved, AdminId: 42, ClientId: sold}).Error; err != nil {
		t.Fatal(err)
	}

	out, err := PanelAutoGroups(context.Background(), cs, "admin", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 || out[0].Group != "admin-42" || !out[0].New || out[0].Moved != 2 || out[1].Group != "admin-43" || out[1].Moved != 0 {
		t.Fatalf("auto groups = %+v", out)
	}
	if _, scopes, _ := accessSettings(t); scopes != "42=admin-42\n43=admin-43" {
		t.Fatalf("scopes = %q", scopes)
	}
	groups := map[string]string{}
	for _, c := range allClients() {
		groups[c.Name] = c.Group
	}
	if groups["bound"] != "admin-42" || groups["sold"] != "admin-42" || groups["other"] != "" {
		t.Fatalf("groups = %v", groups)
	}

	view, err := PanelAdmins(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Admins) != 3 || view.Admins[0].Role != "owner" || view.Admins[1].Role != "group" || view.Admins[1].Clients != 2 || view.Admins[1].Customers != 1 || view.Admins[1].Outside != 0 {
		t.Fatalf("admins = %+v", view.Admins)
	}

	if err := PanelSetAdminGroup(cs, "admin", "", 1000, "x"); err == nil {
		t.Fatal("the owner was limited to a group")
	}
	if err := PanelSetAdminGroup(cs, "admin", "", 42, ""); err != nil {
		t.Fatal(err)
	}
	admins, scopes, _ := accessSettings(t)
	if scopes != "43=admin-43" || admins != "42, 43" {
		t.Fatalf("admins = %q, scopes = %q", admins, scopes)
	}
	if err := PanelSetAdminGroup(cs, "admin", "", 43, "VIP"); err != nil {
		t.Fatal(err)
	}
	if _, scopes, _ := accessSettings(t); scopes != "43=VIP" {
		t.Fatalf("scopes = %q", scopes)
	}
	if err := PanelSetAdminGroup(cs, "admin", "", 999, "VIP"); err == nil {
		t.Fatal("somebody who is not an administrator got a group")
	}
}

func TestSellerGroupAndCreatedNote(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "bot.db")); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]string{"tgBotOwner": "1000", "tgBotAdmins": "42", "tgBotScopes": "77=Sales"})
	if err := (&service.SettingService{}).Save(database.GetDB(), raw); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	b := newBot(cfg)
	if b.sellerGroup(77) != "Sales" || b.sellerGroup(42) != "" || b.sellerGroup(1000) != "" || b.sellerGroup(5) != "" {
		t.Fatal("seller groups are wrong")
	}
	one := b.createdNote("new", model.Client{Name: "a1", Group: "Sales", Volume: 10 << 30})
	many := b.createdNote("addbulk", []model.Client{{Name: "b1"}, {Name: "b2"}})
	if one == "" || many == "" || b.createdNote("new", 5) != "" {
		t.Fatalf("notes = %q / %q", one, many)
	}
}
