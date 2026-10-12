package service

import (
	"strings"
	"testing"

	"github.com/Danialrostamani/drnetwork-panel/database/model"
)

func TestShopWelcomeSettingAndPlanDesc(t *testing.T) {
	shopDB(t)
	s := &ShopService{}
	if err := CheckShopSetting("shopWelcome", DefaultShopWelcome); err != nil {
		t.Fatal(err)
	}
	if err := CheckShopSetting("shopWelcome", strings.Repeat("x", 4001)); err == nil {
		t.Fatal("overlong welcome was taken")
	}
	if st := s.Settings(); st.Welcome != DefaultShopWelcome {
		t.Fatalf("default welcome %q", st.Welcome)
	}
	p := &model.ShopPlan{Name: "Eco 10", Volume: 10 << 30, Days: 0, Price: 46000, Enable: true, Category: "اقتصادی", Desc: "بدون محدودیت زمانی"}
	if err := s.SavePlan(p); err != nil {
		t.Fatal(err)
	}
	got, err := s.Plan(p.Id)
	if err != nil || got.Desc != p.Desc || got.Category != p.Category {
		t.Fatalf("plan %+v err %v", got, err)
	}
	p.Desc = strings.Repeat("d", 501)
	if err := s.SavePlan(p); err == nil {
		t.Fatal("overlong desc was taken")
	}
}

func TestListUsersNewestFirst(t *testing.T) {
	shopDB(t)
	s := &ShopService{}
	if _, err := s.Touch(1, "A", "a", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Touch(2, "B", "b", 0); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListUsers(10)
	if err != nil || len(list) != 2 {
		t.Fatalf("list %+v %v", list, err)
	}
}
