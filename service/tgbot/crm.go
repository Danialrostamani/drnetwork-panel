package tgbot

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/service"
)

// Customer management in the bot: find a customer, see their wallet, orders,
// services and tickets, and charge the wallet, block them, give a service
// volume or days, or write to them.

func (b *bot) userLabel(u model.ShopUser) string {
	name := strings.TrimSpace(u.Name)
	if u.Username != "" {
		name = strings.TrimSpace(name + " @" + u.Username)
	}
	if name == "" {
		name = strconv.FormatInt(u.TgId, 10)
	}
	return name
}

// usersFound answers a search: one customer's profile, or a list to pick.
func (b *bot) usersFound(query string) (string, [][]button, error) {
	t := b.tr
	users, err := b.shopSvc().FindUsers(normalizeDigits(query), 10)
	if err != nil {
		return "", nil, err
	}
	switch len(users) {
	case 0:
		return "", nil, errors.New(t("کاربری پیدا نشد", "no such user"))
	case 1:
		text, kb := b.userProfile(users[0].TgId)
		return text, kb, nil
	}
	var kb [][]button
	for _, u := range users {
		kb = append(kb, []button{{Text: b.userLabel(u), Data: fmt.Sprintf("q:u:%d", u.TgId)}})
	}
	return "🔎 " + t("چند کاربر پیدا شد؛ یکی را انتخاب کنید:", "Several users match; pick one:"), append(kb, b.navRow("q:menu")), nil
}

func (b *bot) userProfile(tgID int64) (string, [][]button) {
	t := b.tr
	svc := b.shopSvc()
	u := svc.User(tgID)
	if u == nil {
		return t("کاربری پیدا نشد.", "No such user."), [][]button{b.navRow("q:menu")}
	}
	sum := svc.Summary(tgID)
	stamp := func(ts int64) string {
		if ts <= 0 {
			return "—"
		}
		return time.Unix(ts, 0).In(b.loc).Format("2006-01-02 15:04")
	}
	lines := []string{"👤 <b>" + esc(b.userLabel(*u)) + "</b> · <code>" + strconv.FormatInt(tgID, 10) + "</code>"}
	if u.Blocked {
		lines = append(lines, "🚫 "+t("مسدود", "Blocked"))
	}
	lines = append(lines,
		t("عضویت: ", "Joined: ")+stamp(u.JoinedAt)+" · "+t("آخرین فعالیت: ", "last seen: ")+stamp(u.SeenAt),
		"👛 "+t("کیف پول: ", "Wallet: ")+"<b>"+b.money(svc.Balance(tgID))+"</b>",
		fmt.Sprintf(t("🧾 خریدها: %d · جمع: %s", "🧾 Purchases: %d · total: %s"), sum.Orders, b.money(sum.Spent)),
		fmt.Sprintf(t("🎫 تیکت‌ها: %d (%d باز)", "🎫 Tickets: %d (%d open)"), sum.Tickets, sum.Open),
		fmt.Sprintf(t("👥 دعوت‌شده‌ها: %d", "👥 Invited: %d"), svc.Referrals(tgID)))
	if u.ReferredBy != 0 {
		lines = append(lines, fmt.Sprintf(t("دعوت‌شده توسط <code>%d</code>", "Invited by <code>%d</code>"), u.ReferredBy))
	}
	bound := boundClients(tgID)
	if len(bound) > 0 {
		lines = append(lines, "", "📦 <b>"+t("سرویس‌ها", "Services")+"</b>")
		now := time.Now()
		for _, c := range bound {
			used := humanBytes(c.Up + c.Down)
			total := t("نامحدود", "unlimited")
			if c.Volume > 0 {
				total = humanBytes(c.Volume)
			}
			state := "🟢"
			if !c.Enable || (c.Expiry > 0 && c.Expiry <= now.Unix()) || (c.Volume > 0 && c.Up+c.Down >= c.Volume) {
				state = "🔴"
			}
			lines = append(lines, fmt.Sprintf("%s %s · %s / %s · %s", state, esc(c.Name), used, total, b.expiryText(c, now)))
		}
	}
	if orders, _ := svc.Orders("", tgID, 5); len(orders) > 0 {
		lines = append(lines, "", "🧾 <b>"+t("آخرین سفارش‌ها", "Last orders")+"</b>")
		for _, o := range orders {
			lines = append(lines, fmt.Sprintf("#%d · %s · %s · %s", o.Id, esc(b.orderTitle(o)), b.money(o.Paid), b.orderStatus(o.Status)))
		}
	}
	id := strconv.FormatInt(tgID, 10)
	block := button{Text: t("🚫 مسدود کردن", "🚫 Block"), Data: "q:ub:" + id}
	if u.Blocked {
		block = button{Text: t("✅ رفع مسدودی", "✅ Unblock"), Data: "q:ub:" + id}
	}
	kb := [][]button{
		{{Text: t("👛 شارژ / کسر کیف پول", "👛 Charge / take out"), Data: "q:uw:" + id}, block},
		{{Text: t("✉️ پیام به کاربر", "✉️ Message"), Data: "q:um:" + id}},
	}
	if len(bound) > 0 {
		kb[1] = append(kb[1], button{Text: t("🎁 هدیه حجم / روز", "🎁 Give volume / days"), Data: "q:ug:" + id})
	}
	if sum.Tickets > 0 {
		kb = append(kb, []button{{Text: t("🎫 تیکت‌ها", "🎫 Tickets"), Data: "q:ut:" + id}})
	}
	kb = append(kb, []button{{Text: t("🔎 جستجوی دیگر", "🔎 Another search"), Data: "q:usr"}}, b.navRow("q:menu"))
	return strings.Join(lines, "\n"), kb
}

// crmCallback is q:<usr|u|uw|ub|um|ug|ut>[:tg[:client]].
func (b *bot) crmCallback(ctx context.Context, cbID string, chatID, msgID int64, parts []string) {
	t := b.tr
	var tg int64
	if len(parts) > 2 {
		tg, _ = strconv.ParseInt(parts[2], 10, 64)
	}
	show := func(text string, kb [][]button) {
		b.answer(ctx, cbID, "")
		b.edit(ctx, chatID, msgID, text, kb)
	}
	ask := func(kind, key string, id uint, prompt string) {
		b.answer(ctx, cbID, "")
		b.ask(ctx, chatID, msgID, kind, key, id, "q:u:"+parts[len(parts)-1], prompt)
	}
	key := strconv.FormatInt(tg, 10)
	switch parts[1] {
	case "usr":
		b.answer(ctx, cbID, "")
		b.ask(ctx, chatID, msgID, "sh.usr", "", 0, "q:menu", t("شناسه تلگرام، @نام‌کاربری یا بخشی از نام را بفرستید:", "Send a Telegram ID, an @username or a part of the name:"))
	case "u":
		show(b.userProfile(tg))
	case "ub":
		u := b.shopSvc().User(tg)
		if u == nil || b.isAdmin(tg) {
			show(t("کاربری پیدا نشد.", "No such user."), [][]button{b.navRow("q:menu")})
			return
		}
		if err := b.shopSvc().SetBlocked(tg, !u.Blocked); err != nil {
			show(b.t("failed", esc(err.Error())), [][]button{b.navRow("q:u:" + key)})
			return
		}
		show(b.userProfile(tg))
	case "uw":
		ask("sh.uw", key, 0, fmt.Sprintf(t("مبلغ شارژ کیف پول <code>%d</code> را بفرستید (منفی برای کسر)، و اگر خواستید توضیح:\n<code>50000 جبران قطعی</code>", "Send the amount for the wallet of <code>%d</code> (negative takes out), and a note if you like:\n<code>50000 outage</code>"), tg))
	case "um":
		ask("sh.um", key, 0, fmt.Sprintf(t("متن پیام به <code>%d</code> را بفرستید:", "Send the message for <code>%d</code>:"), tg))
	case "ug":
		bound := boundClients(tg)
		var cid uint
		if len(parts) > 3 {
			n, _ := strconv.ParseUint(parts[3], 10, 32)
			cid = uint(n)
		} else if len(bound) == 1 {
			cid = bound[0].Id
		}
		if cid == 0 {
			var kb [][]button
			for _, c := range bound {
				kb = append(kb, []button{{Text: c.Name, Data: fmt.Sprintf("q:ug:%d:%d", tg, c.Id)}})
			}
			show(t("کدام سرویس؟", "Which service?"), append(kb, b.navRow("q:u:"+key)))
			return
		}
		owned := false
		for _, c := range bound {
			owned = owned || c.Id == cid
		}
		if !owned {
			show(t("این سرویس مال این کاربر نیست.", "That service is not this user's."), [][]button{b.navRow("q:u:" + key)})
			return
		}
		b.answer(ctx, cbID, "")
		b.ask(ctx, chatID, msgID, "sh.ug", key, cid, "q:u:"+key, t("حجم (گیگ) و روز هدیه را بفرستید؛ مثلا <code>5 3</code> یا <code>0 7</code>:", "Send the gift's GB and days, e.g. <code>5 3</code> or <code>0 7</code>:"))
	case "ut":
		ts, _ := b.shopSvc().Tickets(tg, false, 10)
		var kb [][]button
		lines := []string{"🎫 <b>" + t("تیکت‌ها", "Tickets") + "</b>"}
		for _, tk := range ts {
			lines = append(lines, fmt.Sprintf("#%d · %s · %s", tk.Id, b.ticketState(tk.Status), esc(tk.Subject)))
			kb = append(kb, []button{{Text: fmt.Sprintf("#%d · %s", tk.Id, tk.Subject), Data: fmt.Sprintf("q:tk:v:%d", tk.Id)}})
		}
		show(strings.Join(lines, "\n"), append(kb, b.navRow("q:u:"+key)))
	default:
		b.answer(ctx, cbID, "")
	}
}

// crmPending handles the typed answers of the customer screens; it reports
// whether the prompt was one of them.
func (b *bot) crmPending(ctx context.Context, p *pending, text string, retry func(error), finish func(string, [][]button)) bool {
	t := b.tr
	svc := b.shopSvc()
	tg, _ := strconv.ParseInt(p.key, 10, 64)
	back := [][]button{b.navRow("q:u:" + p.key)}
	switch p.kind {
	case "sh.usr":
		out, kb, err := b.usersFound(text)
		if err != nil {
			retry(err)
			return true
		}
		finish(out, kb)
	case "sh.uw":
		f := strings.Fields(normalizeDigitsKeepSpaces(text))
		if len(f) == 0 {
			retry(errors.New(b.t("badNumber")))
			return true
		}
		amount, err := strconv.ParseInt(f[0], 10, 64)
		if err != nil || amount == 0 {
			retry(errors.New(b.t("badNumber")))
			return true
		}
		reason := "admin"
		if len(f) > 1 {
			reason = strings.Join(f[1:], " ")
		}
		bal, err := svc.AddFunds(nil, tg, amount, reason, 0, b.self)
		if err != nil {
			retry(err)
			return true
		}
		if amount > 0 {
			b.send(ctx, tg, fmt.Sprintf(t("👛 %s به کیف پول شما اضافه شد.\nموجودی: %s", "👛 %s was added to your wallet.\nBalance: %s"), b.money(amount), b.money(bal)))
		}
		text, kb := b.userProfile(tg)
		finish(b.t("done")+"\n\n"+text, kb)
	case "sh.um":
		if strings.TrimSpace(text) == "" {
			retry(errors.New("empty"))
			return true
		}
		b.sendKeyboard(ctx, tg, "✉️ <b>"+t("پیام از پشتیبانی", "A message from the support")+"</b>\n\n"+esc(text), [][]button{{{Text: t("✍️ پاسخ", "✍️ Reply"), Data: "p:support"}}})
		finish(t("✅ پیام فرستاده شد.", "✅ The message is sent."), back)
	case "sh.ug":
		f := strings.Fields(normalizeDigitsKeepSpaces(text))
		gb, ok1 := 0.0, len(f) == 2
		days, ok2 := 0, ok1
		if ok1 {
			gb, ok1 = parseFloatArg(f[0])
			days, ok2 = parseIntArg(f[1])
		}
		if !ok1 || !ok2 || gb < 0 || days < 0 || (gb == 0 && days == 0) {
			retry(errors.New(b.t("badNumber")))
			return true
		}
		c, err := b.addPack(p.id, int64(gb*float64(gib)), days)
		if err != nil {
			retry(err)
			return true
		}
		b.send(ctx, tg, fmt.Sprintf(t("🎁 هدیه برای سرویس %s: %s و %d روز.", "🎁 A gift for %s: %s and %d days."), esc(c.Name), humanBytes(int64(gb*float64(gib))), days))
		text, kb := b.userProfile(tg)
		finish(b.t("done")+"\n\n"+text, kb)
	default:
		return false
	}
	return true
}

// ---- broadcast audiences ----

func (b *bot) audienceRows() [][]button {
	t := b.tr
	return [][]button{
		{{Text: t("👥 همه", "👥 Everybody"), Data: "q:bca:all"}},
		{{Text: t("🛒 خریداران", "🛒 Buyers"), Data: "q:bca:" + service.AudienceBuyers}, {Text: t("🆕 بدون خرید", "🆕 Never bought"), Data: "q:bca:" + service.AudienceNever}},
		{{Text: t("💤 بی‌خرید در ۳۰ روز اخیر", "💤 No purchase in 30 days"), Data: "q:bca:" + service.AudienceIdle30}},
		{{Text: t("🟢 دارای سرویس فعال", "🟢 With an active service"), Data: "q:bca:" + service.AudienceActive}, {Text: t("🔴 فقط سرویس تمام‌شده", "🔴 Only ended services"), Data: "q:bca:" + service.AudienceExpired}},
		{{Text: t("🖥 کاربران یک نود", "🖥 Users of a node"), Data: "q:bcn"}, {Text: t("🏷 کاربران یک گروه", "🏷 Users of a group"), Data: "q:bcg"}},
		{{Text: t("✖️ انصراف", "✖️ Cancel"), Data: "q:menu"}},
	}
}

func (b *bot) audienceName(aud string) string {
	t := b.tr
	switch {
	case aud == "" || aud == "all":
		return t("همه", "everybody")
	case aud == service.AudienceBuyers:
		return t("خریداران", "buyers")
	case aud == service.AudienceNever:
		return t("بدون خرید", "never bought")
	case aud == service.AudienceIdle30:
		return t("بی‌خرید در ۳۰ روز اخیر", "no purchase in 30 days")
	case aud == service.AudienceActive:
		return t("دارای سرویس فعال", "with an active service")
	case aud == service.AudienceExpired:
		return t("فقط سرویس تمام‌شده", "only ended services")
	case strings.HasPrefix(aud, "node:"):
		for _, n := range b.shopSvc().NodeNames() {
			if "node:"+strconv.FormatUint(uint64(n.Id), 10) == aud {
				return t("نود ", "node ") + n.Name
			}
		}
	case strings.HasPrefix(aud, "group:"):
		return t("گروه ", "group ") + strings.TrimPrefix(aud, "group:")
	}
	return aud
}

// audienceOf turns a button's audience (groups go by their place in the
// list, a name may not fit in the button) into the service's.
func (b *bot) audienceOf(parts []string) string {
	if len(parts) < 3 || parts[2] == "all" {
		return ""
	}
	if parts[2] == "grp" && len(parts) > 3 {
		i, _ := strconv.Atoi(parts[3])
		if g := b.shopSvc().ClientGroups(); i >= 0 && i < len(g) {
			return "group:" + g[i]
		}
		return "group:"
	}
	return strings.Join(parts[2:], ":")
}

// audienceCallback is q:bca:<audience> (count and confirm), q:bcn / q:bcg
// (pick a node or a group) and q:bcy[:<audience>] (send).
func (b *bot) audienceCallback(ctx context.Context, cbID string, chatID, msgID int64, parts []string) {
	t := b.tr
	show := func(text string, kb [][]button) {
		b.answer(ctx, cbID, "")
		b.edit(ctx, chatID, msgID, text, kb)
	}
	back := []button{{Text: t("⬅️ مخاطبان", "⬅️ Audiences"), Data: "q:bcp"}}
	switch parts[1] {
	case "bcp":
		text := peekDraft(chatID)
		if text == "" {
			show(t("پیش‌نویس پیدا نشد؛ دوباره بفرستید.", "The draft is gone; send it again."), [][]button{b.navRow("q:menu")})
			return
		}
		show(t("📣 پیش‌نمایش:", "📣 Preview:")+"\n\n"+esc(text)+"\n\n"+t("به چه کسانی فرستاده شود؟", "Who should get it?"), b.audienceRows())
	case "bcn":
		var kb [][]button
		for _, n := range b.shopSvc().NodeNames() {
			kb = append(kb, []button{{Text: "🖥 " + n.Name, Data: fmt.Sprintf("q:bca:node:%d", n.Id)}})
		}
		show(t("کاربرانِ کدام نود؟", "The users of which node?"), append(kb, back))
	case "bcg":
		var kb [][]button
		for i, g := range b.shopSvc().ClientGroups() {
			kb = append(kb, []button{{Text: "🏷 " + g, Data: fmt.Sprintf("q:bca:grp:%d", i)}})
		}
		show(t("کاربرانِ کدام گروه؟", "The users of which group?"), append(kb, back))
	case "bca":
		aud := b.audienceOf(parts)
		ids, err := b.shopSvc().AudienceTargets(aud, time.Now())
		if err != nil {
			show(b.t("failed", esc(err.Error())), [][]button{back})
			return
		}
		data := "q:bcy:" + strings.Join(parts[2:], ":")
		show(fmt.Sprintf(t("📣 مخاطب: %s · %d نفر", "📣 Audience: %s · %d people"), esc(b.audienceName(aud)), len(ids)),
			[][]button{{{Text: t("✅ ارسال", "✅ Send"), Data: data}}, back})
	case "bcy":
		text := takeDraft(chatID)
		if text == "" {
			show(t("پیش‌نویس پیدا نشد؛ دوباره بفرستید.", "The draft is gone; send it again."), [][]button{b.navRow("q:menu")})
			return
		}
		ids, err := b.shopSvc().AudienceTargets(b.audienceOf(parts), time.Now())
		if err != nil {
			show(b.t("failed", esc(err.Error())), [][]button{b.navRow("q:menu")})
			return
		}
		show(fmt.Sprintf(t("📣 ارسال به %d نفر شروع شد…", "📣 Sending to %d people…"), len(ids)), [][]button{b.navRow("q:menu")})
		go b.broadcastTo(ctx, chatID, ids, text)
	}
}
