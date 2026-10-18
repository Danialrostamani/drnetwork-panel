package tgbot

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/service"
)

func shopBot(t *testing.T, s shopConfig) (*bot, func() []sent) {
	t.Helper()
	b, got := testBot(t)
	setShop(t, s)
	b.cfg.Lang = "en"
	b.cfg.Owner = 42
	b.setAccess(b.cfg.access())
	return b, got
}

func setShop(t *testing.T, s shopConfig) {
	t.Helper()
	raw, _ := jsonShop(s)
	if err := (&service.SettingService{}).SetTgBotShop(raw); err != nil {
		t.Fatal(err)
	}
}

func lastOrder(t *testing.T, tg int64) model.BotOrder {
	t.Helper()
	var o model.BotOrder
	if err := database.GetDB().Where("tg_id = ?", tg).Order("id desc").First(&o).Error; err != nil {
		t.Fatal(err)
	}
	return o
}

func clientCount(t *testing.T) int64 {
	t.Helper()
	var n int64
	database.GetDB().Model(&model.Client{}).Count(&n)
	return n
}

func photoMessage(from int64, fileID string, document bool) update {
	var u update
	media := `"photo":[{"file_id":"small"},{"file_id":` + mustJSON(fileID) + `}]`
	if document {
		media = `"document":{"file_id":` + mustJSON(fileID) + `}`
	}
	raw := `{"update_id":3,"message":{` + media + `,"chat":{"id":` + itoa(from) + `,"type":"private"},"from":{"id":` + itoa(from) + `}}}`
	_ = json.Unmarshal([]byte(raw), &u)
	return u
}

func TestParseMoney(t *testing.T) {
	for in, want := range map[string]int64{
		"150,000 تومان": 150000, "۱۵۰٬۰۰۰ تومان": 150000, "150 هزار تومان": 150000,
		"1.5 میلیون": 1500000, "50000": 50000, "20k": 20000, "$5": 5,
	} {
		if got, ok := parseMoney(in); !ok || got != want {
			t.Errorf("parseMoney(%q) = %d, %v; want %d", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "رایگان", "free", "0"} {
		if _, ok := parseMoney(in); ok {
			t.Errorf("parseMoney(%q) parsed", in)
		}
	}
	if formatMoney(1500000) != "1,500,000" || formatMoney(999) != "999" {
		t.Fatal("formatMoney")
	}
}

// An order without a plan used to be approved into an unlimited client that
// never expires.
func TestOrderWithoutPlanIsNeverDelivered(t *testing.T) {
	b, got := shopBot(t, shopConfig{PayCard: "6037"})
	ctx := context.Background()
	// The empty shop offers a message to support, not an order.
	b.handle(ctx, callbackFrom(99, "u:ask:buy"))
	b.handle(ctx, privateMessage(99, "I want 50 GB"))
	var n int64
	database.GetDB().Model(&model.BotOrder{}).Count(&n)
	if n != 0 {
		t.Fatalf("an empty-shop request made %d orders", n)
	}
	// A plan that does not exist is refused when ordered.
	setShop(t, shopConfig{PayCard: "6037", Plans: []shopPlan{{ID: "1", Name: "Gold", Days: 30, GB: 40, Price: "100"}}})
	b.handle(ctx, callbackFrom(99, "u:buy:9"))
	database.GetDB().Model(&model.BotOrder{}).Count(&n)
	if n != 0 || !strings.Contains(lastText(got(), "sendMessage"), "no longer in the shop") {
		t.Fatalf("missing plan: orders=%d, said %q", n, lastText(got(), "sendMessage"))
	}
	// An old order with no plan copy and no plan left is not approved.
	o := &model.BotOrder{TgId: 99, Topic: "buy", PlanID: "gone", Status: orderPending}
	if err := b.saveOrder(o); err != nil {
		t.Fatal(err)
	}
	b.handle(ctx, callbackFrom(42, "j:ok:"+itoa(int64(o.Id))))
	if clientCount(t) != 0 {
		t.Fatal("a buy without a plan made a client")
	}
	if fresh, _ := loadOrder(o.Id); fresh.Status != orderPending {
		t.Fatalf("status = %s", fresh.Status)
	}
	// Nor is a renew: it would report success while adding nothing.
	c := model.Client{Name: "alice", Enable: true, TgId: 99, Inbounds: []byte("[]"), Links: []byte("[]")}
	database.GetDB().Create(&c)
	r := &model.BotOrder{TgId: 99, Topic: "renew", PlanID: "gone", Extra: itoa(int64(c.Id)), Status: orderPending}
	_ = b.saveOrder(r)
	if err := b.approveOrder(r); err == nil {
		t.Fatal("a renew without a plan was approved")
	}
}

func TestOrderDeliversThePlanAsOrdered(t *testing.T) {
	b, _ := shopBot(t, shopConfig{PayCard: "6037", Plans: []shopPlan{{ID: "1", Name: "Gold", Days: 30, GB: 40, Price: "100"}}})
	ctx := context.Background()
	b.handle(ctx, callbackFrom(99, "u:buy:1"))
	o := lastOrder(t, 99)
	if !o.PlanSet || o.PlanGB != 40 || o.PlanDays != 30 || o.Amount != 100 {
		t.Fatalf("plan copy = %+v", o)
	}
	// The owner edits the plan before approving: the customer still gets
	// what they paid for.
	setShop(t, shopConfig{PayCard: "6037", Plans: []shopPlan{{ID: "1", Name: "Gold", Days: 60, GB: 400, Price: "100"}}})
	b.handle(ctx, callbackFrom(42, "j:ok:"+itoa(int64(o.Id))))
	o = lastOrder(t, 99)
	if o.Status != orderApproved {
		t.Fatalf("status = %s", o.Status)
	}
	var c model.Client
	database.GetDB().First(&c, o.ClientId)
	if c.Volume != 40*gib {
		t.Fatalf("volume = %d", c.Volume)
	}
}

func TestWalletTopUpThenPayAPlan(t *testing.T) {
	b, got := shopBot(t, shopConfig{
		PayCard: "6037",
		Plans:   []shopPlan{{ID: "1", Name: "Gold", Days: 30, GB: 40, Price: "100,000 تومان"}},
		Wallets: []shopWallet{{ID: "w", Label: "150,000", Amount: "150000"}},
	})
	ctx := context.Background()
	b.handle(ctx, callbackFrom(99, "u:wal:w"))
	top := lastOrder(t, 99)
	if top.Amount != 150000 {
		t.Fatalf("top-up amount = %d", top.Amount)
	}
	b.handle(ctx, photoMessage(99, "receipt1", false))
	b.handle(ctx, callbackFrom(42, "j:ok:"+itoa(int64(top.Id))))
	if bal := walletBalance(99); bal != 150000 {
		t.Fatalf("balance after top-up = %d", bal)
	}
	if !strings.Contains(lastText(got(), "sendMessage"), "150,000") {
		t.Fatalf("customer told %q", lastText(got(), "sendMessage"))
	}
	// The pay screen of a plan now offers the wallet.
	n := len(got())
	b.handle(ctx, callbackFrom(99, "u:buy:1"))
	buy := lastOrder(t, 99)
	offered := false
	for _, m := range got()[n:] {
		if m.Markup != nil {
			for _, row := range m.Markup.Keyboard {
				for _, btn := range row {
					offered = offered || btn.Data == "u:wp:"+itoa(int64(buy.Id))
				}
			}
		}
	}
	if !offered {
		t.Fatal("no Pay from wallet button")
	}
	b.handle(ctx, callbackFrom(99, "u:wp:"+itoa(int64(buy.Id))))
	buy = lastOrder(t, 99)
	if buy.Status != orderApproved || !buy.WalletPaid || buy.ClientId == 0 {
		t.Fatalf("wallet order = %+v", buy)
	}
	if bal := walletBalance(99); bal != 50000 {
		t.Fatalf("balance after paying = %d", bal)
	}
	// Not enough left for a second one.
	b.handle(ctx, callbackFrom(99, "u:buy:1"))
	again := lastOrder(t, 99)
	b.handle(ctx, callbackFrom(99, "u:wp:"+itoa(int64(again.Id))))
	if again = lastOrder(t, 99); again.Status != orderPending || walletBalance(99) != 50000 {
		t.Fatalf("paid without enough balance: %+v, balance %d", again, walletBalance(99))
	}
}

func TestWalletPaymentGoesBackWhenTheSaleFails(t *testing.T) {
	e := newAccessEnv(t)
	giveQuota(t, salesAdmin, 5)
	setShop(t, shopConfig{Plans: []shopPlan{{ID: "1", Name: "Gold", Days: 30, GB: 40, Price: "100"}}})
	if _, err := walletCredit(99, 500, walletMemo{kind: walletManual}); err != nil {
		t.Fatal(err)
	}
	o := &model.BotOrder{TgId: 99, Topic: "buy", PlanID: "1", AdminId: salesAdmin, Status: orderPending,
		PlanSet: true, PlanGB: 40, PlanDays: 30, Amount: 100}
	if err := e.b.saveOrder(o); err != nil {
		t.Fatal(err)
	}
	e.press(99, "u:wp:"+itoa(int64(o.Id)))
	if bal := walletBalance(99); bal != 500 {
		t.Fatalf("balance = %d, want the payment back", bal)
	}
	if fresh, _ := loadOrder(o.Id); fresh.Status != orderPending || fresh.WalletPaid {
		t.Fatalf("order = %+v", fresh)
	}
}

// A shop sale takes its plan volume from the admin's limit; the sold client's
// traffic must not be counted on top of it.
func TestSoldClientIsNotCountedTwice(t *testing.T) {
	e := newAccessEnv(t)
	giveQuota(t, salesAdmin, 100)
	setShop(t, shopConfig{Plans: []shopPlan{{ID: "1", Name: "Gold", Days: 30, GB: 10, Group: salesGroup}}})
	o := &model.BotOrder{TgId: 99, Topic: "buy", PlanID: "1", AdminId: salesAdmin, Status: orderPending,
		PlanSet: true, PlanGB: 10, PlanDays: 30, PlanGroup: salesGroup, Hint: "buyer"}
	if err := e.b.saveOrder(o); err != nil {
		t.Fatal(err)
	}
	if err := e.b.approveOrder(o); err != nil {
		t.Fatal(err)
	}
	if q := e.quota(salesAdmin); q.Used != 10*gib {
		t.Fatalf("after sale used = %d", q.Used)
	}
	e.consume("buyer", 6*gib)
	if q := e.quota(salesAdmin); q.Used != 10*gib {
		t.Fatalf("traffic of a sold client counted again: used = %d", q.Used)
	}
	// A renew of it takes the new plan volume, once.
	r := &model.BotOrder{TgId: 99, Topic: "renew", PlanID: "1", Extra: itoa(int64(o.ClientId)), AdminId: salesAdmin,
		Status: orderPending, PlanSet: true, PlanGB: 10, PlanDays: 30}
	_ = e.b.saveOrder(r)
	if err := e.b.approveOrder(r); err != nil {
		t.Fatal(err)
	}
	if q := e.quota(salesAdmin); q.Used != 20*gib {
		t.Fatalf("after renew used = %d", q.Used)
	}
}

func TestTrialApprovedOnlyOnce(t *testing.T) {
	b, _ := shopBot(t, shopConfig{})
	if err := database.GetDB().Create(&model.BotOrder{TgId: 99, Topic: "test", Status: orderApproved}).Error; err != nil {
		t.Fatal(err)
	}
	o := &model.BotOrder{TgId: 99, Topic: "test", Status: orderPending}
	_ = b.saveOrder(o)
	if err := b.approveOrder(o); err == nil || clientCount(t) != 0 {
		t.Fatal("a second test account was approved")
	}
}

func TestReceiptAfterCancelDoesNotReachStaff(t *testing.T) {
	b, got := shopBot(t, shopConfig{PayCard: "6037", Plans: []shopPlan{{ID: "1", Name: "Gold", Days: 30, GB: 40, Price: "100"}}})
	ctx := context.Background()
	b.handle(ctx, callbackFrom(99, "u:buy:1"))
	o := lastOrder(t, 99)
	b.handle(ctx, callbackFrom(99, "u:cx:"+itoa(int64(o.Id))))
	n := len(got())
	b.handle(ctx, photoMessage(99, "late", false))
	for _, m := range got()[n:] {
		if m.ChatID == 42 {
			t.Fatalf("staff got %s %q for a cancelled order", m.Method, m.Text)
		}
	}
	// And an old receipt button for it does not reopen it.
	b.handle(ctx, callbackFrom(99, "u:pay:"+itoa(int64(o.Id))))
	b.handle(ctx, photoMessage(99, "late2", false))
	for _, m := range got()[n:] {
		if m.ChatID == 42 {
			t.Fatalf("staff got %s for a cancelled order", m.Method)
		}
	}
}

func TestReceiptIsForwardedOnceInItsKind(t *testing.T) {
	b, got := shopBot(t, shopConfig{PayCard: "6037", Plans: []shopPlan{{ID: "1", Name: "Gold", Days: 30, GB: 40, Price: "100"}}})
	ctx := context.Background()
	b.handle(ctx, callbackFrom(99, "u:buy:1"))
	n := len(got())
	b.handle(ctx, photoMessage(99, "file1", true))
	photos, docs := 0, 0
	for _, m := range got()[n:] {
		if m.ChatID != 42 {
			continue
		}
		switch m.Method {
		case "sendPhoto":
			photos++
		case "sendDocument":
			docs++
		}
	}
	if photos != 0 || docs != 1 {
		t.Fatalf("file receipt sent as %d photos and %d documents", photos, docs)
	}
	if o := lastOrder(t, 99); o.ReceiptID != receiptDocPrefix+"file1" {
		t.Fatalf("receipt = %q", o.ReceiptID)
	}
}
