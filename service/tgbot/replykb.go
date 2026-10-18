package tgbot

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
)

// The bottom menu is a persistent reply keyboard that stays under the chat.
// Its buttons send their label as an ordinary message, so each label is
// matched back to the inline button it stands for, and runs like that button
// would - with the same permission checks - answering with a new message.

type menuItem struct {
	fa, en string
	data   string
}

var menuItems = []menuItem{
	{"خرید اشتراک 🔐", "Buy a plan 🔐", "p:buy"},
	{"تمدید سرویس ♻️", "Renew ♻️", "p:renew"},
	{"سرویس های من 🛍️", "My services 🛍️", "p:my"},
	{"اتصال لینک 🔗", "Connect link 🔗", "p:link"},
	{"کیف پول + شارژ 🏦", "Wallet 🏦", "p:wallet"},
	{"اکانت تست 🔑", "Test account 🔑", "p:trial"},
	{"تعرفه اشتراک ها 💵", "Plans 💵", "p:tariff"},
	{"آموزش 📚", "Guide 📚", "p:guide"},
	{"پشتیبانی ☎️", "Support ☎️", "p:support"},
	{"🛍 نمای مشتری", "🛍 Customer view", "k:shop"},
	{"🛍 ویرایش فروشگاه", "🛍 Edit shop", "k:edit"},
	{"📋 سفارش‌ها", "📋 Orders", "k:ordsP"},
	{"📊 گزارش فروش", "📊 Sales report", "k:sales"},
	{"🔙 پنل مدیریت", "🔙 Admin panel", "k:admin"},
	{"👑 پنل مالک", "👑 Owner panel", "q:home"},
	{"🏠 خانه", "🏠 Home", "h:home"},
	{"📋 منوی اصلی", "📋 Main menu", "m:menu"},
	{"👥 کلاینت‌ها", "👥 Clients", "c:ls:a:0"},
	{"🖥 نودها", "🖥 Nodes", "o:nd:ls:0"},
	{"📊 آمار", "📊 Stats", "t:d1"},
}

type fromUser struct {
	ID                            int64
	FirstName, LastName, Username string
}

func menuData(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	for _, it := range menuItems {
		if text == it.fa || text == it.en {
			return it.data
		}
	}
	return ""
}

func (b *bot) menuLabel(data string) string {
	if strings.HasPrefix(data, "p:") {
		it := b.loadShop().btn(data)
		if it.Fa != "" {
			return b.tr(it.Fa, nonempty(it.En, it.Fa))
		}
	}
	for _, it := range menuItems {
		if it.data == data {
			return b.tr(it.fa, it.en)
		}
	}
	return data
}

// adminMenu is the bottom menu of an administrator: the sections their role
// opens.
func (b *bot) adminMenu() [][]string {
	var rows [][]string
	for _, r := range [][]string{{"h:home", "m:menu"}, {"c:ls:a:0", "o:nd:ls:0"}, {"t:d1"}} {
		var row []string
		for _, d := range r {
			if b.allowed(strings.Split(d, ":")) {
				row = append(row, d)
			}
		}
		if len(row) > 0 {
			rows = append(rows, row)
		}
	}
	if len(rows) == 0 {
		rows = [][]string{{"m:menu"}}
	}
	extra := []string{"k:shop"}
	if !b.restricted() || b.isAdmin(b.self) {
		extra = append(extra, "k:edit", "k:ordsP", "k:sales")
	}
	rows = append(rows, extra)
	if b.canManageAdmins() {
		rows = append(rows, []string{"q:home"})
	}
	return rows
}

func (b *bot) replyMarkup(rows [][]string) map[string]any {
	kb := make([][]map[string]string, 0, len(rows))
	for _, r := range rows {
		row := make([]map[string]string, 0, len(r))
		for _, d := range r {
			row = append(row, map[string]string{"text": b.menuLabel(d)})
		}
		kb = append(kb, row)
	}
	return map[string]any{
		"keyboard":                kb,
		"resize_keyboard":         true,
		"is_persistent":           true,
		"input_field_placeholder": b.tr("از منوی پایین انتخاب کنید", "Pick from the menu below"),
	}
}

func (b *bot) sendMenu(ctx context.Context, chatID int64, text string, rows [][]string) {
	b.sendMarkup(ctx, chatID, text, b.replyMarkup(rows))
}

// menuChat carries the chat of a bottom-menu press, where a callback's
// answer goes as a message: there is no button to show it on.
type menuChat struct{}

// menuPress runs the bottom-menu button with the given callback data. It
// reports whether the text was a button for this person.
func (b *bot) menuPress(ctx context.Context, chatID int64, from *fromUser, data string) bool {
	admin := b.isAdmin(from.ID)
	switch {
	case data == "k:shop" || data == "k:admin" || data == "k:edit" || data == "k:ords" || data == "k:ordsP" || data == "k:sales":
		if !admin {
			return false
		}
		b.pend.clear(chatID)
		bb := b.as(from.ID)
		if data == "k:shop" {
			b.sendMenu(ctx, chatID, bb.shopHomeText(), bb.customerMenu(from.ID))
			return true
		}
		if data == "k:edit" {
			if !bb.allowed([]string{"k", "edit"}) {
				return false
			}
			// The bottom menu opens the owner's own shop, not a seller's.
			b.clearDesk(chatID)
			text, kb := bb.shopEditHome()
			bb.sendKeyboard(ctx, chatID, text, kb)
			return true
		}
		if data == "k:ords" || data == "k:ordsP" || data == "k:sales" {
			if !bb.allowed([]string{"k", "edit"}) {
				return false
			}
			var text string
			var kb [][]button
			switch data {
			case "k:ords":
				text, kb = bb.orderQueueScreen(false)
			case "k:ordsP":
				text, kb = bb.orderQueueScreen(true)
			default:
				text, kb = bb.salesReportScreen()
			}
			bb.sendKeyboard(ctx, chatID, text, kb)
			return true
		}
		b.sendAdminMenu(ctx, chatID, "🔐 <b>"+b.tr("پنل مدیریت", "Admin panel")+"</b>", false)
		text, kb := b.menuScreen()
		b.sendKeyboard(ctx, chatID, text, kb)
		return true
	case strings.HasPrefix(data, "p:"):
		b.customerPress(ctx, chatID, from, data)
		return true
	case !admin:
		return false
	}
	ctx = context.WithValue(ctx, menuChat{}, chatID)
	b.handleCallback(ctx, menuUpdate(chatID, from.ID, data))
	return true
}

func menuUpdate(chatID, from int64, data string) update {
	var u update
	raw, _ := json.Marshal(map[string]any{"callback_query": map[string]any{
		"id": "", "data": data, "from": map[string]any{"id": from},
		"message": map[string]any{"message_id": 0, "chat": map[string]any{"id": chatID, "type": "private"}},
	}})
	_ = json.Unmarshal(raw, &u)
	return u
}

// menuState remembers, since the bot started, the administrators who got
// their bottom menu.
type menuState struct {
	admins sync.Map
}

// sendAdminMenu sends an administrator their bottom menu with text; once
// tells to do it only if they did not get it since the bot started.
func (b *bot) sendAdminMenu(ctx context.Context, chatID int64, text string, once bool) bool {
	if b.menus != nil {
		if _, done := b.menus.admins.LoadOrStore(chatID, true); done && once {
			return false
		}
	}
	b.sendMenu(ctx, chatID, text, b.adminMenu())
	return true
}
