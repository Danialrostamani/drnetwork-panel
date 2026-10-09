package service

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
)

func TestParseDepositSms(t *testing.T) {
	cases := []struct {
		name   string
		text   string
		rial   bool
		amount int64
		ok     bool
	}{
		{"melli", "بانک ملی\nواریز: 1,250,000 ریال\nحساب: 0123456789\nمانده: 5,000,000\n1402/07/15 10:20", false, 1250000, true},
		{"persian digits", "واریز ۵۰۰,۰۰۰ ریال به حساب ۱۲۳۴۵۶۷۸\nمانده ۹۰۰,۰۰۰", false, 500000, true},
		{"rials to tomans", "واریز 1,250,000 ریال\nمانده: 3,000,000", true, 125000, true},
		{"toman stays", "واریز 125,000 تومان", true, 125000, true},
		{"plus sign", "حساب 1234***5678\n+2,000,000\n02/07/15-10:20\nمانده 7,000,000", false, 2000000, true},
		{"sign after digits", "1234567\n2,000,000+\nمانده 7,000,000", false, 2000000, true},
		{"next line", "واریز به حساب\n350,000\nمانده 1,000,000", false, 350000, true},
		{"english", "Deposit 1,500,000 IRR\nBal: 9,000,000", false, 1500000, true},
		{"withdrawal", "برداشت 1,000,000 ریال\nمانده 4,000,000", false, 0, false},
		{"minus", "حساب 1234\n-1,000,000\nمانده 4,000,000", false, 0, false},
		{"purchase", "خرید 250,000 ریال\nمانده 4,000,000", false, 0, false},
		{"two amounts", "واریز 100,000\nواریز 200,000", false, 0, false},
		{"only balance", "مانده 4,000,000", false, 0, false},
		{"odd rials", "واریز 1,250,005 ریال", true, 0, false},
		{"card number", "واریز به کارت 6037991234567890\nمبلغ واریز 300,000", false, 300000, true},
	}
	for _, c := range cases {
		amount, ok, why := ParseDepositSms(c.text, c.rial)
		if ok != c.ok || amount != c.amount {
			t.Errorf("%s: got %d %v (%s), want %d %v", c.name, amount, ok, why, c.amount, c.ok)
		}
	}
}

func TestShopCardsAndRotation(t *testing.T) {
	shopDB(t)
	cards := ShopCards("6037 1111\nAli\n---\n6219 2222\nSara\r\n---\n\n")
	if len(cards) != 2 || cards[0] != "6037 1111\nAli" || cards[1] != "6219 2222\nSara" {
		t.Fatalf("cards = %q", cards)
	}
	s := &ShopService{}
	used := map[string]int{}
	for i := 0; i < 4; i++ {
		c := s.pickCard(cards)
		used[c]++
		if err := database.GetDB().Create(&model.ShopOrder{TgId: 1, Card: c, Method: model.PayCard, Status: model.OrderPending, CreatedAt: time.Now().Unix()}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if used[cards[0]] != 2 || used[cards[1]] != 2 {
		t.Fatalf("the cards did not take turns: %v", used)
	}
}

func setShop(t *testing.T, kv map[string]string) {
	t.Helper()
	for k, v := range kv {
		if err := (&ShopService{}).SetSetting(k, v); err != nil {
			t.Fatalf("%s: %v", k, err)
		}
	}
}

// Card orders made at once all get different amounts.
func TestShopUniqueAmounts(t *testing.T) {
	shopDB(t)
	setShop(t, map[string]string{"shopCard": "6037 1111", "shopUniqueAmount": "50"})
	s := &ShopService{}
	var wg sync.WaitGroup
	orders := make([]*model.ShopOrder, 40)
	for i := range orders {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			o := &model.ShopOrder{TgId: int64(i + 1), Kind: model.OrderTopup, Amount: 100000, Paid: 100000, Method: model.PayCard}
			if err := s.CreateOrder(o); err != nil {
				t.Error(err)
			}
			orders[i] = o
		}(i)
	}
	wg.Wait()
	seen := map[int64]bool{}
	for _, o := range orders {
		if o.Extra < 1 || o.Extra > 50 || seen[ToPay(o)] {
			t.Fatalf("order %d: extra %d, to pay %d (seen %v)", o.Id, o.Extra, ToPay(o), seen[ToPay(o)])
		}
		seen[ToPay(o)] = true
		if o.Card != "6037 1111" {
			t.Fatalf("card %q", o.Card)
		}
	}
	// A wallet order adds nothing.
	w := &model.ShopOrder{TgId: 1, Kind: model.OrderBuy, Paid: 100000, Method: model.PayWallet}
	if err := s.CreateOrder(w); err != nil || w.Extra != 0 || w.Card != "" {
		t.Fatalf("wallet order: %+v %v", w, err)
	}
	// With every amount taken there is no extra left to give.
	setShop(t, map[string]string{"shopUniqueAmount": "3"})
	got := map[int64]bool{}
	for i := 0; i < 4; i++ {
		o := &model.ShopOrder{TgId: 99, Kind: model.OrderTopup, Paid: 5000, Method: model.PayCard}
		if err := s.CreateOrder(o); err != nil {
			t.Fatal(err)
		}
		if i < 3 && (o.Extra < 1 || o.Extra > 3 || got[o.Extra]) || i == 3 && o.Extra != 0 {
			t.Fatalf("order %d of 4: extra %d (%v)", i+1, o.Extra, got)
		}
		got[o.Extra] = true
	}
}

func TestShopReceiptIsUsedOnce(t *testing.T) {
	shopDB(t)
	s := &ShopService{}
	a := &model.ShopOrder{TgId: 1, Kind: model.OrderTopup, Paid: 1000, Method: model.PayCard}
	b := &model.ShopOrder{TgId: 2, Kind: model.OrderTopup, Paid: 1000, Method: model.PayCard}
	for _, o := range []*model.ShopOrder{a, b} {
		if err := s.CreateOrder(o); err != nil {
			t.Fatal(err)
		}
	}
	key := ReceiptKey("photo", "AQADuniq", "")
	if key != "photo:AQADuniq" || ReceiptKey("photo", "", "") != "" {
		t.Fatalf("photo key %q", key)
	}
	if err := s.SetReceiptKeyed(a.Id, "photo:file1", key); err != nil {
		t.Fatal(err)
	}
	// The same order may send it again.
	if err := s.SetReceiptKeyed(a.Id, "photo:file1b", key); err != nil {
		t.Fatal(err)
	}
	var used ErrReceiptUsed
	if err := s.SetReceiptKeyed(b.Id, "photo:file2", key); !errors.As(err, &used) || used.Order != a.Id {
		t.Fatalf("second order took the receipt: %v", err)
	}
	// Tracking numbers are compared by their digits.
	k1 := ReceiptKey("text", "", "پیگیری: ۱۲۳۴۵۶۷۸۹")
	k2 := ReceiptKey("text", "", "ref 123456789 ok")
	if k1 == "" || k1 != k2 || ReceiptKey("text", "", "paid") != "" {
		t.Fatalf("text keys %q %q", k1, k2)
	}
	// Once the first order is rejected, its receipt is free again.
	if _, err := s.Decide(a.Id, model.OrderRejected, 9); err != nil {
		t.Fatal(err)
	}
	if err := s.SetReceiptKeyed(b.Id, "photo:file2", key); err != nil {
		t.Fatalf("after rejection: %v", err)
	}
}

func TestShopSmsApprovesTheOneMatchingOrder(t *testing.T) {
	shopDB(t)
	setShop(t, map[string]string{"shopCard": "6037 1111", "shopUniqueAmount": "0", "shopSmsRial": "true"})
	s := &ShopService{}
	var decided []uint
	var told []int64
	oldDecider, oldTold := ShopDecider, ShopSmsApproved
	defer func() { ShopDecider, ShopSmsApproved = oldDecider, oldTold }()
	ShopDecider = func(id uint, approve bool) (string, error) {
		if !approve {
			t.Fatal("rejected")
		}
		if _, err := s.Decide(id, model.OrderApproved, 0); err != nil {
			return "", err
		}
		decided = append(decided, id)
		return "ok", nil
	}
	ShopSmsApproved = func(id uint, amount int64) { told = append(told, amount) }

	one := &model.ShopOrder{TgId: 1, Kind: model.OrderTopup, Paid: 150000, Method: model.PayCard}
	twinA := &model.ShopOrder{TgId: 2, Kind: model.OrderTopup, Paid: 90000, Method: model.PayCard}
	twinB := &model.ShopOrder{TgId: 3, Kind: model.OrderTopup, Paid: 90000, Method: model.PayCard}
	for _, o := range []*model.ShopOrder{one, twinA, twinB} {
		if err := s.CreateOrder(o); err != nil {
			t.Fatal(err)
		}
	}
	msg := "واریز 1,500,000 ریال\nمانده 9,000,000"
	res, err := s.HandleSms(msg, "+98900")
	if err != nil || res.Status != model.SmsApproved || res.OrderId != one.Id || res.Amount != 150000 {
		t.Fatalf("first message: %+v %v", res, err)
	}
	// The phone sending it again changes nothing.
	res, err = s.HandleSms(msg+"\n", "+98900")
	if err != nil || res.Status != model.SmsDuplicate {
		t.Fatalf("repeat: %+v %v", res, err)
	}
	// Two open orders of the amount: nobody is approved.
	res, _ = s.HandleSms("واریز 900,000 ریال", "")
	if res.Status != model.SmsUnmatched {
		t.Fatalf("ambiguous: %+v", res)
	}
	res, _ = s.HandleSms("برداشت 900,000 ریال", "")
	if res.Status != model.SmsIgnored {
		t.Fatalf("withdrawal: %+v", res)
	}
	if len(decided) != 1 || decided[0] != one.Id || len(told) != 1 || told[0] != 150000 {
		t.Fatalf("decided %v, told %v", decided, told)
	}
	log, _ := s.SmsLog(10)
	if len(log) != 3 || log[0].Status != model.SmsIgnored || log[2].OrderId != one.Id {
		t.Fatalf("log: %+v", log)
	}
	// Without the bot nothing can be approved.
	ShopDecider = nil
	res, _ = s.HandleSms("واریز 777,770 ریال", "")
	if res.Status != model.SmsUnmatched {
		t.Fatalf("no order of the amount: %+v", res)
	}
}
