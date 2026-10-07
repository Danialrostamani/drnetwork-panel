package tgbot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/logger"
	"github.com/Danialrostamani/drnetwork-panel/service"
)

// The shop: anybody may start the bot, buy a plan, renew one of their
// clients, get a free trial, keep a wallet and invite friends. A bot
// administrator uses the same screens as a reseller: with their discount,
// for clients of their group, and the purchase is not bound to them.
//
// Callback data: p:… are the shop's own screens, open to everybody (each
// handler checks who is asking); q:… are the management screens of the
// "shop" section (shopadmin.go).

var botUsername atomic.Value

func (b *bot) shopSvc() *service.ShopService { return &service.ShopService{} }

// shopSession is what the shop remembers about one chat between messages.
type shopSession struct {
	code    string
	codePct int
	// await is what the next typed message (or photo) is: "code", "topup",
	// "receipt" or "rname".
	await   string
	planID  uint
	client  uint
	orderID uint
	at      time.Time
}

type shopSessions struct {
	mu sync.Mutex
	m  map[int64]*shopSession
}

var sessions = &shopSessions{m: map[int64]*shopSession{}}

const shopSessionTTL = 30 * time.Minute

func (s *shopSessions) get(chatID int64) *shopSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	ss := s.m[chatID]
	if ss == nil || time.Since(ss.at) > shopSessionTTL {
		ss = &shopSession{}
		s.m[chatID] = ss
	}
	ss.at = time.Now()
	// Old sessions go when new ones come.
	if len(s.m) > 5000 {
		for k, v := range s.m {
			if time.Since(v.at) > shopSessionTTL {
				delete(s.m, k)
			}
		}
	}
	return ss
}

// peek returns the session only when it waits for something.
func (s *shopSessions) peek(chatID int64) *shopSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	ss := s.m[chatID]
	if ss == nil || ss.await == "" || time.Since(ss.at) > shopSessionTTL {
		return nil
	}
	return ss
}

func (s *shopSessions) update(chatID int64, fn func(*shopSession)) {
	ss := s.get(chatID)
	s.mu.Lock()
	fn(ss)
	s.mu.Unlock()
}

// ---- formatting ----

func (b *bot) money(n int64) string {
	neg := n < 0
	if neg {
		n = -n
	}
	digits := strconv.FormatInt(n, 10)
	var out []byte
	for i := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, digits[i])
	}
	s := string(out)
	if neg {
		s = "-" + s
	}
	cur := b.shopSvc().Settings().Currency
	if cur == "" {
		cur = b.tr("تومان", "Toman")
	}
	return s + " " + cur
}

// normalizeDigits turns Persian and Arabic digits into ASCII ones and drops
// thousands separators.
func normalizeDigits(s string) string {
	var sb strings.Builder
	for _, r := range strings.TrimSpace(s) {
		switch {
		case r >= '۰' && r <= '۹':
			sb.WriteRune('0' + (r - '۰'))
		case r >= '٠' && r <= '٩':
			sb.WriteRune('0' + (r - '٠'))
		case r == ',' || r == '،' || r == '٬' || r == ' ':
		default:
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

func (b *bot) planLine(p model.ShopPlan) string {
	vol := b.tr("نامحدود", "unlimited")
	if p.Volume > 0 {
		vol = humanBytes(p.Volume)
	}
	days := b.tr("بدون محدودیت زمان", "no time limit")
	if p.Days > 0 {
		days = fmt.Sprintf(b.tr("%d روز", "%d days"), p.Days)
	}
	line := vol + " · " + days
	if p.LimitIp > 0 {
		line += " · " + fmt.Sprintf(b.tr("%d کاربره", "%d devices"), p.LimitIp)
	}
	return line
}

func (b *bot) homeRow() []button {
	return []button{{Text: b.tr("🏠 منوی فروشگاه", "🏠 Shop menu"), Data: "p:home"}}
}

// ---- who is asking ----

// shopper is the person using the shop screens.
type shopper struct {
	id       int64
	reseller bool
	percent  int
	group    string
}

func (b *bot) shopperOf(id int64) shopper {
	sh := shopper{id: id}
	if b.isAdmin(id) {
		sh.reseller = true
		sh.group = b.access().roleOf(id).group
		if u := b.shopSvc().User(id); u != nil {
			sh.percent = u.ResellerPercent
		}
	}
	return sh
}

// ---- entry points ----

// shopOpen tells whether the shop serves this person.
func (b *bot) shopOpen(id int64) bool {
	if !b.shopSvc().Settings().Enable {
		return false
	}
	if u := b.shopSvc().User(id); u != nil && u.Blocked {
		return false
	}
	return true
}

// shopStart answers /start of somebody who is not an administrator.
func (b *bot) shopStart(ctx context.Context, chatID int64, from *fromUser, arg string) {
	var ref int64
	if strings.HasPrefix(arg, "ref") {
		ref, _ = strconv.ParseInt(strings.TrimPrefix(arg, "ref"), 10, 64)
	}
	if _, err := b.shopSvc().Touch(from.ID, strings.TrimSpace(from.FirstName+" "+from.LastName), from.Username, ref); err != nil {
		logger.Warning("telegram bot: shop user: ", err)
	}
	text, kb := b.shopHome(from.ID)
	b.sendKeyboard(ctx, chatID, text, kb)
}

type fromUser struct {
	ID                            int64
	FirstName, LastName, Username string
}

func (b *bot) shopHome(id int64) (string, [][]button) {
	st := b.shopSvc().Settings()
	sh := b.shopperOf(id)
	t := b.tr
	lines := []string{"🛍 <b>" + t("فروشگاه", "Shop") + "</b>"}
	if sh.reseller {
		lines = append(lines, t("حالت نماینده: خرید برای مشتری از کیف پول یا کارت.", "Reseller mode: buy for a customer from your wallet or by card."))
		if sh.percent > 0 {
			lines = append(lines, fmt.Sprintf(t("تخفیف نمایندگی شما: %d%%", "Your reseller discount: %d%%"), sh.percent))
		}
	}
	lines = append(lines, "👛 "+t("موجودی کیف پول: ", "Wallet balance: ")+b.money(b.shopSvc().Balance(id)))
	kb := [][]button{
		{{Text: t("🛒 خرید سرویس", "🛒 Buy"), Data: "p:plans:0"}, {Text: t("🔄 تمدید سرویس", "🔄 Renew"), Data: "p:renew"}},
		{{Text: t("👛 کیف پول", "👛 Wallet"), Data: "p:wallet"}, {Text: t("🧾 سفارش‌های من", "🧾 My orders"), Data: "p:orders"}},
	}
	var row []button
	if !sh.reseller && (st.TrialGB > 0 || st.TrialDays > 0) {
		if u := b.shopSvc().User(id); u == nil || u.TrialAt == 0 {
			row = append(row, button{Text: t("🎁 تست رایگان", "🎁 Free trial"), Data: "p:trial"})
		}
	}
	if !sh.reseller && st.RefPercent > 0 {
		row = append(row, button{Text: t("👥 دعوت دوستان", "👥 Invite friends"), Data: "p:ref"})
	}
	if len(row) > 0 {
		kb = append(kb, row)
	}
	if !sh.reseller && len(boundClients(id)) > 0 {
		kb = append(kb, []button{{Text: t("📊 سرویس‌های من", "📊 My services"), Data: "p:my"}})
	}
	if strings.TrimSpace(st.Support) != "" {
		lines = append(lines, "", "💬 "+t("پشتیبانی: ", "Support: ")+esc(st.Support))
	}
	if sh.reseller {
		kb = append(kb, []button{{Text: t("🏠 منوی مدیریت", "🏠 Admin menu"), Data: "m:menu"}})
	}
	return strings.Join(lines, "\n"), kb
}

// ---- typed answers and receipts ----

// shopMessage handles a message of somebody the shop waits on. It reports
// whether it took the message.
func (b *bot) shopMessage(ctx context.Context, chatID int64, from int64, text, photo, doc string) bool {
	ss := sessions.peek(chatID)
	if ss == nil || !b.shopOpen(from) {
		return false
	}
	t := b.tr
	switch ss.await {
	case "code":
		d, err := b.shopSvc().Discount(text)
		if err != nil {
			b.sendKeyboard(ctx, chatID, t("❌ کد تخفیف معتبر نیست.", "❌ That discount code is not valid."), [][]button{{{Text: t("⬅️ بازگشت", "⬅️ Back"), Data: fmt.Sprintf("p:plan:%d:%d", ss.planID, ss.client)}}})
			sessions.update(chatID, func(s *shopSession) { s.await = "" })
			return true
		}
		sessions.update(chatID, func(s *shopSession) { s.await, s.code, s.codePct = "", d.Code, d.Percent })
		text, kb := b.planScreen(from, ss.planID, ss.client)
		b.sendKeyboard(ctx, chatID, text, kb)
	case "topup":
		n, err := strconv.ParseInt(normalizeDigits(text), 10, 64)
		if err != nil || n <= 0 || n > 1_000_000_000_000 {
			b.send(ctx, chatID, t("❌ مبلغ را فقط با عدد بفرستید.", "❌ Send the amount as a number."))
			return true
		}
		o := &model.ShopOrder{TgId: from, TgName: b.shopName(from), Kind: model.OrderTopup, Amount: n, Paid: n, Method: model.PayCard, Reseller: b.isAdmin(from)}
		if err := b.shopSvc().CreateOrder(o); err != nil {
			b.fail(ctx, chatID, err)
			return true
		}
		b.askReceipt(ctx, chatID, o)
	case "receipt":
		receipt := ""
		switch {
		case photo != "":
			receipt = "photo:" + photo
		case doc != "":
			receipt = "doc:" + doc
		case len([]rune(strings.TrimSpace(text))) >= 4:
			receipt = "text:" + strings.TrimSpace(text)
		default:
			b.send(ctx, chatID, t("عکس رسید یا شماره پیگیری را بفرستید.", "Send a photo of the receipt or the tracking number."))
			return true
		}
		o, err := b.shopSvc().Order(ss.orderID)
		if err != nil || o.TgId != from || o.Status != model.OrderPending {
			sessions.update(chatID, func(s *shopSession) { s.await = "" })
			return false
		}
		if err := b.shopSvc().SetReceipt(o.Id, receipt); err != nil {
			b.fail(ctx, chatID, err)
			return true
		}
		o.Receipt = receipt
		sessions.update(chatID, func(s *shopSession) { s.await = "" })
		b.sendKeyboard(ctx, chatID, fmt.Sprintf(t("✅ رسید سفارش #%d دریافت شد. پس از تایید، نتیجه همین‌جا اعلام می‌شود.", "✅ The receipt of order #%d is in. You will hear here once it is checked."), o.Id), [][]button{b.homeRow()})
		b.notifyApprovers(ctx, o)
	case "rname":
		sh := b.shopperOf(from)
		c := b.as(from).findClientByName(strings.TrimSpace(text))
		if c == nil || !sh.reseller {
			b.send(ctx, chatID, t("❌ کلاینتی با این نام پیدا نشد. دوباره بفرستید.", "❌ No client by that name. Send it again."))
			return true
		}
		sessions.update(chatID, func(s *shopSession) { s.await = "" })
		text, kb := b.plansScreen(from, c.Id)
		b.sendKeyboard(ctx, chatID, text, kb)
	default:
		return false
	}
	return true
}

func (b *bot) shopName(id int64) string {
	if u := b.shopSvc().User(id); u != nil {
		if u.Username != "" {
			return "@" + u.Username
		}
		return u.Name
	}
	return ""
}

func (b *bot) askReceipt(ctx context.Context, chatID int64, o *model.ShopOrder) {
	st := b.shopSvc().Settings()
	sessions.update(chatID, func(s *shopSession) { s.await, s.orderID = "receipt", o.Id })
	card := strings.TrimSpace(st.Card)
	if card == "" {
		card = b.tr("(شماره کارت هنوز تنظیم نشده؛ با پشتیبانی تماس بگیرید)", "(no card number set yet; contact support)")
	}
	text := fmt.Sprintf(b.tr("🧾 سفارش #%d\n💰 مبلغ قابل پرداخت: <b>%s</b>\n\n💳 لطفا مبلغ را به این کارت واریز کنید:\n%s\n\n📸 سپس عکس رسید یا شماره پیگیری را همین‌جا بفرستید.",
		"🧾 Order #%d\n💰 To pay: <b>%s</b>\n\n💳 Please transfer the amount to:\n%s\n\n📸 Then send a photo of the receipt or the tracking number here."), o.Id, b.money(o.Paid), esc(card))
	b.sendKeyboard(ctx, chatID, text, [][]button{{{Text: b.tr("✖️ لغو سفارش", "✖️ Cancel order"), Data: fmt.Sprintf("p:cancel:%d", o.Id)}}})
}

// ---- callbacks ----

func (b *bot) shopCallback(ctx context.Context, cbID string, chatID, msgID, from int64, parts []string) {
	if !b.shopOpen(from) {
		b.answer(ctx, cbID, b.tr("فروشگاه فعال نیست.", "The shop is closed."))
		return
	}
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
	t := b.tr
	sessions.update(chatID, func(s *shopSession) { s.await = "" })
	switch parts[1] {
	case "home":
		show(b.shopHome(from))
	case "plans":
		sessions.update(chatID, func(s *shopSession) { s.code, s.codePct = "", 0 })
		show(b.plansScreen(from, arg(2)))
	case "plan":
		show(b.planScreen(from, arg(2), arg(3)))
	case "renew":
		if b.isAdmin(from) {
			sessions.update(chatID, func(s *shopSession) { s.await = "rname" })
			show(t("✏️ نام کلاینتی را که می‌خواهید تمدید شود بفرستید.", "✏️ Send the name of the client to renew."), [][]button{b.homeRow()})
			return
		}
		bound := boundClients(from)
		if len(bound) == 0 {
			show(t("شما هنوز سرویسی ندارید.", "You have no service yet."), [][]button{{{Text: t("🛒 خرید سرویس", "🛒 Buy"), Data: "p:plans:0"}}, b.homeRow()})
			return
		}
		var btns []button
		for _, c := range bound {
			btns = append(btns, button{Text: "🔄 " + c.Name, Data: fmt.Sprintf("p:plans:%d", c.Id)})
		}
		show(t("کدام سرویس تمدید شود؟", "Which service to renew?"), append(rows2(btns), b.homeRow()))
	case "code":
		sessions.update(chatID, func(s *shopSession) { s.await, s.planID, s.client = "code", arg(2), arg(3) })
		show(t("🏷 کد تخفیف را بفرستید.", "🏷 Send the discount code."), [][]button{{{Text: t("⬅️ بازگشت", "⬅️ Back"), Data: fmt.Sprintf("p:plan:%d:%d", arg(2), arg(3))}}})
	case "card":
		o, err := b.newOrder(chatID, from, arg(2), arg(3), model.PayCard)
		if err != nil {
			b.answer(ctx, cbID, b.errText(err))
			return
		}
		b.answer(ctx, cbID, "")
		b.askReceipt(ctx, chatID, o)
	case "wal":
		o, err := b.quote(chatID, from, arg(2), arg(3))
		if err != nil {
			b.answer(ctx, cbID, b.errText(err))
			return
		}
		bal := b.shopSvc().Balance(from)
		if bal < o.Paid {
			show(fmt.Sprintf(t("❌ موجودی کافی نیست.\nموجودی: %s\nمبلغ: %s", "❌ Not enough balance.\nBalance: %s\nPrice: %s"), b.money(bal), b.money(o.Paid)),
				[][]button{{{Text: t("➕ شارژ کیف پول", "➕ Top up"), Data: "p:topup"}}, {{Text: t("⬅️ بازگشت", "⬅️ Back"), Data: fmt.Sprintf("p:plan:%d:%d", arg(2), arg(3))}}})
			return
		}
		show(fmt.Sprintf(t("پرداخت %s از کیف پول برای «%s»؟", "Pay %s from the wallet for “%s”?"), b.money(o.Paid), esc(o.PlanName)),
			[][]button{{{Text: t("✅ پرداخت", "✅ Pay"), Data: fmt.Sprintf("p:walY:%d:%d", arg(2), arg(3))}, {Text: t("⬅️ بازگشت", "⬅️ Back"), Data: fmt.Sprintf("p:plan:%d:%d", arg(2), arg(3))}}})
	case "walY":
		b.answer(ctx, cbID, "")
		b.payFromWallet(ctx, chatID, msgID, from, arg(2), arg(3))
	case "wallet":
		show(b.walletScreen(from))
	case "topup":
		sessions.update(chatID, func(s *shopSession) { s.await = "topup" })
		show(t("➕ مبلغ شارژ را (به عدد) بفرستید.", "➕ Send the amount to add, as a number."), [][]button{{{Text: t("⬅️ بازگشت", "⬅️ Back"), Data: "p:wallet"}}})
	case "orders":
		show(b.myOrdersScreen(from))
	case "rcpt":
		o, err := b.shopSvc().Order(arg(2))
		if err != nil || o.TgId != from || o.Status != model.OrderPending {
			b.answer(ctx, cbID, t("این سفارش باز نیست.", "That order is not open."))
			return
		}
		b.answer(ctx, cbID, "")
		b.askReceipt(ctx, chatID, o)
	case "cancel":
		o, err := b.shopSvc().Order(arg(2))
		if err != nil || o.TgId != from {
			b.answer(ctx, cbID, "")
			return
		}
		if _, err := b.shopSvc().Decide(o.Id, model.OrderCanceled, from); err != nil {
			b.answer(ctx, cbID, t("این سفارش قبلا بررسی شده.", "That order was already handled."))
			return
		}
		show(fmt.Sprintf(t("سفارش #%d لغو شد.", "Order #%d is canceled."), o.Id), [][]button{b.homeRow()})
	case "trial":
		b.answer(ctx, cbID, "")
		b.giveTrial(ctx, chatID, from)
	case "ref":
		show(b.referralScreen(from))
	case "my":
		b.answer(ctx, cbID, "")
		for _, c := range boundClients(from) {
			text, kb := b.userView(c)
			b.sendKeyboard(ctx, chatID, text, kb)
		}
	default:
		b.answer(ctx, cbID, "")
	}
}

func (b *bot) plansScreen(from int64, clientID uint) (string, [][]button) {
	t := b.tr
	plans, _ := b.shopSvc().Plans(true)
	sh := b.shopperOf(from)
	title := t("🛒 <b>پلن‌ها</b>", "🛒 <b>Plans</b>")
	if clientID != 0 {
		c := b.renewTarget(from, clientID)
		if c == nil {
			return t("این سرویس پیدا نشد.", "That service was not found."), [][]button{b.homeRow()}
		}
		title = fmt.Sprintf(t("🔄 <b>تمدید %s</b>", "🔄 <b>Renew %s</b>"), esc(c.Name))
	}
	if len(plans) == 0 {
		return title + "\n\n" + t("فعلا پلنی برای فروش نیست.", "No plans for sale right now."), [][]button{b.homeRow()}
	}
	lines := []string{title, ""}
	var kb [][]button
	for _, p := range plans {
		_, paid := service.Price(p.Price, sh.percent, 0)
		lines = append(lines, "• <b>"+esc(p.Name)+"</b> — "+b.planLine(p)+" — "+b.money(paid))
		kb = append(kb, []button{{Text: p.Name + " · " + b.money(paid), Data: fmt.Sprintf("p:plan:%d:%d", p.Id, clientID)}})
	}
	return strings.Join(lines, "\n"), append(kb, b.homeRow())
}

// renewTarget is the client a renewal is for, if the person may renew it.
func (b *bot) renewTarget(from int64, id uint) *model.Client {
	if b.isAdmin(from) {
		return b.as(from).clientByID(id)
	}
	for _, c := range boundClients(from) {
		if c.Id == id {
			c := c
			return &c
		}
	}
	return nil
}

// quote builds the order a plan would make, without saving it.
func (b *bot) quote(chatID, from int64, planID, clientID uint) (*model.ShopOrder, error) {
	p, err := b.shopSvc().Plan(planID)
	if err != nil || !p.Enable {
		return nil, errors.New(b.tr("این پلن موجود نیست", "this plan is not available"))
	}
	sh := b.shopperOf(from)
	ss := sessions.get(chatID)
	code, pct := ss.code, ss.codePct
	if code != "" {
		// The code may have run out since it was typed.
		if d, err := b.shopSvc().Discount(code); err == nil {
			pct = d.Percent
		} else {
			code, pct = "", 0
		}
	}
	o := &model.ShopOrder{TgId: from, TgName: b.shopName(from), Kind: model.OrderBuy, PlanId: p.Id, PlanName: p.Name, Volume: p.Volume, Days: p.Days,
		Amount: p.Price, Code: code, Reseller: sh.reseller, Group: p.Group}
	if sh.group != "" {
		o.Group = sh.group
	}
	o.Discount, o.Paid = service.Price(p.Price, sh.percent, pct)
	if clientID != 0 {
		c := b.renewTarget(from, clientID)
		if c == nil {
			return nil, errors.New(b.tr("این سرویس پیدا نشد", "that service was not found"))
		}
		o.Kind, o.ClientId, o.ClientName, o.Group = model.OrderRenew, c.Id, c.Name, c.Group
	}
	return o, nil
}

func (b *bot) newOrder(chatID, from int64, planID, clientID uint, method string) (*model.ShopOrder, error) {
	o, err := b.quote(chatID, from, planID, clientID)
	if err != nil {
		return nil, err
	}
	o.Method = method
	return o, b.shopSvc().CreateOrder(o)
}

func (b *bot) planScreen(from int64, planID, clientID uint) (string, [][]button) {
	t := b.tr
	o, err := b.quote(from, from, planID, clientID)
	if err != nil {
		return "❌ " + esc(err.Error()), [][]button{b.homeRow()}
	}
	p, _ := b.shopSvc().Plan(planID)
	lines := []string{"📦 <b>" + esc(p.Name) + "</b>", b.planLine(*p)}
	if o.Kind == model.OrderRenew {
		lines = append(lines, fmt.Sprintf(t("🔄 تمدید: %s", "🔄 Renews: %s"), esc(o.ClientName)))
	}
	lines = append(lines, "")
	if o.Discount > 0 {
		lines = append(lines, t("قیمت: ", "Price: ")+"<s>"+b.money(o.Amount)+"</s>", t("تخفیف: ", "Discount: ")+b.money(o.Discount))
		if o.Code != "" {
			lines = append(lines, "🏷 "+esc(o.Code))
		}
	}
	lines = append(lines, "💰 "+t("مبلغ قابل پرداخت: ", "To pay: ")+"<b>"+b.money(o.Paid)+"</b>")
	ids := fmt.Sprintf("%d:%d", planID, clientID)
	kb := [][]button{
		{{Text: t("💳 کارت به کارت", "💳 Card transfer"), Data: "p:card:" + ids}, {Text: t("👛 پرداخت از کیف پول", "👛 Pay from wallet"), Data: "p:wal:" + ids}},
		{{Text: t("🏷 کد تخفیف", "🏷 Discount code"), Data: "p:code:" + ids}},
		{{Text: t("⬅️ بازگشت", "⬅️ Back"), Data: fmt.Sprintf("p:plans:%d", clientID)}, {Text: t("🏠 منو", "🏠 Menu"), Data: "p:home"}},
	}
	if o.Paid == 0 {
		kb[0] = []button{{Text: t("✅ دریافت رایگان", "✅ Get it free"), Data: "p:walY:" + ids}}
	}
	return strings.Join(lines, "\n"), kb
}

func (b *bot) payFromWallet(ctx context.Context, chatID, msgID, from int64, planID, clientID uint) {
	o, err := b.newOrder(chatID, from, planID, clientID, model.PayWallet)
	if err != nil {
		b.edit(ctx, chatID, msgID, "❌ "+esc(b.errText(err)), [][]button{b.homeRow()})
		return
	}
	if _, err := b.shopSvc().AddFunds(nil, from, -o.Paid, "order", o.Id, from); err != nil {
		_, _ = b.shopSvc().Decide(o.Id, model.OrderCanceled, from)
		text := "❌ " + esc(b.errText(err))
		if errors.Is(err, service.ErrNoFunds) {
			text = b.tr("❌ موجودی کافی نیست.", "❌ Not enough balance.")
		}
		b.edit(ctx, chatID, msgID, text, [][]button{b.homeRow()})
		return
	}
	if _, err := b.shopSvc().Decide(o.Id, model.OrderApproved, from); err != nil {
		_, _ = b.shopSvc().AddFunds(nil, from, o.Paid, "refund", o.Id, 0)
		b.edit(ctx, chatID, msgID, "❌ "+esc(b.errText(err)), [][]button{b.homeRow()})
		return
	}
	b.edit(ctx, chatID, msgID, fmt.Sprintf(b.tr("⏳ سفارش #%d در حال آماده‌سازی…", "⏳ Preparing order #%d…"), o.Id), nil)
	if err := b.deliver(ctx, o); err != nil {
		// Nothing was made: the money goes back.
		_, _ = b.shopSvc().AddFunds(nil, from, o.Paid, "refund", o.Id, 0)
		database.GetDB().Model(&model.ShopOrder{}).Where("id = ?", o.Id).Update("status", model.OrderCanceled)
		b.sendKeyboard(ctx, chatID, b.t("failed", esc(b.errText(err)))+"\n"+b.tr("مبلغ به کیف پول برگشت.", "The money is back in the wallet."), [][]button{b.homeRow()})
		return
	}
	sessions.update(chatID, func(s *shopSession) { s.code, s.codePct = "", 0 })
}

func (b *bot) walletScreen(from int64) (string, [][]button) {
	t := b.tr
	lines := []string{"👛 <b>" + t("کیف پول", "Wallet") + "</b>", t("موجودی: ", "Balance: ") + "<b>" + b.money(b.shopSvc().Balance(from)) + "</b>"}
	if txs, _ := b.shopSvc().WalletTxs(from, 8); len(txs) > 0 {
		lines = append(lines, "", t("آخرین تراکنش‌ها:", "Last changes:"))
		for _, tx := range txs {
			sign := "➕"
			if tx.Amount < 0 {
				sign = "➖"
			}
			lines = append(lines, fmt.Sprintf("%s %s · %s · %s", sign, b.money(abs64(tx.Amount)), b.txReason(tx.Reason), time.Unix(tx.CreatedAt, 0).In(b.loc).Format("01/02 15:04")))
		}
	}
	return strings.Join(lines, "\n"), [][]button{{{Text: t("➕ شارژ کیف پول", "➕ Top up"), Data: "p:topup"}}, b.homeRow()}
}

func abs64(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

func (b *bot) txReason(r string) string {
	switch r {
	case "topup":
		return b.tr("شارژ", "top-up")
	case "order":
		return b.tr("خرید", "purchase")
	case "refund":
		return b.tr("بازگشت وجه", "refund")
	case "referral":
		return b.tr("پاداش دعوت", "referral reward")
	case "admin":
		return b.tr("تغییر توسط مدیر", "changed by an admin")
	}
	return esc(r)
}

func (b *bot) orderStatus(s string) string {
	switch s {
	case model.OrderPending:
		return b.tr("⏳ در انتظار", "⏳ pending")
	case model.OrderApproved:
		return b.tr("✅ تایید شده", "✅ approved")
	case model.OrderRejected:
		return b.tr("❌ رد شده", "❌ rejected")
	case model.OrderCanceled:
		return b.tr("✖️ لغو شده", "✖️ canceled")
	}
	return s
}

func (b *bot) orderTitle(o model.ShopOrder) string {
	switch o.Kind {
	case model.OrderTopup:
		return b.tr("شارژ کیف پول", "Wallet top-up")
	case model.OrderRenew:
		return fmt.Sprintf(b.tr("تمدید %s با %s", "Renew %s with %s"), esc(o.ClientName), esc(o.PlanName))
	}
	return esc(o.PlanName)
}

func (b *bot) myOrdersScreen(from int64) (string, [][]button) {
	t := b.tr
	orders, _ := b.shopSvc().Orders("", from, 10)
	if len(orders) == 0 {
		return t("هنوز سفارشی ندارید.", "No orders yet."), [][]button{b.homeRow()}
	}
	lines := []string{"🧾 <b>" + t("سفارش‌های من", "My orders") + "</b>", ""}
	var kb [][]button
	for _, o := range orders {
		lines = append(lines, fmt.Sprintf("#%d · %s · %s · %s", o.Id, b.orderTitle(o), b.money(o.Paid), b.orderStatus(o.Status)))
		if o.Status == model.OrderPending && o.Method == model.PayCard {
			label := t("📸 ارسال رسید #%d", "📸 Send receipt #%d")
			if o.Receipt != "" {
				label = t("📸 رسید جدید #%d", "📸 New receipt #%d")
			}
			kb = append(kb, []button{{Text: fmt.Sprintf(label, o.Id), Data: fmt.Sprintf("p:rcpt:%d", o.Id)}, {Text: fmt.Sprintf(t("✖️ لغو #%d", "✖️ Cancel #%d"), o.Id), Data: fmt.Sprintf("p:cancel:%d", o.Id)}})
		}
	}
	return strings.Join(lines, "\n"), append(kb, b.homeRow())
}

func (b *bot) referralScreen(from int64) (string, [][]button) {
	t := b.tr
	st := b.shopSvc().Settings()
	name, _ := botUsername.Load().(string)
	link := "https://t.me/" + name + "?start=ref" + strconv.FormatInt(from, 10)
	lines := []string{
		"👥 <b>" + t("دعوت دوستان", "Invite friends") + "</b>",
		fmt.Sprintf(t("با این لینک دوستانتان را دعوت کنید؛ از اولین خرید هر کدام %d%% به کیف پول شما اضافه می‌شود.", "Invite friends with this link: you get %d%% of each one's first purchase in your wallet."), st.RefPercent),
		"", "<code>" + esc(link) + "</code>", "",
		fmt.Sprintf(t("دعوت‌شده‌ها: %d", "Invited: %d"), b.shopSvc().Referrals(from)),
	}
	return strings.Join(lines, "\n"), [][]button{b.homeRow()}
}

// ---- trial ----

func (b *bot) giveTrial(ctx context.Context, chatID, from int64) {
	st := b.shopSvc().Settings()
	t := b.tr
	if b.isAdmin(from) || (st.TrialGB <= 0 && st.TrialDays <= 0) {
		b.send(ctx, chatID, t("تست رایگان فعال نیست.", "There is no free trial."))
		return
	}
	ok, err := b.shopSvc().ClaimTrial(from)
	if err != nil {
		b.fail(ctx, chatID, err)
		return
	}
	if !ok {
		b.sendKeyboard(ctx, chatID, t("شما قبلا از تست رایگان استفاده کرده‌اید.", "You already had your free trial."), [][]button{b.homeRow()})
		return
	}
	name := b.freeName(st.Prefix + "t" + strconv.FormatInt(from, 10))
	c, err := b.shopCreate(name, "", int64(st.TrialGB*float64(gib)), st.TrialDays, 0, from)
	if err != nil {
		b.shopSvc().ReleaseTrial(from)
		b.fail(ctx, chatID, err)
		return
	}
	b.send(ctx, chatID, t("🎁 اکانت تست شما آماده است:", "🎁 Your trial account is ready:"))
	b.sendSub(ctx, chatID, *c)
}

// ---- delivery ----

// root is the bot with full rights, used to carry out an order: what the
// shop makes was paid for, so it does not count against a volume limit.
func (b *bot) root() *bot {
	rb := *b
	rb.scope, rb.sections, rb.owner, rb.self = "", nil, false, 0
	return &rb
}

// freeName returns name, or name with a number added if a client has it.
func (b *bot) freeName(name string) string {
	taken := map[string]bool{}
	for _, c := range allClients() {
		taken[strings.ToLower(c.Name)] = true
	}
	if !taken[strings.ToLower(name)] {
		return name
	}
	for i := 2; ; i++ {
		n := fmt.Sprintf("%s_%d", name, i)
		if !taken[strings.ToLower(n)] {
			return n
		}
	}
}

// shopCreate makes a client on every inbound and binds it to tgID (0: not
// bound).
func (b *bot) shopCreate(name, group string, volume int64, days, limitIP int, tgID int64) (*model.Client, error) {
	rb := b.root()
	var ids []uint
	if err := database.GetDB().Model(&model.Inbound{}).Order("id").Pluck("id", &ids).Error; err != nil {
		return nil, err
	}
	if ids == nil {
		ids = []uint{}
	}
	inbounds, _ := json.Marshal(ids)
	c := model.Client{Enable: true, Name: name, Config: newClientConfig(name), Inbounds: inbounds, Links: json.RawMessage(`[]`), Volume: volume, LimitIp: limitIP, Group: group}
	if days > 0 {
		c.Expiry = time.Now().Add(time.Duration(days) * 24 * time.Hour).Unix()
	}
	if err := rb.save("new", c); err != nil {
		return nil, err
	}
	made := rb.findClientByName(name)
	if made == nil {
		return nil, errors.New("client not found after saving")
	}
	if tgID != 0 {
		if err := rb.bindClient(made.Id, tgID); err != nil {
			return nil, err
		}
		made.TgId = tgID
	}
	return made, nil
}

// renewClient applies a renewal: a client that ran out starts over with the
// plan; one still running gets the plan's volume and days on top.
func (b *bot) renewClient(id uint, volume int64, days int) (*model.Client, error) {
	rb := b.root()
	err := rb.editClient(id, func(c *model.Client) error {
		now := time.Now()
		over := !c.Enable || (c.Volume > 0 && c.Up+c.Down >= c.Volume) || (c.Expiry > 0 && c.Expiry <= now.Unix())
		switch {
		case volume <= 0:
			c.Volume = 0
		case over || c.Volume <= 0:
			c.Up, c.Down, c.Volume = 0, 0, volume
		default:
			c.Volume += volume
		}
		switch {
		case days <= 0:
			c.Expiry = 0
		default:
			base := now
			if !over && c.Expiry > now.Unix() {
				base = time.Unix(c.Expiry, 0)
			}
			c.DelayStart = false
			c.Expiry = base.Add(time.Duration(days) * 24 * time.Hour).Unix()
		}
		c.Enable = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	return rawFullClient(id)
}

// deliver carries out an approved order and tells the customer.
func (b *bot) deliver(ctx context.Context, o *model.ShopOrder) error {
	t := b.tr
	switch o.Kind {
	case model.OrderTopup:
		bal, err := b.shopSvc().AddFunds(nil, o.TgId, o.Paid, "topup", o.Id, o.DecidedBy)
		if err != nil {
			return err
		}
		b.sendKeyboard(ctx, o.TgId, fmt.Sprintf(t("✅ کیف پول شما %s شارژ شد.\nموجودی: %s", "✅ %s was added to your wallet.\nBalance: %s"), b.money(o.Paid), b.money(bal)), [][]button{b.homeRow()})
		return nil
	case model.OrderBuy:
		bind := o.TgId
		if o.Reseller {
			bind = 0
		}
		c, err := b.shopCreate(b.freeName(b.shopSvc().Settings().Prefix+strconv.FormatUint(uint64(o.Id), 10)), o.Group, o.Volume, o.Days, b.planLimit(o.PlanId), bind)
		if err != nil {
			return err
		}
		o.ClientId, o.ClientName = c.Id, c.Name
		_ = b.shopSvc().Finish(o)
		b.send(ctx, o.TgId, fmt.Sprintf(t("✅ سفارش #%d آماده است.", "✅ Order #%d is ready."), o.Id))
		b.sendSub(ctx, o.TgId, *c)
	case model.OrderRenew:
		c, err := b.renewClient(o.ClientId, o.Volume, o.Days)
		if err != nil {
			return err
		}
		_ = b.shopSvc().Finish(o)
		b.send(ctx, o.TgId, fmt.Sprintf(t("✅ سرویس %s تمدید شد (سفارش #%d).", "✅ %s is renewed (order #%d)."), esc(c.Name), o.Id))
		text, kb := b.userView(*c)
		b.sendKeyboard(ctx, o.TgId, text, kb)
	}
	if who, got := b.shopSvc().ReferralReward(o); got > 0 {
		b.send(ctx, who, fmt.Sprintf(t("🎉 %s پاداش دعوت به کیف پول شما اضافه شد.", "🎉 %s referral reward was added to your wallet."), b.money(got)))
	}
	return nil
}

func (b *bot) planLimit(id uint) int {
	if p, err := b.shopSvc().Plan(id); err == nil {
		return p.LimitIp
	}
	return 0
}
