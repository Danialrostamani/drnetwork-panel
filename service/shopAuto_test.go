package service

import (
	"errors"
	"testing"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
)

func TestShopAutoRenewSettings(t *testing.T) {
	shopDB(t)
	s := &ShopService{}
	p := &model.ShopPlan{Name: "m", Days: 30, Price: 1000, Enable: true}
	if err := s.SavePlan(p); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetAutoRenew(7, 1, true); !errors.Is(err, ErrNoRenewPlan) {
		t.Fatalf("no order yet: %v", err)
	}
	db := database.GetDB()
	db.Create(&model.ShopOrder{TgId: 1, Kind: model.OrderBuy, PlanId: p.Id, ClientId: 7, Status: model.OrderApproved})
	got, err := s.SetAutoRenew(7, 1, true)
	if err != nil || got.Id != p.Id {
		t.Fatalf("on: %+v %v", got, err)
	}
	r, ok := s.AutoRenewOf(7)
	if !ok || !r.Enable || r.TgId != 1 || r.PlanId != p.Id {
		t.Fatalf("row: %+v %v", r, ok)
	}
	s.AutoRenewed(7, 0, 100)
	s.AutoRenewed(7, 0, 200)
	if r, _ := s.AutoRenewOf(7); r.Fails != 2 || r.LastFailAt != 200 {
		t.Fatalf("failures: %+v", r)
	}
	s.AutoRenewed(7, p.Id, 300)
	if r, _ := s.AutoRenewOf(7); r.Fails != 0 || r.LastFailAt != 0 || r.LastAt != 300 {
		t.Fatalf("renewed: %+v", r)
	}
	// Turning it on again keeps one row.
	if _, err := s.SetAutoRenew(7, 1, true); err != nil {
		t.Fatal(err)
	}
	if rows, _ := s.AutoRenews(); len(rows) != 1 {
		t.Fatalf("rows: %+v", rows)
	}
	if _, err := s.SetAutoRenew(7, 1, false); err != nil {
		t.Fatal(err)
	}
	if rows, _ := s.AutoRenews(); len(rows) != 0 {
		t.Fatalf("still on: %+v", rows)
	}
	// A plan taken off sale cannot renew.
	p.Enable = false
	if err := s.SavePlan(p); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RenewPlanOf(7); !errors.Is(err, ErrNoRenewPlan) {
		t.Fatalf("plan off sale: %v", err)
	}
	s.DropAutoRenew(7)
	if _, ok := s.AutoRenewOf(7); ok {
		t.Fatal("row not dropped")
	}
}

func TestAutoRenewDue(t *testing.T) {
	now := time.Now().Unix()
	cases := []struct {
		c   model.Client
		due bool
	}{
		{model.Client{Expiry: now + 3600}, true},
		{model.Client{Expiry: now - 3600}, true},
		{model.Client{Expiry: now + 3*86400}, false},
		{model.Client{Expiry: now + 3600, DelayStart: true}, false},
		{model.Client{Volume: 100, Up: 50, Down: 45}, true},
		{model.Client{Volume: 100, Up: 50, Down: 40}, false},
		{model.Client{}, false},
	}
	for i, c := range cases {
		if got := AutoRenewDue(&c.c, now); got != c.due {
			t.Errorf("case %d: due = %v", i, got)
		}
	}
}
