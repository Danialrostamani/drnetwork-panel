package tgbot

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/service"
)

const (
	buyer    = int64(5000)
	referrer = int64(6000)
)

func shopEnv(t *testing.T) (*scopeEnv, *model.ShopPlan) {
	t.Helper()
	e := newScopeEnv(t)
	s := &service.ShopService{}
	for k, v := range map[string]string{"shopEnable": "true", "shopCard": "6037-0000 Ali", "shopCurrency": "T", "shopRefPercent": "10", "shopTrial": "1 2"} {
		if err := s.SetSetting(k, v); err != nil {
			t.Fatal(err)
		}
	}
	p := &model.ShopPlan{Name: "Gold", Volume: 10 * gib, Days: 30, Price: 1000, Enable: true}
	if err := s.SavePlan(p); err != nil {
		t.Fatal(err)
	}
	return e, p
}

func photoFrom(from int64, fileID string) update {
	var u update
	raw := `{"update_id":3,"message":{"message_id":5,"photo":[{"file_id":"small"},{"file_id":` + mustJSON(fileID) + `}],"chat":{"id":` + itoa(from) + `,"type":"private"},"from":{"id":` + itoa(from) + `,"first_name":"Buyer"}}}`
	_ = json.Unmarshal([]byte(raw), &u)
	return u
}

func lastOrder(t *testing.T) model.ShopOrder {
	t.Helper()
	var o model.ShopOrder
	if err := database.GetDB().Order("id DESC").First(&o).Error; err != nil {
		t.Fatal(err)
	}
	return o
}

func TestShopCardPurchaseIsApprovedOnce(t *testing.T) {
	e, p := shopEnv(t)
	pid := itoa(int64(p.Id))
	e.say(buyer, "/start")
	if txt := lastText(e.got(), "sendMessage"); !strings.Contains(txt, "Shop") {
		t.Fatalf("start: %s", txt)
	}
	e.press(buyer, "p:plans:0")
	if txt := lastText(e.got(), "editMessageText"); !strings.Contains(txt, "Gold") || !strings.Contains(txt, "1,000 T") {
		t.Fatalf("plans: %s", txt)
	}
	e.press(buyer, "p:card:"+pid+":0")
	if txt := lastText(e.got(), "sendMessage"); !strings.Contains(txt, "6037-0000 Ali") {
		t.Fatalf("payment: %s", txt)
	}
	o := lastOrder(t)
	if o.Status != model.OrderPending || o.Paid != 1000 || o.TgId != buyer {
		t.Fatalf("order: %+v", o)
	}
	n := len(e.got())
	e.b.handle(context.Background(), photoFrom(buyer, "receipt-1"))
	photos := 0
	for _, m := range e.since(n) {
		if m.Method == "sendPhoto" {
			photos++
		}
	}
	// Both full administrators see the receipt; the group-limited one does not.
	if photos != 1 {
		t.Fatalf("%d receipts sent to approvers", photos)
	}
	if o = lastOrder(t); o.Receipt != "photo:receipt-1" {
		t.Fatalf("receipt = %q", o.Receipt)
	}
	// A customer cannot approve.
	n = len(e.got())
	e.press(buyer, fmt.Sprintf("q:ok:%d", o.Id))
	if lastOrder(t).Status != model.OrderPending {
		t.Fatal("a customer approved an order")
	}
	e.press(fullAdmin, fmt.Sprintf("q:ok:%d", o.Id))
	c := e.b.findClientByName("u" + itoa(int64(o.Id)))
	if c == nil || c.TgId != buyer || c.Volume != 10*gib || c.Expiry == 0 {
		t.Fatalf("client: %+v", c)
	}
	if o = lastOrder(t); o.Status != model.OrderApproved || o.ClientId != c.Id {
		t.Fatalf("order after approval: %+v", o)
	}
	e.press(fullAdmin, fmt.Sprintf("q:ok:%d", o.Id))
	if txt := lastText(e.got(), "sendMessage"); !strings.Contains(txt, "already handled") {
		t.Fatalf("second approval: %s", txt)
	}
	if n := len(allClients()); n != 8 {
		t.Fatalf("%d clients, want one new", n)
	}
}

func TestShopWalletCodeReferralAndRenewal(t *testing.T) {
	e, p := shopEnv(t)
	s := &service.ShopService{}
	pid := itoa(int64(p.Id))
	e.say(referrer, "/start")
	e.say(buyer, "/start ref"+itoa(referrer))
	if err := s.SaveDiscount(&model.ShopDiscount{Code: "HALF", Percent: 50, Enable: true}); err != nil {
		t.Fatal(err)
	}
	// Not enough money.
	e.press(buyer, "p:wal:"+pid+":0")
	if txt := lastText(e.got(), "editMessageText"); !strings.Contains(txt, "Not enough") {
		t.Fatalf("empty wallet: %s", txt)
	}
	if _, err := s.AddFunds(nil, buyer, 2000, "admin", 0, fullAdmin); err != nil {
		t.Fatal(err)
	}
	e.press(buyer, "p:code:"+pid+":0")
	e.say(buyer, "half")
	if txt := lastText(e.got(), "sendMessage"); !strings.Contains(txt, "500 T") {
		t.Fatalf("with the code: %s", txt)
	}
	e.press(buyer, "p:walY:"+pid+":0")
	if bal := s.Balance(buyer); bal != 1500 {
		t.Fatalf("balance = %d", bal)
	}
	if got := s.Balance(referrer); got != 50 {
		t.Fatalf("referrer got %d", got)
	}
	o := lastOrder(t)
	c := e.b.findClientByName(o.ClientName)
	if o.Status != model.OrderApproved || c == nil || c.TgId != buyer {
		t.Fatalf("order %+v client %+v", o, c)
	}

	// Renewing a used-up client starts it over.
	full, _ := rawFullClient(c.Id)
	full.Up, full.Enable = full.Volume, false
	database.GetDB().Save(full)
	e.press(buyer, fmt.Sprintf("p:walY:%s:%d", pid, c.Id))
	full, _ = rawFullClient(c.Id)
	if !full.Enable || full.Up != 0 || full.Volume != 10*gib {
		t.Fatalf("after renewal: %+v", full)
	}
	if bal := s.Balance(buyer); bal != 500 {
		t.Fatalf("balance after renewal = %d", bal)
	}
	// Nobody renews a client that is not theirs.
	e.press(referrer, fmt.Sprintf("p:walY:%s:%d", pid, c.Id))
	if lastOrder(t).TgId == referrer {
		t.Fatal("renewed somebody else's client")
	}
}

func TestShopTrialAndReseller(t *testing.T) {
	e, p := shopEnv(t)
	s := &service.ShopService{}
	e.press(buyer, "p:trial")
	e.press(buyer, "p:trial")
	trials := 0
	for _, c := range allClients() {
		if strings.HasPrefix(c.Name, "ut") {
			trials++
			if c.TgId != buyer || c.Volume != gib {
				t.Fatalf("trial: %+v", c)
			}
		}
	}
	if trials != 1 {
		t.Fatalf("%d trials", trials)
	}

	// The group-limited administrator buys as a reseller, with a discount.
	if err := s.SetResellerPercent(salesAdmin, 20); err != nil {
		t.Fatal(err)
	}
	_, _ = s.AddFunds(nil, salesAdmin, 800, "admin", 0, 0)
	e.press(salesAdmin, fmt.Sprintf("p:walY:%d:0", p.Id))
	o := lastOrder(t)
	c := e.b.findClientByName(o.ClientName)
	if o.Paid != 800 || !o.Reseller || c == nil || c.Group != salesGroup || c.TgId != 0 || s.Balance(salesAdmin) != 0 {
		t.Fatalf("reseller order %+v client %+v", o, c)
	}

	// A closed shop answers nothing but the old self-service.
	_ = s.SetSetting("shopEnable", "false")
	n := len(e.got())
	e.press(buyer, "p:plans:0")
	if txt := lastText(e.since(n), "editMessageText"); strings.Contains(txt, "Gold") {
		t.Fatal("closed shop showed plans")
	}
}

func TestShopAdminPlansAndBroadcast(t *testing.T) {
	e, _ := shopEnv(t)
	broadcastGap = 0
	e.press(fullAdmin, "q:pn")
	e.say(fullAdmin, "Silver | ۲۰ | 30 | 500,000 | 2")
	plans, _ := (&service.ShopService{}).Plans(false)
	if len(plans) != 2 || plans[1].Name != "Silver" || plans[1].Volume != 20*gib || plans[1].Price != 500000 || plans[1].LimitIp != 2 {
		t.Fatalf("plans: %+v", plans)
	}
	// A group-limited administrator has no shop admin.
	e.press(salesAdmin, "q:pn")
	e.say(salesAdmin, "Bad | 1 | 1 | 1")
	if plans, _ = (&service.ShopService{}).Plans(false); len(plans) != 2 {
		t.Fatal("a limited admin made a plan")
	}
	e.say(buyer, "/start")
	e.press(fullAdmin, "q:bc")
	e.say(fullAdmin, "hello all")
	e.press(fullAdmin, "q:bcy")
	deadline := 0
	for deadline < 100 {
		if strings.Contains(lastText(e.got(), "sendMessage"), "Broadcast done") {
			break
		}
		deadline++
		sleepMs(20)
	}
	got := false
	for _, m := range e.got() {
		if m.ChatID == buyer && m.Text == "hello all" {
			got = true
		}
	}
	if !got {
		t.Fatal("the broadcast did not reach the shop user")
	}
}

func sleepMs(n int) { time.Sleep(time.Duration(n) * time.Millisecond) }
