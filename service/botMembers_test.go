package service

import (
	"testing"

	"github.com/Danialrostamani/drnetwork-panel/database/model"
)

// The members page counts each person's test account and purchases from the
// bot's orders, and lists people known only from an order too.
func TestBotMembersList(t *testing.T) {
	db := settingTestDB(t)
	rows := []any{
		&model.BotCustomer{TgId: 10, Name: "Ali", Username: "ali", AdminId: 7, FirstSeen: 100, LastSeen: 500},
		&model.BotCustomer{TgId: 20, Name: "Sara", FirstSeen: 200},
		&model.BotOrder{TgId: 10, Topic: "test", Status: "approved", CreatedAt: 110, UpdatedAt: 120},
		&model.BotOrder{TgId: 10, Topic: "buy", Status: "approved", Amount: 300, CreatedAt: 130, UpdatedAt: 140},
		&model.BotOrder{TgId: 10, Topic: "renew", Status: "approved", Amount: 200, CreatedAt: 150, UpdatedAt: 160},
		&model.BotOrder{TgId: 10, Topic: "buy", Status: "rejected", Amount: 999, CreatedAt: 170},
		&model.BotOrder{TgId: 10, Topic: "buy", Status: "pending", Amount: 300, CreatedAt: 180},
		&model.BotOrder{TgId: 30, Topic: "wallet", Status: "approved", Amount: 50, CreatedAt: 50},
		&model.BotRef{TgId: 10, Word: "shop1", Group: "g1"},
		&model.BotWallet{TgId: 10, Balance: 75},
		&model.Client{Name: "ali-1", TgId: 10, Enable: true},
	}
	for _, r := range rows {
		if err := db.Create(r).Error; err != nil {
			t.Fatal(err)
		}
	}
	list, err := (&BotMemberService{}).List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 || list[0].TgId != 20 || list[1].TgId != 10 || list[2].TgId != 30 {
		t.Fatalf("members (newest first) = %+v", list)
	}
	ali := list[1]
	if ali.TrialAt != 120 || ali.Buys != 1 || ali.Renews != 1 || ali.Spent != 500 || ali.LastBuy != 160 || ali.Pending != 1 {
		t.Fatalf("ali = %+v", ali)
	}
	if ali.AdminId != 7 || ali.Ref != "shop1" || ali.RefGroup != "g1" || ali.Balance != 75 || len(ali.Clients) != 1 || ali.Clients[0] != "ali-1" {
		t.Fatalf("ali = %+v", ali)
	}
	if list[0].TrialAt != 0 || list[0].Buys != 0 || list[0].Clients == nil {
		t.Fatalf("sara = %+v", list[0])
	}
	if list[2].FirstSeen != 50 || list[2].Spent != 0 {
		t.Fatalf("order-only member = %+v", list[2])
	}
}

// The introducer can only be one of the shop's words, kept as the shop spells
// it with its group; an empty word removes it.
func TestBotMemberSetRef(t *testing.T) {
	db := settingTestDB(t)
	s := &SettingService{}
	if err := s.SetTgBotShop(`{"refs":[{"id":"1","word":"Shop1","group":"g1"},{"id":"2","word":"other","group":""}]}`); err != nil {
		t.Fatal(err)
	}
	ms := &BotMemberService{}
	if got := ms.Refs(); len(got) != 2 || got[0].Word != "Shop1" || got[0].Group != "g1" {
		t.Fatalf("refs = %+v", got)
	}
	if err := ms.SetRef(10, " shop1 "); err != nil {
		t.Fatal(err)
	}
	var r model.BotRef
	if err := db.First(&r, 10).Error; err != nil || r.Word != "Shop1" || r.Group != "g1" {
		t.Fatalf("ref = %+v, %v", r, err)
	}
	if err := ms.SetRef(10, "nope"); err == nil {
		t.Fatal("an unknown word was accepted")
	}
	if err := ms.SetRef(10, "other"); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&r, 10).Error; err != nil || r.Word != "other" || r.Group != "" {
		t.Fatalf("ref = %+v, %v", r, err)
	}
	if err := ms.SetRef(10, ""); err != nil {
		t.Fatal(err)
	}
	var n int64
	db.Model(&model.BotRef{}).Where("tg_id = ?", 10).Count(&n)
	if n != 0 {
		t.Fatal("the introducer was not removed")
	}
	if err := ms.SetRef(0, "other"); err == nil {
		t.Fatal("telegram id 0 was accepted")
	}
}
