package tgbot

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/logger"
	"github.com/Danialrostamani/drnetwork-panel/service"
)

// Support tickets in the bot. A customer writes from «💬 Support» - text,
// a voice message, a photo or a file - and every administrator of the shop
// gets it with buttons to answer or close; the answer comes back in the
// customer's chat.

func (b *bot) ticketState(s string) string {
	switch s {
	case model.TicketOpen:
		return b.tr("🟡 در انتظار پاسخ", "🟡 waiting")
	case model.TicketAnswered:
		return b.tr("🟢 پاسخ داده شد", "🟢 answered")
	}
	return b.tr("⚪️ بسته", "⚪️ closed")
}

func (b *bot) ticketKind(kind string) string {
	switch kind {
	case "photo":
		return b.tr("🖼 عکس", "🖼 photo")
	case "voice":
		return b.tr("🎤 پیام صوتی", "🎤 voice message")
	case "doc":
		return b.tr("📎 فایل", "📎 file")
	}
	return ""
}

// supportScreen invites the customer to write; the shop then takes their
// next messages as the ticket's.
func (b *bot) supportScreen(from int64) (string, [][]button) {
	t := b.tr
	lines := []string{"💬 <b>" + t("پشتیبانی", "Support") + "</b>",
		t("پیامتان را همین‌جا بفرستید: متن، پیام صوتی، عکس (مثلاً اسکرین‌شات) یا فایل. پاسخ پشتیبانی در همین گفتگو می‌رسد.",
			"Send your message here: text, a voice message, a photo (a screenshot, say) or a file. The support answers in this chat.")}
	if s := strings.TrimSpace(b.shopSvc().Settings().Support); s != "" {
		lines = append(lines, "", t("راه دیگر: ", "Or reach: ")+esc(s))
	}
	kb := [][]button{}
	if ts, _ := b.shopSvc().Tickets(from, false, 1); len(ts) > 0 {
		kb = append(kb, []button{{Text: t("📂 تیکت‌های من", "📂 My tickets"), Data: "p:tk:ls"}})
	}
	return strings.Join(lines, "\n"), append(kb, b.homeRow())
}

func (b *bot) myTicketsScreen(from int64) (string, [][]button) {
	t := b.tr
	ts, _ := b.shopSvc().Tickets(from, false, 10)
	lines := []string{"📂 <b>" + t("تیکت‌های من", "My tickets") + "</b>"}
	var kb [][]button
	for _, tk := range ts {
		lines = append(lines, fmt.Sprintf("#%d · %s · %s", tk.Id, b.ticketState(tk.Status), esc(tk.Subject)))
		kb = append(kb, []button{{Text: fmt.Sprintf("#%d · %s", tk.Id, tk.Subject), Data: fmt.Sprintf("p:tk:v:%d", tk.Id)}})
	}
	if len(ts) == 0 {
		lines = append(lines, t("هنوز تیکتی ندارید.", "No tickets yet."))
	}
	kb = append(kb, []button{{Text: t("✍️ پیام جدید", "✍️ New message"), Data: "p:support"}}, b.homeRow())
	return strings.Join(lines, "\n"), kb
}

// ticketLog is the last messages of a ticket as text.
func (b *bot) ticketLog(tk *model.ShopTicket, customer string) string {
	t := b.tr
	lines := []string{fmt.Sprintf("🎫 <b>%s #%d</b> · %s", t("تیکت", "Ticket"), tk.Id, b.ticketState(tk.Status))}
	msgs, _ := b.shopSvc().TicketMsgs(tk.Id, 12)
	for _, m := range msgs {
		who := "👤 " + customer
		if m.Answer {
			who = "🎧 " + t("پشتیبانی", "Support")
		}
		body := esc(m.Text)
		if k := b.ticketKind(m.Kind); k != "" {
			body = strings.TrimSpace("[" + k + "] " + body)
		}
		lines = append(lines, "", "<b>"+who+"</b> · "+time.Unix(m.CreatedAt, 0).In(b.loc).Format("01/02 15:04"), body)
	}
	return strings.Join(lines, "\n")
}

func (b *bot) myTicketScreen(from int64, id uint) (string, [][]button) {
	t := b.tr
	tk, err := b.shopSvc().Ticket(id)
	if err != nil || tk.TgId != from {
		return t("تیکت پیدا نشد.", "No such ticket."), [][]button{b.homeRow()}
	}
	kb := [][]button{{{Text: t("✍️ نوشتن پیام", "✍️ Write"), Data: "p:support"}}}
	if tk.Status != model.TicketClosed {
		kb[0] = append(kb[0], button{Text: t("✔️ بستن تیکت", "✔️ Close it"), Data: fmt.Sprintf("p:tk:x:%d", tk.Id)})
	}
	kb = append(kb, []button{{Text: t("📂 تیکت‌های من", "📂 My tickets"), Data: "p:tk:ls"}}, b.homeRow())
	return b.ticketLog(tk, t("شما", "You")), kb
}

// ticketCallback is a customer's ticket button: p:tk:<ls|v|x>[:id].
func (b *bot) ticketCallback(ctx context.Context, chatID, from int64, parts []string, show func(string, [][]button)) {
	t := b.tr
	var id uint
	if len(parts) > 3 {
		n, _ := strconv.ParseUint(parts[3], 10, 32)
		id = uint(n)
	}
	switch {
	case len(parts) > 2 && parts[2] == "v":
		show(b.myTicketScreen(from, id))
	case len(parts) > 2 && parts[2] == "x":
		if _, err := b.shopSvc().TicketClose(id, from); err != nil {
			show(t("تیکت پیدا نشد.", "No such ticket."), [][]button{b.homeRow()})
			return
		}
		show(fmt.Sprintf(t("✔️ تیکت #%d بسته شد. پیام بعدی شما تیکت تازه‌ای باز می‌کند.", "✔️ Ticket #%d is closed. Your next message opens a new one."), id),
			[][]button{{{Text: t("📂 تیکت‌های من", "📂 My tickets"), Data: "p:tk:ls"}}, b.homeRow()})
	default:
		show(b.myTicketsScreen(from))
	}
}

// ticketMessage files what the customer sent while the support screen waits.
func (b *bot) ticketMessage(ctx context.Context, chatID, from int64, in shopInput) {
	t := b.tr
	kind, file := "text", ""
	switch {
	case in.photo != "":
		kind, file = "photo", in.photo
	case in.voice != "":
		kind, file = "voice", in.voice
	case in.doc != "":
		kind, file = "doc", in.doc
	}
	tk, created, err := b.shopSvc().TicketWrite(from, kind, in.text, file)
	switch {
	case errors.Is(err, service.ErrTicketEmpty):
		b.send(ctx, chatID, t("فقط متن، پیام صوتی، عکس یا فایل فرستاده می‌شود.", "Only a text, a voice message, a photo or a file can be sent."))
		return
	case errors.Is(err, service.ErrTicketFlood):
		b.send(ctx, chatID, t("⏳ پیام‌های قبلی شما هنوز در انتظار پاسخ است؛ کمی صبر کنید.", "⏳ Your earlier messages are still waiting for an answer; please wait a little."))
		return
	case err != nil:
		b.fail(ctx, chatID, err)
		return
	}
	note := t("✅ پیام شما به تیکت #%d اضافه شد.", "✅ Added to ticket #%d.")
	if created {
		note = t("✅ تیکت #%d ثبت شد. پاسخ پشتیبانی همین‌جا می‌رسد؛ اگر چیزی مانده، بفرستید.", "✅ Ticket #%d is open. The answer comes here; send anything else you want to add.")
	}
	b.sendKeyboard(ctx, chatID, fmt.Sprintf(note, tk.Id), [][]button{{{Text: t("📂 تیکت‌های من", "📂 My tickets"), Data: "p:tk:ls"}}, b.homeRow()})
	b.notifySupport(ctx, tk, created, kind, in.text, file)
}

// notifySupport hands a customer's message to every administrator of the
// shop, with the buttons to answer.
func (b *bot) notifySupport(ctx context.Context, tk *model.ShopTicket, created bool, kind, text, file string) {
	t := b.tr
	head := "➕ " + t("پیام تازه در تیکت", "New message on ticket")
	if created {
		head = "🎫 " + t("تیکت تازه", "New ticket")
	}
	who := b.shopName(tk.TgId)
	if who == "" {
		who = t("کاربر", "User")
	}
	caption := fmt.Sprintf("<b>%s #%d</b>\n👤 %s · <code>%d</code>", head, tk.Id, esc(who), tk.TgId)
	if text != "" {
		caption += "\n\n" + esc(text)
	}
	row := []button{
		{Text: t("↩️ پاسخ", "↩️ Answer"), Data: fmt.Sprintf("q:tk:r:%d", tk.Id)},
		{Text: t("✔️ بستن", "✔️ Close"), Data: fmt.Sprintf("q:tk:x:%d", tk.Id)},
		{Text: t("📜 گفتگو", "📜 Thread"), Data: fmt.Sprintf("q:tk:v:%d", tk.Id)},
	}
	method, field := "", ""
	switch kind {
	case "photo":
		method, field = "sendPhoto", "photo"
	case "voice":
		method, field = "sendVoice", "voice"
	case "doc":
		method, field = "sendDocument", "document"
	}
	for _, id := range b.approvers() {
		// Each administrator's buttons are filtered by their own rights.
		ab := b.as(id)
		if method != "" {
			err := ab.call(ctx, method, map[string]any{"chat_id": id, field: file, "caption": caption, "parse_mode": "HTML",
				"reply_markup": map[string]any{"inline_keyboard": [][]button{row}}}, nil)
			if err == nil {
				continue
			}
			logger.Warning("telegram bot: forward a ticket: ", err)
		}
		ab.sendKeyboard(ctx, id, caption, [][]button{row})
	}
}

// ---- the support's side ----

func (b *bot) ticketsAdminScreen() (string, [][]button) {
	t := b.tr
	ts, _ := b.shopSvc().Tickets(0, true, 20)
	lines := []string{"🎫 <b>" + t("تیکت‌های باز", "Open tickets") + "</b>"}
	var kb [][]button
	for _, tk := range ts {
		who := b.shopName(tk.TgId)
		if who == "" {
			who = strconv.FormatInt(tk.TgId, 10)
		}
		lines = append(lines, fmt.Sprintf("#%d · %s · %s · %s", tk.Id, b.ticketState(tk.Status), esc(who), esc(tk.Subject)))
		kb = append(kb, []button{{Text: fmt.Sprintf("#%d · %s", tk.Id, who), Data: fmt.Sprintf("q:tk:v:%d", tk.Id)}})
	}
	if len(ts) == 0 {
		lines = append(lines, t("تیکت بازی نیست.", "No open tickets."))
	}
	return strings.Join(lines, "\n"), append(kb, b.navRow("q:menu"))
}

func (b *bot) ticketAdminScreen(id uint) (string, [][]button) {
	t := b.tr
	tk, err := b.shopSvc().Ticket(id)
	if err != nil {
		return t("تیکت پیدا نشد.", "No such ticket."), [][]button{b.navRow("q:tk:ls")}
	}
	who := b.shopName(tk.TgId)
	if who == "" {
		who = strconv.FormatInt(tk.TgId, 10)
	}
	text := b.ticketLog(tk, who) + fmt.Sprintf("\n\n👤 <code>%d</code>", tk.TgId)
	var row []button
	if tk.Status != model.TicketClosed {
		row = append(row, button{Text: t("↩️ پاسخ", "↩️ Answer"), Data: fmt.Sprintf("q:tk:r:%d", tk.Id)},
			button{Text: t("✔️ بستن", "✔️ Close"), Data: fmt.Sprintf("q:tk:x:%d", tk.Id)})
	}
	kb := [][]button{}
	if row != nil {
		kb = append(kb, row)
	}
	return text, append(kb, b.navRow("q:tk:ls"))
}

// ticketAdminCallback is q:tk:<ls|v|r|x>[:id].
func (b *bot) ticketAdminCallback(ctx context.Context, cbID string, chatID, msgID int64, parts []string) {
	t := b.tr
	var id uint
	if len(parts) > 3 {
		n, _ := strconv.ParseUint(parts[3], 10, 32)
		id = uint(n)
	}
	op := ""
	if len(parts) > 2 {
		op = parts[2]
	}
	b.answer(ctx, cbID, "")
	switch op {
	case "v":
		text, kb := b.ticketAdminScreen(id)
		b.sendKeyboard(ctx, chatID, text, kb)
	case "r":
		tk, err := b.shopSvc().Ticket(id)
		if err != nil || tk.Status == model.TicketClosed {
			b.send(ctx, chatID, t("این تیکت بسته است.", "That ticket is closed."))
			return
		}
		// The ticket may have come as a photo: the prompt is a message of
		// its own rather than an edit.
		b.ask(ctx, chatID, 0, "sh.tkans", "", id, fmt.Sprintf("q:tk:v:%d", id),
			fmt.Sprintf(t("پاسخ تیکت #%d را بنویسید:", "Write the answer to ticket #%d:"), id))
	case "x":
		tk, err := b.shopSvc().TicketClose(id, 0)
		if err != nil {
			b.send(ctx, chatID, b.t("failed", esc(err.Error())))
			return
		}
		b.dropButtons(ctx, chatID, msgID)
		b.send(ctx, chatID, fmt.Sprintf(t("✔️ تیکت #%d بسته شد.", "✔️ Ticket #%d is closed."), id))
		b.send(ctx, tk.TgId, fmt.Sprintf(t("✔️ تیکت #%d توسط پشتیبانی بسته شد. برای کار تازه از «💬 پشتیبانی» پیام بدهید.", "✔️ The support closed ticket #%d. Write from «💬 Support» for anything new."), id))
	default:
		text, kb := b.ticketsAdminScreen()
		b.edit(ctx, chatID, msgID, text, kb)
	}
}

// ticketAnswer sends the support's answer to the customer.
func (b *bot) ticketAnswer(ctx context.Context, adminChat int64, id uint, text string) (*model.ShopTicket, error) {
	tk, err := b.shopSvc().TicketAnswer(id, adminChat, text)
	if err != nil {
		return nil, err
	}
	t := b.tr
	b.sendKeyboard(ctx, tk.TgId, fmt.Sprintf("🎧 <b>%s</b> · %s #%d\n\n%s", t("پاسخ پشتیبانی", "Support's answer"), t("تیکت", "ticket"), tk.Id, esc(text)),
		[][]button{{{Text: t("✍️ پاسخ دادن", "✍️ Reply"), Data: "p:support"}, {Text: t("✔️ بستن تیکت", "✔️ Close it"), Data: fmt.Sprintf("p:tk:x:%d", tk.Id)}}})
	return tk, nil
}
