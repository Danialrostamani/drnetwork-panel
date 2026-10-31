package tgbot

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/service"
)

const (
	boss   = int64(1000) // the owner
	seller = int64(77)   // limited to the group Sales
	helper = int64(88)   // full access, but not the owner
	buyer  = int64(5001) // a customer
)

func ownerBot(t *testing.T, s shopConfig) (*bot, func() []sent) {
	t.Helper()
	b, got := testBot(t)
	setShop(t, s)
	b.cfg.Lang = "en"
	useAccess(b, botConfig{Owner: boss, Admins: []int64{boss, seller, helper}, Scopes: map[int64]string{seller: "Sales"}})
	runBroadcast = func(f func()) { f() }
	broadcastGap = 0
	t.Cleanup(func() { runBroadcast = func(f func()) { go f() }; broadcastGap = 60 * time.Millisecond })
	return b, got
}

func sentTo(msgs []sent, chat int64) []sent {
	var out []sent
	for _, m := range msgs {
		if m.ChatID == chat && m.Method != "answerCallbackQuery" {
			out = append(out, m)
		}
	}
	return out
}

func lastTo(t *testing.T, msgs []sent, chat int64) string {
	t.Helper()
	list := sentTo(msgs, chat)
	if len(list) == 0 {
		t.Fatalf("nothing was sent to %d", chat)
	}
	return list[len(list)-1].Text
}

func answers(msgs []sent) []string { return texts(msgs, "answerCallbackQuery") }

func TestOwnerAndFullPartnersReachTheManagementPanel(t *testing.T) {
	b, got := ownerBot(t, shopConfig{})
	ctx := context.Background()
	if !hasData(b.as(boss).mainMenu(), "q:home") {
		t.Fatal("the owner's menu has no owner panel")
	}
	for _, id := range []int64{seller} {
		if hasData(b.as(id).mainMenu(), "q:home") {
			t.Fatalf("admin %d sees the owner panel", id)
		}
		n := len(got())
		b.handle(ctx, callbackFrom(id, "q:home"))
		b.handle(ctx, callbackFrom(id, "q:cw:5001:p"))
		for _, m := range got()[n:] {
			if m.Method == "editMessageText" {
				t.Fatalf("admin %d opened %q", id, m.Text)
			}
		}
		if b.pend.get(id) != nil {
			t.Fatalf("admin %d was asked for a wallet amount", id)
		}
	}
	b.handle(ctx, callbackFrom(boss, "q:home"))
	if !strings.Contains(lastEdit(t, got), "Owner panel") {
		t.Fatalf("owner got %q", lastEdit(t, got))
	}
	if !hasData(b.as(helper).mainMenu(), "q:home") {
		t.Fatal("the full partner's menu has no management panel")
	}
	b.handle(ctx, callbackFrom(helper, "q:home"))
	if !strings.Contains(lastEdit(t, got), "Owner panel") {
		t.Fatalf("full partner got %q", lastEdit(t, got))
	}
	// Without an owner, a full administrator runs the shop.
	useAccess(b, botConfig{Admins: []int64{helper}})
	if !hasData(b.as(helper).mainMenu(), "q:home") {
		t.Fatal("a full admin without an owner has no owner panel")
	}
}

func TestCustomersAreRecordedFoundAndBlocked(t *testing.T) {
	b, got := ownerBot(t, shopConfig{Plans: []shopPlan{{ID: "1", Name: "Gold", Days: 30, GB: 10, Price: "100"}}})
	ctx := context.Background()
	u := privateMessage(buyer, "/start")
	u.Message.From.Username, u.Message.From.FirstName = "ali_r", "Ali"
	b.handle(ctx, u)
	c, ok := customerRow(buyer)
	if !ok || c.Username != "ali_r" || c.Name != "Ali" || c.LastSeen == 0 {
		t.Fatalf("customer row = %+v, %v", c, ok)
	}
	if found := b.findCustomers("@ali"); len(found) != 1 || found[0].TgId != buyer {
		t.Fatalf("search by username = %+v", found)
	}
	if found := b.findCustomers("5001"); len(found) != 1 {
		t.Fatalf("search by id = %+v", found)
	}
	// Somebody who only ordered shows up too.
	database.GetDB().Create(&model.BotOrder{TgId: 6002, Topic: "buy", Status: orderRejected, CreatedAt: 1})
	b.seen.synced = time.Time{} // the list is rebuilt at most every half minute
	if list := b.customerList(); len(list) != 2 {
		t.Fatalf("customers = %+v", list)
	}
	b.handle(ctx, callbackFrom(boss, "q:cv:5001"))
	if !strings.Contains(lastEdit(t, got), "ali_r") {
		t.Fatalf("profile = %q", lastEdit(t, got))
	}

	b.handle(ctx, callbackFrom(boss, "q:cb:5001"))
	if !customerBlocked(buyer) {
		t.Fatal("not blocked")
	}
	n := len(got())
	b.handle(ctx, privateMessage(buyer, "/buy"))
	if txt := lastTo(t, got()[n:], buyer); !strings.Contains(txt, "blocked") {
		t.Fatalf("a blocked customer got %q", txt)
	}
	b.handle(ctx, callbackFrom(buyer, "u:buy:1"))
	var orders int64
	database.GetDB().Model(&model.BotOrder{}).Where("tg_id = ?", buyer).Count(&orders)
	if orders != 0 {
		t.Fatal("a blocked customer placed an order")
	}
	b.handle(ctx, callbackFrom(boss, "q:cb:5001"))
	b.handle(ctx, callbackFrom(buyer, "u:buy:1"))
	if lastOrder(t, buyer).Status != orderPending {
		t.Fatal("unblocked customer could not order")
	}
}

func TestOwnerCorrectsAWalletWithHistory(t *testing.T) {
	b, got := ownerBot(t, shopConfig{})
	ctx := context.Background()
	b.handle(ctx, callbackFrom(boss, "q:cw:5001:p"))
	b.handle(ctx, privateMessage(boss, "۵۰ هزار gift"))
	if bal := walletBalance(buyer); bal != 50000 {
		t.Fatalf("balance = %d", bal)
	}
	if txt := lastTo(t, got(), buyer); !strings.Contains(txt, "50,000") {
		t.Fatalf("customer told %q", txt)
	}
	b.handle(ctx, callbackFrom(boss, "q:cw:5001:m"))
	b.handle(ctx, privateMessage(boss, "80000"))
	if bal := walletBalance(buyer); bal != 0 {
		t.Fatalf("balance after taking more than there is = %d", bal)
	}
	h := walletHistory(buyer, 10)
	if len(h) != 2 || h[0].Delta != -50000 || h[1].Delta != 50000 || h[1].Note != "gift" || h[1].Actor != boss || h[1].Kind != walletManual {
		t.Fatalf("history = %+v", h)
	}
	b.handle(ctx, callbackFrom(boss, "q:ct:5001"))
	if !strings.Contains(lastEdit(t, got), "gift") {
		t.Fatalf("history screen = %q", lastEdit(t, got))
	}
}

func TestWalletHistoryRecordsShopMoney(t *testing.T) {
	b, _ := ownerBot(t, shopConfig{
		PayCard: "6037",
		Plans:   []shopPlan{{ID: "1", Name: "Gold", Days: 30, GB: 10, Price: "100"}},
		Wallets: []shopWallet{{ID: "w", Label: "500", Amount: "500"}},
	})
	ctx := context.Background()
	b.handle(ctx, callbackFrom(buyer, "u:wal:w"))
	top := lastOrder(t, buyer)
	b.handle(ctx, photoMessage(buyer, "r", false))
	b.handle(ctx, callbackFrom(boss, "j:ok:"+itoa(int64(top.Id))))
	b.handle(ctx, callbackFrom(buyer, "u:buy:1"))
	buy := lastOrder(t, buyer)
	b.handle(ctx, callbackFrom(buyer, "u:wp:"+itoa(int64(buy.Id))))
	h := walletHistory(buyer, 10)
	if len(h) != 2 || h[1].Kind != walletTopUp || h[1].Delta != 500 || h[0].Kind != walletPay || h[0].Delta != -100 || h[0].Balance != 400 {
		t.Fatalf("history = %+v", h)
	}
}

func TestOwnerMovesACustomerToAnotherSeller(t *testing.T) {
	b, _ := ownerBot(t, shopConfig{})
	ctx := context.Background()
	if got := b.customerAdminID(buyer); got != boss {
		t.Fatalf("default seller = %d", got)
	}
	b.handle(ctx, callbackFrom(boss, "q:cas:5001:77"))
	if got := b.customerAdminID(buyer); got != seller {
		t.Fatalf("seller after the move = %d", got)
	}
	if list := b.sellerCustomers(seller); len(list) != 1 || list[0].TgId != buyer {
		t.Fatalf("seller customers = %+v", list)
	}
	b.handle(ctx, callbackFrom(boss, "q:cas:5001:0"))
	if got := b.customerAdminID(buyer); got != boss {
		t.Fatalf("seller back to automatic = %d", got)
	}
}

func TestSuspendedSellerIsLockedOut(t *testing.T) {
	b, got := ownerBot(t, shopConfig{Refs: []shopReferrer{{ID: "1", Word: "ali", Group: "Sales", Admin: seller}}})
	ctx := context.Background()
	if err := saveBotRef(buyer, "ali", "Sales"); err != nil {
		t.Fatal(err)
	}
	if got := b.customerAdminID(buyer); got != seller {
		t.Fatalf("seller = %d", got)
	}
	b.handle(ctx, callbackFrom(boss, "q:as:77"))
	if !adminMeta(seller).Suspended {
		t.Fatal("not suspended")
	}
	if txt := lastTo(t, got(), seller); !strings.Contains(txt, "suspended") {
		t.Fatalf("seller told %q", txt)
	}
	n := len(got())
	b.handle(ctx, privateMessage(seller, "/menu"))
	if txt := lastTo(t, got()[n:], seller); !strings.Contains(txt, "suspended") {
		t.Fatalf("suspended seller got %q", txt)
	}
	b.handle(ctx, callbackFrom(seller, "k:edit"))
	for _, m := range got()[n:] {
		if m.Method == "editMessageText" {
			t.Fatalf("suspended seller opened %q", m.Text)
		}
	}
	if got := b.customerAdminID(buyer); got != boss {
		t.Fatalf("a suspended seller's customer goes to %d", got)
	}
	b.handle(ctx, callbackFrom(boss, "q:as:77"))
	if adminMeta(seller).Suspended || b.as(seller).paused {
		t.Fatal("still suspended")
	}
}

func TestOwnerOnlyApprovalAndMinimumPrice(t *testing.T) {
	b, got := ownerBot(t, shopConfig{
		PayCard: "6037",
		Plans:   []shopPlan{{ID: "1", Name: "Gold", Days: 30, GB: 10, Price: "100"}},
	})
	ctx := context.Background()
	b.handle(ctx, callbackFrom(boss, "q:cas:5001:77"))
	b.handle(ctx, callbackFrom(boss, "q:ao:77"))
	b.handle(ctx, callbackFrom(buyer, "u:buy:1"))
	o := lastOrder(t, buyer)
	if o.AdminId != seller {
		t.Fatalf("order goes to %d", o.AdminId)
	}
	n := len(got())
	b.handle(ctx, photoMessage(buyer, "r", false))
	for _, m := range sentTo(got()[n:], seller) {
		if m.Markup != nil && hasData(m.Markup.Keyboard, "j:ok:"+itoa(int64(o.Id))) {
			t.Fatal("the seller got an Approve button")
		}
	}
	b.handle(ctx, callbackFrom(seller, "j:ok:"+itoa(int64(o.Id))))
	if lastOrder(t, buyer).Status != orderPending {
		t.Fatal("the seller approved an owner-only order")
	}
	b.handle(ctx, callbackFrom(boss, "j:ok:"+itoa(int64(o.Id))))
	if lastOrder(t, buyer).Status != orderApproved {
		t.Fatal("the owner could not approve")
	}

	// 10 GB for 100 is under 50 per GB.
	b.handle(ctx, callbackFrom(boss, "q:amin:77"))
	b.handle(ctx, privateMessage(boss, "50"))
	if adminMeta(seller).MinPerGB != 50 {
		t.Fatalf("min = %d", adminMeta(seller).MinPerGB)
	}
	var before int64
	database.GetDB().Model(&model.BotOrder{}).Count(&before)
	n = len(got())
	b.handle(ctx, callbackFrom(buyer, "u:buy:1"))
	var after int64
	database.GetDB().Model(&model.BotOrder{}).Count(&after)
	if after != before {
		t.Fatal("an order under the minimum price was placed")
	}
	if txt := lastTo(t, got()[n:], seller); !strings.Contains(txt, "minimum") {
		t.Fatalf("seller told %q", txt)
	}
}

func TestSellerApprovalIsToldToTheOwner(t *testing.T) {
	b, got := ownerBot(t, shopConfig{PayCard: "6037", Plans: []shopPlan{{ID: "1", Name: "Gold", Days: 30, GB: 10, Price: "100"}}})
	ctx := context.Background()
	b.handle(ctx, callbackFrom(boss, "q:cas:5001:88"))
	b.handle(ctx, callbackFrom(buyer, "u:buy:1"))
	o := lastOrder(t, buyer)
	b.handle(ctx, photoMessage(buyer, "r", false))
	n := len(got())
	b.handle(ctx, callbackFrom(helper, "j:ok:"+itoa(int64(o.Id))))
	if lastOrder(t, buyer).Status != orderApproved {
		t.Fatal("not approved")
	}
	if txt := lastTo(t, got()[n:], boss); !strings.Contains(txt, "approved an order") {
		t.Fatalf("owner told %q", txt)
	}
}

func TestSettlementAndFinance(t *testing.T) {
	b, got := ownerBot(t, shopConfig{})
	ctx := context.Background()
	now := time.Now().Unix()
	db := database.GetDB()
	db.Create(&model.BotOrder{TgId: buyer, AdminId: seller, Topic: "buy", Status: orderApproved, PlanSet: true, PlanGB: 10, Amount: 1000, CreatedAt: now, UpdatedAt: now})
	db.Create(&model.BotOrder{TgId: buyer, AdminId: seller, Topic: "renew", Status: orderApproved, PlanSet: true, PlanGB: 20, Amount: 2000, WalletPaid: true, CreatedAt: now, UpdatedAt: now})
	db.Create(&model.BotOrder{TgId: buyer, AdminId: seller, Topic: "buy", Status: orderRejected, PlanGB: 50, Amount: 9000, CreatedAt: now, UpdatedAt: now})
	db.Create(&model.BotOrder{TgId: buyer, AdminId: boss, Topic: "wallet", Status: orderApproved, Amount: 700, CreatedAt: now, UpdatedAt: now})
	s := sumSales(seller, 0, 0)
	if s.Orders != 2 || s.Revenue != 3000 || s.GB != 30 || s.Wallet != 2000 {
		t.Fatalf("sales = %+v", s)
	}
	if n, a := sumTopUps(0, 0); n != 1 || a != 700 {
		t.Fatalf("top-ups = %d, %d", n, a)
	}
	b.handle(ctx, callbackFrom(boss, "q:arate:77"))
	b.handle(ctx, privateMessage(boss, "100"))
	if _, _, owed := unsettled(seller); owed != 3000 {
		t.Fatalf("owed = %d", owed)
	}
	b.handle(ctx, callbackFrom(boss, "q:fin"))
	if !strings.Contains(lastEdit(t, got), "3,000") {
		t.Fatalf("finance = %q", lastEdit(t, got))
	}
	b.handle(ctx, callbackFrom(boss, "q:asety:77"))
	b.handle(ctx, callbackFrom(boss, "q:asetgo:77"))
	hist := settlements(seller, 5)
	if len(hist) != 1 || hist[0].Amount != 3000 || hist[0].GB != 30 {
		t.Fatalf("settlements = %+v", hist)
	}
	if _, us, owed := unsettled(seller); us.Orders != 0 || owed != 0 {
		t.Fatalf("after settling: %+v owed %d", us, owed)
	}
	if !strings.Contains(lastTo(t, got(), seller), "Settled") {
		t.Fatal("the seller was not told")
	}
	csv := string(ordersCSV())
	if strings.Count(csv, "\n") != 5 || !strings.Contains(csv, ",renew,approved,") {
		t.Fatalf("csv = %q", csv)
	}
	b.handle(ctx, callbackFrom(boss, "q:csv"))
	if len(texts(got(), "sendDocument")) == 0 && !hasMethod(got(), "sendDocument") {
		t.Fatal("no CSV was sent")
	}
}

func hasMethod(msgs []sent, method string) bool {
	for _, m := range msgs {
		if m.Method == method {
			return true
		}
	}
	return false
}

func TestBroadcastReachesTheChosenCustomers(t *testing.T) {
	b, got := ownerBot(t, shopConfig{})
	ctx := context.Background()
	for _, id := range []int64{buyer, 5002, 5003} {
		b.handle(ctx, privateMessage(id, "/start"))
	}
	b.handle(ctx, callbackFrom(boss, "q:cb:5003"))
	b.handle(ctx, callbackFrom(boss, "q:cas:5002:77"))
	if n := len(b.audience("all")); n != 2 {
		t.Fatalf("everyone = %d", n)
	}
	if ids := b.audience("a77"); len(ids) != 1 || ids[0] != 5002 {
		t.Fatalf("seller audience = %v", ids)
	}
	b.handle(ctx, callbackFrom(boss, "q:bca:all"))
	b.handle(ctx, privateMessage(boss, "Hello <all>"))
	n := len(got())
	b.handle(ctx, callbackFrom(boss, "q:bcy"))
	after := got()[n:]
	if txt := lastTo(t, after, buyer); !strings.Contains(txt, "Hello &lt;all&gt;") {
		t.Fatalf("customer got %q", txt)
	}
	if len(sentTo(after, 5003)) != 0 {
		t.Fatal("a blocked customer got the broadcast")
	}
	reported := false
	for _, m := range sentTo(after, boss) {
		reported = reported || strings.Contains(m.Text, "✅ 2 · ❌ 0")
	}
	if !reported {
		t.Fatalf("owner report missing: %+v", sentTo(after, boss))
	}
}

func TestStaleOrdersAreRemindedOnce(t *testing.T) {
	b, got := ownerBot(t, shopConfig{})
	ctx := context.Background()
	old := time.Now().Add(-5 * time.Hour).Unix()
	db := database.GetDB()
	db.Create(&model.BotOrder{TgId: buyer, Topic: "buy", Status: orderPending, ReceiptID: "x", Detail: "Gold", CreatedAt: old, UpdatedAt: old})
	db.Create(&model.BotOrder{TgId: buyer, Topic: "buy", Status: orderPending, Detail: "no receipt", CreatedAt: old, UpdatedAt: old})
	db.Create(&model.BotOrder{TgId: buyer, Topic: "buy", Status: orderPending, ReceiptID: "y", Detail: "fresh", CreatedAt: time.Now().Unix()})
	b.ownerWatch(ctx, time.Now())
	txt := lastTo(t, got(), boss)
	if !strings.Contains(txt, "1 orders are waiting") || !strings.Contains(txt, "Gold") || strings.Contains(txt, "fresh") {
		t.Fatalf("reminder = %q", txt)
	}
	n := len(got())
	b.ownerWatch(ctx, time.Now())
	if len(sentTo(got()[n:], boss)) != 0 {
		t.Fatal("reminded twice")
	}
	b.handle(ctx, callbackFrom(boss, "q:alt:"+prefStale))
	if prefOn(prefStale) {
		t.Fatal("alert not switched off")
	}
}

func TestSellerDeletingAClientIsToldToTheOwner(t *testing.T) {
	b, got := ownerBot(t, shopConfig{})
	if err := b.as(boss).createClient("sales-one", "Sales", 10*gib, 30, 0, false); err != nil {
		t.Fatal(err)
	}
	c := loadByName(t, "sales-one")
	n := len(got())
	if err := b.as(seller).deleteClient(c.Id); err != nil {
		t.Fatal(err)
	}
	if txt := lastTo(t, got()[n:], boss); !strings.Contains(txt, "Deleted client") || !strings.Contains(txt, "sales-one") {
		t.Fatalf("owner told %q", txt)
	}
	// The owner's own changes are not reported back to them.
	if err := b.as(boss).createClient("own", "", 10*gib, 30, 0, false); err != nil {
		t.Fatal(err)
	}
	n = len(got())
	if err := b.as(boss).deleteClient(loadByName(t, "own").Id); err != nil {
		t.Fatal(err)
	}
	if len(sentTo(got()[n:], boss)) != 0 {
		t.Fatal("the owner was told of their own change")
	}
}

func TestOwnerEditsASellersShop(t *testing.T) {
	b, got := ownerBot(t, shopConfig{PayCard: "OWNER"})
	ctx := context.Background()
	b.handle(ctx, callbackFrom(boss, "q:adesk:77"))
	if !strings.Contains(lastEdit(t, got), "77") {
		t.Fatalf("desk screen = %q", lastEdit(t, got))
	}
	b.handle(ctx, callbackFrom(boss, "k:pcard"))
	b.handle(ctx, privateMessage(boss, "SELLER-CARD"))
	s := b.loadShopRaw()
	if s.PayCard != "OWNER" || s.Desks["77"].PayCard != "SELLER-CARD" {
		t.Fatalf("shop = %+v", s)
	}
	b.handle(ctx, callbackFrom(boss, "q:dx"))
	b.handle(ctx, callbackFrom(boss, "k:pcard"))
	b.handle(ctx, privateMessage(boss, "OWNER-2"))
	if s := b.loadShopRaw(); s.PayCard != "OWNER-2" || s.Desks["77"].PayCard != "SELLER-CARD" {
		t.Fatalf("shop after leaving = %+v", s)
	}
}

func TestQuotaUsedUpIsAnnounced(t *testing.T) {
	b, got := ownerBot(t, shopConfig{})
	ctx := context.Background()
	if _, err := setQuotaTotal(seller, 10*gib); err != nil {
		t.Fatal(err)
	}
	if err := addQuotaSold(seller, 10*gib); err != nil {
		t.Fatal(err)
	}
	b.ownerWatch(ctx, time.Now())
	if txt := lastTo(t, got(), boss); !strings.Contains(txt, "used up") {
		t.Fatalf("owner told %q", txt)
	}
	if txt := lastTo(t, got(), seller); !strings.Contains(txt, "used up") {
		t.Fatalf("seller told %q", txt)
	}
	n := len(got())
	b.ownerWatch(ctx, time.Now())
	if len(sentTo(got()[n:], boss)) != 0 {
		t.Fatal("announced twice")
	}
}

func TestPanelUpdateFromTheBot(t *testing.T) {
	b, got := ownerBot(t, shopConfig{})
	ctx := context.Background()
	prevInfo, prevStart := panelUpdateInfo, panelUpdateStart
	t.Cleanup(func() { panelUpdateInfo, panelUpdateStart = prevInfo, prevStart })
	panelUpdateInfo = func(bool) service.PanelUpdate {
		return service.PanelUpdate{Current: "v49.5", Latest: "v49.6", Newer: true}
	}
	started := ""
	panelUpdateStart = func(lang, by, tag string) (string, error) { started = tag + " by " + by; return tag, nil }
	b.handle(ctx, callbackFrom(boss, "q:up"))
	if !strings.Contains(lastEdit(t, got), "v49.6") {
		t.Fatalf("update screen = %q", lastEdit(t, got))
	}
	b.handle(ctx, callbackFrom(boss, "q:upy:v49.6"))
	b.handle(ctx, callbackFrom(boss, "q:upgo:v49.6"))
	if started != "v49.6 by telegram:1000" {
		t.Fatalf("started = %q", started)
	}
	b.handle(ctx, callbackFrom(helper, "q:upgo:v49.7"))
	if started != "v49.7 by telegram:88" {
		t.Fatalf("a full partner could not start an attributed update: %q", started)
	}
	b.handle(ctx, callbackFrom(seller, "q:upgo:v49.8"))
	if started != "v49.7 by telegram:88" {
		t.Fatal("a restricted seller started an update")
	}
}

func TestSupportMessageOffersAReply(t *testing.T) {
	b, got := ownerBot(t, shopConfig{})
	ctx := context.Background()
	b.handle(ctx, privateMessage(buyer, "/support"))
	b.handle(ctx, privateMessage(buyer, "help me"))
	list := sentTo(got(), boss)
	if len(list) == 0 || list[len(list)-1].Markup == nil || !hasData(list[len(list)-1].Markup.Keyboard, "q:cm:5001") {
		t.Fatal("no reply button on the support message")
	}
	b.handle(ctx, callbackFrom(boss, "q:cm:5001"))
	b.handle(ctx, privateMessage(boss, "on it"))
	if txt := lastTo(t, got(), buyer); !strings.Contains(txt, "on it") {
		t.Fatalf("customer got %q", txt)
	}
}

func TestIntroducerWordLinksTheCustomerToItsSeller(t *testing.T) {
	b, _ := ownerBot(t, shopConfig{Refs: []shopReferrer{{ID: "1", Word: "ali", Group: "Sales", Admin: seller}}})
	ctx := context.Background()
	b.handle(ctx, privateMessage(buyer, "/start ali"))
	if c, ok := customerRow(buyer); !ok || c.AdminId != seller {
		t.Fatalf("customer row = %+v %v", c, ok)
	}
	// The owner can still clear the seller, and a seller set stays set.
	b.handle(ctx, callbackFrom(boss, "q:cas:5001:0"))
	if c, _ := customerRow(buyer); c.AdminId != 0 {
		t.Fatalf("owner's choice not kept: %+v", c)
	}
	linkRefSeller(buyer, boss)
	if c, _ := customerRow(buyer); c.AdminId != boss {
		t.Fatalf("relink = %+v", c)
	}
	linkRefSeller(buyer, seller)
	if c, _ := customerRow(buyer); c.AdminId != boss {
		t.Fatalf("a chosen seller was replaced: %+v", c)
	}
}
