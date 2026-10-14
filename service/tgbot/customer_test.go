package tgbot

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
)

func TestCustomerCommandMenuMatchesTheShopBots(t *testing.T) {
	names := make([]string, 0, len(userCommands))
	for _, c := range userCommands {
		names = append(names, c.name)
	}
	got := strings.Join(names, " ")
	if got != "start buy services wallet help support" {
		t.Fatalf("customer commands = %s", got)
	}
	if userCommands[0].fa != "restart ♻️ شروع مجدد" || userCommands[1].fa != "خرید اشتراک 🛒" {
		t.Fatalf("command descriptions = %+v", userCommands)
	}
}

func TestCustomerStartShowsTheBottomMenu(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "bot.db")); err != nil {
		t.Fatal(err)
	}
	b, got := fakeTelegram(t)
	b.cfg.Lang = "fa"
	ctx := context.Background()
	b.handle(ctx, privateMessage(99, "/start"))
	msgs := got()
	if len(msgs) != 1 || msgs[0].ChatID != 99 || msgs[0].Markup == nil {
		t.Fatalf("/start = %+v", msgs)
	}
	var labels []string
	for _, row := range msgs[0].Markup.Reply {
		if len(row) != 2 {
			t.Fatalf("row %#v is not two columns", row)
		}
		for _, cell := range row {
			labels = append(labels, cell.Text)
		}
	}
	want := []string{
		"خرید اشتراک 🔐", "تمدید سرویس ♻️",
		"سرویس های من 🛍️", "کیف پول + شارژ 🏦",
		"اکانت تست 🔑", "تعرفه اشتراک ها 💵",
		"پشتیبانی ☎️", "آموزش 📚",
	}
	if strings.Join(labels, "|") != strings.Join(want, "|") {
		t.Fatalf("keyboard = %#v", labels)
	}
	if !msgs[0].Markup.Persistent {
		t.Fatal("the bottom menu is not persistent")
	}
}

func TestCustomerMenuButtonsAndSupportReachAdmins(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "bot.db")); err != nil {
		t.Fatal(err)
	}
	b, got := fakeTelegram(t)
	b.cfg.Lang = "fa"
	ctx := context.Background()

	b.handle(ctx, privateMessage(99, "سرویس های من 🛍️"))
	if text := lastText(got(), "sendMessage"); !strings.Contains(text, "سرویسی") {
		t.Fatalf("my services with none: %q", text)
	}

	n := len(got())
	b.handle(ctx, privateMessage(99, "/help"))
	if text := lastText(got()[n:], "sendMessage"); !strings.Contains(text, "آموزش اتصال") {
		t.Fatalf("/help = %q", lastText(got()[n:], "sendMessage"))
	}

	n = len(got())
	b.handle(ctx, privateMessage(99, "پشتیبانی ☎️"))
	b.handle(ctx, privateMessage(99, "خط وصل نمیشه"))
	msgs := got()[n:]
	var toAdmin, toUser string
	for _, m := range msgs {
		if m.ChatID == 42 {
			toAdmin = m.Text
		}
		if m.ChatID == 99 {
			toUser = m.Text
		}
	}
	if !strings.Contains(toAdmin, "خط وصل نمیشه") || !strings.Contains(toAdmin, "99") {
		t.Fatalf("admin was not told: %q", toAdmin)
	}
	if !strings.Contains(toUser, "پشتیبانی") && !strings.Contains(toUser, "رسید") {
		t.Fatalf("customer was not thanked: %q", toUser)
	}
}

func TestBoundCustomerSeesTheirServiceFromTheMenu(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "bot.db")); err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().Create(&model.Client{
		Name: "bob", Enable: true, TgId: 99, Volume: 1000,
		Inbounds: json.RawMessage(`[]`), Links: json.RawMessage(`[]`),
	}).Error; err != nil {
		t.Fatal(err)
	}
	b, got := fakeTelegram(t)
	ctx := context.Background()
	b.handle(ctx, privateMessage(99, "My services 🛍️"))
	found := false
	for _, m := range got() {
		if strings.Contains(m.Text, "bob") {
			found = true
		}
	}
	if !found {
		t.Fatalf("my services = %q", texts(got(), "sendMessage"))
	}
	n := len(got())
	b.handle(ctx, privateMessage(99, "/usage"))
	if text := lastText(got()[n:], "sendMessage"); !strings.Contains(text, "bob") {
		t.Fatalf("/usage = %q", lastText(got()[n:], "sendMessage"))
	}
}

func TestAdminStartIsStillThePanel(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "bot.db")); err != nil {
		t.Fatal(err)
	}
	b, got := fakeTelegram(t)
	ctx := context.Background()
	b.handle(ctx, privateMessage(42, "/start"))
	found := false
	for _, m := range got() {
		if strings.Contains(m.Text, "Admin panel") || strings.Contains(m.Text, "management") {
			found = true
		}
		if strings.Contains(m.Text, "Buy a plan") {
			t.Fatalf("admin /start showed the customer menu: %+v", m)
		}
	}
	if !found {
		t.Fatalf("admin /start = %+v", got())
	}
}

func TestAdminCanOpenTheCustomerMenu(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "bot.db")); err != nil {
		t.Fatal(err)
	}
	b, got := fakeTelegram(t)
	b.cfg.Lang = "fa"
	ctx := context.Background()
	b.handle(ctx, privateMessage(42, "/start"))
	n := len(got())
	b.handle(ctx, privateMessage(42, "🛍 نمای مشتری"))
	msgs := got()[n:]
	if len(msgs) == 0 || msgs[0].Markup == nil {
		t.Fatalf("customer view = %+v", msgs)
	}
	var labels []string
	for _, row := range msgs[0].Markup.Reply {
		for _, cell := range row {
			labels = append(labels, cell.Text)
		}
	}
	joined := strings.Join(labels, "|")
	for _, want := range []string{"خرید اشتراک 🔐", "تمدید سرویس ♻️", "سرویس های من 🛍️", "پشتیبانی ☎️", "🔙 پنل مدیریت"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("customer view keyboard missing %q: %#v", want, labels)
		}
	}
	n = len(got())
	b.handle(ctx, privateMessage(42, "🔙 پنل مدیریت"))
	back := lastText(got()[n:], "sendMessage")
	if !strings.Contains(back, "پنل") && !strings.Contains(back, "Admin") && !strings.Contains(back, "DrNetwork") {
		t.Fatalf("back to admin = %q", back)
	}
}
