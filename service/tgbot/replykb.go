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
	if !b.isAdmin(from.ID) {
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
