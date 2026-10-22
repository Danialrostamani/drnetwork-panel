package tgbot

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/service"
)

// The owner panel ("q:" buttons): customers, sellers, money, broadcasts,
// alerts and the panel update. Only the owner reaches it, or a full
// administrator while no owner is set (secBoss).

func (b *bot) pausedText() string {
	return b.tr("⏸ دسترسی شما به ربات توسط مالک معلق شده است.", "⏸ The owner suspended your access to the bot.")
}

func (b *bot) blockedText() string {
	return b.tr("⛔ دسترسی شما به این ربات مسدود شده است.", "⛔ Your access to this bot is blocked.")
}

// customerGate records a customer's visit and stops a blocked one. It
// reports whether the message was dealt with.
func (b *bot) customerGate(ctx context.Context, chatID int64, f *fromUser) bool {
	if b.touchCustomer(f) && prefOn(prefNewCust) {
		b.notifyIDs(ctx, b.ownerIDs(), "🆕 "+b.tr("مشتری جدید: ", "New customer: ")+b.customerLabelFrom(f),
			[][]button{{{Text: b.tr("👤 پروفایل", "👤 Profile"), Data: fmt.Sprintf("q:cv:%d", f.ID)}}})
	}
	if customerBlocked(f.ID) {
		b.send(ctx, chatID, b.blockedText())
		return true
	}
	return false
}

// ownerIDs is who gets the owner's alerts: the owner, or the full
// administrators while no owner is set.
func (b *bot) ownerIDs() []int64 { return b.staffIDs() }

func (b *bot) customerLabelFrom(f *fromUser) string {
	c := model.BotCustomer{TgId: f.ID, Name: displayName(f), Username: f.Username}
	return b.customerLabel(c)
}

func (b *bot) customerLabel(c model.BotCustomer) string {
	parts := []string{}
	if c.Name != "" {
		parts = append(parts, "<b>"+esc(c.Name)+"</b>")
	}
	if c.Username != "" {
		parts = append(parts, "@"+esc(c.Username))
	}
	parts = append(parts, fmt.Sprintf("<code>%d</code>", c.TgId))
	return strings.Join(parts, " ")
}

func customerButtonText(c model.BotCustomer) string {
	label := c.Name
	if label == "" && c.Username != "" {
		label = "@" + c.Username
	}
	if label == "" {
		label = strconv.FormatInt(c.TgId, 10)
	}
	label = truncate(label, 22)
	if c.Blocked {
		label = "⛔ " + label
	}
	return label
}

func (b *bot) sellerName(ctx context.Context, id int64) string {
	if id == 0 {
		return b.tr("بدون فروشنده", "no seller")
	}
	name := b.adminNames(ctx, []int64{id})[id]
	if b.access().isOwner(id) {
		name = strings.TrimSpace(b.tr("مالک ", "Owner ") + name)
	}
	if name == "" {
		return strconv.FormatInt(id, 10)
	}
	return name
}

// sellers are the bot's administrators other than the owner.
func (b *bot) sellers() []int64 {
	a := b.access()
	var out []int64
	for _, id := range a.order {
		if !a.isOwner(id) {
			out = append(out, id)
		}
	}
	return out
}

func (b *bot) sellerMinPrice(adminID int64, gb int) int64 {
	if adminID == 0 || gb <= 0 || b.access().isOwner(adminID) {
		return 0
	}
	if m := adminMeta(adminID).MinPerGB; m > 0 {
		return m * int64(gb)
	}
	return 0
}

func (b *bot) sellerAndOwner(adminID int64) []int64 {
	ids := []int64{}
	if adminID != 0 {
		ids = append(ids, adminID)
	}
	for _, id := range b.ownerIDs() {
		if id != adminID {
			ids = append(ids, id)
		}
	}
	return ids
}

// ownerApproves tells a seller whose sales only the owner may approve.
func (b *bot) ownerApproves(adminID int64) bool {
	a := b.access()
	return a.owner != 0 && adminID != 0 && !a.isOwner(adminID) && adminMeta(adminID).RequireOwner
}

func (b *bot) tellOwnerApproval(ctx context.Context, o *model.BotOrder) {
	a := b.access()
	if b.owner || b.self <= 0 || a.owner == 0 || !prefOn(prefApprovals) {
		return
	}
	lines := []string{
		"✅ " + b.tr("ادمین ", "Admin ") + "<b>" + esc(b.sellerName(ctx, b.self)) + "</b> " + b.tr("سفارش را تأیید کرد", "approved an order") + fmt.Sprintf(" #%d", o.Id),
		esc(b.orderTopicLabel(o.Topic) + " · " + o.Detail),
	}
	if o.Amount > 0 {
		lines = append(lines, b.tr("مبلغ: ", "Amount: ")+formatMoney(o.Amount))
	}
	lines = append(lines, b.tr("مشتری: ", "Customer: ")+fmt.Sprintf("<code>%d</code>", o.TgId))
	b.notifyIDs(ctx, []int64{a.owner}, strings.Join(lines, "\n"), [][]button{{
		{Text: b.tr("👤 مشتری", "👤 Customer"), Data: fmt.Sprintf("q:cv:%d", o.TgId)},
		{Text: b.tr("👮 فروشنده", "👮 Seller"), Data: fmt.Sprintf("q:ad:%d", b.self)},
	}})
}

// adminActNote describes, before it is saved, a client write of a seller
// that the owner wants to hear of: deleting clients or giving volume. It is
// "" for anything else.
func (b *bot) adminActNote(act string, payload interface{}) string {
	a := b.access()
	if b.owner || b.self <= 0 || a.owner == 0 || !a.isMember(b.self) {
		return ""
	}
	if act == "new" || act == "addbulk" {
		if !prefOn(prefAdminAdds) {
			return ""
		}
		return b.createdNote(act, payload)
	}
	if !prefOn(prefAdminActs) {
		return ""
	}
	nameOf := func(id uint) string {
		var c model.Client
		if database.GetDB().Select("id", "name").First(&c, id).Error == nil {
			return c.Name
		}
		return fmt.Sprintf("#%d", id)
	}
	switch act {
	case "del":
		if id, ok := payload.(uint); ok {
			return "🗑 " + b.tr("حذف کلاینت: ", "Deleted client: ") + "<b>" + esc(nameOf(id)) + "</b>"
		}
	case "delbulk":
		if ids, ok := payload.([]uint); ok && len(ids) > 0 {
			var names []string
			for i, id := range ids {
				if i == 5 {
					names = append(names, "…")
					break
				}
				names = append(names, esc(nameOf(id)))
			}
			return "🗑 " + b.tr(fmt.Sprintf("حذف %d کلاینت: ", len(ids)), fmt.Sprintf("Deleted %d clients: ", len(ids))) + strings.Join(names, ", ")
		}
	case "edit":
		c, ok := payload.(*model.Client)
		if !ok || c == nil {
			return ""
		}
		before, err := rawFullClient(c.Id)
		if err != nil || !givesVolumeTo(before, *c) {
			return ""
		}
		vol := func(n int64) string {
			if n <= 0 {
				return "∞"
			}
			return humanBytes(n)
		}
		line := "📈 " + b.tr("افزایش حجم: ", "Gave volume: ") + "<b>" + esc(c.Name) + "</b> " + vol(before.Volume) + " → " + vol(c.Volume)
		if c.Up == 0 && c.Down == 0 && usedOf(*before) > 0 {
			line += " · " + b.tr("ریست ترافیک", "traffic reset")
		}
		return line
	case "editbulk":
		list, ok := payload.([]model.Client)
		if !ok {
			return ""
		}
		n := 0
		for _, c := range list {
			if before, err := rawFullClient(c.Id); err == nil && givesVolumeTo(before, c) {
				n++
			}
		}
		if n > 0 {
			return "📈 " + b.tr(fmt.Sprintf("افزایش حجم/ریست گروهی %d کلاینت", n), fmt.Sprintf("Gave volume to %d clients at once", n))
		}
	}
	return ""
}

// createdNote describes clients a seller creates, for the owner.
func (b *bot) createdNote(act string, payload interface{}) string {
	var list []model.Client
	switch v := payload.(type) {
	case model.Client:
		list = []model.Client{v}
	case *model.Client:
		if v != nil {
			list = []model.Client{*v}
		}
	case []model.Client:
		list = v
	}
	if len(list) == 0 {
		return ""
	}
	c := list[0]
	vol := "∞"
	if c.Volume > 0 {
		vol = humanBytes(c.Volume)
	}
	detail := vol
	if c.Group != "" {
		detail += " · " + b.tr("گروه ", "group ") + esc(c.Group)
	}
	if len(list) == 1 {
		return "➕ " + b.tr("ساخت کلاینت: ", "Created client: ") + "<b>" + esc(c.Name) + "</b> · " + detail
	}
	return "➕ " + b.tr(fmt.Sprintf("ساخت %d کلاینت: ", len(list)), fmt.Sprintf("Created %d clients: ", len(list))) + "<b>" + esc(c.Name) + "</b>… · " + detail
}

func (b *bot) tellOwnerAct(note string) {
	if note == "" {
		return
	}
	a := b.access()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	text := "👁 " + b.tr("ادمین ", "Admin ") + "<b>" + esc(b.sellerName(ctx, b.self)) + "</b>\n" + note
	b.notifyIDs(ctx, []int64{a.owner}, text, [][]button{{{Text: b.tr("👮 فروشنده", "👮 Seller"), Data: fmt.Sprintf("q:ad:%d", b.self)}}})
}

// ---- the owner editing a seller's shop ----

func (b *bot) clearDesk(chatID int64) {
	if b.desks != nil {
		b.desks.Delete(chatID)
	}
}

func (b *bot) deskTarget(chatID int64) int64 {
	if b.desks == nil {
		return 0
	}
	if v, ok := b.desks.Load(chatID); ok {
		return v.(int64)
	}
	return 0
}

// deskView is the bot as the seller whose shop the owner is editing in this
// chat, or the bot itself when the owner edits their own.
func (b *bot) deskView(chatID int64) *bot {
	if !b.canManageAdmins() {
		return b
	}
	id := b.deskTarget(chatID)
	if id == 0 {
		return b
	}
	a := b.access()
	if !a.isMember(id) || a.isOwner(id) || id == b.self {
		b.clearDesk(chatID)
		return b
	}
	c := *b
	c.owner, c.self, c.deskBy, c.scope, c.sections, c.paused = false, id, b.self, "", nil, false
	return &c
}

// ---- screens ----

func (b *bot) ownerHome(ctx context.Context) (string, [][]button) {
	now := time.Now()
	today := dayStart(b.loc, now)
	customers := b.customerList()
	blocked := 0
	for _, c := range customers {
		if c.Blocked {
			blocked++
		}
	}
	pending := countOrders(0, true, orderPending, 0)
	t := sumSales(0, today, 0)
	m := sumSales(0, now.AddDate(0, 0, -30).Unix(), 0)
	wn, wt := walletsTotal()
	lines := []string{
		b.header("👑", b.tr("پنل مالک", "Owner panel")),
		"👤 " + b.tr("مشتری‌ها: ", "Customers: ") + strconv.Itoa(len(customers)) + b.tr(" · مسدود: ", " · blocked: ") + strconv.Itoa(blocked),
		"👮 " + b.tr("فروشنده‌ها: ", "Sellers: ") + strconv.Itoa(len(b.sellers())),
		"📋 " + b.tr("سفارش در انتظار: ", "Pending orders: ") + strconv.FormatInt(pending, 10),
		"💵 " + b.tr("فروش امروز: ", "Sales today: ") + strconv.FormatInt(t.Orders, 10) + " · " + formatMoney(t.Revenue),
		"📆 " + b.tr("فروش ۳۰ روز: ", "Sales 30 days: ") + strconv.FormatInt(m.Orders, 10) + " · " + formatMoney(m.Revenue),
		"🏦 " + b.tr("موجودی کیف پول مشتری‌ها: ", "Customer wallet balances: ") + formatMoney(wt) + fmt.Sprintf(" (%d)", wn),
	}
	kb := [][]button{
		{{Text: b.tr("👤 مشتری‌ها", "👤 Customers"), Data: "q:cu:0"}, {Text: b.tr("🔎 جستجوی مشتری", "🔎 Find customer"), Data: "q:cs"}},
		{{Text: b.tr("👮 فروشنده‌ها", "👮 Sellers"), Data: "q:ads"}, {Text: b.tr("💰 مالی و تسویه", "💰 Money"), Data: "q:fin"}},
		{{Text: b.tr("📣 پیام همگانی", "📣 Broadcast"), Data: "q:bc"}, {Text: b.tr("🔔 هشدارها", "🔔 Alerts"), Data: "q:al"}},
		{{Text: b.tr("📋 سفارش‌ها", "📋 Orders"), Data: "k:ordsP"}, {Text: b.tr("📄 خروجی CSV", "📄 CSV export"), Data: "q:csv"}},
		{{Text: b.tr("⬆️ آپدیت پنل", "⬆️ Update panel"), Data: "q:up"}, {Text: b.tr("🔄 ریستارت پنل", "🔄 Restart panel"), Data: "m:prest"}},
		b.menuRow(),
	}
	return strings.Join(lines, "\n"), kb
}

const ownerPage = 10

func (b *bot) customersScreen(page int) (string, [][]button) {
	list := b.customerList()
	from, to, page, pages := pageSlice(len(list), page, ownerPage)
	lines := []string{b.header("👤", b.tr("مشتری‌ها", "Customers")) + fmt.Sprintf(" (%d)", len(list))}
	if len(list) == 0 {
		lines = append(lines, b.tr("هنوز مشتری‌ای نیست.", "No customers yet."))
	}
	var btns []button
	for _, c := range list[from:to] {
		btns = append(btns, button{Text: customerButtonText(c), Data: fmt.Sprintf("q:cv:%d", c.TgId)})
	}
	kb := rows2(btns)
	if p := pager("q:cu", page, pages); p != nil {
		kb = append(kb, p)
	}
	kb = append(kb, []button{{Text: b.tr("🔎 جستجو", "🔎 Search"), Data: "q:cs"}}, b.navRow("q:home"))
	return strings.Join(lines, "\n"), kb
}

func (b *bot) clientBrief(c model.Client) string {
	used := c.Up + c.Down
	vol := "∞"
	if c.Volume > 0 {
		vol = humanBytes(c.Volume)
	}
	exp := "∞"
	if c.Expiry > 0 {
		exp = b.stamp(c.Expiry)
	}
	state := "🟢"
	if !c.Enable {
		state = "🔴"
	}
	return state + " <b>" + esc(c.Name) + "</b> — " + humanBytes(used) + " / " + vol + " · " + exp
}

func (b *bot) customerScreen(ctx context.Context, tgID int64) (string, [][]button) {
	c, _ := customerRow(tgID)
	c.TgId = tgID
	sid := strconv.FormatInt(tgID, 10)
	lines := []string{b.header("👤", b.tr("مشتری", "Customer")), b.customerLabel(c)}
	if c.Blocked {
		lines = append(lines, "⛔ <b>"+b.tr("مسدود", "Blocked")+"</b>")
	}
	if c.FirstSeen > 0 || c.LastSeen > 0 {
		lines = append(lines, "🕒 "+b.tr("اولین/آخرین بازدید: ", "First/last seen: ")+b.stamp(c.FirstSeen)+" / "+b.stamp(c.LastSeen))
	}
	seller := b.customerAdminID(tgID)
	how := b.tr("خودکار", "automatic")
	if c.AdminId != 0 {
		how = b.tr("دستی", "set by you")
	}
	lines = append(lines, "👮 "+b.tr("فروشنده: ", "Seller: ")+esc(b.sellerName(ctx, seller))+" ("+how+")")
	if r := loadBotRef(tgID); r != nil {
		lines = append(lines, "🧭 "+b.tr("معرف: ", "Introducer: ")+esc(r.Word))
	}
	lines = append(lines, "🏦 "+b.tr("موجودی: ", "Balance: ")+"<b>"+formatMoney(walletBalance(tgID))+"</b>")
	bought := sumSalesOf(tgID)
	lines = append(lines, "💵 "+b.tr("مجموع خرید: ", "Total bought: ")+formatMoney(bought.Revenue)+fmt.Sprintf(" (%d)", bought.Orders))
	counts := map[string]int{}
	for _, o := range customerOrders(tgID) {
		counts[o.Status]++
	}
	lines = append(lines, "📋 "+b.tr("سفارش‌ها: ", "Orders: ")+fmt.Sprintf("✅%d ⏳%d ❌%d ✖️%d", counts[orderApproved], counts[orderPending], counts[orderRejected], counts[orderCancelled]))
	bound := boundClients(tgID)
	lines = append(lines, "🛍 "+b.tr("سرویس‌ها: ", "Services: ")+strconv.Itoa(len(bound)))
	var svc []button
	for i, cl := range bound {
		if i < 8 {
			lines = append(lines, "  "+b.clientBrief(cl))
			svc = append(svc, button{Text: "📱 " + truncate(cl.Name, 20), Data: fmt.Sprintf("c:view:%d", cl.Id)})
		}
	}
	if strings.TrimSpace(c.Note) != "" {
		lines = append(lines, "📝 "+esc(c.Note))
	}
	block := button{Text: b.tr("⛔ مسدود کردن", "⛔ Block"), Data: "q:cb:" + sid}
	if c.Blocked {
		block = button{Text: b.tr("✅ رفع مسدودی", "✅ Unblock"), Data: "q:cb:" + sid}
	}
	kb := [][]button{
		{{Text: b.tr("➕ شارژ کیف پول", "➕ Add to wallet"), Data: "q:cw:" + sid + ":p"}, {Text: b.tr("➖ کسر از کیف پول", "➖ Take from wallet"), Data: "q:cw:" + sid + ":m"}},
		{{Text: b.tr("🧾 تراکنش‌ها", "🧾 Wallet history"), Data: "q:ct:" + sid}, {Text: b.tr("📋 سفارش‌ها", "📋 Orders"), Data: "q:co:" + sid}},
		{{Text: b.tr("✉️ پیام مستقیم", "✉️ Message"), Data: "q:cm:" + sid}, {Text: b.tr("📝 یادداشت", "📝 Note"), Data: "q:cn:" + sid}},
		{{Text: b.tr("👮 تغییر فروشنده", "👮 Change seller"), Data: "q:ca:" + sid}, block},
	}
	kb = append(kb, rows2(svc)...)
	kb = append(kb, b.navRow("q:cu:0"))
	return strings.Join(lines, "\n"), kb
}

// sumSalesOf is what one customer bought.
func sumSalesOf(tgID int64) salesSum {
	var rows []model.BotOrder
	database.GetDB().Where("tg_id = ? AND status = ? AND topic IN ?", tgID, orderApproved, []string{"buy", "renew"}).Find(&rows)
	var s salesSum
	for _, o := range rows {
		s.Orders++
		s.Revenue += o.Amount
		s.GB += int64(o.PlanGB)
	}
	return s
}

func (b *bot) customerOrdersScreen(tgID int64) (string, [][]button) {
	lines := []string{b.header("📋", b.tr("سفارش‌های مشتری", "Customer orders")) + fmt.Sprintf(" <code>%d</code>", tgID)}
	orders := customerOrders(tgID)
	if len(orders) == 0 {
		lines = append(lines, b.tr("سفارشی نیست.", "No orders."))
	}
	var kb [][]button
	for _, o := range orders {
		line := fmt.Sprintf("#%d · %s · %s · %s", o.Id, b.stamp(o.CreatedAt), b.orderTopicLabel(o.Topic), b.orderStatusLabel(o.Status))
		if o.Detail != "" {
			line += " · " + o.Detail
		}
		if o.Amount > 0 {
			line += " · " + formatMoney(o.Amount)
		}
		if o.WalletPaid {
			line += " · 🏦"
		}
		lines = append(lines, esc(line))
		if o.Status == orderPending {
			kb = append(kb, []button{
				{Text: fmt.Sprintf("✅ #%d", o.Id), Data: fmt.Sprintf("j:ok:%d", o.Id)},
				{Text: fmt.Sprintf("❌ #%d", o.Id), Data: fmt.Sprintf("j:no:%d", o.Id)},
			})
		}
	}
	return strings.Join(lines, "\n"), append(kb, b.navRow(fmt.Sprintf("q:cv:%d", tgID)))
}

func (b *bot) walletKindLabel(kind string) string {
	switch kind {
	case walletTopUp:
		return b.tr("شارژ", "top-up")
	case walletPay:
		return b.tr("خرید", "payment")
	case walletRefund:
		return b.tr("برگشت", "refund")
	case walletManual:
		return b.tr("دستی", "manual")
	}
	return kind
}

func (b *bot) walletHistoryScreen(tgID int64) (string, [][]button) {
	lines := []string{b.header("🧾", b.tr("تراکنش‌های کیف پول", "Wallet history")) + fmt.Sprintf(" <code>%d</code>", tgID),
		b.tr("موجودی: ", "Balance: ") + "<b>" + formatMoney(walletBalance(tgID)) + "</b>"}
	txs := walletHistory(tgID, 20)
	if len(txs) == 0 {
		lines = append(lines, b.tr("تراکنشی ثبت نشده.", "No transactions recorded."))
	}
	for _, t := range txs {
		sign := "+"
		if t.Delta < 0 {
			sign = ""
		}
		line := fmt.Sprintf("%s · %s%s · %s → %s", b.stamp(t.CreatedAt), sign, formatMoney(t.Delta), b.walletKindLabel(t.Kind), formatMoney(t.Balance))
		if t.OrderId != 0 {
			line += fmt.Sprintf(" · #%d", t.OrderId)
		}
		if t.Note != "" {
			line += " · " + t.Note
		}
		lines = append(lines, esc(line))
	}
	return strings.Join(lines, "\n"), [][]button{b.navRow(fmt.Sprintf("q:cv:%d", tgID))}
}

func (b *bot) sellerChooser(ctx context.Context, tgID int64) (string, [][]button) {
	sid := strconv.FormatInt(tgID, 10)
	a := b.access()
	names := b.adminNames(ctx, a.order)
	var btns []button
	for _, id := range a.order {
		label := names[id]
		if label == "" {
			label = strconv.FormatInt(id, 10)
		}
		if a.isOwner(id) {
			label = "👑 " + label
		}
		btns = append(btns, button{Text: truncate(label, 24), Data: fmt.Sprintf("q:cas:%s:%d", sid, id)})
	}
	kb := rows2(btns)
	kb = append(kb, []button{{Text: b.tr("🔄 خودکار (از معرف/گروه)", "🔄 Automatic (introducer/group)"), Data: "q:cas:" + sid + ":0"}}, b.navRow("q:cv:"+sid))
	return b.header("👮", b.tr("فروشنده این مشتری", "This customer's seller")) + "\n" +
		b.tr("سفارش‌های بعدی این مشتری به فروشنده انتخابی می‌رود و پلن‌های او را می‌بیند.", "This customer's next orders go to the chosen seller, and they see that seller's plans."), kb
}

func (b *bot) sellersScreen(ctx context.Context) (string, [][]button) {
	ids := b.sellers()
	lines := []string{b.header("👮", b.tr("فروشنده‌ها", "Sellers"))}
	if len(ids) == 0 {
		lines = append(lines, b.tr("ادمینی جز شما نیست. از «مدیران» ادمین اضافه کنید.", "There is no admin besides you. Add one from Admins."))
	}
	a := b.access()
	names := b.adminNames(ctx, ids)
	start := time.Now().AddDate(0, 0, -30).Unix()
	var btns []button
	for _, id := range ids {
		m := adminMeta(id)
		s := sumSales(id, start, 0)
		line := b.adminLine(a, id, names[id])
		if m.Suspended {
			line = "⏸ " + line
		}
		line += "\n   " + b.tr("۳۰ روز: ", "30 days: ") + strconv.FormatInt(s.Orders, 10) + " · " + formatMoney(s.Revenue) + " · " + strconv.FormatInt(s.GB, 10) + " GB"
		lines = append(lines, line)
		label := names[id]
		if label == "" {
			label = strconv.FormatInt(id, 10)
		}
		if m.Suspended {
			label = "⏸ " + label
		}
		btns = append(btns, button{Text: truncate(label, 24), Data: fmt.Sprintf("q:ad:%d", id)})
	}
	kb := rows2(btns)
	kb = append(kb, []button{{Text: b.tr("🔐 دسترسی ادمین‌ها", "🔐 Admin access"), Data: "a:ls"}}, b.navRow("q:home"))
	return strings.Join(lines, "\n"), kb
}

func moneyOrNone(b *bot, n int64) string {
	if n <= 0 {
		return b.tr("ندارد", "none")
	}
	return formatMoney(n)
}

func (b *bot) sellerScreen(ctx context.Context, id int64) (string, [][]button) {
	a := b.access()
	if !a.isMember(id) || a.isOwner(id) {
		return b.sellersScreen(ctx)
	}
	sid := strconv.FormatInt(id, 10)
	m := adminMeta(id)
	now := time.Now()
	lines := []string{b.header("👮", b.tr("فروشنده", "Seller")), b.adminLine(a, id, b.adminNames(ctx, []int64{id})[id])}
	if line := b.quotaStateLine(id); line != "" {
		lines = append(lines, line)
	}
	state := "✅ " + b.tr("فعال", "active")
	if m.Suspended {
		state = "⏸ <b>" + b.tr("معلق", "suspended") + "</b>"
	}
	lines = append(lines,
		b.tr("وضعیت: ", "State: ")+state,
		"🔒 "+b.tr("تأیید سفارش فقط با مالک: ", "Orders approved by the owner only: ")+onOff(m.RequireOwner),
		"💵 "+b.tr("حداقل قیمت هر گیگ: ", "Lowest price per GB: ")+moneyOrNone(b, m.MinPerGB),
		"🧮 "+b.tr("نرخ تسویه هر گیگ: ", "Settlement rate per GB: ")+moneyOrNone(b, m.RatePerGB),
		"")
	for _, p := range []struct {
		fa, en string
		since  int64
	}{
		{"امروز", "Today", dayStart(b.loc, now)},
		{"۷ روز", "7 days", now.AddDate(0, 0, -7).Unix()},
		{"۳۰ روز", "30 days", now.AddDate(0, 0, -30).Unix()},
		{"کل", "All", 0},
	} {
		s := sumSales(id, p.since, 0)
		lines = append(lines, "📊 "+b.tr(p.fa, p.en)+": "+strconv.FormatInt(s.Orders, 10)+b.tr(" فروش · ", " sales · ")+formatMoney(s.Revenue)+" · "+strconv.FormatInt(s.GB, 10)+" GB")
	}
	_, us, owed := unsettled(id)
	lines = append(lines, "💼 "+b.tr("تسویه‌نشده: ", "Unsettled: ")+strconv.FormatInt(us.Orders, 10)+" · "+strconv.FormatInt(us.GB, 10)+" GB · "+b.tr("بدهی: ", "owed: ")+"<b>"+formatMoney(owed)+"</b>")
	lines = append(lines, "👥 "+b.tr("مشتری‌ها: ", "Customers: ")+strconv.Itoa(len(b.sellerCustomers(id)))+" · "+b.tr("کلاینت‌ها: ", "Clients: ")+strconv.Itoa(len(b.sellerClients(id))))
	pause := button{Text: b.tr("⏸ تعلیق", "⏸ Suspend"), Data: "q:as:" + sid}
	if m.Suspended {
		pause = button{Text: b.tr("▶️ رفع تعلیق", "▶️ Resume"), Data: "q:as:" + sid}
	}
	kb := [][]button{
		{pause, {Text: onOff(m.RequireOwner) + " " + b.tr("تأیید با مالک", "Owner approves"), Data: "q:ao:" + sid}},
		{{Text: b.tr("💵 حداقل قیمت", "💵 Lowest price"), Data: "q:amin:" + sid}, {Text: b.tr("🧮 نرخ تسویه", "🧮 Rate"), Data: "q:arate:" + sid}},
		{{Text: b.tr("🛍 فروشگاهش", "🛍 Their shop"), Data: "q:adesk:" + sid}, {Text: b.tr("💼 تسویه", "💼 Settle"), Data: "q:aset:" + sid}},
		{{Text: b.tr("👥 مشتری‌ها", "👥 Customers"), Data: "q:acu:" + sid + ":0"}, {Text: b.tr("📱 کلاینت‌ها", "📱 Clients"), Data: "q:acl:" + sid + ":0"}},
		{{Text: b.tr("🧾 فعالیت", "🧾 Activity"), Data: "q:aact:" + sid}, {Text: b.tr("📦 سهمیه", "📦 Quota"), Data: "a:q:" + sid}},
		{{Text: b.tr("✏️ دسترسی‌ها", "✏️ Access"), Data: "a:e:" + sid}, {Text: b.tr("📣 پیام به مشتری‌هایش", "📣 Message their customers"), Data: "q:bca:a" + sid}},
		b.navRow("q:ads"),
	}
	return strings.Join(lines, "\n"), kb
}

// sellerCustomers are the customers whose orders go to a seller.
func (b *bot) sellerCustomers(id int64) []model.BotCustomer {
	var out []model.BotCustomer
	for _, c := range b.customerList() {
		if b.customerAdminID(c.TgId) == id {
			out = append(out, c)
		}
	}
	return out
}

// sellerClients are the clients a seller sold, counts against their limit,
// or manages through their group.
func (b *bot) sellerClients(id int64) []model.Client {
	ids := map[uint]bool{}
	var qc []model.BotQuotaClient
	database.GetDB().Where("tg_id = ?", id).Find(&qc)
	for _, r := range qc {
		ids[r.ClientId] = true
	}
	var orders []model.BotOrder
	database.GetDB().Where("admin_id = ? AND status = ? AND client_id <> 0", id, orderApproved).Find(&orders)
	for _, o := range orders {
		ids[o.ClientId] = true
	}
	group := strings.TrimSpace(b.access().roleOf(id).group)
	var out []model.Client
	for _, c := range allClients() {
		if ids[c.Id] || (group != "" && sameGroup(c.Group, group)) {
			out = append(out, c)
		}
	}
	return out
}

func (b *bot) sellerCustomersScreen(ctx context.Context, id int64, page int) (string, [][]button) {
	list := b.sellerCustomers(id)
	from, to, page, pages := pageSlice(len(list), page, ownerPage)
	lines := []string{b.header("👥", b.tr("مشتری‌های ", "Customers of ")+esc(b.sellerName(ctx, id))) + fmt.Sprintf(" (%d)", len(list))}
	var btns []button
	for _, c := range list[from:to] {
		btns = append(btns, button{Text: customerButtonText(c), Data: fmt.Sprintf("q:cv:%d", c.TgId)})
	}
	kb := rows2(btns)
	if p := pager(fmt.Sprintf("q:acu:%d", id), page, pages); p != nil {
		kb = append(kb, p)
	}
	return strings.Join(lines, "\n"), append(kb, b.navRow(fmt.Sprintf("q:ad:%d", id)))
}

func (b *bot) sellerClientsScreen(ctx context.Context, id int64, page int) (string, [][]button) {
	list := b.sellerClients(id)
	from, to, page, pages := pageSlice(len(list), page, ownerPage)
	lines := []string{b.header("📱", b.tr("کلاینت‌های ", "Clients of ")+esc(b.sellerName(ctx, id))) + fmt.Sprintf(" (%d)", len(list))}
	var btns []button
	for _, c := range list[from:to] {
		lines = append(lines, b.clientBrief(c))
		btns = append(btns, button{Text: truncate(c.Name, 22), Data: fmt.Sprintf("c:view:%d", c.Id)})
	}
	kb := rows2(btns)
	if p := pager(fmt.Sprintf("q:acl:%d", id), page, pages); p != nil {
		kb = append(kb, p)
	}
	return strings.Join(lines, "\n"), append(kb, b.navRow(fmt.Sprintf("q:ad:%d", id)))
}

func (b *bot) sellerActivityScreen(ctx context.Context, id int64) (string, [][]button) {
	lines := []string{b.header("🧾", b.tr("فعالیت ", "Activity of ")+esc(b.sellerName(ctx, id)))}
	var changes []model.Changes
	if b.configService != nil {
		changes = b.configService.GetChanges(service.TelegramActor+":"+strconv.FormatInt(id, 10), "", "20")
	}
	if len(changes) == 0 {
		lines = append(lines, b.tr("تغییری از این ادمین ثبت نشده.", "No changes recorded from this admin."))
	}
	for _, c := range changes {
		what := ""
		var m map[string]interface{}
		if json.Unmarshal(c.Obj, &m) == nil {
			what = scalar(m["name"])
			if what == "" {
				what = scalar(m["tag"])
			}
		} else {
			what = scalar(string(c.Obj))
		}
		line := fmt.Sprintf("🕒 %s · %s.%s", time.Unix(c.DateTime, 0).In(b.loc).Format("01-02 15:04"), esc(c.Key), esc(c.Action))
		if what != "" {
			line += " · " + esc(truncate(what, 30))
		}
		lines = append(lines, line)
	}
	orders := listOrders(id, false, false, 10)
	if len(orders) > 0 {
		lines = append(lines, "", "📋 "+b.tr("آخرین سفارش‌ها:", "Latest orders:"))
		for _, o := range orders {
			lines = append(lines, esc(fmt.Sprintf("#%d · %s · %s · %s", o.Id, b.stamp(o.UpdatedAt), b.orderTopicLabel(o.Topic), b.orderStatusLabel(o.Status))))
		}
	}
	return strings.Join(lines, "\n"), [][]button{b.navRow(fmt.Sprintf("q:ad:%d", id))}
}

func (b *bot) financeScreen(ctx context.Context) (string, [][]button) {
	now := time.Now()
	lines := []string{b.header("💰", b.tr("مالی", "Money"))}
	for _, p := range []struct {
		fa, en string
		since  int64
	}{
		{"امروز", "Today", dayStart(b.loc, now)},
		{"۷ روز", "7 days", now.AddDate(0, 0, -7).Unix()},
		{"۳۰ روز", "30 days", now.AddDate(0, 0, -30).Unix()},
		{"کل", "All", 0},
	} {
		s := sumSales(0, p.since, 0)
		lines = append(lines, "📊 "+b.tr(p.fa, p.en)+": "+strconv.FormatInt(s.Orders, 10)+b.tr(" فروش · ", " sales · ")+"<b>"+formatMoney(s.Revenue)+"</b> · "+strconv.FormatInt(s.GB, 10)+" GB")
	}
	m30 := now.AddDate(0, 0, -30).Unix()
	tn, ta := sumTopUps(0, m30)
	s30 := sumSales(0, m30, 0)
	wn, wt := walletsTotal()
	lines = append(lines, "",
		"🏦 "+b.tr("شارژ کیف پول (۳۰ روز): ", "Wallet top-ups (30 days): ")+strconv.FormatInt(tn, 10)+" · "+formatMoney(ta),
		"💳 "+b.tr("پرداخت از کیف پول (۳۰ روز): ", "Paid from wallets (30 days): ")+formatMoney(s30.Wallet),
		"💼 "+b.tr("موجودی فعلی کیف پول‌ها: ", "Wallet balances now: ")+formatMoney(wt)+fmt.Sprintf(" (%d)", wn))
	ids := b.sellers()
	var btns []button
	if len(ids) > 0 {
		lines = append(lines, "", "👮 "+b.tr("فروشنده‌ها (۳۰ روز / تسویه‌نشده):", "Sellers (30 days / unsettled):"))
		names := b.adminNames(ctx, ids)
		for _, id := range ids {
			s := sumSales(id, m30, 0)
			_, us, owed := unsettled(id)
			name := names[id]
			if name == "" {
				name = strconv.FormatInt(id, 10)
			}
			lines = append(lines, "• <b>"+esc(name)+"</b>: "+formatMoney(s.Revenue)+" · "+strconv.FormatInt(s.GB, 10)+" GB | "+
				strconv.FormatInt(us.GB, 10)+" GB · "+b.tr("بدهی ", "owed ")+formatMoney(owed))
			btns = append(btns, button{Text: "💼 " + truncate(name, 20), Data: fmt.Sprintf("q:aset:%d", id)})
		}
	}
	lines = append(lines, "", b.tr("ℹ️ مبلغ فروش از قیمت عددی پلن در زمان سفارش است. بدهی فروشنده = گیگ فروخته‌شده از آخرین تسویه × نرخ تسویه او.",
		"ℹ️ Sales amounts are the plan's numeric price at order time. A seller owes the GB sold since the last settlement × their rate."))
	kb := rows2(btns)
	kb = append(kb, []button{{Text: b.tr("📄 خروجی CSV سفارش‌ها", "📄 Orders CSV"), Data: "q:csv"}}, b.navRow("q:home"))
	return strings.Join(lines, "\n"), kb
}

func (b *bot) settleScreen(ctx context.Context, id int64) (string, [][]button) {
	sid := strconv.FormatInt(id, 10)
	from, s, owed := unsettled(id)
	m := adminMeta(id)
	since := b.tr("از ابتدا", "from the start")
	if from > 0 {
		since = b.stamp(from)
	}
	lines := []string{
		b.header("💼", b.tr("تسویه با ", "Settle with ")+esc(b.sellerName(ctx, id))),
		b.tr("از آخرین تسویه: ", "Since the last settlement: ") + since,
		b.tr("فروش: ", "Sales: ") + strconv.FormatInt(s.Orders, 10) + " · " + formatMoney(s.Revenue),
		b.tr("حجم فروخته‌شده: ", "Volume sold: ") + strconv.FormatInt(s.GB, 10) + " GB",
		b.tr("نرخ هر گیگ: ", "Rate per GB: ") + moneyOrNone(b, m.RatePerGB),
		"💰 " + b.tr("بدهی فروشنده: ", "Seller owes: ") + "<b>" + formatMoney(owed) + "</b>",
	}
	if hist := settlements(id, 8); len(hist) > 0 {
		lines = append(lines, "", "🗂 "+b.tr("تسویه‌های قبلی:", "Earlier settlements:"))
		for _, h := range hist {
			lines = append(lines, fmt.Sprintf("• %s · %d GB · %s", b.stamp(h.To), h.GB, formatMoney(h.Amount)))
		}
	}
	var kb [][]button
	if s.Orders > 0 {
		kb = append(kb, []button{{Text: b.tr("✅ ثبت تسویه", "✅ Mark settled"), Data: "q:asety:" + sid}})
	}
	kb = append(kb, []button{{Text: b.tr("🧮 نرخ تسویه", "🧮 Rate"), Data: "q:arate:" + sid}}, b.navRow("q:ad:"+sid))
	return strings.Join(lines, "\n"), kb
}

// audience is who a broadcast reaches: "all", "act" (with an active
// service) or "a<seller ID>"; blocked customers never.
func (b *bot) audience(aud string) []int64 {
	now := time.Now().Unix()
	var seller int64 = -1
	if strings.HasPrefix(aud, "a") && aud != "act" && aud != "all" {
		if id, err := strconv.ParseInt(aud[1:], 10, 64); err == nil {
			seller = id
		}
	}
	var out []int64
	for _, c := range b.customerList() {
		if c.Blocked {
			continue
		}
		switch {
		case aud == "all":
		case aud == "act":
			if !customerHasActive(c.TgId, now) {
				continue
			}
		case seller >= 0:
			if b.customerAdminID(c.TgId) != seller {
				continue
			}
		default:
			continue
		}
		out = append(out, c.TgId)
	}
	return out
}

func (b *bot) audienceLabel(ctx context.Context, aud string) string {
	switch aud {
	case "all":
		return b.tr("همه مشتری‌ها", "all customers")
	case "act":
		return b.tr("مشتری‌های دارای سرویس فعال", "customers with an active service")
	}
	id, _ := strconv.ParseInt(strings.TrimPrefix(aud, "a"), 10, 64)
	return b.tr("مشتری‌های ", "customers of ") + b.sellerName(ctx, id)
}

func (b *bot) broadcastScreen(ctx context.Context) (string, [][]button) {
	lines := []string{b.header("📣", b.tr("پیام همگانی", "Broadcast")),
		b.tr("گیرنده‌ها را انتخاب کنید، بعد متن پیام را بفرستید. مشتری‌های مسدود پیام نمی‌گیرند.", "Pick who gets it, then send the text. Blocked customers get nothing.")}
	kb := [][]button{
		{{Text: fmt.Sprintf("%s (%d)", b.tr("👥 همه", "👥 Everyone"), len(b.audience("all"))), Data: "q:bca:all"}},
		{{Text: fmt.Sprintf("%s (%d)", b.tr("🟢 دارای سرویس فعال", "🟢 With an active service"), len(b.audience("act"))), Data: "q:bca:act"}},
	}
	names := b.adminNames(ctx, b.sellers())
	var btns []button
	for _, id := range b.sellers() {
		name := names[id]
		if name == "" {
			name = strconv.FormatInt(id, 10)
		}
		btns = append(btns, button{Text: fmt.Sprintf("👮 %s (%d)", truncate(name, 16), len(b.audience(fmt.Sprintf("a%d", id)))), Data: fmt.Sprintf("q:bca:a%d", id)})
	}
	kb = append(kb, rows2(btns)...)
	return strings.Join(lines, "\n"), append(kb, b.navRow("q:home"))
}

type broadcastDraft struct {
	aud, text string
}

// Broadcasts run in the background, a little apart so Telegram does not
// throttle the bot. Tests run them in place.
var (
	runBroadcast = func(f func()) { go f() }
	broadcastGap = 60 * time.Millisecond
)

func (b *bot) startBroadcast(ctx context.Context, chatID int64, d broadcastDraft) int {
	ids := b.audience(d.aud)
	text := "📣 " + esc(d.text)
	runBroadcast(func() {
		sent, failed := 0, 0
		for i, id := range ids {
			if ctx.Err() != nil {
				break
			}
			if i > 0 && broadcastGap > 0 {
				time.Sleep(broadcastGap)
			}
			err := b.call(ctx, "sendMessage", map[string]any{"chat_id": id, "text": text, "parse_mode": "HTML", "disable_web_page_preview": true}, nil)
			if err != nil {
				failed++
			} else {
				sent++
			}
		}
		b.send(ctx, chatID, fmt.Sprintf("📣 %s: ✅ %d · ❌ %d", b.tr("پیام همگانی تمام شد", "Broadcast finished"), sent, failed))
	})
	return len(ids)
}

type alertDef struct {
	key, fa, en string
}

var ownerAlerts = []alertDef{
	{prefStale, "⏰ سفارش‌های معطل", "⏰ Orders waiting too long"},
	{prefQuotaFull, "🚫 تمام شدن سهمیه فروشنده", "🚫 A seller's quota used up"},
	{prefAdminActs, "🗑 حذف کلاینت / افزایش حجم توسط ادمین‌ها", "🗑 Admins deleting clients / giving volume"},
	{prefApprovals, "✅ تأیید سفارش توسط ادمین‌ها", "✅ Admins approving orders"},
	{prefAdminAdds, "➕ ساخت کلاینت توسط ادمین‌ها", "➕ Admins creating clients"},
	{prefNewCust, "🆕 مشتری جدید", "🆕 New customer"},
	{prefRenew, "⏳ یادآوری تمدید به مشتری‌ها", "⏳ Renewal reminders to customers"},
}

var staleHourChoices = []int{1, 3, 6, 12, 24}

func (b *bot) alertsScreen() (string, [][]button) {
	hrs := prefInt(prefStaleHrs, 3)
	lines := []string{b.header("🔔", b.tr("هشدارهای مالک", "Owner alerts")),
		b.tr("روی هر مورد بزنید تا روشن یا خاموش شود.", "Tap an item to switch it on or off."),
		b.tr(fmt.Sprintf("سفارش معطل: سفارشی که رسیدش آمده و بیش از %d ساعت تأیید نشده.", hrs), fmt.Sprintf("A waiting order: one with a receipt not handled for more than %d hours.", hrs)),
		b.tr("هشدار نود و هسته و گزارش زمان‌بندی‌شده در تنظیمات ربات (پنل وب) است.", "Node/core alerts and the scheduled report are in the bot settings of the web panel.")}
	lines = append(lines, b.renewLines()...)
	var kb [][]button
	for _, d := range ownerAlerts {
		kb = append(kb, []button{{Text: onOff(prefOn(d.key)) + " " + b.tr(d.fa, d.en), Data: "q:alt:" + d.key}})
	}
	var row []button
	for _, h := range staleHourChoices {
		label := fmt.Sprintf("%dh", h)
		if h == hrs {
			label = "• " + label
		}
		row = append(row, button{Text: label, Data: fmt.Sprintf("q:alh:%d", h)})
	}
	days := prefInt(prefRenewDays, 3)
	var drow []button
	for _, d := range renewDayChoices {
		label := b.tr(fmt.Sprintf("⏳ %d روز", d), fmt.Sprintf("⏳ %dd", d))
		if d == days {
			label = "• " + label
		}
		drow = append(drow, button{Text: label, Data: fmt.Sprintf("q:ald:%d", d)})
	}
	kb = append(kb, row, drow, b.navRow("q:home"))
	return strings.Join(lines, "\n"), kb
}

// The panel update, as variables so tests do not reach GitHub.
var (
	panelUpdateInfo  = func(check bool) service.PanelUpdate { return (&service.PanelUpdateService{}).Info(check) }
	panelUpdateStart = func(lang, by, tag string) (string, error) {
		return (&service.PanelUpdateService{}).StartAt(lang, by, tag, false)
	}
)

func (b *bot) updateWhy(why string) string {
	switch why {
	case "docker":
		return b.tr("پنل داخل داکر است؛ با به‌روزرسانی ایمیج آپدیت کنید.", "The panel runs in Docker; update the image instead.")
	case "os":
		return b.tr("سیستم‌عامل پشتیبانی نمی‌شود.", "The operating system is not supported.")
	case "root":
		return b.tr("پنل با دسترسی root اجرا نمی‌شود.", "The panel does not run as root.")
	}
	return b.tr("این پنل نمی‌تواند خودش را آپدیت کند: ", "This panel cannot update itself: ") + esc(why) + b.tr(" — از ترمینال s-ui update بزنید.", " — run s-ui update in a terminal.")
}

func (b *bot) updateScreen(check bool) (string, [][]button) {
	info := panelUpdateInfo(check)
	lines := []string{b.header("⬆️", b.tr("آپدیت پنل", "Panel update")),
		b.tr("نسخه فعلی: ", "Current version: ") + "<b>" + esc(info.Current) + "</b>"}
	switch {
	case info.CheckError != "":
		lines = append(lines, "⚠️ "+b.tr("بررسی نسخه جدید ممکن نشد: ", "Could not check for a new version: ")+esc(truncate(info.CheckError, 120)))
	case info.Latest != "":
		latest := b.tr("آخرین نسخه: ", "Latest release: ") + "<b>" + esc(info.Latest) + "</b>"
		if info.Published > 0 {
			latest += " · " + b.stamp(info.Published)
		}
		lines = append(lines, latest)
		if info.Newer {
			lines = append(lines, "🆕 "+b.tr("نسخه جدید آماده است.", "A new version is available."))
		} else {
			lines = append(lines, "✅ "+b.tr("پنل به‌روز است.", "The panel is up to date."))
		}
	}
	if info.Running {
		lines = append(lines, "⏳ "+b.tr("در حال آپدیت به ", "Updating to ")+esc(info.Target))
	} else if info.Exit != nil && info.Target != "" {
		if *info.Exit == 0 {
			lines = append(lines, "✅ "+b.tr("آخرین آپدیت موفق بود: ", "The last update succeeded: ")+esc(info.Target))
		} else {
			lines = append(lines, "❌ "+b.tr("آخرین آپدیت ناموفق بود: ", "The last update failed: ")+esc(info.Target))
			if tail := lastLines(info.Log, 6); tail != "" {
				lines = append(lines, "<pre>"+esc(tail)+"</pre>")
			}
		}
	}
	if info.Unsupported != "" {
		lines = append(lines, "ℹ️ "+b.updateWhy(info.Unsupported))
	}
	var kb [][]button
	if info.Newer && info.Unsupported == "" && !info.Running && info.Latest != "" {
		kb = append(kb, []button{{Text: b.tr("⬆️ آپدیت به ", "⬆️ Update to ") + info.Latest, Data: "q:upy:" + info.Latest}})
	}
	kb = append(kb,
		[]button{{Text: b.tr("🔄 بررسی دوباره", "🔄 Check again"), Data: "q:up:c"}, {Text: b.tr("🔄 ریستارت پنل", "🔄 Restart panel"), Data: "m:prest"}},
		b.navRow("q:home"))
	return strings.Join(lines, "\n"), kb
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// ordersCSV is every shop order, for a spreadsheet.
func ordersCSV() []byte {
	var orders []model.BotOrder
	database.GetDB().Order("id").Find(&orders)
	var sb strings.Builder
	sb.WriteString("\ufeffid,created,updated,customer,seller,topic,status,detail,plan_gb,plan_days,amount,wallet_paid,client_id\n")
	field := func(s string) string {
		if strings.ContainsAny(s, ",\"\n\r") {
			return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
		}
		return s
	}
	ts := func(n int64) string {
		if n <= 0 {
			return ""
		}
		return time.Unix(n, 0).UTC().Format("2006-01-02 15:04:05")
	}
	for _, o := range orders {
		sb.WriteString(strings.Join([]string{
			strconv.FormatUint(uint64(o.Id), 10), ts(o.CreatedAt), ts(o.UpdatedAt),
			strconv.FormatInt(o.TgId, 10), strconv.FormatInt(o.AdminId, 10), field(o.Topic), field(o.Status), field(o.Detail),
			strconv.Itoa(o.PlanGB), strconv.Itoa(o.PlanDays), strconv.FormatInt(o.Amount, 10), strconv.FormatBool(o.WalletPaid),
			strconv.FormatUint(uint64(o.ClientId), 10),
		}, ","))
		sb.WriteString("\n")
	}
	return []byte(sb.String())
}

// ---- alerts the watcher sends ----

const ownerCheckEvery = 5 * time.Minute

func (b *bot) ownerWatch(ctx context.Context, now time.Time) {
	if ownerDB() == nil {
		return
	}
	if prefOn(prefStale) {
		b.checkStaleOrders(ctx, now)
	}
	for id := range allQuotas() {
		b.maybeQuotaWarn(ctx, id)
	}
}

// checkStaleOrders tells the owner, once per order, of orders that wait
// for approval too long: those with a receipt, and link requests.
func (b *bot) checkStaleOrders(ctx context.Context, now time.Time) {
	cut := now.Add(-time.Duration(prefInt(prefStaleHrs, 3)) * time.Hour).Unix()
	var rows []model.BotOrder
	database.GetDB().Where("status = ? AND reminded = ? AND created_at < ? AND (receipt_id <> '' OR topic = ?)", orderPending, false, cut, "bind").
		Order("id").Limit(30).Find(&rows)
	if len(rows) == 0 {
		return
	}
	lines := []string{"⏰ <b>" + b.tr(fmt.Sprintf("%d سفارش منتظر تأیید است", len(rows)), fmt.Sprintf("%d orders are waiting for approval", len(rows))) + "</b>"}
	var kb [][]button
	ids := make([]uint, 0, len(rows))
	for i, o := range rows {
		ids = append(ids, o.Id)
		age := b.humanDuration(now.Sub(time.Unix(o.CreatedAt, 0)))
		lines = append(lines, esc(fmt.Sprintf("#%d · %s · %s · %s", o.Id, b.orderTopicLabel(o.Topic), o.Detail, age)))
		if i < 10 {
			kb = append(kb, []button{
				{Text: fmt.Sprintf("✅ #%d", o.Id), Data: fmt.Sprintf("j:ok:%d", o.Id)},
				{Text: fmt.Sprintf("❌ #%d", o.Id), Data: fmt.Sprintf("j:no:%d", o.Id)},
			})
		}
	}
	if err := database.GetDB().Model(&model.BotOrder{}).Where("id IN ?", ids).Update("reminded", true).Error; err != nil {
		return
	}
	kb = append(kb, []button{{Text: b.tr("📋 سفارش‌ها", "📋 Orders"), Data: "k:ordsP"}})
	b.notifyIDs(ctx, b.ownerIDs(), strings.Join(lines, "\n"), kb)
}

// ---- buttons ----

func (b *bot) ownerCallback(ctx context.Context, cbID string, chatID, msgID int64, parts []string) {
	show := func(text string, kb [][]button) {
		b.answer(ctx, cbID, "")
		b.edit(ctx, chatID, msgID, text, kb)
	}
	verb := parts[1]
	arg := func(i int) string {
		if len(parts) > i {
			return parts[i]
		}
		return ""
	}
	num := func(i int) int64 {
		n, _ := strconv.ParseInt(arg(i), 10, 64)
		return n
	}
	ask := func(kind string, id int64, back, prompt string, data map[string]string) {
		if data == nil {
			data = map[string]string{}
		}
		data["id"] = strconv.FormatInt(id, 10)
		b.pend.set(chatID, &pending{kind: kind, msgID: msgID, back: back, data: data})
		show(prompt, [][]button{b.cancelRow()})
	}
	if verb != "adesk" {
		b.clearDesk(chatID)
	}
	switch verb {
	case "home":
		show(b.ownerHome(ctx))
	case "cu":
		page, _ := strconv.Atoi(arg(2))
		show(b.customersScreen(page))
	case "cs":
		ask("q.find", 0, "q:cu:0", "🔎 "+b.tr("شناسه عددی، @یوزرنیم، نام یا نام کلاینت مشتری را بفرستید.", "Send the customer's numeric ID, @username, name or client name."), nil)
	case "cv":
		show(b.customerScreen(ctx, num(2)))
	case "co":
		show(b.customerOrdersScreen(num(2)))
	case "ct":
		show(b.walletHistoryScreen(num(2)))
	case "cw":
		id := num(2)
		prompt := b.tr("مبلغی که به کیف پول اضافه شود را بفرستید (مثلاً 50000 یا ۵۰ هزار). می‌توانید بعد از مبلغ، توضیح هم بنویسید.", "Send the amount to add (e.g. 50000 or 50k). You may add a note after it.")
		if arg(3) == "m" {
			prompt = b.tr("مبلغی که از کیف پول کم شود را بفرستید. می‌توانید بعد از مبلغ، توضیح هم بنویسید.", "Send the amount to take. You may add a note after it.")
		}
		prompt += "\n" + b.tr("موجودی فعلی: ", "Balance now: ") + formatMoney(walletBalance(id))
		ask("q.wal", id, fmt.Sprintf("q:cv:%d", id), "🏦 "+prompt, map[string]string{"sign": arg(3)})
	case "cb":
		id := num(2)
		c, _ := customerRow(id)
		c.TgId = id
		c.Blocked = !c.Blocked
		if c.FirstSeen == 0 {
			c.FirstSeen = time.Now().Unix()
		}
		if err := saveCustomer(c); err != nil {
			b.answer(ctx, cbID, b.t("failed", b.errText(err)))
			return
		}
		if c.Blocked {
			b.pend.clear(id)
		}
		show(b.customerScreen(ctx, id))
	case "ca":
		show(b.sellerChooser(ctx, num(2)))
	case "cas":
		id, seller := num(2), num(3)
		if seller != 0 && !b.access().isMember(seller) {
			b.answer(ctx, cbID, b.t("notFound"))
			return
		}
		c, _ := customerRow(id)
		c.TgId, c.AdminId = id, seller
		if c.FirstSeen == 0 {
			c.FirstSeen = time.Now().Unix()
		}
		if err := saveCustomer(c); err != nil {
			b.answer(ctx, cbID, b.t("failed", b.errText(err)))
			return
		}
		show(b.customerScreen(ctx, id))
	case "cm":
		id := num(2)
		ask("q.msg", id, fmt.Sprintf("q:cv:%d", id), "✉️ "+b.tr("متن پیام به این مشتری را بفرستید:", "Send the message for this customer:")+"\n"+b.customerLabel(model.BotCustomer{TgId: id}), nil)
	case "cn":
		id := num(2)
		ask("q.note", id, fmt.Sprintf("q:cv:%d", id), "📝 "+b.tr("یادداشت این مشتری را بفرستید (فقط شما می‌بینید). «-» پاکش می‌کند.", "Send a note for this customer (only you see it). “-” clears it."), nil)
	case "ads":
		show(b.sellersScreen(ctx))
	case "ad":
		show(b.sellerScreen(ctx, num(2)))
	case "as", "ao":
		id := num(2)
		a := b.access()
		if !a.isMember(id) || a.isOwner(id) {
			b.answer(ctx, cbID, b.accessErrText(errNoSuchAdmin))
			return
		}
		m, err := patchAdminMeta(id, func(m *model.BotAdminMeta) {
			if verb == "as" {
				m.Suspended = !m.Suspended
			} else {
				m.RequireOwner = !m.RequireOwner
			}
		})
		if err != nil {
			b.answer(ctx, cbID, b.t("failed", b.errText(err)))
			return
		}
		if verb == "as" {
			b.pend.clear(id)
			msg := b.tr("▶️ دسترسی شما به ربات برگشت.", "▶️ Your access to the bot is back.")
			if m.Suspended {
				msg = b.pausedText()
			}
			b.notifyIDs(ctx, []int64{id}, msg, nil)
		}
		show(b.sellerScreen(ctx, id))
	case "amin", "arate":
		id := num(2)
		cur := adminMeta(id)
		prompt := b.tr("کمترین قیمتی که این ادمین می‌تواند برای هر گیگ بفروشد را بفرستید (مثلاً 2000). پلن ارزان‌تر فروخته نمی‌شود. «0» یعنی بدون حداقل.", "Send the lowest price per GB this admin may sell at (e.g. 2000). A cheaper plan is not sold. “0” means no minimum.") +
			"\n" + b.tr("فعلی: ", "Now: ") + moneyOrNone(b, cur.MinPerGB)
		kind := "q.min"
		if verb == "arate" {
			kind = "q.rate"
			prompt = b.tr("مبلغی که این ادمین برای هر گیگ فروخته‌شده به شما بدهکار می‌شود را بفرستید (مثلاً 1500). «0» یعنی بدون نرخ.", "Send what this admin owes you per GB sold (e.g. 1500). “0” means no rate.") +
				"\n" + b.tr("فعلی: ", "Now: ") + moneyOrNone(b, cur.RatePerGB)
		}
		ask(kind, id, fmt.Sprintf("q:ad:%d", id), "💵 "+prompt, nil)
	case "acu":
		page, _ := strconv.Atoi(arg(3))
		show(b.sellerCustomersScreen(ctx, num(2), page))
	case "acl":
		page, _ := strconv.Atoi(arg(3))
		show(b.sellerClientsScreen(ctx, num(2), page))
	case "aact":
		show(b.sellerActivityScreen(ctx, num(2)))
	case "adesk":
		id := num(2)
		a := b.access()
		if !a.isMember(id) || a.isOwner(id) || b.desks == nil {
			b.answer(ctx, cbID, b.accessErrText(errNoSuchAdmin))
			return
		}
		b.desks.Store(chatID, id)
		show(b.deskView(chatID).shopEditHome())
	case "dx":
		show(b.ownerHome(ctx))
	case "fin":
		show(b.financeScreen(ctx))
	case "aset":
		show(b.settleScreen(ctx, num(2)))
	case "asety":
		id := num(2)
		_, s, owed := unsettled(id)
		show("💼 "+b.tr(fmt.Sprintf("تسویه %d فروش (%d گیگ) با بدهی %s ثبت شود؟", s.Orders, s.GB, formatMoney(owed)), fmt.Sprintf("Mark %d sales (%d GB) owing %s as settled?", s.Orders, s.GB, formatMoney(owed))),
			[][]button{{{Text: b.t("btnConfirm"), Data: fmt.Sprintf("q:asetgo:%d", id)}, {Text: b.t("btnCancel"), Data: fmt.Sprintf("q:aset:%d", id)}}})
	case "asetgo":
		id := num(2)
		rec, err := settle(id)
		if err != nil {
			b.answer(ctx, cbID, b.t("failed", b.errText(err)))
			return
		}
		b.notifyIDs(ctx, []int64{id}, "💼 "+b.tr(fmt.Sprintf("تسویه ثبت شد: %d فروش، %d گیگ، مبلغ %s.", rec.Orders, rec.GB, formatMoney(rec.Amount)), fmt.Sprintf("Settled: %d sales, %d GB, amount %s.", rec.Orders, rec.GB, formatMoney(rec.Amount))), nil)
		text, kb := b.settleScreen(ctx, id)
		show(b.t("done")+"\n\n"+text, kb)
	case "csv":
		b.answer(ctx, cbID, "")
		if err := b.upload(ctx, "sendDocument", "document", chatID, "orders-"+time.Now().Format("20060102")+".csv", "📄 "+b.tr("همه سفارش‌های فروشگاه", "Every shop order"), ordersCSV()); err != nil {
			b.send(ctx, chatID, b.t("failed", esc(err.Error())))
		}
	case "bc":
		show(b.broadcastScreen(ctx))
	case "bca":
		aud := arg(2)
		n := len(b.audience(aud))
		b.pend.set(chatID, &pending{kind: "q.bc", msgID: msgID, back: "q:bc", data: map[string]string{"aud": aud}})
		show("📣 "+b.tr("متن پیام را بفرستید. گیرنده‌ها: ", "Send the message text. Recipients: ")+esc(b.audienceLabel(ctx, aud))+fmt.Sprintf(" (%d)", n), [][]button{b.cancelRow()})
	case "bcy":
		v, ok := b.drafts.LoadAndDelete(chatID)
		if !ok {
			show(b.broadcastScreen(ctx))
			return
		}
		n := b.startBroadcast(ctx, chatID, v.(broadcastDraft))
		show("📣 "+b.tr(fmt.Sprintf("ارسال به %d نفر شروع شد. پایانش را خبر می‌دهم.", n), fmt.Sprintf("Sending to %d people. You will be told when it ends.", n)), [][]button{b.navRow("q:home")})
	case "bcn":
		b.drafts.Delete(chatID)
		show(b.broadcastScreen(ctx))
	case "al":
		show(b.alertsScreen())
	case "alt":
		key := arg(2)
		known := false
		for _, d := range ownerAlerts {
			known = known || d.key == key
		}
		if !known {
			b.answer(ctx, cbID, "")
			return
		}
		v := "1"
		if prefOn(key) {
			v = "0"
		}
		if err := setPref(key, v); err != nil {
			b.answer(ctx, cbID, b.t("failed", b.errText(err)))
			return
		}
		show(b.alertsScreen())
	case "alh":
		h, _ := strconv.Atoi(arg(2))
		if h <= 0 || h > 168 {
			b.answer(ctx, cbID, "")
			return
		}
		_ = setPref(prefStaleHrs, strconv.Itoa(h))
		show(b.alertsScreen())
	case "ald":
		d, _ := strconv.Atoi(arg(2))
		if d <= 0 || d > 30 {
			b.answer(ctx, cbID, "")
			return
		}
		_ = setPref(prefRenewDays, strconv.Itoa(d))
		show(b.alertsScreen())
	case "up":
		show(b.updateScreen(arg(2) == "c"))
	case "upy":
		tag := arg(2)
		show("⬆️ "+b.tr(fmt.Sprintf("پنل به %s آپدیت شود؟ پنل و ربات حدود یک دقیقه قطع می‌شوند و بعد خودشان برمی‌گردند.", esc(tag)), fmt.Sprintf("Update the panel to %s? The panel and the bot go away for about a minute and come back by themselves.", esc(tag))),
			[][]button{{{Text: b.t("btnConfirm"), Data: "q:upgo:" + tag}, {Text: b.t("btnCancel"), Data: "q:up"}}})
	case "upgo":
		target, err := panelUpdateStart(b.cfg.Lang, b.actor(), arg(2))
		if err != nil {
			show("❌ "+esc(err.Error()), [][]button{b.navRow("q:up")})
			return
		}
		show("⏳ "+b.tr(fmt.Sprintf("آپدیت به %s شروع شد. وقتی ربات دوباره بالا آمد پیام می‌دهد.", esc(target)), fmt.Sprintf("The update to %s started. The bot says so when it is back.", esc(target))), [][]button{b.navRow("q:up")})
	default:
		b.answer(ctx, cbID, "")
	}
}

// ---- typed answers ----

func (b *bot) ownerPending(ctx context.Context, chatID int64, p *pending, text string, retry func(error), finish func(string, [][]button)) {
	text = strings.TrimSpace(text)
	id, _ := strconv.ParseInt(p.data["id"], 10, 64)
	switch p.kind {
	case "q.find":
		found := b.findCustomers(text)
		lines := []string{b.header("🔎", b.tr("نتیجه جستجو", "Search results")) + fmt.Sprintf(" (%d)", len(found))}
		if len(found) == 0 {
			lines = append(lines, b.tr("مشتری‌ای پیدا نشد.", "No customer found."))
			if n, err := strconv.ParseInt(latinDigits(text), 10, 64); err == nil && n > 0 {
				// An ID nobody has seen yet can still be looked at.
				lines = append(lines, b.tr("می‌توانید پروفایل همین شناسه را باز کنید.", "You can still open this ID's profile."))
				finish(strings.Join(lines, "\n"), [][]button{{{Text: fmt.Sprintf("👤 %d", n), Data: fmt.Sprintf("q:cv:%d", n)}}, b.navRow("q:cu:0")})
				return
			}
		}
		var btns []button
		for i, c := range found {
			if i == 20 {
				break
			}
			btns = append(btns, button{Text: customerButtonText(c), Data: fmt.Sprintf("q:cv:%d", c.TgId)})
		}
		finish(strings.Join(lines, "\n"), append(rows2(btns), b.navRow("q:cu:0")))
	case "q.wal":
		amount, ok := parseMoney(text)
		if !ok {
			retry(fmt.Errorf("%s", b.tr("مبلغ را به عدد بفرستید", "send the amount as a number")))
			return
		}
		note := ""
		if m := moneyNumber.FindStringIndex(moneyDigits.Replace(text)); m != nil {
			note = strings.TrimSpace(moneyDigits.Replace(text)[m[1]:])
		}
		delta := amount
		if p.data["sign"] == "m" {
			delta = -amount
		}
		bal, err := walletCredit(id, delta, walletMemo{kind: walletManual, actor: b.self, note: truncate(note, 80)})
		if err != nil {
			retry(err)
			return
		}
		msg := b.tr("🏦 به کیف پول شما اضافه شد: ", "🏦 Added to your wallet: ") + "<b>" + formatMoney(amount) + "</b>"
		if delta < 0 {
			msg = b.tr("🏦 از کیف پول شما کم شد: ", "🏦 Taken from your wallet: ") + "<b>" + formatMoney(amount) + "</b>"
		}
		if note != "" {
			msg += "\n" + esc(note)
		}
		msg += "\n" + b.tr("موجودی: ", "Balance: ") + "<b>" + formatMoney(bal) + "</b>"
		b.sendMenu(ctx, id, msg, b.customerMenu(id))
		t, kb := b.customerScreen(ctx, id)
		finish(b.t("done")+"\n\n"+t, kb)
	case "q.msg":
		if text == "" {
			retry(fmt.Errorf("%s", b.tr("خالی بود", "it was empty")))
			return
		}
		err := b.call(ctx, "sendMessage", map[string]any{"chat_id": id, "text": "✉️ <b>" + b.tr("پیام پشتیبانی", "Message from support") + "</b>\n\n" + esc(text), "parse_mode": "HTML"}, nil)
		if err != nil {
			retry(err)
			return
		}
		t, kb := b.customerScreen(ctx, id)
		finish("✅ "+b.tr("پیام ارسال شد.", "Message sent.")+"\n\n"+t, kb)
	case "q.note":
		c, _ := customerRow(id)
		c.TgId = id
		c.Note = text
		if text == "-" {
			c.Note = ""
		}
		if c.FirstSeen == 0 {
			c.FirstSeen = time.Now().Unix()
		}
		if err := saveCustomer(c); err != nil {
			retry(err)
			return
		}
		t, kb := b.customerScreen(ctx, id)
		finish(b.t("done")+"\n\n"+t, kb)
	case "q.min", "q.rate":
		n := int64(0)
		if v := strings.TrimSpace(latinDigits(text)); v != "0" {
			var ok bool
			if n, ok = parseMoney(text); !ok {
				retry(fmt.Errorf("%s", b.tr("یک عدد بفرستید", "send a number")))
				return
			}
		}
		if _, err := patchAdminMeta(id, func(m *model.BotAdminMeta) {
			if p.kind == "q.min" {
				m.MinPerGB = n
			} else {
				m.RatePerGB = n
			}
		}); err != nil {
			retry(err)
			return
		}
		t, kb := b.sellerScreen(ctx, id)
		finish(b.t("done")+"\n\n"+t, kb)
	case "q.bc":
		if text == "" {
			retry(fmt.Errorf("%s", b.tr("خالی بود", "it was empty")))
			return
		}
		aud := p.data["aud"]
		n := len(b.audience(aud))
		b.drafts.Store(chatID, broadcastDraft{aud: aud, text: text})
		finish("📣 "+b.tr("پیش‌نمایش:", "Preview:")+"\n\n📣 "+esc(text)+"\n\n"+b.tr("گیرنده‌ها: ", "Recipients: ")+esc(b.audienceLabel(ctx, aud))+fmt.Sprintf(" (%d)", n),
			[][]button{{{Text: b.tr(fmt.Sprintf("✅ ارسال به %d نفر", n), fmt.Sprintf("✅ Send to %d", n)), Data: "q:bcy"}, {Text: b.t("btnCancel"), Data: "q:bcn"}}})
	default:
		b.pend.clear(chatID)
	}
}
