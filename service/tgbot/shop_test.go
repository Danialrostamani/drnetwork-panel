package tgbot

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"encoding/json"
	"github.com/Danialrostamani/drnetwork-panel/service"
)

func TestCustomerBuyIsButtonsAndOwnerEditsPlans(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "bot.db")); err != nil {
		t.Fatal(err)
	}
	b, got := fakeTelegram(t)
	b.cfg.Lang = "en"
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

	n = len(got())
	b.handle(ctx, callbackFrom(99, "u:buy:"+id))
	b.handle(ctx, callbackFrom(99, "u:nm:buy:"+id))
	var toAdmin string
	for _, m := range got()[n:] {
		if m.ChatID == 42 {
			toAdmin = m.Text
		}
	}
	if !strings.Contains(toAdmin, "Silver") || !strings.Contains(toAdmin, "99") {
		t.Fatalf("admin was not told of the buy: %q", toAdmin)
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
