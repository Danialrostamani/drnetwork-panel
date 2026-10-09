package tgbot

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/logger"
	"github.com/Danialrostamani/drnetwork-panel/service"
)

// The "shop" section: approving receipts, plans, discount codes, the shop's
// settings, wallets, resellers and messages to every user.

func (b *bot) orderText(o *model.ShopOrder) string {
	t := b.tr
	who := esc(o.TgName)
	if who == "" {
		who = "—"
	}
	lines := []string{
		fmt.Sprintf(t("🧾 <b>سفارش #%d</b> · %s", "🧾 <b>Order #%d</b> · %s"), o.Id, b.orderStatus(o.Status)),
		t("مشتری: ", "Customer: ") + who + fmt.Sprintf(" (<code>%d</code>)", o.TgId),
		t("سفارش: ", "Order: ") + b.orderTitle(*o),
	}
	if o.Kind != model.OrderTopup {
		vol := t("نامحدود", "unlimited")
		if o.Volume > 0 {
			vol = humanBytes(o.Volume)
		}
		lines = append(lines, fmt.Sprintf(t("حجم/زمان: %s · %d روز", "Volume/time: %s · %d days"), vol, o.Days))
	}
	if o.Reseller {
		lines = append(lines, t("👔 خرید نماینده", "👔 Reseller purchase"))
	}
	if o.Auto {
		lines = append(lines, t("🔁 تمدید خودکار از کیف پول", "🔁 Automatic renewal from the wallet"))
	}
	if o.Discount > 0 {
		lines = append(lines, t("تخفیف: ", "Discount: ")+b.money(o.Discount)+map[bool]string{true: " (" + esc(o.Code) + ")", false: ""}[o.Code != ""])
	}
	lines = append(lines, "💰 "+t("مبلغ: ", "Amount: ")+"<b>"+b.money(service.ToPay(o))+"</b>")
	if o.Extra > 0 {
		lines = append(lines, fmt.Sprintf(t("(%s آن برای یکتا شدن مبلغ است و به کیف پول مشتری می‌رود)", "(%s of it makes the amount unique and goes to the customer's wallet)"), b.money(o.Extra)))
	}
	if o.Card != "" && len(b.shopSvc().Settings().Cards) > 1 {
		first, _, _ := strings.Cut(o.Card, "\n")
		lines = append(lines, "💳 "+esc(strings.TrimSpace(first)))
	}
	if r, ok := strings.CutPrefix(o.Receipt, "text:"); ok {
		lines = append(lines, t("رسید: ", "Receipt: ")+"<code>"+esc(r)+"</code>")
	}
	lines = append(lines, time.Unix(o.CreatedAt, 0).In(b.loc).Format("2006-01-02 15:04"))
	return strings.Join(lines, "\n")
}

func (b *bot) decideRow(id uint) []button {
	return []button{
		{Text: b.tr("✅ تایید", "✅ Approve"), Data: fmt.Sprintf("q:ok:%d", id)},
		{Text: b.tr("❌ رد", "❌ Reject"), Data: fmt.Sprintf("q:no:%d", id)},
	}
}

// approvers are the administrators with the shop section.
func (b *bot) approvers() []int64 { return b.access().with("shop") }

// sendReceipt posts an order with its receipt and the approve/reject buttons.
func (b *bot) sendReceipt(ctx context.Context, chatID int64, o *model.ShopOrder) {
	caption := b.orderText(o)
	kb := map[string]any{"inline_keyboard": [][]button{b.decideRow(o.Id)}}
	kind, id, _ := strings.Cut(o.Receipt, ":")
	method, field := "", ""
	switch kind {
	case "photo":
		method, field = "sendPhoto", "photo"
	case "doc":
		method, field = "sendDocument", "document"
	}
	if method != "" {
		err := b.call(ctx, method, map[string]any{"chat_id": chatID, field: id, "caption": caption, "parse_mode": "HTML", "reply_markup": kb}, nil)
		if err == nil {
			return
		}
		logger.Warning("telegram bot: send receipt: ", err)
	}
	b.sendKeyboard(ctx, chatID, caption, [][]button{b.decideRow(o.Id)})
}

func (b *bot) notifyApprovers(ctx context.Context, o *model.ShopOrder) {
	for _, id := range b.approvers() {
		b.sendReceipt(ctx, id, o)
	}
}

// dropButtons takes the keyboard off a message; it works on photos too.
func (b *bot) dropButtons(ctx context.Context, chatID, msgID int64) {
	_ = b.call(ctx, "editMessageReplyMarkup", map[string]any{"chat_id": chatID, "message_id": msgID, "reply_markup": map[string]any{"inline_keyboard": [][]button{}}}, nil)
}

// approve carries out a pending order an administrator accepted.
func (b *bot) approve(ctx context.Context, chatID int64, id uint) string {
	msg, _ := b.approveOrder(ctx, id)
	return msg
}

func (b *bot) approveOrder(ctx context.Context, id uint) (string, error) {
	t := b.tr
	o, err := b.shopSvc().Decide(id, model.OrderApproved, b.self)
	if err != nil {
		if errors.Is(err, service.ErrDecided) {
			if cur, e := b.shopSvc().Order(id); e == nil {
				return fmt.Sprintf(t("سفارش #%d قبلا بررسی شده: %s", "Order #%d was already handled: %s"), id, b.orderStatus(cur.Status)), err
			}
		}
		return b.t("failed", esc(b.errText(err))), err
	}
	if err := b.deliver(ctx, o); err != nil {
		// Nothing was delivered: the order stays open for another try.
		b.shopSvc().Reopen(o.Id)
		return b.t("failed", esc(b.errText(err))) + "\n" + t("سفارش باز ماند؛ دوباره تلاش کنید.", "The order stays open; try again."), err
	}
	return fmt.Sprintf(t("✅ سفارش #%d تایید و تحویل شد.", "✅ Order #%d is approved and delivered."), id), nil
}

func (b *bot) reject(ctx context.Context, id uint) string {
	msg, _ := b.rejectOrder(ctx, id)
	return msg
}

func (b *bot) rejectOrder(ctx context.Context, id uint) (string, error) {
	t := b.tr
	o, err := b.shopSvc().Decide(id, model.OrderRejected, b.self)
	if err != nil {
		if errors.Is(err, service.ErrDecided) {
			return fmt.Sprintf(t("سفارش #%d قبلا بررسی شده.", "Order #%d was already handled."), id), err
		}
		return b.t("failed", esc(b.errText(err))), err
	}
	st := b.shopSvc().Settings()
	msg := fmt.Sprintf(t("❌ رسید سفارش #%d تایید نشد.", "❌ The receipt of order #%d was not accepted."), o.Id)
	if strings.TrimSpace(st.Support) != "" {
		msg += "\n💬 " + t("پشتیبانی: ", "Support: ") + esc(st.Support)
	}
	b.sendKeyboard(ctx, o.TgId, msg, [][]button{b.homeRow()})
	return fmt.Sprintf(t("سفارش #%d رد شد.", "Order #%d is rejected."), id), nil
}

// running is the bot the supervisor runs, for the panel's order decisions.
var running atomic.Pointer[runningBot]

type runningBot struct {
	b   *bot
	ctx context.Context
}

// smsApproved is service.ShopSmsApproved: it tells the approvers about an
// order a bank message approved.
func smsApproved(id uint, amount int64) {
	rb := running.Load()
	if rb == nil {
		return
	}
	b := rb.b.root()
	o, err := b.shopSvc().Order(id)
	if err != nil {
		return
	}
	text := fmt.Sprintf(b.tr("🤖 سفارش #%d با پیامک واریز %s خودکار تایید شد.", "🤖 Order #%d was approved by a bank message of %s."), id, b.money(amount)) + "\n\n" + b.orderText(o)
	for _, chat := range b.approvers() {
		b.send(rb.ctx, chat, text)
	}
}

// decideFromPanel is service.ShopDecider.
func decideFromPanel(id uint, approve bool) (string, error) {
	rb := running.Load()
	if rb == nil {
		return "", service.ErrBotDown
	}
	b := rb.b.root()
	var msg string
	var err error
	if approve {
		msg, err = b.approveOrder(rb.ctx, id)
	} else {
		msg, err = b.rejectOrder(rb.ctx, id)
	}
	if err != nil {
		return "", err
	}
	return plainText(msg), nil
}

// ---- screens ----

func (b *bot) shopAdminScreen() (string, [][]button) {
	t := b.tr
	st, _ := b.shopSvc().Stats(b.loc, 7)
	set := b.shopSvc().Settings()
	state := t("🟢 باز", "🟢 open")
	if !set.Enable {
		state = t("🔴 بسته", "🔴 closed")
	}
	lines := []string{"🛒 <b>" + t("فروشگاه", "Shop") + "</b> · " + state}
	if st != nil {
		lines = append(lines,
			t("فروش امروز: ", "Sales today: ")+b.money(st.RevenueToday),
			t("۷ روز: ", "7 days: ")+b.money(st.Revenue7)+" · "+t("۳۰ روز: ", "30 days: ")+b.money(st.Revenue30),
			fmt.Sprintf(t("سفارش در انتظار: %d · کاربران: %d", "Pending orders: %d · Users: %d"), st.Pending, st.Users))
	}
	return strings.Join(lines, "\n"), [][]button{
		{{Text: t("⏳ سفارش‌های در انتظار", "⏳ Pending orders"), Data: "q:pend"}, {Text: t("📈 گزارش فروش", "📈 Sales report"), Data: "q:rep"}},
		{{Text: t("📦 پلن‌ها", "📦 Plans"), Data: "q:pl"}, {Text: t("🏷 کدهای تخفیف", "🏷 Discount codes"), Data: "q:dc"}},
		{{Text: t("⚙️ تنظیمات فروش", "⚙️ Shop settings"), Data: "q:set"}, {Text: t("👛 کیف پول کاربر", "👛 User wallet"), Data: "q:wl"}},
		{{Text: t("👔 نماینده‌ها", "👔 Resellers"), Data: "q:rs"}, {Text: t("📣 پیام همگانی", "📣 Broadcast"), Data: "q:bc"}},
		{{Text: t("🚫 مسدود کردن کاربر", "🚫 Block a user"), Data: "q:blk"}, {Text: t("🛍 فروشگاه (نمای مشتری)", "🛍 Shop (customer view)"), Data: "p:home"}},
		b.menuRow(),
	}
}

func (b *bot) pendingOrdersScreen() (string, [][]button) {
	t := b.tr
	orders, _ := b.shopSvc().Orders(model.OrderPending, 0, 30)
	var lines []string
	var kb [][]button
	for _, o := range orders {
		if o.Receipt == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("#%d · %s · %s · %s", o.Id, esc(o.TgName), b.orderTitle(o), b.money(o.Paid)))
		kb = append(kb, []button{{Text: fmt.Sprintf(t("🔎 سفارش #%d", "🔎 Order #%d"), o.Id), Data: fmt.Sprintf("q:ord:%d", o.Id)}})
	}
	if len(lines) == 0 {
		return t("سفارشی در انتظار بررسی نیست.", "No orders wait for a check."), [][]button{b.navRow("q:menu")}
	}
	return "⏳ <b>" + t("سفارش‌های در انتظار", "Pending orders") + "</b>\n\n" + strings.Join(lines, "\n"), append(kb, b.navRow("q:menu"))
}

func (b *bot) salesReport() string {
	t := b.tr
	st, err := b.shopSvc().Stats(b.loc, 7)
	if err != nil {
		return b.t("failed", esc(err.Error()))
	}
	lines := []string{"📈 <b>" + t("گزارش فروش", "Sales report") + "</b>",
		t("امروز: ", "Today: ") + b.money(st.RevenueToday),
		t("۷ روز: ", "7 days: ") + b.money(st.Revenue7),
		t("۳۰ روز: ", "30 days: ") + b.money(st.Revenue30) + fmt.Sprintf(t(" (%d سفارش، %d تمدید)", " (%d orders, %d renewals)"), st.Orders30, st.Renewals30),
		t("کل: ", "All time: ") + b.money(st.RevenueAll),
		"",
		fmt.Sprintf(t("👤 کاربران: %d (۳۰ روز اخیر: %d) · تست: %d", "👤 Users: %d (last 30 days: %d) · Trials: %d"), st.Users, st.NewUsers30, st.Trials),
		t("👛 موجودی کیف پول‌ها: ", "👛 Wallets hold: ") + b.money(st.WalletTotal),
		fmt.Sprintf(t("👥 کلاینت‌ها: %d · فعال: %d · آنلاین ۲۴ساعت: %d", "👥 Clients: %d · enabled: %d · online in 24h: %d"), st.Clients, st.ActiveClients, st.Online24h),
		fmt.Sprintf(t("⏰ انقضا تا ۳ روز: %d · منقضی: %d · حجم تمام: %d", "⏰ Expiring in 3 days: %d · expired: %d · out of volume: %d"), st.Expiring3d, st.Expired, st.Depleted),
		"", t("فروش روزانه:", "Daily sales:"),
	}
	for _, d := range st.Days {
		lines = append(lines, fmt.Sprintf("%s · %s · %d", d.Day[5:], b.money(d.Revenue), d.Orders))
	}
	if len(st.TopPlans) > 0 {
		lines = append(lines, "", t("پرفروش‌ترین پلن‌ها (۳۰ روز):", "Best plans (30 days):"))
		for _, p := range st.TopPlans {
			lines = append(lines, fmt.Sprintf("• %s · %d · %s", esc(p.Name), p.Orders, b.money(p.Revenue)))
		}
	}
	if len(st.TopConsumers) > 0 {
		lines = append(lines, "", t("پرمصرف‌ترین کلاینت‌ها:", "Top clients by usage:"))
		for i, c := range st.TopConsumers {
			if i == 5 {
				break
			}
			lines = append(lines, fmt.Sprintf("• %s · %s", esc(c.Name), humanBytes(c.Usage)))
		}
	}
	return strings.Join(lines, "\n")
}

func (b *bot) plansAdminScreen() (string, [][]button) {
	t := b.tr
	plans, _ := b.shopSvc().Plans(false)
	lines := []string{"📦 <b>" + t("پلن‌ها", "Plans") + "</b>"}
	var btns []button
	for _, p := range plans {
		on := "🟢"
		if !p.Enable {
			on = "⚪️"
		}
		lines = append(lines, fmt.Sprintf("%s <b>%s</b> — %s — %s", on, esc(p.Name), b.planLine(p), b.money(p.Price)))
		btns = append(btns, button{Text: on + " " + p.Name, Data: fmt.Sprintf("q:pv:%d", p.Id)})
	}
	if len(plans) == 0 {
		lines = append(lines, t("هنوز پلنی ساخته نشده.", "No plans yet."))
	}
	kb := rows2(btns)
	kb = append(kb, []button{{Text: t("➕ پلن جدید", "➕ New plan"), Data: "q:pn"}}, b.navRow("q:menu"))
	return strings.Join(lines, "\n"), kb
}

func (b *bot) planAdminScreen(id uint) (string, [][]button) {
	t := b.tr
	p, err := b.shopSvc().Plan(id)
	if err != nil {
		return t("پلن پیدا نشد.", "Plan not found."), [][]button{b.navRow("q:pl")}
	}
	state := t("🟢 در حال فروش", "🟢 on sale")
	toggle := t("⏸ توقف فروش", "⏸ Stop selling")
	if !p.Enable {
		state, toggle = t("⚪️ متوقف", "⚪️ stopped"), t("▶️ شروع فروش", "▶️ Start selling")
	}
	group := p.Group
	if group == "" {
		group = "—"
	}
	text := fmt.Sprintf("📦 <b>%s</b> · %s\n%s\n💰 %s\n🏷 %s: %s\n↕️ %s: %d", esc(p.Name), state, b.planLine(*p), b.money(p.Price), t("گروه", "Group"), esc(group), t("ترتیب", "Order"), p.Sort)
	sid := strconv.FormatUint(uint64(p.Id), 10)
	return text, [][]button{
		{{Text: t("✏️ ویرایش", "✏️ Edit"), Data: "q:pe:" + sid}, {Text: toggle, Data: "q:pt:" + sid}},
		{{Text: t("🏷 گروه", "🏷 Group"), Data: "q:pg:" + sid}, {Text: t("↕️ ترتیب", "↕️ Order"), Data: "q:po:" + sid}},
		{{Text: t("🗑 حذف", "🗑 Delete"), Data: "q:pd:" + sid}},
		b.navRow("q:pl"),
	}
}

func (b *bot) discountsScreen() (string, [][]button) {
	t := b.tr
	list, _ := b.shopSvc().Discounts()
	lines := []string{"🏷 <b>" + t("کدهای تخفیف", "Discount codes") + "</b>"}
	var btns []button
	for _, d := range list {
		uses := fmt.Sprintf("%d", d.Used)
		if d.MaxUses > 0 {
			uses += fmt.Sprintf("/%d", d.MaxUses)
		}
		exp := ""
		if d.Expiry > 0 {
			exp = " · " + t("تا ", "until ") + time.Unix(d.Expiry, 0).In(b.loc).Format("2006-01-02")
		}
		value := fmt.Sprintf("%d%%", d.Percent)
		if d.IsGift() {
			value = "🎁 " + b.money(d.Amount)
		}
		var limits []string
		if d.PlanIds != "" {
			limits = append(limits, t("پلن‌های ", "plans ")+esc(d.PlanIds))
		}
		switch d.Kinds {
		case model.OrderBuy:
			limits = append(limits, t("فقط خرید", "purchases only"))
		case model.OrderRenew:
			limits = append(limits, t("فقط تمدید", "renewals only"))
		}
		if d.OncePerUser && !d.IsGift() {
			limits = append(limits, t("یک بار برای هر مشتری", "once per customer"))
		}
		if !d.Enable {
			limits = append(limits, t("خاموش", "off"))
		}
		if len(limits) > 0 {
			exp += " · " + strings.Join(limits, "، ")
		}
		lines = append(lines, fmt.Sprintf("• <code>%s</code> · %s · %s%s", esc(d.Code), value, uses, exp))
		btns = append(btns, button{Text: "🗑 " + d.Code, Data: fmt.Sprintf("q:dd:%d", d.Id)})
	}
	if len(list) == 0 {
		lines = append(lines, t("کدی ساخته نشده.", "No codes yet."))
	}
	kb := rows2(btns)
	kb = append(kb, []button{{Text: t("➕ کد جدید", "➕ New code"), Data: "q:dn"}}, b.navRow("q:menu"))
	return strings.Join(lines, "\n"), kb
}

// shopSettingInfo describes the shop settings on the settings screen.
var shopSettingInfo = []struct{ key, fa, en, hintFa, hintEn string }{
	{"shopCard", "💳 اطلاعات کارت", "💳 Card details", "شماره کارت، نام صاحب کارت و بانک را بفرستید (چند خطی مجاز است).", "Send the card number, holder and bank (several lines are fine)."},
	{"shopCurrency", "💱 واحد پول", "💱 Currency", "واحد پول را بفرستید، مثلا: تومان", "Send the currency, e.g. USD"},
	{"shopTrial", "🎁 تست رایگان", "🎁 Free trial", "حجم (GB) و روز را بفرستید، مثلا <code>1 1</code>. برای خاموش کردن <code>0 0</code>.", "Send GB and days, e.g. <code>1 1</code>. <code>0 0</code> turns it off."},
	{"shopRefPercent", "👥 پاداش دعوت (٪)", "👥 Referral reward (%)", "درصد پاداش دعوت از اولین خرید را بفرستید (0 = خاموش).", "Send the percent of the first purchase a referrer gets (0 = off)."},
	{"shopSupport", "💬 پشتیبانی", "💬 Support", "آیدی یا متن پشتیبانی را بفرستید، مثلا @support", "Send the support contact, e.g. @support"},
	{"shopPrefix", "🔤 پیشوند نام کلاینت", "🔤 Client name prefix", "پیشوند نام کلاینت‌های فروشگاه را بفرستید، مثلا u", "Send the prefix of shop client names, e.g. u"},
	{"shopUniqueAmount", "🔢 مبلغ یکتا", "🔢 Unique amount", "بیشترین مبلغی را بفرستید که به هر سفارش کارت به کارت اضافه می‌شود تا مبلغش با سفارش‌های باز دیگر یکی نباشد (0 = خاموش، تا 9999). این مبلغ پس از تایید به کیف پول مشتری برمی‌گردد و تایید خودکار با پیامک بانک به آن نیاز دارد.",
		"Send the most a card order's amount may grow by so no other open order has the same amount (0 = off, up to 9999). It goes to the customer's wallet once approved; confirming by bank message needs it."},
}

func (b *bot) shopSettingsScreen() (string, [][]button) {
	t := b.tr
	raw := b.shopSvc().RawSettings()
	on, _ := strconv.ParseBool(raw["shopEnable"])
	lines := []string{"⚙️ <b>" + t("تنظیمات فروش", "Shop settings") + "</b>"}
	toggle := t("🟢 باز کردن فروشگاه", "🟢 Open the shop")
	if on {
		toggle = t("🔴 بستن فروشگاه", "🔴 Close the shop")
		lines = append(lines, t("وضعیت: باز — هر کسی می‌تواند ربات را استارت کند و خرید کند.", "State: open — anybody can start the bot and buy."))
	} else {
		lines = append(lines, t("وضعیت: بسته", "State: closed"))
	}
	var btns []button
	for _, s := range shopSettingInfo {
		v := strings.TrimSpace(raw[s.key])
		if v == "" {
			v = "—"
		}
		lines = append(lines, "", "<b>"+t(s.fa, s.en)+"</b>", esc(v))
		btns = append(btns, button{Text: t(s.fa, s.en), Data: "q:sk:" + s.key})
	}
	auto, _ := strconv.ParseBool(raw["shopAutoRenew"])
	autoText := t("🔁 تمدید خودکار از کیف پول: خاموش", "🔁 Auto-renew from the wallet: off")
	if auto {
		autoText = t("🔁 تمدید خودکار از کیف پول: روشن", "🔁 Auto-renew from the wallet: on")
	}
	lines = append(lines, "", "<b>"+autoText+"</b>")
	if strings.TrimSpace(raw["shopSmsSecret"]) != "" {
		lines = append(lines, "", t("🤖 تایید خودکار با پیامک بانک: روشن (از صفحه فروش پنل)", "🤖 Confirming by bank message: on (from the panel's Sales page)"))
	}
	kb := [][]button{{{Text: toggle, Data: "q:sen"}}, {{Text: autoText, Data: "q:sar"}}}
	kb = append(kb, rows2(btns)...)
	return strings.Join(lines, "\n"), append(kb, b.navRow("q:menu"))
}

func (b *bot) resellersScreen() (string, [][]button) {
	t := b.tr
	lines := []string{"👔 <b>" + t("نماینده‌ها", "Resellers") + "</b>",
		t("هر مدیر ربات می‌تواند از «🛍 فروشگاه» برای مشتری‌هایش خرید کند؛ با کیف پول خودش و با درصد تخفیفی که اینجا می‌دهید. کلاینت‌ها در گروه همان مدیر ساخته می‌شوند.",
			"Every bot administrator can buy for their customers from “🛍 Shop”, with their own wallet and the discount set here. The clients go to that administrator's group."), ""}
	for _, id := range b.access().order {
		u := b.shopSvc().User(id)
		pct := 0
		if u != nil {
			pct = u.ResellerPercent
		}
		lines = append(lines, fmt.Sprintf("• <code>%d</code> · %d%% · %s", id, pct, b.money(b.shopSvc().Balance(id))))
	}
	return strings.Join(lines, "\n"), [][]button{{{Text: t("✏️ تعیین تخفیف نماینده", "✏️ Set a reseller discount"), Data: "q:rsp"}}, b.navRow("q:menu")}
}

// ---- callbacks ----

func (b *bot) shopAdminCallback(ctx context.Context, cbID string, chatID, msgID int64, parts []string) {
	t := b.tr
	arg := func(i int) uint {
		if i < len(parts) {
			n, _ := strconv.ParseUint(parts[i], 10, 32)
			return uint(n)
		}
		return 0
	}
	show := func(text string, kb [][]button) {
		b.answer(ctx, cbID, "")
		b.edit(ctx, chatID, msgID, text, kb)
	}
	ask := func(kind, key string, id uint, back, prompt string) {
		b.answer(ctx, cbID, "")
		b.ask(ctx, chatID, msgID, kind, key, id, back, prompt)
	}
	switch parts[1] {
	case "menu":
		show(b.shopAdminScreen())
	case "pend":
		show(b.pendingOrdersScreen())
	case "ord":
		o, err := b.shopSvc().Order(arg(2))
		if err != nil {
			b.answer(ctx, cbID, "")
			return
		}
		b.answer(ctx, cbID, "")
		if o.Status == model.OrderPending {
			b.sendReceipt(ctx, chatID, o)
			return
		}
		b.sendKeyboard(ctx, chatID, b.orderText(o), [][]button{b.navRow("q:pend")})
	case "ok", "no":
		b.answer(ctx, cbID, "")
		b.dropButtons(ctx, chatID, msgID)
		var result string
		if parts[1] == "ok" {
			result = b.approve(ctx, chatID, arg(2))
		} else {
			result = b.reject(ctx, arg(2))
		}
		b.sendKeyboard(ctx, chatID, result, [][]button{{{Text: t("⏳ سفارش‌های در انتظار", "⏳ Pending orders"), Data: "q:pend"}}})
	case "rep":
		show(b.salesReport(), [][]button{b.navRow("q:menu")})
	case "pl":
		show(b.plansAdminScreen())
	case "pv":
		show(b.planAdminScreen(arg(2)))
	case "pn":
		ask("sh.plan", "", 0, "q:pl", t("پلن را در یک خط بفرستید:\n<code>نام | حجم GB | روز | قیمت | تعداد کاربر</code>\nمثال: <code>طلایی | 50 | 30 | 250000 | 2</code>\nحجم 0 = نامحدود، روز 0 = بدون محدودیت زمان، تعداد کاربر اختیاری است.",
			"Send the plan on one line:\n<code>name | GB | days | price | devices</code>\nExample: <code>Gold | 50 | 30 | 25 | 2</code>\nGB 0 = unlimited, days 0 = no time limit; devices is optional."))
	case "pe":
		ask("sh.plan", "", arg(2), fmt.Sprintf("q:pv:%d", arg(2)), t("پلن را دوباره در یک خط بفرستید:\n<code>نام | حجم GB | روز | قیمت | تعداد کاربر</code>", "Send the plan again on one line:\n<code>name | GB | days | price | devices</code>"))
	case "pg":
		ask("sh.pgroup", "", arg(2), fmt.Sprintf("q:pv:%d", arg(2)), t("گروه کلاینت‌های این پلن را بفرستید (- برای بدون گروه).", "Send the client group of this plan (- for none)."))
	case "po":
		ask("sh.psort", "", arg(2), fmt.Sprintf("q:pv:%d", arg(2)), t("عدد ترتیب نمایش را بفرستید (کوچک‌تر بالاتر).", "Send the display order number (smaller comes first)."))
	case "pt":
		if p, err := b.shopSvc().Plan(arg(2)); err == nil {
			p.Enable = !p.Enable
			if err := b.shopSvc().SavePlan(p); err != nil {
				b.answer(ctx, cbID, b.errText(err))
				return
			}
		}
		show(b.planAdminScreen(arg(2)))
	case "pd":
		show(t("این پلن حذف شود؟ سفارش‌های قبلی باقی می‌مانند.", "Delete this plan? Past orders stay."), [][]button{{{Text: t("🗑 حذف", "🗑 Delete"), Data: fmt.Sprintf("q:pdy:%d", arg(2))}, {Text: t("✖️ انصراف", "✖️ Cancel"), Data: fmt.Sprintf("q:pv:%d", arg(2))}}})
	case "pdy":
		_ = b.shopSvc().DeletePlan(arg(2))
		show(b.plansAdminScreen())
	case "dc":
		show(b.discountsScreen())
	case "dn":
		ask("sh.code", "", 0, "q:dc", t("کد را بفرستید.\nکد تخفیف: <code>کد درصد [حداکثر استفاده] [روز اعتبار] [یکبار]</code>\nمثال: <code>NOROOZ 20 100 7 یکبار</code> («یکبار»: هر مشتری یک بار)\n\nکد هدیه (شارژ کیف پول، هر مشتری یک بار): <code>هدیه کد مبلغ [حداکثر استفاده] [روز اعتبار]</code>\nمثال: <code>هدیه GIFT50 50000 100 30</code>\n\nمحدود کردن کد به پلن‌ها یا فقط خرید/تمدید از صفحه فروش پنل.",
			"Send the code.\nDiscount: <code>CODE percent [max uses] [days valid] [once]</code>\nExample: <code>SPRING 20 100 7 once</code> (once: one use per customer)\n\nGift (wallet credit, once per customer): <code>gift CODE amount [max uses] [days valid]</code>\nExample: <code>gift GIFT50 50000 100 30</code>\n\nLimit a code to plans, or to purchases or renewals, on the panel's Sales page."))
	case "dd":
		_ = b.shopSvc().DeleteDiscount(arg(2))
		show(b.discountsScreen())
	case "set":
		show(b.shopSettingsScreen())
	case "sen":
		on := b.shopSvc().Settings().Enable
		if err := b.shopSvc().SetSetting("shopEnable", strconv.FormatBool(!on)); err != nil {
			b.answer(ctx, cbID, b.errText(err))
			return
		}
		show(b.shopSettingsScreen())
	case "sar":
		on := b.shopSvc().Settings().AutoRenew
		if err := b.shopSvc().SetSetting("shopAutoRenew", strconv.FormatBool(!on)); err != nil {
			b.answer(ctx, cbID, b.errText(err))
			return
		}
		show(b.shopSettingsScreen())
	case "sk":
		key := ""
		if len(parts) > 2 {
			key = parts[2]
		}
		for _, s := range shopSettingInfo {
			if s.key == key {
				ask("sh.set", key, 0, "q:set", t(s.hintFa, s.hintEn))
				return
			}
		}
		b.answer(ctx, cbID, "")
	case "wl":
		ask("sh.wallet", "", 0, "q:menu", t("شناسه تلگرام و مبلغ را بفرستید (منفی برای کم کردن):\n<code>شناسه مبلغ [توضیح]</code>\nمثال: <code>123456789 50000</code>\nفقط شناسه را بفرستید تا موجودی را ببینید.", "Send the Telegram ID and the amount (negative takes out):\n<code>ID amount [note]</code>\nExample: <code>123456789 50</code>\nSend only the ID to see the balance."))
	case "rs":
		show(b.resellersScreen())
	case "rsp":
		ask("sh.resel", "", 0, "q:rs", t("شناسه مدیر و درصد تخفیف را بفرستید:\n<code>شناسه درصد</code>", "Send the administrator's ID and the discount:\n<code>ID percent</code>"))
	case "blk":
		ask("sh.block", "", 0, "q:menu", t("شناسه تلگرام را بفرستید تا مسدود شود؛ برای رفع مسدودی <code>-شناسه</code>.", "Send a Telegram ID to block it; <code>-ID</code> unblocks."))
	case "bc":
		ask("sh.bc", "", 0, "q:menu", t("متن پیام همگانی را بفرستید. به همه کاربران فروشگاه و کلاینت‌های متصل به تلگرام ارسال می‌شود.", "Send the broadcast text. It goes to every shop user and every client bound to Telegram."))
	case "bcy":
		text := takeDraft(chatID)
		if text == "" {
			show(t("پیش‌نویس پیدا نشد؛ دوباره بفرستید.", "The draft is gone; send it again."), [][]button{b.navRow("q:menu")})
			return
		}
		ids, err := b.shopSvc().BroadcastTargets()
		if err != nil {
			show(b.t("failed", esc(err.Error())), [][]button{b.navRow("q:menu")})
			return
		}
		show(fmt.Sprintf(t("📣 ارسال به %d نفر شروع شد…", "📣 Sending to %d people…"), len(ids)), [][]button{b.navRow("q:menu")})
		go b.broadcastTo(ctx, chatID, ids, text)
	default:
		b.answer(ctx, cbID, "")
	}
}

// ---- typed answers ----

func (b *bot) shopPending(ctx context.Context, chatID int64, p *pending, text string, retry func(error), finish func(string, [][]button)) {
	t := b.tr
	svc := b.shopSvc()
	switch p.kind {
	case "sh.plan":
		np, err := service.ParsePlanLine(normalizeDigitsKeepSpaces(text))
		if err != nil {
			retry(err)
			return
		}
		if p.id != 0 {
			old, err := svc.Plan(p.id)
			if err != nil {
				retry(err)
				return
			}
			np.Id, np.Group, np.Sort, np.Enable = old.Id, old.Group, old.Sort, old.Enable
		}
		if err := svc.SavePlan(np); err != nil {
			retry(err)
			return
		}
		text, kb := b.planAdminScreen(np.Id)
		finish(b.t("done")+"\n\n"+text, kb)
	case "sh.pgroup":
		pl, err := svc.Plan(p.id)
		if err != nil {
			retry(err)
			return
		}
		g := strings.TrimSpace(text)
		if g == "-" {
			g = ""
		} else if g, err = b.normalizeGroup(g); err != nil {
			retry(err)
			return
		}
		pl.Group = g
		if err := svc.SavePlan(pl); err != nil {
			retry(err)
			return
		}
		text, kb := b.planAdminScreen(pl.Id)
		finish(b.t("done")+"\n\n"+text, kb)
	case "sh.psort":
		n, err := strconv.Atoi(normalizeDigits(text))
		pl, err2 := svc.Plan(p.id)
		if err != nil || err2 != nil {
			retry(errors.New(b.t("badNumber")))
			return
		}
		pl.Sort = n
		if err := svc.SavePlan(pl); err != nil {
			retry(err)
			return
		}
		text, kb := b.planAdminScreen(pl.Id)
		finish(b.t("done")+"\n\n"+text, kb)
	case "sh.code":
		d, err := service.ParseDiscountLine(normalizeDigitsKeepSpaces(text))
		if err == nil {
			err = svc.SaveDiscount(d)
		}
		if err != nil {
			retry(err)
			return
		}
		text, kb := b.discountsScreen()
		finish(b.t("done")+"\n\n"+text, kb)
	case "sh.set":
		v := text
		if p.key == "shopTrial" || p.key == "shopRefPercent" || p.key == "shopUniqueAmount" {
			v = normalizeDigitsKeepSpaces(v)
		}
		if err := svc.SetSetting(p.key, v); err != nil {
			retry(err)
			return
		}
		text, kb := b.shopSettingsScreen()
		finish(b.t("done")+"\n\n"+text, kb)
	case "sh.wallet":
		f := strings.Fields(normalizeDigitsKeepSpaces(text))
		if len(f) == 0 {
			retry(errors.New(b.t("badNumber")))
			return
		}
		id, err := strconv.ParseInt(f[0], 10, 64)
		if err != nil || id <= 0 {
			retry(errors.New(b.t("badNumber")))
			return
		}
		if len(f) == 1 {
			finish(fmt.Sprintf(t("👛 موجودی <code>%d</code>: %s", "👛 Balance of <code>%d</code>: %s"), id, b.money(svc.Balance(id))), [][]button{b.navRow("q:menu")})
			return
		}
		amount, err := strconv.ParseInt(f[1], 10, 64)
		if err != nil || amount == 0 {
			retry(errors.New(b.t("badNumber")))
			return
		}
		reason := "admin"
		if len(f) > 2 {
			reason = strings.Join(f[2:], " ")
		}
		bal, err := svc.AddFunds(nil, id, amount, reason, 0, b.self)
		if err != nil {
			retry(err)
			return
		}
		if amount > 0 {
			b.send(ctx, id, fmt.Sprintf(t("👛 %s به کیف پول شما اضافه شد.\nموجودی: %s", "👛 %s was added to your wallet.\nBalance: %s"), b.money(amount), b.money(bal)))
		}
		finish(fmt.Sprintf(t("✅ موجودی جدید <code>%d</code>: %s", "✅ New balance of <code>%d</code>: %s"), id, b.money(bal)), [][]button{b.navRow("q:menu")})
	case "sh.resel":
		f := strings.Fields(normalizeDigitsKeepSpaces(text))
		if len(f) != 2 {
			retry(errors.New(b.t("badNumber")))
			return
		}
		id, err1 := strconv.ParseInt(f[0], 10, 64)
		pct, err2 := strconv.Atoi(strings.TrimSuffix(f[1], "%"))
		if err1 != nil || err2 != nil || !b.isAdmin(id) {
			retry(errors.New(t("شناسه باید یکی از مدیران ربات باشد", "the ID must be one of the bot's administrators")))
			return
		}
		if err := svc.SetResellerPercent(id, pct); err != nil {
			retry(err)
			return
		}
		text, kb := b.resellersScreen()
		finish(b.t("done")+"\n\n"+text, kb)
	case "sh.block":
		s := normalizeDigits(text)
		unblock := strings.HasPrefix(s, "-")
		id, err := strconv.ParseInt(strings.TrimPrefix(s, "-"), 10, 64)
		if err != nil || id <= 0 || b.isAdmin(id) {
			retry(errors.New(b.t("badNumber")))
			return
		}
		if err := svc.SetBlocked(id, !unblock); err != nil {
			retry(err)
			return
		}
		finish(b.t("done"), [][]button{b.navRow("q:menu")})
	case "sh.bc":
		if strings.TrimSpace(text) == "" {
			retry(errors.New("empty"))
			return
		}
		putDraft(p.chatID, text)
		finish(t("📣 پیش‌نمایش:", "📣 Preview:")+"\n\n"+esc(text), [][]button{{{Text: t("✅ ارسال", "✅ Send"), Data: "q:bcy"}, {Text: t("✖️ انصراف", "✖️ Cancel"), Data: "q:menu"}}})
	default:
		retry(errors.New("unknown prompt"))
	}
}

// normalizeDigitsKeepSpaces turns Persian digits into ASCII ones but keeps
// the spaces and separators that split fields.
func normalizeDigitsKeepSpaces(s string) string {
	var sb strings.Builder
	for _, r := range strings.TrimSpace(s) {
		switch {
		case r >= '۰' && r <= '۹':
			sb.WriteRune('0' + (r - '۰'))
		case r >= '٠' && r <= '٩':
			sb.WriteRune('0' + (r - '٠'))
		default:
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// ---- broadcast ----

var (
	draftMu sync.Mutex
	drafts  = map[int64]string{}
)

func putDraft(chatID int64, text string) {
	draftMu.Lock()
	drafts[chatID] = text
	draftMu.Unlock()
}

func takeDraft(chatID int64) string {
	draftMu.Lock()
	defer draftMu.Unlock()
	t := drafts[chatID]
	delete(drafts, chatID)
	return t
}

// broadcastGap keeps a broadcast under Telegram's limit of about 30 messages
// a second.
var broadcastGap = 50 * time.Millisecond

func (b *bot) broadcastTo(ctx context.Context, adminChat int64, ids []int64, text string) {
	sent, failed := 0, 0
	for _, id := range ids {
		select {
		case <-ctx.Done():
			return
		case <-time.After(broadcastGap):
		}
		err := b.call(ctx, "sendMessage", map[string]any{"chat_id": id, "text": text, "disable_web_page_preview": true}, nil)
		if err != nil {
			failed++
			if strings.Contains(err.Error(), "blocked") || strings.Contains(err.Error(), "deactivated") || strings.Contains(err.Error(), "chat not found") {
				b.shopSvc().MarkGone(id)
			}
			continue
		}
		sent++
	}
	b.send(ctx, adminChat, fmt.Sprintf(b.tr("📣 پیام همگانی تمام شد: %d ارسال، %d ناموفق.", "📣 Broadcast done: %d sent, %d failed."), sent, failed))
}
