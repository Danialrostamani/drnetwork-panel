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

// Customer items open shop screens ("p:"), administrator items the panel's
// sections; "k:" ones switch between the two keyboards.
var menuItems = []menuItem{
	{"🛒 خرید سرویس", "🛒 Buy a service", "p:plans:0"},
	{"📦 سرویس‌های من", "📦 My services", "p:my"},
	{"🔄 تمدید سرویس", "🔄 Renew", "p:renew"},
	{"👛 کیف پول", "👛 Wallet", "p:wallet"},
	{"🎁 تست رایگان", "🎁 Free trial", "p:trial"},
	{"👥 دعوت دوستان", "👥 Invite friends", "p:ref"},
	{"📚 آموزش اتصال", "📚 How to connect", "p:guide"},
	{"💬 پشتیبانی", "💬 Support", "p:support"},
	{"🏠 خانه", "🏠 Home", "h:home"},
	{"📋 منوی اصلی", "📋 Main menu", "m:menu"},
	{"👥 کلاینت‌ها", "👥 Clients", "c:ls:a:0"},
	{"🖥 نودها", "🖥 Nodes", "o:nd:ls:0"},
	{"🛒 مدیریت فروشگاه", "🛒 Shop admin", "q:menu"},
	{"📊 آمار", "📊 Stats", "t:d1"},
	{"🛍 نمای کاربر", "🛍 Customer view", "k:shop"},
	{"🔙 پنل مدیریت", "🔙 Admin panel", "k:admin"},
}

// menuData is the callback a bottom-menu label stands for, in either
// language: a keyboard sent before a language switch keeps working.
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
	for _, it := range menuItems {
		if it.data == data {
			return b.tr(it.fa, it.en)
		}
	}
	return data
}

// customerMenu is the bottom menu of the shop. An administrator in the
// customer view also gets the way back to the panel.
func (b *bot) customerMenu(id int64, admin bool) [][]string {
	if !b.shopOpen(id) {
		rows := [][]string{{"p:my", "p:guide"}}
		if strings.TrimSpace(b.shopSvc().Settings().Support) != "" {
			rows = append(rows, []string{"p:support"})
		}
		return rows
	}
	st := b.shopSvc().Settings()
	rows := [][]string{{"p:plans:0", "p:my"}, {"p:renew", "p:wallet"}}
	var row []string
	if !admin && (st.TrialGB > 0 || st.TrialDays > 0) {
		if u := b.shopSvc().User(id); u == nil || u.TrialAt == 0 {
			row = append(row, "p:trial")
		}
	}
	if !admin && st.RefPercent > 0 {
		row = append(row, "p:ref")
	}
	if len(row) > 0 {
		rows = append(rows, row)
	}
	rows = append(rows, []string{"p:guide", "p:support"})
	if admin {
		rows = append(rows, []string{"k:admin"})
	}
	return rows
}

// adminMenu is the bottom menu of an administrator: the sections their role
// opens.
func (b *bot) adminMenu() [][]string {
	var rows [][]string
	for _, r := range [][]string{{"h:home", "m:menu"}, {"c:ls:a:0", "o:nd:ls:0"}, {"q:menu", "t:d1"}} {
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
	if b.shopSvc().Settings().Enable {
		rows = append(rows, []string{"k:shop"})
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

// sendMenu sends text with a bottom menu.
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
	case data == "k:shop" || data == "k:admin":
		if !admin {
			return false
		}
		b.pend.clear(chatID)
		if data == "k:shop" {
			text, _ := b.shopHome(from.ID)
			b.sendMenu(ctx, chatID, text, b.customerMenu(from.ID, true))
			return true
		}
		b.sendAdminMenu(ctx, chatID, "🔐 <b>"+b.tr("پنل مدیریت", "Admin panel")+"</b>", false)
		text, kb := b.menuScreen()
		b.sendKeyboard(ctx, chatID, text, kb)
		return true
	case strings.HasPrefix(data, "p:"):
		if !b.shopOpen(from.ID) {
			b.closedShopPress(ctx, chatID, from.ID, data)
			return true
		}
	case !admin:
		return false
	}
	ctx = context.WithValue(ctx, menuChat{}, chatID)
	b.handleCallback(ctx, menuUpdate(chatID, from.ID, data))
	return true
}

// closedShopPress answers the few buttons a closed shop keeps: the
// services of the person, the guide and the support contact.
func (b *bot) closedShopPress(ctx context.Context, chatID, from int64, data string) {
	switch data {
	case "p:guide":
		text, kb := b.guideScreen("")
		b.sendKeyboard(ctx, chatID, text, kb)
	case "p:support":
		b.send(ctx, chatID, b.supportText())
	case "p:my":
		if bound := boundClients(from); len(bound) > 0 {
			b.userCommand(ctx, chatID, bound, "usage")
			return
		}
		b.send(ctx, chatID, b.tr("هنوز سرویسی ندارید.", "You have no services yet."))
	default:
		b.send(ctx, chatID, b.tr("فروشگاه فعال نیست.", "The shop is closed."))
	}
}

// menuUpdate is the callback a bottom-menu button stands for, with no message
// to edit: the answer goes in a new one.
func menuUpdate(chatID, from int64, data string) update {
	var u update
	raw, _ := json.Marshal(map[string]any{"callback_query": map[string]any{
		"id": "", "data": data, "from": map[string]any{"id": from},
		"message": map[string]any{"message_id": 0, "chat": map[string]any{"id": chatID, "type": "private"}},
	}})
	_ = json.Unmarshal(raw, &u)
	return u
}

// menuVersion grows with every change of the bottom menu: a customer with an
// older one, or with none because they started the bot before it existed,
// gets it once with their next message.
const menuVersion = 1

// menuState remembers, since the bot started, the customers checked for the
// current bottom menu and the administrators who got theirs.
type menuState struct {
	customers, admins sync.Map
}

// refreshMenu sends a customer the current bottom menu if they do not have
// it yet.
func (b *bot) refreshMenu(ctx context.Context, chatID, from int64) {
	if b.menus != nil {
		if _, done := b.menus.customers.LoadOrStore(from, true); done {
			return
		}
	}
	if !b.shopSvc().TakeMenu(from, menuVersion) {
		return
	}
	b.sendMenu(ctx, chatID, b.tr("📋 منوی ربات حالا پایین صفحه است.", "📋 The bot's menu is now at the bottom of the chat."), b.customerMenu(from, false))
}

// markMenu notes that a customer was just sent the current bottom menu.
func (b *bot) markMenu(from int64) {
	if b.menus != nil {
		b.menus.customers.Store(from, true)
	}
	b.shopSvc().TakeMenu(from, menuVersion)
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

// shopClosedHint tells an administrator who runs the shop that it is closed
// and where to open it: until then a new user only hears that it is closed.
func (b *bot) shopClosedHint() string {
	if !b.can("shop") || b.shopSvc().Settings().Enable {
		return ""
	}
	return "\n\n" + b.tr("🛒 فروشگاه بسته است و کاربران جدید نمی‌توانند خرید کنند. برای شروع فروش: «🛒 مدیریت فروشگاه» ← «🟢 باز کردن فروشگاه» (یا صفحه فروش پنل).",
		"🛒 The shop is closed, so new users cannot buy. To start selling: “🛒 Shop admin” → “🟢 Open the shop” (or the panel's Shop page).")
}
