package tgbot

import (
	"context"
	"fmt"
	"strings"
)

// The customer bot is what anybody who is not an administrator sees when they
// open the bot: a Telegram command menu and a two-column bottom keyboard.
func (b *bot) customerMenu(id int64) [][]string {
	rows := [][]string{
		{"p:buy", "p:renew"},
		{"p:my", "p:wallet"},
		{"p:trial", "p:tariff"},
		{"p:support", "p:guide"},
	}
	if b.isAdmin(id) {
		rows = append(rows, []string{"k:admin"})
	}
	return rows
}

func (b *bot) customerHome() string {
	return b.tr(
		"👋 خوش آمدید\nاز دکمه‌های پایین استفاده کنید.",
		"👋 Welcome\nUse the buttons below.")
}

func (b *bot) customerStart(ctx context.Context, chatID int64, from *fromUser) {
	id := int64(0)
	if from != nil {
		b.pend.clear(chatID)
		id = from.ID
	}
	b.sendMenu(ctx, chatID, b.customerHome(), b.customerMenu(id))
}

func (b *bot) customerPress(ctx context.Context, chatID int64, from *fromUser, data string) {
	b.pend.clear(chatID)
	switch data {
	case "p:buy":
		b.customerAsk(ctx, chatID, from, "buy", b.tr(
			"🔐 <b>خرید اشتراک</b>\nپلن یا مدت مورد نظرتان را بنویسید (مثلا ۳۰ روز ۵۰ گیگ). پیام شما به پشتیبانی می‌رسد.",
			"🔐 <b>Buy a plan</b>\nWrite the plan or duration you want (for example 30 days 50 GB). Your message goes to support."))
	case "p:renew":
		if b.customerServices(ctx, chatID, from.ID, true) {
			return
		}
		b.customerAsk(ctx, chatID, from, "renew", b.tr(
			"♻️ <b>تمدید سرویس</b>\nسرویسی روی این حساب نیست. نام پلن را بنویسید تا پشتیبانی تمدید را انجام دهد.",
			"♻️ <b>Renew</b>\nNo service is on this account. Write the plan you want and support will renew it."))
	case "p:my":
		b.customerServices(ctx, chatID, from.ID, false)
	case "p:wallet":
		b.customerAsk(ctx, chatID, from, "wallet", b.tr(
			"🏦 <b>کیف پول + شارژ</b>\nمبلغ شارژ را بنویسید. پیام شما به پشتیبانی می‌رسد.",
			"🏦 <b>Wallet</b>\nWrite the amount to add. Your message goes to support."))
	case "p:trial":
		b.notifyAdmins(ctx, b.customerNotice(from, "test", ""))
		b.sendMenu(ctx, chatID, b.tr(
			"🔑 <b>اکانت تست</b>\nدرخواست شما برای پشتیبانی ارسال شد. به‌زودی پاسخ می‌گیرید.",
			"🔑 <b>Test account</b>\nYour request was sent to support. They will reply soon."), b.customerMenu(from.ID))
	case "p:tariff":
		b.sendMenu(ctx, chatID, b.tr(
			"💵 <b>تعرفه اشتراک‌ها</b>\nبرای دیدن قیمت‌ها و خرید، روی <b>خرید اشتراک</b> بزنید یا به پشتیبانی پیام دهید.",
			"💵 <b>Plans</b>\nTo see prices and buy, tap <b>Buy a plan</b> or message support."), b.customerMenu(from.ID))
	case "p:guide":
		b.sendMenu(ctx, chatID, b.customerGuide(), b.customerMenu(from.ID))
	case "p:support":
		b.customerAsk(ctx, chatID, from, "support", b.tr(
			"☎️ <b>پشتیبانی</b>\nپیام خود را بنویسید؛ برای پشتیبانی ارسال می‌شود.",
			"☎️ <b>Support</b>\nWrite your message; it is sent to support."))
	default:
		b.customerStart(ctx, chatID, from)
	}
}

func (b *bot) customerCommand(ctx context.Context, chatID int64, from *fromUser, cmd string) {
	switch cmd {
	case "start", "menu":
		b.customerStart(ctx, chatID, from)
	case "buy":
		b.customerPress(ctx, chatID, from, "p:buy")
	case "services":
		b.customerPress(ctx, chatID, from, "p:my")
	case "wallet":
		b.customerPress(ctx, chatID, from, "p:wallet")
	case "help":
		b.customerPress(ctx, chatID, from, "p:guide")
	case "support":
		b.customerPress(ctx, chatID, from, "p:support")
	case "usage", "sub":
		bound := boundClients(from.ID)
		if len(bound) == 0 {
			b.sendMenu(ctx, chatID, b.t("usageNone"), b.customerMenu(from.ID))
			return
		}
		b.userCommand(ctx, chatID, bound, cmd)
	default:
		b.customerStart(ctx, chatID, from)
	}
}

func (b *bot) customerServices(ctx context.Context, chatID, from int64, renew bool) bool {
	bound := boundClients(from)
	if len(bound) == 0 {
		b.sendMenu(ctx, chatID, b.tr(
			"🛍️ سرویسی روی این حساب نیست. از <b>خرید اشتراک</b> استفاده کنید.",
			"🛍️ No service is on this account. Use <b>Buy a plan</b>."), b.customerMenu(from))
		return false
	}
	if renew {
		b.send(ctx, chatID, b.tr(
			"♻️ برای تمدید، روی پشتیبانی بزنید و نام سرویس را بنویسید.",
			"♻️ To renew, tap Support and write the service name."))
	} else {
		b.send(ctx, chatID, b.tr("🛍️ <b>سرویس‌های من</b>", "🛍️ <b>My services</b>"))
	}
	for _, c := range bound {
		text, kb := b.userView(c)
		b.sendKeyboard(ctx, chatID, text, kb)
	}
	return true
}

func (b *bot) customerAsk(ctx context.Context, chatID int64, from *fromUser, topic, text string) {
	b.pend.set(chatID, &pending{kind: "cu.ask", chatID: chatID, data: map[string]string{"topic": topic}})
	b.sendMenu(ctx, chatID, text, b.customerMenu(from.ID))
}

func (b *bot) customerPending(ctx context.Context, chatID int64, from *fromUser, text string) {
	p := b.pend.get(chatID)
	topic := "support"
	if p != nil && p.data != nil && p.data["topic"] != "" {
		topic = p.data["topic"]
	}
	b.pend.clear(chatID)
	b.notifyAdmins(ctx, b.customerNotice(from, topic, text))
	b.sendMenu(ctx, chatID, b.tr(
		"✅ پیام شما به پشتیبانی رسید.",
		"✅ Your message was sent to support."), b.customerMenu(from.ID))
}

func (b *bot) customerNotice(from *fromUser, topic, text string) string {
	who := fmt.Sprintf("<code>%d</code>", from.ID)
	name := strings.TrimSpace(from.FirstName + " " + from.LastName)
	if name != "" {
		who = esc(name) + " " + who
	}
	if from.Username != "" {
		who += " @" + esc(from.Username)
	}
	title := map[string][2]string{
		"buy":     {"🔐 درخواست خرید", "🔐 Buy request"},
		"renew":   {"♻️ درخواست تمدید", "♻️ Renew request"},
		"wallet":  {"🏦 درخواست شارژ کیف پول", "🏦 Wallet top-up"},
		"test":    {"🔑 درخواست اکانت تست", "🔑 Test account request"},
		"support": {"☎️ پیام پشتیبانی", "☎️ Support message"},
	}[topic]
	if title[0] == "" {
		title = [2]string{"☎️ پیام مشتری", "☎️ Customer message"}
	}
	head := "📬 " + b.tr(title[0], title[1]) + "\n" + who
	if strings.TrimSpace(text) == "" {
		return head
	}
	return head + "\n\n" + esc(text)
}

func (b *bot) notifyAdmins(ctx context.Context, text string) {
	for _, id := range b.access().order {
		if id != 0 {
			b.send(ctx, id, text)
		}
	}
}

func (b *bot) customerGuide() string {
	return b.tr(
		`📚 <b>آموزش اتصال</b>

۱. از <b>سرویس های من</b> لینک اشتراک را کپی کنید.
۲. برنامه را نصب کنید و اشتراک را اضافه کنید:

• اندروید: <a href="https://github.com/2dust/v2rayNG/releases">v2rayNG</a> یا Happ
• آیفون: Streisand یا V2Box
• ویندوز: <a href="https://github.com/2dust/v2rayN/releases">v2rayN</a> یا Happ
• مک / لینوکس: Nekoray یا Happ

۳. در برنامه: افزودن از کلپ‌بورد / subscription، بعد اتصال.

اگر وصل نشدید، به پشتیبانی پیام دهید.`,
		`📚 <b>How to connect</b>

1. Copy the subscription link from <b>My services</b>.
2. Install an app and add the subscription:

• Android: <a href="https://github.com/2dust/v2rayNG/releases">v2rayNG</a> or Happ
• iPhone: Streisand or V2Box
• Windows: <a href="https://github.com/2dust/v2rayN/releases">v2rayN</a> or Happ
• macOS / Linux: Nekoray or Happ

3. In the app: add from clipboard / subscription, then connect.

If it does not connect, message support.`)
}
