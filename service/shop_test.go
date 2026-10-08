package service

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
)

func shopDB(t *testing.T) {
	t.Helper()
	if err := database.InitDB(filepath.Join(t.TempDir(), "shop.db")); err != nil {
		t.Fatal(err)
	}
}

func TestShopWalletNeverGoesNegative(t *testing.T) {
	shopDB(t)
	s := &ShopService{}
	if _, err := s.AddFunds(nil, 7, -1, "x", 0, 0); !errors.Is(err, ErrNoFunds) {
		t.Fatalf("empty wallet paid: %v", err)
	}
	if bal, err := s.AddFunds(nil, 7, 100, "topup", 1, 42); err != nil || bal != 100 {
		t.Fatalf("topup = %d, %v", bal, err)
	}
	// Ten payments of 30 race for 100: three get through.
	var wg sync.WaitGroup
	var mu sync.Mutex
	ok := 0
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.AddFunds(nil, 7, -30, "buy", 0, 0); err == nil {
				mu.Lock()
				ok++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if ok != 3 || s.Balance(7) != 10 {
		t.Fatalf("%d payments went through, balance %d", ok, s.Balance(7))
	}
	txs, _ := s.WalletTxs(7, 50)
	if len(txs) != 4 || txs[0].Balance != 10 {
		t.Fatalf("ledger: %+v", txs)
	}
}

// Every ledger line shows the balance its own change left, even when changes
// race: adding up the amounts line by line gives each line's balance.
func TestShopLedgerFollowsTheBalance(t *testing.T) {
	shopDB(t)
	s := &ShopService{}
	if _, err := s.AddFunds(nil, 9, 100, "topup", 0, 0); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			amount := int64(20)
			if i%2 == 1 {
				amount = -30
			}
			_, _ = s.AddFunds(nil, 9, amount, "race", 0, 0)
		}(i)
	}
	wg.Wait()
	var txs []model.ShopWalletTx
	if err := database.GetDB().Where("tg_id = ?", 9).Order("id ASC").Find(&txs).Error; err != nil {
		t.Fatal(err)
	}
	var sum int64
	for _, tx := range txs {
		sum += tx.Amount
		if tx.Balance != sum || sum < 0 {
			t.Fatalf("line %d shows balance %d, the changes up to it add up to %d", tx.Id, tx.Balance, sum)
		}
	}
	if len(txs) < 21 || sum != s.Balance(9) {
		t.Fatalf("%d lines adding up to %d, wallet holds %d", len(txs), sum, s.Balance(9))
	}
}

func TestShopOrderIsDecidedOnce(t *testing.T) {
	shopDB(t)
	s := &ShopService{}
	o := &model.ShopOrder{TgId: 5, Kind: model.OrderBuy, Paid: 100}
	if err := s.CreateOrder(o); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Decide(o.Id, model.OrderApproved, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Decide(o.Id, model.OrderRejected, 2); !errors.Is(err, ErrDecided) {
		t.Fatalf("decided twice: %v", err)
	}
	got, _ := s.Order(o.Id)
	if got.Status != model.OrderApproved || got.DecidedBy != 1 {
		t.Fatalf("order: %+v", got)
	}
}

func TestShopPriceAndCodes(t *testing.T) {
	shopDB(t)
	s := &ShopService{}
	if d, p := Price(1000, 10, 50); d != 550 || p != 450 {
		t.Fatalf("price = %d off, %d paid", d, p)
	}
	d, err := ParseDiscountLine("off20 20 1 10")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SaveDiscount(d); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveDiscount(&model.ShopDiscount{Code: "OFF20", Percent: 5}); err == nil {
		t.Fatal("duplicate code saved")
	}
	got, err := s.Discount(" Off20 ")
	if err != nil || got.Percent != 20 {
		t.Fatalf("code: %+v %v", got, err)
	}
	o := &model.ShopOrder{TgId: 5, Kind: model.OrderBuy, Code: "OFF20"}
	_ = s.CreateOrder(o)
	if err := s.Finish(o); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Discount("OFF20"); !errors.Is(err, ErrBadCode) {
		t.Fatal("a used-up code still works")
	}
	if _, err := ParsePlanLine("Gold | 50 | 30 | 250,000 | 2"); err != nil {
		t.Fatal(err)
	}
	if _, err := ParsePlanLine("Gold | x | 30 | 1"); err == nil {
		t.Fatal("bad plan accepted")
	}
}

func TestShopReferralPaysOnce(t *testing.T) {
	shopDB(t)
	s := &ShopService{}
	if err := s.SetSetting("shopRefPercent", "10"); err != nil {
		t.Fatal(err)
	}
	_, _ = s.Touch(1, "ref", "", 0)
	_, _ = s.Touch(2, "new", "", 1)
	_, _ = s.Touch(2, "new", "", 3) // a later /start does not change the referrer
	if u := s.User(2); u.ReferredBy != 1 {
		t.Fatalf("referrer = %d", u.ReferredBy)
	}
	o := &model.ShopOrder{Id: 9, TgId: 2, Kind: model.OrderBuy, Paid: 500}
	if who, got := s.ReferralReward(o); who != 1 || got != 50 {
		t.Fatalf("reward %d to %d", got, who)
	}
	if _, got := s.ReferralReward(o); got != 0 {
		t.Fatal("paid twice")
	}
	if s.Balance(1) != 50 || s.Referrals(1) != 1 {
		t.Fatalf("balance %d, referrals %d", s.Balance(1), s.Referrals(1))
	}
}

func TestShopTrialOnce(t *testing.T) {
	shopDB(t)
	s := &ShopService{}
	if ok, err := s.ClaimTrial(3); !ok || err != nil {
		t.Fatal(ok, err)
	}
	if ok, _ := s.ClaimTrial(3); ok {
		t.Fatal("trial twice")
	}
	s.ReleaseTrial(3)
	if ok, _ := s.ClaimTrial(3); !ok {
		t.Fatal("released trial not available")
	}
}

func TestShopStats(t *testing.T) {
	shopDB(t)
	s := &ShopService{}
	for _, paid := range []int64{100, 200} {
		o := &model.ShopOrder{TgId: 5, Kind: model.OrderBuy, Paid: paid, PlanName: "Gold"}
		_ = s.CreateOrder(o)
		_, _ = s.Decide(o.Id, model.OrderApproved, 1)
	}
	top := &model.ShopOrder{TgId: 5, Kind: model.OrderTopup, Paid: 1000}
	_ = s.CreateOrder(top)
	_, _ = s.Decide(top.Id, model.OrderApproved, 1)
	st, err := s.Stats(time.UTC, 7)
	if err != nil {
		t.Fatal(err)
	}
	if st.RevenueToday != 300 || st.Orders30 != 2 || len(st.Days) != 7 || st.Days[6].Revenue != 300 || len(st.TopPlans) != 1 {
		t.Fatalf("stats: %+v", st)
	}
}
