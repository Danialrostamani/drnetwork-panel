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

// The panel edits a customer's note and configs, gives them another test
// account, and forgets them while their paid orders stay for the accounts.
func TestPanelCustomerEditResetDelete(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "bot.db")); err != nil {
		t.Fatal(err)
	}
	if err := (&service.SettingService{}).SetMaintenance(true); err != nil {
		t.Fatal(err)
	}
	cs := service.NewConfigService(core.NewCore())
	db := database.GetDB()
	add := func(name string, tg int64) uint {
		c := model.Client{Enable: true, Name: name, Config: newClientConfig(name), Inbounds: json.RawMessage(`[]`), Links: json.RawMessage(`[]`), TgId: tg}
		data, _ := json.Marshal(c)
		if _, err := cs.Save("clients", "new", data, "", "admin", ""); err != nil {
			t.Fatal(err)
		}
		var got model.Client
		if err := db.Where("name = ?", name).First(&got).Error; err != nil {
			t.Fatal(err)
		}
		return got.Id
	}
	trial := add("trial1", 5)
	paid := add("paid1", 0)
	other := add("other1", 9)
	db.Create(&model.BotCustomer{TgId: 5, Name: "Ali", FirstSeen: 1})
	db.Create(&model.BotOrder{TgId: 5, Topic: "test", Status: orderApproved, ClientId: trial})
	db.Create(&model.BotOrder{TgId: 5, Topic: "buy", Status: orderApproved, Amount: 100, AdminId: 42, ClientId: paid})
	db.Create(&model.BotOrder{TgId: 5, Topic: "buy", Status: orderPending})
	db.Create(&model.BotRef{TgId: 5, Word: "insta"})
	if _, err := walletCredit(5, 50, walletMemo{kind: walletManual}); err != nil {
		t.Fatal(err)
	}

	// edit: the note, and the configs become exactly paid1 and other1
	if err := PanelEditCustomer(5, "  vip  ", true, []uint{paid, other, paid}); err != nil {
		t.Fatal(err)
	}
	if c, _ := customerRow(5); c.Note != "vip" || c.Name != "Ali" {
		t.Fatalf("customer = %+v", c)
	}
	tg := func(id uint) int64 {
		var c model.Client
		db.Select("tg_id").Where("id = ?", id).First(&c)
		return c.TgId
	}
	if tg(trial) != 0 || tg(paid) != 5 || tg(other) != 5 {
		t.Fatalf("bound: trial=%d paid=%d other=%d", tg(trial), tg(paid), tg(other))
	}
	if PanelEditCustomer(5, "", true, []uint{99999}) == nil {
		t.Fatal("a client that does not exist was bound")
	}
	if tg(paid) != 5 {
		t.Fatal("a failed edit changed the bound clients")
	}
	// a note alone leaves the clients
	if err := PanelEditCustomer(5, "x", false, nil); err != nil || tg(paid) != 5 {
		t.Fatal("editing the note changed the clients: ", err)
	}

	// trial reset: the test order stops counting, the test client is deleted
	if !hasUsedTrial(5) {
		t.Fatal("the customer had a trial")
	}
	out, err := PanelResetTrial(context.Background(), cs, "admin", "", 5, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if hasUsedTrial(5) || out.Orders != 1 || len(out.Deleted) != 1 || out.Deleted[0] != "trial1" || out.Bound != 2 || out.Notified {
		t.Fatalf("reset = %+v", out)
	}
	var n int64
	db.Model(&model.Client{}).Where("id = ?", trial).Count(&n)
	if n != 0 {
		t.Fatal("the test client was not deleted")
	}
	if _, err := PanelResetTrial(context.Background(), cs, "admin", "", 5, true, false); err == nil {
		t.Fatal("a customer without a test account was reset")
	}

	// delete: configs unbound, profile, ref, wallet and pending orders gone,
	// the paid order kept without the customer
	del, err := PanelDeleteCustomer(cs, "admin", "", 5, false)
	if err != nil {
		t.Fatal(err)
	}
	if del.Unbound != 2 || del.Deleted != 0 || del.Balance != 50 {
		t.Fatalf("delete = %+v", del)
	}
	if tg(paid) != 0 || tg(other) != 0 {
		t.Fatal("the clients were not unbound")
	}
	if _, ok := customerRow(5); ok || loadBotRef(5) != nil || walletBalance(5) != 0 {
		t.Fatal("the customer was not forgotten")
	}
	var left []model.BotOrder
	db.Find(&left)
	if len(left) != 1 || left[0].Topic != "buy" {
		t.Fatalf("orders = %+v", left)
	}
	for _, o := range left {
		if o.TgId != 0 || o.Status == orderPending {
			t.Fatalf("order kept the customer: %+v", o)
		}
	}
	members, err := (&service.BotMemberService{}).List()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range members {
		if m.TgId == 5 {
			t.Fatal("the deleted customer is still listed")
		}
	}

	// delete with configs
	db.Model(&model.Client{}).Where("id = ?", other).Update("tg_id", 6)
	db.Create(&model.BotCustomer{TgId: 6, FirstSeen: 1})
	if del, err = PanelDeleteCustomer(cs, "admin", "", 6, true); err != nil || del.Deleted != 1 {
		t.Fatalf("delete = %+v, %v", del, err)
	}
	db.Model(&model.Client{}).Where("id = ?", other).Count(&n)
	if n != 0 {
		t.Fatal("the customer's client was not deleted")
	}
	if _, err := PanelDeleteCustomer(cs, "admin", "", 0, false); err == nil {
		t.Fatal("a bad id was accepted")
	}
}
