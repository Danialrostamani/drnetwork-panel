package service

import (
	"errors"
	"sync"
	"testing"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
)

func TestShopDiscountLimits(t *testing.T) {
	shopDB(t)
	s := &ShopService{}
	p1 := &model.ShopPlan{Name: "a", Days: 30, Price: 1000, Enable: true}
	p2 := &model.ShopPlan{Name: "b", Days: 30, Price: 2000, Enable: true}
	for _, p := range []*model.ShopPlan{p1, p2} {
		if err := s.SavePlan(p); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.SaveDiscount(&model.ShopDiscount{Code: "nope", Percent: 10, PlanIds: "99", Enable: true}); err == nil {
		t.Fatal("a code for a plan that does not exist was saved")
	}
	d := &model.ShopDiscount{Code: "only1", Percent: 10, PlanIds: " 1, 1 ", Kinds: "renew", OncePerUser: true, Enable: true}
	if err := s.SaveDiscount(d); err != nil || d.Code != "ONLY1" || d.PlanIds != "1" || d.Kind != model.CodeDiscount {
		t.Fatalf("save: %+v %v", d, err)
	}
	if _, err := s.DiscountFor("only1", 5, p2.Id, model.OrderRenew); !errors.Is(err, ErrCodeNotHere) {
		t.Fatalf("other plan: %v", err)
	}
	if _, err := s.DiscountFor("only1", 5, p1.Id, model.OrderBuy); !errors.Is(err, ErrCodeNotHere) {
		t.Fatalf("purchase: %v", err)
	}
	if got, err := s.DiscountFor("only1", 5, p1.Id, model.OrderRenew); err != nil || got.Percent != 10 {
		t.Fatalf("renewal: %+v %v", got, err)
	}
	// An open order with the code counts as the customer's one use.
	o := &model.ShopOrder{TgId: 5, Kind: model.OrderRenew, PlanId: p1.Id, Code: "ONLY1", Paid: 900, Method: model.PayCard}
	if err := s.CreateOrder(o); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DiscountFor("only1", 5, p1.Id, model.OrderRenew); !errors.Is(err, ErrCodeUsed) {
		t.Fatalf("second use: %v", err)
	}
	if _, err := s.DiscountFor("only1", 6, p1.Id, model.OrderRenew); err != nil {
		t.Fatalf("another customer: %v", err)
	}
	// Canceled, the order no longer uses it; approved and finished, it does.
	if _, err := s.Decide(o.Id, model.OrderCanceled, 5); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DiscountFor("only1", 5, p1.Id, model.OrderRenew); err != nil {
		t.Fatalf("after cancel: %v", err)
	}
	o2 := &model.ShopOrder{TgId: 5, Kind: model.OrderRenew, PlanId: p1.Id, Code: "ONLY1", Paid: 900, Method: model.PayWallet}
	if err := s.CreateOrder(o2); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Decide(o2.Id, model.OrderApproved, 0); err != nil {
		t.Fatal(err)
	}
	if err := s.Finish(o2); err != nil {
		t.Fatal(err)
	}
	var got model.ShopDiscount
	database.GetDB().First(&got, d.Id)
	if got.Used != 1 {
		t.Fatalf("used = %d", got.Used)
	}
	if _, err := s.DiscountFor("only1", 5, p1.Id, model.OrderRenew); !errors.Is(err, ErrCodeUsed) {
		t.Fatalf("after use: %v", err)
	}
	if _, err := s.DiscountFor("missing", 5, p1.Id, model.OrderRenew); !errors.Is(err, ErrBadCode) {
		t.Fatalf("unknown code: %v", err)
	}
}

func TestShopGiftCodeOncePerCustomer(t *testing.T) {
	shopDB(t)
	s := &ShopService{}
	if err := s.SaveDiscount(&model.ShopDiscount{Code: "g0", Kind: model.CodeGift, Enable: true}); err == nil {
		t.Fatal("a gift of nothing was saved")
	}
	g := &model.ShopDiscount{Code: "gift50", Kind: model.CodeGift, Amount: 50000, Percent: 30, PlanIds: "1", MaxUses: 3, Enable: true}
	if err := s.SaveDiscount(g); err != nil || g.Percent != 0 || g.PlanIds != "" || !g.OncePerUser {
		t.Fatalf("gift: %+v %v", g, err)
	}
	if _, err := s.DiscountFor("gift50", 1, 1, model.OrderBuy); !errors.Is(err, ErrGiftCode) {
		t.Fatalf("a gift as a discount: %v", err)
	}
	if err := s.SaveDiscount(&model.ShopDiscount{Code: "off", Percent: 10, Enable: true}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.RedeemGift(1, "off"); !errors.Is(err, ErrNotGift) {
		t.Fatalf("a discount as a gift: %v", err)
	}
	// One customer racing with themselves gets it once.
	var wg sync.WaitGroup
	var mu sync.Mutex
	got := 0
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, _, err := s.RedeemGift(1, "GIFT50"); err == nil {
				mu.Lock()
				got++
				mu.Unlock()
			} else if !errors.Is(err, ErrCodeUsed) {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if got != 1 || s.Balance(1) != 50000 {
		t.Fatalf("redeemed %d times, balance %d", got, s.Balance(1))
	}
	if amount, bal, err := s.RedeemGift(2, "gift50"); err != nil || amount != 50000 || bal != 50000 {
		t.Fatalf("second customer: %d %d %v", amount, bal, err)
	}
	if _, _, err := s.RedeemGift(3, "gift50"); err != nil {
		t.Fatal(err)
	}
	// Three uses was the most.
	if _, _, err := s.RedeemGift(4, "gift50"); !errors.Is(err, ErrBadCode) {
		t.Fatalf("fourth use: %v", err)
	}
	txs, _ := s.WalletTxs(2, 5)
	if len(txs) != 1 || txs[0].Reason != "gift" {
		t.Fatalf("ledger: %+v", txs)
	}
}

func TestParseDiscountLine(t *testing.T) {
	d, err := ParseDiscountLine("NOROOZ 20 100 7 یکبار")
	if err != nil || d.Percent != 20 || d.MaxUses != 100 || d.Expiry == 0 || !d.OncePerUser || d.Kind != model.CodeDiscount {
		t.Fatalf("discount: %+v %v", d, err)
	}
	d, err = ParseDiscountLine("هدیه GIFT 50000 10")
	if err != nil || d.Kind != model.CodeGift || d.Amount != 50000 || d.MaxUses != 10 || d.Code != "GIFT" {
		t.Fatalf("gift: %+v %v", d, err)
	}
	for _, bad := range []string{"", "X", "gift X", "gift X -5", "X ten", "X 10 1 2 3"} {
		if _, err := ParseDiscountLine(bad); err == nil {
			t.Errorf("%q parsed", bad)
		}
	}
}
