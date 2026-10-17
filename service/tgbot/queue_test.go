package tgbot

import (
	"context"
	"strings"
	"testing"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/service"
)

func TestUniqueClientNamePrefersUsername(t *testing.T) {
	if err := database.InitDB(t.TempDir() + "/bot.db"); err != nil {
		t.Fatal(err)
	}
	b, _ := fakeTelegram(t)
	if got := b.uniqueClientName(99, "ali_reza"); got != "ali_reza" {
		t.Fatalf("username = %q", got)
	}
	if got := b.uniqueClientName(99, ""); got != "u99" {
		t.Fatalf("empty = %q", got)
	}
	if got := clientNameHint(&fromUser{ID: 99, Username: "Ali.Reza", FirstName: "علی"}); got != "Ali.Reza" {
		t.Fatalf("hint username = %q", got)
	}
	if got := clientNameHint(&fromUser{ID: 99, FirstName: "علی"}); got != "" {
		t.Fatalf("persian hint = %q", got)
	}
	if err := database.GetDB().Create(&model.Client{Name: "ali_reza", Enable: true, Inbounds: []byte("[]"), Links: []byte("[]")}).Error; err != nil {
		t.Fatal(err)
	}
	if got := b.uniqueClientName(99, "ali_reza"); got != "ali_reza_2" {
		t.Fatalf("conflict = %q", got)
	}
}

func TestCustomerCanCancelAPendingOrder(t *testing.T) {
	if err := database.InitDB(t.TempDir() + "/bot.db"); err != nil {
		t.Fatal(err)
	}
	raw, _ := jsonShop(shopConfig{
		Plans:   []shopPlan{{ID: "1", Name: "Gold", Days: 30, GB: 40, Price: "100"}},
		PayCard: "603799",
	})
	if err := (&service.SettingService{}).SetTgBotShop(raw); err != nil {
		t.Fatal(err)
	}
	b, got := fakeTelegram(t)
	b.cfg.Lang = "en"
	b.cfg.Owner = 42
	b.setAccess(b.cfg.access())
	ctx := context.Background()
	b.handle(ctx, callbackFrom(99, "u:buy:1"))
	var o model.BotOrder
	if err := database.GetDB().Where("tg_id = ?", 99).First(&o).Error; err != nil {
		t.Fatal(err)
	}
	if o.Status != orderPending {
		t.Fatalf("status = %s", o.Status)
	}
	n := len(got())
	b.handle(ctx, callbackFrom(99, "u:cx:"+itoa(int64(o.Id))))
	if err := database.GetDB().First(&o, o.Id).Error; err != nil {
		t.Fatal(err)
	}
	if o.Status != orderCancelled {
		t.Fatalf("after cancel = %s", o.Status)
	}
	var toCustomer, toAdmin string
	for _, m := range got()[n:] {
		if m.ChatID == 99 && m.Method == "sendMessage" {
			toCustomer = m.Text
		}
		if m.ChatID == 42 && m.Text != "" {
			toAdmin = m.Text
		}
	}
	if !strings.Contains(toCustomer, "cancelled") {
		t.Fatalf("customer = %q", toCustomer)
	}
	if !strings.Contains(toAdmin, "cancelled") {
		t.Fatalf("admin = %q", toAdmin)
	}
}

func TestTrialIsOnlyOnce(t *testing.T) {
	if err := database.InitDB(t.TempDir() + "/bot.db"); err != nil {
		t.Fatal(err)
	}
	b, got := fakeTelegram(t)
	b.cfg.Lang = "en"
	if err := database.GetDB().Create(&model.BotOrder{TgId: 99, Topic: "test", Status: orderApproved}).Error; err != nil {
		t.Fatal(err)
	}
	b.autoTrial(context.Background(), 99, &fromUser{ID: 99})
	text := lastText(got(), "sendMessage")
	if !strings.Contains(text, "only once") {
		t.Fatalf("second trial = %q", text)
	}
}

func TestAdminOrderQueueAndSalesReport(t *testing.T) {
	e := newAccessEnv(t)
	e.b.cfg.Lang = "en"
	if err := database.GetDB().Create(&model.BotOrder{TgId: 99, Topic: "buy", Status: orderPending, AdminId: fullAdmin, Detail: "Gold"}).Error; err != nil {
		t.Fatal(err)
	}
	giveQuota(t, fullAdmin, 100)
	if err := e.b.consumeSaleQuota(fullAdmin, 40*gib); err != nil {
		t.Fatal(err)
	}
	text, kb := e.b.as(fullAdmin).orderQueueScreen(true)
	if !strings.Contains(text, "Gold") || !strings.Contains(text, "pending") {
		t.Fatalf("queue = %q", text)
	}
	found := false
	for _, row := range kb {
		for _, btn := range row {
			if strings.HasPrefix(btn.Data, "j:ok:") {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("no approve button: %+v", kb)
	}
	report, _ := e.b.as(fullAdmin).salesReportScreen()
	if !strings.Contains(report, "Sold") || !strings.Contains(report, "Pending") {
		t.Fatalf("report = %q", report)
	}
}

func TestQuotaWarnsOwnerAtEightyPercent(t *testing.T) {
	e := newAccessEnv(t)
	e.b.cfg.Lang = "en"
	giveQuota(t, fullAdmin, 10)
	if err := e.b.consumeSaleQuota(fullAdmin, 8*gib); err != nil {
		t.Fatal(err)
	}
	n := len(e.got())
	e.b.maybeQuotaWarn(context.Background(), fullAdmin)
	found := false
	for _, m := range e.got()[n:] {
		if m.ChatID == ownerID && strings.Contains(m.Text, "80%") {
			found = true
		}
	}
	if !found {
		t.Fatalf("no 80%% warning: %+v", e.got()[n:])
	}
	n = len(e.got())
	e.b.maybeQuotaWarn(context.Background(), fullAdmin)
	if len(e.got()) != n {
		t.Fatal("warned twice")
	}
}

func TestTrialQuotaErrorIsPlain(t *testing.T) {
	e := newAccessEnv(t)
	e.b.cfg.Lang = "en"
	giveQuota(t, fullAdmin, 1)
	if err := e.b.consumeSaleQuota(fullAdmin, 1*gib); err != nil {
		t.Fatal(err)
	}
	// Owner is the customer's admin when no introducer is set.
	useAccess(e.b, botConfig{Owner: ownerID, Admins: []int64{fullAdmin, salesAdmin}, Scopes: map[int64]string{salesAdmin: salesGroup}})
	// Force the trial onto the limited admin.
	raw, _ := jsonShop(shopConfig{Refs: []shopReferrer{{ID: "1", Word: "x", Group: salesGroup, Admin: fullAdmin}}})
	if err := (&service.SettingService{}).SetTgBotShop(raw); err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().Create(&model.BotRef{TgId: 99, Word: "x", Group: salesGroup}).Error; err != nil {
		t.Fatal(err)
	}
	n := len(e.got())
	e.b.as(99).autoTrial(context.Background(), 99, &fromUser{ID: 99})
	text := lastText(e.got()[n:], "sendMessage")
	if !strings.Contains(strings.ToLower(text), "quota") {
		t.Fatalf("trial quota = %q", text)
	}
	if strings.Contains(text, "config service") || strings.Contains(text, "sql") {
		t.Fatalf("technical error leaked: %q", text)
	}
}
