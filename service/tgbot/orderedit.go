package tgbot

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
)

// Order editing from Telegram: the corrections the panel's orders page makes
// (PanelEditOrder) - the amount, the description, the seller and the status -
// for the owner and the administrators who manage the bot (q: buttons).

func orderEditButton(id uint) button {
	return button{Text: fmt.Sprintf("✏️ #%d", id), Data: fmt.Sprintf("q:oe:%d", id)}
}

// orderEditRows are the ✏️ buttons of a list of orders, four to a row.
func orderEditRows(orders []model.BotOrder) [][]button {
	var rows [][]button
	var row []button
	for _, o := range orders {
		row = append(row, orderEditButton(o.Id))
		if len(row) == 4 {
			rows = append(rows, row)
			row = nil
		}
	}
	if len(row) > 0 {
		rows = append(rows, row)
	}
	return rows
}

func orderCanApproveFrom(st string) bool {
	return st == orderPending || st == orderRejected || st == orderCancelled
}

func orderCanRejectFrom(st string) bool {
	return st == orderPending || st == orderCancelled || st == orderApproved
}

func (b *bot) orderEditScreen(ctx context.Context, id uint) (string, [][]button) {
	o, err := loadOrder(id)
	if err != nil {
		return "❌ " + b.tr("سفارش پیدا نشد.", "Order not found."), [][]button{b.navRow("k:ords")}
	}
	lines := []string{b.header("✏️", b.tr("ویرایش سفارش", "Edit order")) + fmt.Sprintf(" #%d", o.Id),
		b.tr("وضعیت: ", "Status: ") + "<b>" + esc(b.orderStatusLabel(o.Status)) + "</b>",
		b.tr("نوع: ", "Type: ") + esc(b.orderTopicLabel(o.Topic)),
	}
	if o.Detail != "" {
		lines = append(lines, b.tr("توضیح: ", "Description: ")+esc(o.Detail))
	}
	amount := "—"
	if o.Amount > 0 {
		amount = formatMoney(o.Amount)
	}
	if o.WalletPaid {
		amount += " · 🏦 " + b.tr("از کیف پول", "from the wallet")
	}
	lines = append(lines, b.tr("مبلغ: ", "Amount: ")+esc(amount))
	if o.TgId != 0 {
		who := fmt.Sprintf("<code>%d</code>", o.TgId)
		if c, ok := customerRow(o.TgId); ok {
			if n := strings.TrimSpace(nonempty(c.Name, c.Username)); n != "" {
				who = esc(n) + " · " + who
			}
		}
		lines = append(lines, b.tr("مشتری: ", "Customer: ")+who)
	}
	if o.ClientId != 0 {
		var c model.Client
		if database.GetDB().Select("id", "name").First(&c, o.ClientId).Error == nil {
			lines = append(lines, b.tr("کلاینت: ", "Client: ")+"<code>"+esc(c.Name)+"</code>")
		}
	}
	lines = append(lines,
		b.tr("فروشنده: ", "Seller: ")+esc(b.sellerName(ctx, o.AdminId)),
		b.tr("ثبت: ", "Placed: ")+esc(b.stamp(o.CreatedAt)),
	)
	settled := o.SettlementId != 0
	if settled {
		lines = append(lines, "🔒 "+b.tr("این سفارش در تسویه با فروشنده است؛ فقط توضیحش قابل تغییر است.", "This order is in a settlement with the seller; only its description can change."))
	}
	sid := strconv.FormatUint(uint64(o.Id), 10)
	var kb [][]button
	if settled {
		kb = append(kb, []button{{Text: b.tr("📝 توضیح", "📝 Description"), Data: "q:oen:" + sid}})
	} else {
		kb = append(kb, []button{
			{Text: b.tr("💰 مبلغ", "💰 Amount"), Data: "q:oea:" + sid},
			{Text: b.tr("📝 توضیح", "📝 Description"), Data: "q:oen:" + sid},
		})
		kb = append(kb, []button{{Text: b.tr("👤 تغییر فروشنده", "👤 Change seller"), Data: "q:oes:" + sid}})
		var st []button
		if orderCanApproveFrom(o.Status) {
			st = append(st, button{Text: b.tr("✅ تأیید", "✅ Approve"), Data: "q:oet:" + sid + ":a"})
		}
		if orderCanRejectFrom(o.Status) {
			st = append(st, button{Text: b.tr("❌ رد", "❌ Reject"), Data: "q:oet:" + sid + ":r"})
		}
		if len(st) > 0 {
			kb = append(kb, st)
		}
	}
	if o.TgId != 0 {
		kb = append(kb, []button{{Text: b.tr("📋 سفارش‌های این مشتری", "📋 This customer's orders"), Data: fmt.Sprintf("q:co:%d", o.TgId)}})
	}
	kb = append(kb, b.navRow("k:ords"))
	return strings.Join(lines, "\n"), kb
}

// orderSellerScreen lists who the order can be given to.
func (b *bot) orderSellerScreen(ctx context.Context, id uint) (string, [][]button) {
	o, err := loadOrder(id)
	if err != nil {
		return "❌ " + b.tr("سفارش پیدا نشد.", "Order not found."), [][]button{b.navRow("k:ords")}
	}
	sid := strconv.FormatUint(uint64(id), 10)
	a := b.access()
	ids := []int64{0}
	for _, x := range a.order {
		if !a.isOwner(x) {
			ids = append(ids, x)
		}
	}
	names := b.adminNames(ctx, ids)
	var btns []button
	for _, x := range ids {
		label := b.tr("👑 مالک (بدون فروشنده)", "👑 Owner (no seller)")
		if x != 0 {
			label = nonempty(names[x], strconv.FormatInt(x, 10))
		}
		if x == o.AdminId {
			label = "• " + label
		}
		btns = append(btns, button{Text: label, Data: fmt.Sprintf("q:oesv:%s:%d", sid, x)})
	}
	text := b.header("👤", b.tr("فروشنده سفارش", "Order seller")) + fmt.Sprintf(" #%d", id) + "\n" +
		b.tr("فروشنده فعلی: ", "Current seller: ") + esc(b.sellerName(ctx, o.AdminId))
	return text, append(rows2(btns), b.navRow("q:oe:"+sid))
}

// orderStatusConfirm asks before a status change, which delivers or takes
// back what the order bought.
func (b *bot) orderStatusConfirm(id uint, to string) (string, [][]button) {
	sid := strconv.FormatUint(uint64(id), 10)
	text := "⚠️ " + b.tr(fmt.Sprintf("سفارش #%d تأیید شود؟ اکانت/تمدید/شارژ آن مثل تأیید عادی انجام می‌شود.", id), fmt.Sprintf("Approve order #%d? It is delivered as a normal approval would.", id))
	if to == "r" {
		text = "⚠️ " + b.tr(fmt.Sprintf("سفارش #%d رد شود؟ پرداخت از کیف پول برگردانده می‌شود.", id), fmt.Sprintf("Reject order #%d? A wallet payment is given back.", id))
	}
	return text, [][]button{
		{{Text: b.tr("✅ بله و به مشتری خبر بده", "✅ Yes, tell the customer"), Data: "q:oey:" + sid + ":" + to + ":1"}},
		{{Text: b.tr("✅ بله، بدون پیام", "✅ Yes, silently"), Data: "q:oey:" + sid + ":" + to + ":0"}},
		{{Text: b.t("btnCancel"), Data: "q:oe:" + sid}},
	}
}

// editOrder runs one change through PanelEditOrder, as the panel does.
func (b *bot) editOrder(ctx context.Context, e PanelOrderEdit) error {
	_, err := PanelEditOrder(ctx, b.configService, b.actor(), e)
	return err
}

// orderEditCallback handles q:oe* buttons. ask starts a text answer.
func (b *bot) orderEditCallback(ctx context.Context, cbID string, verb string, parts []string, show func(string, [][]button), ask func(kind string, id int64, back, prompt string, data map[string]string)) {
	arg := func(i int) string {
		if len(parts) > i {
			return parts[i]
		}
		return ""
	}
	n, _ := strconv.ParseUint(arg(2), 10, 64)
	id := uint(n)
	sid := strconv.FormatUint(n, 10)
	fail := func(err error) {
		b.answer(ctx, cbID, b.t("failed", b.errText(err)))
	}
	switch verb {
	case "oe":
		show(b.orderEditScreen(ctx, id))
	case "oea":
		cur := "—"
		if o, err := loadOrder(id); err == nil && o.Amount > 0 {
			cur = formatMoney(o.Amount)
		}
		ask("q.oamt", int64(id), "q:oe:"+sid, "💰 "+b.tr("مبلغ تازه سفارش را بفرستید (مثلاً 150000 یا ۱۵۰ هزار).", "Send the new amount of the order (e.g. 150000 or 150k).")+"\n"+b.tr("مبلغ فعلی: ", "Now: ")+cur, nil)
	case "oen":
		ask("q.onote", int64(id), "q:oe:"+sid, "📝 "+b.tr("توضیح تازه سفارش را بفرستید. برای پاک کردن «-» بفرستید.", "Send the new description of the order. Send \"-\" to clear it."), nil)
	case "oes":
		show(b.orderSellerScreen(ctx, id))
	case "oesv":
		seller, err := strconv.ParseInt(arg(3), 10, 64)
		if err != nil {
			b.answer(ctx, cbID, "")
			return
		}
		if err := b.editOrder(ctx, PanelOrderEdit{Id: id, Seller: &seller}); err != nil {
			fail(err)
			return
		}
		show(b.orderEditScreen(ctx, id))
	case "oet":
		show(b.orderStatusConfirm(id, arg(3)))
	case "oey":
		st := orderApproved
		if arg(3) == "r" {
			st = orderRejected
		}
		before, err := loadOrder(id)
		if err != nil {
			fail(err)
			return
		}
		// The customer hears from this bot, not the panel's copy of it.
		if err := b.editOrder(ctx, PanelOrderEdit{Id: id, Status: st}); err != nil {
			fail(err)
			return
		}
		if o, err := loadOrder(id); err == nil && o.Status != before.Status {
			if arg(4) == "1" && o.TgId != 0 && !customerBlocked(o.TgId) {
				b.notifyCustomerOrder(ctx, o, st == orderApproved)
			}
			if st == orderApproved {
				b.maybeQuotaWarn(ctx, o.AdminId)
			}
		}
		t, kb := b.orderEditScreen(ctx, id)
		show(b.t("done")+"\n\n"+t, kb)
	default:
		b.answer(ctx, cbID, "")
	}
}

// orderEditPending takes the typed answers of the order editor.
func (b *bot) orderEditPending(ctx context.Context, p *pending, text string, retry func(error), finish func(string, [][]button)) {
	n, _ := strconv.ParseUint(p.data["id"], 10, 64)
	id := uint(n)
	e := PanelOrderEdit{Id: id}
	switch p.kind {
	case "q.oamt":
		amount, ok := parseMoney(text)
		if strings.TrimSpace(latinDigits(text)) == "0" {
			amount, ok = 0, true
		}
		if !ok || amount < 0 {
			retry(fmt.Errorf("%s", b.tr("مبلغ نامعتبر است", "invalid amount")))
			return
		}
		e.Amount = &amount
	case "q.onote":
		d := strings.TrimSpace(text)
		if d == "-" {
			d = ""
		}
		e.Detail = &d
	}
	if err := b.editOrder(ctx, e); err != nil {
		retry(err)
		return
	}
	t, kb := b.orderEditScreen(ctx, id)
	finish(b.t("done")+"\n\n"+t, kb)
}
