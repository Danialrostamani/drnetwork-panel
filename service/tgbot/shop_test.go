package tgbot

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"encoding/json"
	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/service"
)

func TestCustomerBuyIsButtonsAndOwnerEditsPlans(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "bot.db")); err != nil {
		t.Fatal(err)
	}
	b, got := fakeTelegram(t)
	b.cfg.Lang = "en"
	b.cfg.Owner = 42
	b.setAccess(b.cfg.access())
	ctx := context.Background()

	b.handle(ctx, privateMessage(99, "Buy a plan 🔐"))
	text := lastText(got(), "sendMessage")
	if !strings.Contains(text, "No plan") {
		t.Fatalf("empty shop buy = %q", text)
	}

	b.handle(ctx, privateMessage(42, "🛍 Edit shop"))
	if text := lastText(got(), "sendMessage"); !strings.Contains(text, "Customer shop") {
		t.Fatalf("owner shop = %q", lastText(got(), "sendMessage"))
	}

	b.handle(ctx, callbackFrom(42, "k:pnew"))
	b.handle(ctx, privateMessage(42, "Silver"))
	if text := lastText(got(), "editMessageText"); !strings.Contains(text, "Silver") {
		t.Fatalf("new plan = %q", texts(got(), "editMessageText"))
	}

	s := b.loadShop()
	if len(s.Plans) != 1 || s.Plans[0].Name != "Silver" || s.Plans[0].Days != 30 || s.Plans[0].GB != 50 {
		t.Fatalf("saved plan = %+v", s.Plans)
	}
	id := s.Plans[0].ID
	b.handle(ctx, callbackFrom(42, "k:pds:"+id+":90"))
	b.handle(ctx, callbackFrom(42, "k:pgs:"+id+":100"))
	b.handle(ctx, callbackFrom(42, "k:pps:"+id+":150000"))
	s = b.loadShop()
	if s.Plans[0].Days != 90 || s.Plans[0].GB != 100 || s.Plans[0].Price != "150000" {
		t.Fatalf("patched plan = %+v", s.Plans[0])
	}

	n := len(got())
	b.handle(ctx, privateMessage(99, "Buy a plan 🔐"))
	buy := got()[n:]
	found := false
	for _, m := range buy {
		if m.Markup == nil {
			continue
		}
		for _, row := range m.Markup.Keyboard {
			for _, cell := range row {
				if strings.Contains(cell.Text, "Silver") && strings.Contains(cell.Data, "u:buy:") {
					found = true
				}
			}
		}
	}
	if !found {
		t.Fatalf("customer did not get plan buttons: %+v", buy)
	}

	s = b.loadShop()
	s.PayCard = "6037991234567890"
	if err := b.saveShop(s); err != nil {
		t.Fatal(err)
	}

	n = len(got())
	b.handle(ctx, callbackFrom(99, "u:buy:"+id))
	var toCustomer, toAdmin string
	for _, m := range got()[n:] {
		if m.ChatID == 99 && m.Method == "sendMessage" {
			toCustomer = m.Text
		}
		if m.ChatID == 42 {
			toAdmin = m.Text
		}
	}
	if !strings.Contains(toCustomer, "Payment") || !strings.Contains(toCustomer, "Silver") || !strings.Contains(toCustomer, "150000") || !strings.Contains(toCustomer, "6037991234567890") {
		t.Fatalf("pay screen = %q", toCustomer)
	}
	if !strings.Contains(toCustomer, "90") || !strings.Contains(toCustomer, "100 GB") {
		t.Fatalf("plan details missing: %q", toCustomer)
	}
	if toAdmin != "" {
		t.Fatalf("admin should wait for the receipt, got %q", toAdmin)
	}
}

func TestShopWelcomeAndHiddenTrial(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "bot.db")); err != nil {
		t.Fatal(err)
	}
	off := false
	raw, _ := jsonShop(shopConfig{
		WelcomeEn: "Hello from the shop",
		Trial:     &off,
	})
	if err := (&service.SettingService{}).SetTgBotShop(raw); err != nil {
		t.Fatal(err)
	}
	b, got := fakeTelegram(t)
	b.cfg.Lang = "en"
	ctx := context.Background()
	b.handle(ctx, privateMessage(99, "/start"))
	if text := lastText(got(), "sendMessage"); !strings.Contains(text, "Hello from the shop") {
		t.Fatalf("welcome = %q", text)
	}
	var labels []string
	for _, m := range got() {
		if m.Markup == nil {
			continue
		}
		for _, row := range m.Markup.Reply {
			for _, cell := range row {
				labels = append(labels, cell.Text)
			}
		}
	}
	joined := strings.Join(labels, "|")
	if strings.Contains(joined, "Test account") {
		t.Fatalf("trial stayed on: %q", joined)
	}
}

func jsonShop(s shopConfig) (string, error) {
	b, err := json.Marshal(s)
	return string(b), err
}

func TestFindRefWordIsCaseInsensitive(t *testing.T) {
	s := shopConfig{Refs: []shopReferrer{
		{ID: "1", Word: "Ali", Group: "tehran"},
		{ID: "2", Word: "reza", Group: "mashhad"},
	}}
	if s.findRef("ALI") == nil || s.findRef(" ali ").Group != "tehran" {
		t.Fatalf("ali = %+v", s.findRef("ALI"))
	}
	if s.findRef("Reza").Group != "mashhad" {
		t.Fatalf("reza = %+v", s.findRef("Reza"))
	}
	if s.findRef("nope") != nil || s.findRef("") != nil {
		t.Fatal("unknown word matched")
	}
}

func TestNewCustomerMustSendReferrerWord(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "bot.db")); err != nil {
		t.Fatal(err)
	}
	raw, _ := jsonShop(shopConfig{
		Plans: []shopPlan{{ID: "1", Name: "Gold", Days: 30, GB: 50, Price: "100"}},
		Refs:  []shopReferrer{{ID: "1", Word: "Ali", Group: "tehran"}},
	})
	if err := (&service.SettingService{}).SetTgBotShop(raw); err != nil {
		t.Fatal(err)
	}
	b, got := fakeTelegram(t)
	b.cfg.Lang = "en"
	b.cfg.Owner = 42
	b.setAccess(b.cfg.access())
	ctx := context.Background()

	b.handle(ctx, privateMessage(99, "Buy a plan 🔐"))
	ask := lastText(got(), "sendMessage")
	if !strings.Contains(ask, "Introducer") {
		t.Fatalf("ask ref = %q", ask)
	}
	for _, m := range got() {
		if m.Markup == nil {
			continue
		}
		for _, row := range m.Markup.Keyboard {
			for _, cell := range row {
				if strings.Contains(cell.Text, "Gold") {
					t.Fatalf("plan shown before introducer: %+v", m)
				}
			}
		}
	}

	n := len(got())
	b.handle(ctx, privateMessage(99, "nope"))
	if text := lastText(got()[n:], "sendMessage"); !strings.Contains(text, "not valid") {
		t.Fatalf("bad word = %q", text)
	}

	n = len(got())
	b.handle(ctx, privateMessage(99, "ALI"))
	found := false
	for _, m := range got()[n:] {
		if m.Markup == nil {
			continue
		}
		for _, row := range m.Markup.Keyboard {
			for _, cell := range row {
				if strings.Contains(cell.Text, "Gold") && strings.Contains(cell.Data, "u:buy:") {
					found = true
				}
			}
		}
	}
	if !found {
		t.Fatalf("after introducer = %+v", got()[n:])
	}
	stored := loadBotRef(99)
	if stored == nil || stored.Word != "Ali" || stored.Group != "tehran" {
		t.Fatalf("stored = %+v", stored)
	}

	n = len(got())
	b.handle(ctx, callbackFrom(99, "u:buy:1"))
	pay := lastText(got()[n:], "sendMessage")
	if !strings.Contains(pay, "Payment") || !strings.Contains(pay, "Gold") || !strings.Contains(pay, "100") {
		t.Fatalf("pay screen = %q", pay)
	}
	var toAdmin string
	for _, m := range got()[n:] {
		if m.ChatID == 42 {
			toAdmin = m.Text
		}
	}
	if toAdmin != "" {
		t.Fatalf("admin should wait for the receipt, got %q", toAdmin)
	}

	n = len(got())
	var rec update
	_ = json.Unmarshal([]byte(`{"update_id":3,"message":{"photo":[{"file_id":"rec1"}],"chat":{"id":99,"type":"private"},"from":{"id":99}}}`), &rec)
	b.handle(ctx, rec)
	for _, m := range got()[n:] {
		if m.ChatID == 42 && m.Text != "" {
			toAdmin = m.Text
			break
		}
	}
	if !strings.Contains(toAdmin, "Gold") || !strings.Contains(toAdmin, "Ali") {
		t.Fatalf("admin notice = %q", toAdmin)
	}
}

// A new customer is asked for the introducer word as soon as they open the
// bot; the menu (but Support) waits for it.
func TestStartAsksNewCustomerForReferrerFirst(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "bot.db")); err != nil {
		t.Fatal(err)
	}
	raw, _ := jsonShop(shopConfig{
		Plans: []shopPlan{{ID: "1", Name: "Gold", Days: 30, GB: 50, Price: "100"}},
		Refs:  []shopReferrer{{ID: "1", Word: "Ali", Group: "tehran"}},
	})
	if err := (&service.SettingService{}).SetTgBotShop(raw); err != nil {
		t.Fatal(err)
	}
	b, got := fakeTelegram(t)
	b.cfg.Lang = "en"
	b.cfg.Owner = 42
	b.setAccess(b.cfg.access())
	ctx := context.Background()

	b.handle(ctx, privateMessage(99, "/start"))
	if ask := lastText(got(), "sendMessage"); !strings.Contains(ask, "Introducer") {
		t.Fatalf("start = %q", ask)
	}
	n := len(got())
	b.handle(ctx, privateMessage(99, "/wallet"))
	if ask := lastText(got()[n:], "sendMessage"); !strings.Contains(ask, "Introducer") {
		t.Fatalf("wallet before ref = %q", ask)
	}
	n = len(got())
	b.handle(ctx, privateMessage(99, "/link"))
	if ask := lastText(got()[n:], "sendMessage"); !strings.Contains(ask, "Introducer") {
		t.Fatalf("link before ref = %q", ask)
	}
	n = len(got())
	b.handle(ctx, privateMessage(99, "/support"))
	if ask := lastText(got()[n:], "sendMessage"); !strings.Contains(ask, "Support") || strings.Contains(ask, "Introducer") {
		t.Fatalf("support before ref = %q", ask)
	}
	b.handle(ctx, privateMessage(99, "/start"))
	n = len(got())
	b.handle(ctx, privateMessage(99, "ali"))
	out := strings.Join(texts(got()[n:], "sendMessage"), "\n")
	if !strings.Contains(out, "Introducer saved") || strings.Contains(lastText(got()[n:], "sendMessage"), "Introducer") {
		t.Fatalf("after ref = %q", out)
	}
	if loadBotRef(99) == nil {
		t.Fatal("ref not stored")
	}
	n = len(got())
	b.handle(ctx, privateMessage(99, "/start"))
	if text := lastText(got()[n:], "sendMessage"); strings.Contains(text, "Introducer") {
		t.Fatalf("asked again = %q", text)
	}
}
