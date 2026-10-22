package tgbot

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/logger"
	"github.com/Danialrostamani/drnetwork-panel/service"
)

const (
	orderPending   = "pending"
	orderApproved  = "approved"
	orderRejected  = "rejected"
	orderCancelled = "cancelled"
)

func (b *bot) staffIDs() []int64 {
	a := b.access()
	if a.owner != 0 {
		return []int64{a.owner}
	}
	var ids []int64
	for _, id := range a.order {
		if a.roleOf(id).full() {
			ids = append(ids, id)
		}
	}
	return ids
}

func (b *bot) staffBot() *bot {
	ids := b.staffIDs()
	if len(ids) > 0 {
		return b.as(ids[0])
	}
	return b
}

// sellerGroup is the group a seller other than the owner is limited to, or "".
func (b *bot) sellerGroup(id int64) string {
	a := b.access()
	if id == 0 || !a.isMember(id) || a.isOwner(id) {
		return ""
	}
	return a.roleOf(id).group
}

func (b *bot) canApprove() bool {
	if b.access().owner != 0 {
		return b.owner
	}
	return !b.restricted()
}

func (b *bot) notifyStaff(ctx context.Context, text string, kb [][]button) {
	b.notifyIDs(ctx, b.staffIDs(), text, kb)
}

func (b *bot) notifyOrderStaff(ctx context.Context, o *model.BotOrder, text string, kb [][]button) {
	seen := map[int64]bool{}
	var ids []int64
	add := func(id int64) {
		if id == 0 || seen[id] {
			return
		}
		seen[id] = true
		ids = append(ids, id)
	}
	add(o.AdminId)
	if a := b.access(); a.owner != 0 {
		add(a.owner)
	}
	if len(ids) == 0 {
		for _, id := range b.staffIDs() {
			add(id)
		}
	}
	b.notifyIDs(ctx, ids, text, kb)
}

func (b *bot) notifyIDs(ctx context.Context, ids []int64, text string, kb [][]button) {
	staff := b.staffBot()
	for _, id := range ids {
		if id == 0 {
			continue
		}
		staff.sendKeyboard(ctx, id, text, kb)
	}
}

func (b *bot) saveOrder(o *model.BotOrder) error {
	now := time.Now().Unix()
	if o.CreatedAt == 0 {
		o.CreatedAt = now
	}
	o.UpdatedAt = now
	if o.Id == 0 {
		return database.GetDB().Create(o).Error
	}
	return database.GetDB().Save(o).Error
}

func loadOrder(id uint) (*model.BotOrder, error) {
	var o model.BotOrder
	if err := database.GetDB().First(&o, id).Error; err != nil {
		return nil, err
	}
	return &o, nil
}

func customerOrders(tgID int64) []model.BotOrder {
	var out []model.BotOrder
	database.GetDB().Where("tg_id = ?", tgID).Order("id desc").Limit(20).Find(&out)
	return out
}

func (b *bot) orderDetail(topic, planID, extra string, from *fromUser) string {
	s := b.loadShop()
	detail := ""
	switch topic {
	case "buy", "renew", "test":
		if p := s.plan(planID); p != nil {
			detail = p.label()
		} else if planID != "" {
			detail = planID
		}
		if extra != "" {
			if c := clientByIDForUser(from.ID, extra); c != nil {
				detail = strings.TrimSpace(c.Name + " → " + detail)
			}
		}
	case "wallet":
		if w := s.wallet(planID); w != nil {
			detail = nonempty(w.Label, w.Amount)
		} else {
			detail = planID
		}
	case "bind":
		if extra != "" {
			var c model.Client
			if database.GetDB().First(&c, extra).Error == nil {
				detail = c.Name
			} else {
				detail = extra
			}
		}
	}
	return detail
}

func (b *bot) placeOrder(ctx context.Context, chatID int64, from *fromUser, topic, planID, extra string) {
	if (topic == "buy" || topic == "test") && b.customerNeedsRef(from.ID) {
		b.customerAskRef(ctx, chatID, from, topic)
		return
	}
	ref := ""
	if stored := loadBotRef(from.ID); stored != nil {
		ref = stored.Word
	}
	adminID := b.customerAdminID(from.ID)
	o := &model.BotOrder{
		TgId: from.ID, Topic: topic, PlanID: planID, Extra: extra, Ref: ref,
		AdminId: adminID, Hint: clientNameHint(from),
		Detail: b.orderDetail(topic, planID, extra, from), Status: orderPending,
	}
	// The shop of the admin who will approve the order, the same one
	// approveOrder reads, so the customer pays for what is then delivered.
	s := b.orderShop(o)
	switch topic {
	case "buy", "renew":
		p := s.plan(planID)
		if p == nil {
			b.sendMenu(ctx, chatID, b.tr(
				"❌ این پلن دیگر در فروشگاه نیست. لطفاً دوباره پلن را انتخاب کنید.",
				"❌ That plan is no longer in the shop. Please pick a plan again."), b.customerMenu(from.ID))
			return
		}
		if topic == "renew" && clientByIDForUser(from.ID, extra) == nil {
			b.sendMenu(ctx, chatID, b.tr("❌ سرویس پیدا نشد.", "❌ Service not found."), b.customerMenu(from.ID))
			return
		}
		o.PlanSet, o.PlanGB, o.PlanDays, o.PlanGroup = true, p.GB, p.Days, p.Group
		if n, ok := parseMoney(p.Price); ok {
			o.Amount = n
		}
		if min := b.sellerMinPrice(o.AdminId, p.GB); min > 0 && o.Amount > 0 && o.Amount < min {
			// The seller prices under what the owner allows: the sale waits
			// until the plan is fixed.
			b.sendMenu(ctx, chatID, b.tr(
				"❌ این پلن فعلاً قابل خرید نیست. لطفاً پلن دیگری انتخاب کنید یا به پشتیبانی پیام دهید.",
				"❌ This plan cannot be bought right now. Pick another plan or message support."), b.customerMenu(from.ID))
			b.notifyIDs(ctx, b.sellerAndOwner(o.AdminId), b.tr(
				fmt.Sprintf("⚠️ قیمت پلن «%s» (%s) کمتر از حداقل مجاز است (%s برای %d گیگ). خرید مشتری <code>%d</code> انجام نشد؛ قیمت را اصلاح کنید.", esc(p.Name), esc(p.Price), formatMoney(min), p.GB, from.ID),
				fmt.Sprintf("⚠️ The price of plan “%s” (%s) is under the allowed minimum (%s for %d GB). Customer <code>%d</code> could not buy it; fix the price.", esc(p.Name), esc(p.Price), formatMoney(min), p.GB, from.ID)), nil)
			return
		}
	case "wallet":
		w := s.wallet(planID)
		n, ok := int64(0), false
		if w != nil {
			n, ok = parseMoney(nonempty(w.Amount, w.Label))
		}
		if !ok {
			b.sendMenu(ctx, chatID, b.tr(
				"❌ این مبلغ شارژ در فروشگاه نیست. لطفاً دوباره انتخاب کنید.",
				"❌ That top-up is not in the shop. Please pick again."), b.customerMenu(from.ID))
			return
		}
		o.Amount = n
	}
	if err := b.saveOrder(o); err != nil {
		b.sendMenu(ctx, chatID, b.tr("❌ ثبت درخواست ممکن نشد.", "❌ Could not save the request."), b.customerMenu(from.ID))
		return
	}
	if topic == "buy" || topic == "renew" || topic == "wallet" {
		b.sendPayScreen(ctx, chatID, from, o, s)
		return
	}
	b.finishPendingOrder(ctx, chatID, from, o)
}

func (b *bot) sendPayScreen(ctx context.Context, chatID int64, from *fromUser, o *model.BotOrder, s shopConfig) {
	b.pend.set(chatID, &pending{kind: "cu.receipt", chatID: chatID, data: map[string]string{"id": strconv.FormatUint(uint64(o.Id), 10)}})
	lines := []string{b.tr("💳 <b>پرداخت</b>", "💳 <b>Payment</b>")}
	if p := s.plan(o.PlanID); p != nil && (o.Topic == "buy" || o.Topic == "renew") {
		name := p.Name
		if name == "" {
			name = p.label()
		}
		lines = append(lines, b.tr("پلن: ", "Plan: ")+"<b>"+esc(name)+"</b>")
		if p.GB > 0 {
			lines = append(lines, b.tr("حجم: ", "Data: ")+esc(strconv.Itoa(p.GB)+" GB"))
		} else {
			lines = append(lines, b.tr("حجم: نامحدود", "Data: unlimited"))
		}
		if p.Days > 0 {
			lines = append(lines, b.tr("مدت: ", "Duration: ")+esc(strconv.Itoa(p.Days))+" "+b.tr("روز", "days"))
		}
		if strings.TrimSpace(p.Price) != "" {
			lines = append(lines, b.tr("قیمت: ", "Price: ")+"<b>"+esc(p.Price)+"</b>")
		}
	} else if o.Topic == "wallet" && o.Amount > 0 {
		lines = append(lines, b.tr("مبلغ شارژ: ", "Top-up: ")+"<b>"+esc(formatMoney(o.Amount))+"</b>")
	} else if o.Detail != "" {
		lines = append(lines, esc(o.Detail))
	}
	if strings.TrimSpace(s.PayHint) != "" {
		lines = append(lines, esc(s.PayHint))
	}
	if strings.TrimSpace(s.PayCard) != "" {
		lines = append(lines, b.tr("شماره کارت:", "Card number:")+"\n<code>"+esc(s.PayCard)+"</code>")
	}
	if strings.TrimSpace(s.PayWallet) != "" {
		lines = append(lines, b.tr("کیف پول:", "Wallet:")+"\n<code>"+esc(s.PayWallet)+"</code>")
	}
	if strings.TrimSpace(s.PayCard) == "" && strings.TrimSpace(s.PayWallet) == "" {
		lines = append(lines, b.tr("شماره کارت در فروشگاه ثبت نشده. از پشتیبانی بپرسید.", "No card number is set in the shop. Ask support."))
	}
	lines = append(lines, b.tr(
		"مبلغ را واریز کنید، بعد <b>عکس رسید</b> را بفرستید.\nبعد از تأیید، لینک اشتراک برایتان ارسال می‌شود.",
		"Pay the amount, then send a <b>photo of the receipt</b>.\nAfter approval, the subscription link is sent to you."))
	if o.Topic == "wallet" {
		lines[len(lines)-1] = b.tr(
			"مبلغ را واریز کنید، بعد <b>عکس رسید</b> را بفرستید.\nبعد از تأیید، به موجودی کیف پول اضافه می‌شود.",
			"Pay the amount, then send a <b>photo of the receipt</b>.\nAfter approval, it is added to your wallet balance.")
	}
	var kb [][]button
	if o.Topic == "buy" || o.Topic == "renew" {
		if bal := walletBalance(o.TgId); bal > 0 {
			lines = append(lines, b.tr("موجودی کیف پول: ", "Wallet balance: ")+"<b>"+esc(formatMoney(bal))+"</b>")
			if o.Amount > 0 && bal >= o.Amount {
				kb = append(kb, []button{{Text: b.tr("💰 پرداخت از کیف پول", "💰 Pay from wallet"), Data: fmt.Sprintf("u:wp:%d", o.Id)}})
			}
		}
	}
	kb = append(kb,
		[]button{{Text: b.tr("📤 ارسال رسید", "📤 Send receipt"), Data: fmt.Sprintf("u:pay:%d", o.Id)}},
		[]button{{Text: b.tr("❌ لغو سفارش", "❌ Cancel order"), Data: fmt.Sprintf("u:cx:%d", o.Id)}},
	)
	b.sendKeyboard(ctx, chatID, strings.Join(lines, "\n"), kb)
}

func (b *bot) finishPendingOrder(ctx context.Context, chatID int64, from *fromUser, o *model.BotOrder) {
	text := b.customerNotice(from, o.Topic, o.Detail)
	if o.ReceiptID != "" {
		text += "\n" + b.tr("📎 رسید پیوست شده", "📎 Receipt attached")
	}
	if o.Ref != "" {
		text += "\n" + b.tr("معرف: ", "Introducer: ") + esc(o.Ref)
	}
	kb := [][]button{{
		{Text: b.tr("✅ تأیید", "✅ Approve"), Data: fmt.Sprintf("j:ok:%d", o.Id)},
		{Text: b.tr("❌ رد", "❌ Reject"), Data: fmt.Sprintf("j:no:%d", o.Id)},
	}}
	ownerOnly := b.ownerApproves(o.AdminId)
	ids := []int64{}
	seen := map[int64]bool{}
	add := func(id int64) {
		if id == 0 || seen[id] {
			return
		}
		seen[id] = true
		ids = append(ids, id)
	}
	add(o.AdminId)
	if a := b.access(); a.owner != 0 {
		add(a.owner)
	}
	if len(ids) == 0 {
		ids = b.staffIDs()
	}
	for _, id := range ids {
		if ownerOnly && id == o.AdminId {
			// This seller's sales are approved by the owner alone.
			b.notifyIDs(ctx, []int64{id}, text+"\n"+b.tr("🔒 تأیید این سفارش با مالک است.", "🔒 The owner approves this order."), nil)
			continue
		}
		b.notifyIDs(ctx, []int64{id}, text, kb)
	}
	if o.ReceiptID != "" {
		staff := b.staffBot()
		for _, id := range ids {
			staff.sendReceipt(ctx, id, o.ReceiptID)
		}
	}
	done := b.tr(
		"✅ رسید ثبت شد. بعد از تأیید، لینک اشتراک برایتان می‌آید.\nتا وقتی تأیید نشده می‌توانید سفارش را لغو کنید.",
		"✅ Receipt saved. After approval, the subscription link is sent to you.\nYou can cancel the order until it is approved.")
	if o.Topic == "wallet" {
		done = b.tr(
			"✅ رسید ثبت شد. بعد از تأیید، مبلغ به کیف پول شما اضافه می‌شود.\nتا وقتی تأیید نشده می‌توانید سفارش را لغو کنید.",
			"✅ Receipt saved. After approval, the amount is added to your wallet.\nYou can cancel the order until it is approved.")
	}
	b.sendKeyboard(ctx, chatID, done,
		[][]button{{{Text: b.tr("❌ لغو سفارش", "❌ Cancel order"), Data: fmt.Sprintf("u:cx:%d", o.Id)}}})
}

func (b *bot) askReceipt(ctx context.Context, chatID int64, from *fromUser, orderID uint) {
	o, err := loadOrder(orderID)
	if err != nil || o.TgId != from.ID {
		b.sendMenu(ctx, chatID, b.tr("❌ سفارش پیدا نشد.", "❌ Order not found."), b.customerMenu(from.ID))
		return
	}
	if o.Status != orderPending {
		b.sendMenu(ctx, chatID, b.tr("این سفارش دیگر در انتظار نیست.", "That order is no longer pending."), b.customerMenu(from.ID))
		return
	}
	b.pend.set(chatID, &pending{kind: "cu.receipt", chatID: chatID, data: map[string]string{"id": strconv.FormatUint(uint64(orderID), 10)}})
	b.sendMenu(ctx, chatID, b.tr(
		"📷 عکس یا فایل رسید را بفرستید.",
		"📷 Send a photo or file of the receipt."), b.customerMenu(from.ID))
}

// receiptDocPrefix marks a receipt sent as a file, so staff get it back the
// same way; a bare ID is a photo.
const receiptDocPrefix = "doc:"

// sendReceipt shows staff a customer's receipt once, as the photo or file it
// was sent as. Receipts saved by older versions carry no kind: they are
// tried as a photo, then as a file.
func (b *bot) sendReceipt(ctx context.Context, chatID int64, receipt string) {
	if id, ok := strings.CutPrefix(receipt, receiptDocPrefix); ok {
		_ = b.call(ctx, "sendDocument", map[string]any{"chat_id": chatID, "document": id}, nil)
		return
	}
	if err := b.call(ctx, "sendPhoto", map[string]any{"chat_id": chatID, "photo": receipt}, nil); err != nil {
		_ = b.call(ctx, "sendDocument", map[string]any{"chat_id": chatID, "document": receipt}, nil)
	}
}

func (b *bot) customerReceipt(ctx context.Context, chatID int64, from *fromUser, fileID string) {
	p := b.pend.get(chatID)
	b.pend.clear(chatID)
	if p == nil || p.data == nil {
		return
	}
	id64, _ := strconv.ParseUint(p.data["id"], 10, 32)
	o, err := loadOrder(uint(id64))
	if err != nil || o.TgId != from.ID {
		b.sendMenu(ctx, chatID, b.tr("❌ سفارش پیدا نشد.", "❌ Order not found."), b.customerMenu(from.ID))
		return
	}
	if o.Status != orderPending {
		b.sendMenu(ctx, chatID, b.tr("این سفارش دیگر در انتظار نیست.", "That order is no longer pending."), b.customerMenu(from.ID))
		return
	}
	o.ReceiptID = fileID
	_ = b.saveOrder(o)
	b.finishPendingOrder(ctx, chatID, from, o)
}

// payFromWallet pays a pending buy or renew from the customer's balance and
// delivers it at once. When delivery fails the price goes back to the
// balance and the order stays pending, to be paid by card instead.
func (b *bot) payFromWallet(ctx context.Context, chatID int64, from *fromUser, orderID uint) {
	o, err := loadOrder(orderID)
	if err != nil || o.TgId != from.ID {
		b.sendMenu(ctx, chatID, b.tr("❌ سفارش پیدا نشد.", "❌ Order not found."), b.customerMenu(from.ID))
		return
	}
	if o.Status != orderPending || o.WalletPaid || (o.Topic != "buy" && o.Topic != "renew") || o.Amount <= 0 {
		b.sendMenu(ctx, chatID, b.tr("این سفارش با کیف پول قابل پرداخت نیست.", "That order cannot be paid from the wallet."), b.customerMenu(from.ID))
		return
	}
	ok, err := walletSpend(o.TgId, o.Amount, walletMemo{kind: walletPay, order: o.Id})
	if err != nil || !ok {
		b.sendMenu(ctx, chatID, b.tr(
			"❌ موجودی کیف پول کافی نیست. موجودی: ",
			"❌ The wallet balance is not enough. Balance: ")+esc(formatMoney(walletBalance(o.TgId))), b.customerMenu(from.ID))
		return
	}
	if p := b.pend.get(chatID); p != nil && p.kind == "cu.receipt" {
		b.pend.clear(chatID)
	}
	o.WalletPaid = true
	if err := b.approveOrder(o); err != nil {
		_, _ = walletCredit(o.TgId, o.Amount, walletMemo{kind: walletRefund, order: o.Id})
		if fresh, lerr := loadOrder(o.Id); lerr == nil {
			fresh.WalletPaid = false
			_ = b.saveOrder(fresh)
		}
		msg := b.tr("❌ تحویل سفارش ممکن نشد و مبلغ به کیف پول برگشت. می‌توانید با کارت پرداخت کنید یا به پشتیبانی پیام دهید.",
			"❌ The order could not be delivered and the amount went back to your wallet. You can pay by card or message support.")
		var qe *quotaError
		if errors.As(err, &qe) {
			msg = b.tr("❌ سهمیه فروشنده کافی نیست؛ مبلغ به کیف پول برگشت. لطفاً بعداً تلاش کنید یا به پشتیبانی پیام دهید.",
				"❌ The seller's quota is not enough; the amount went back to your wallet. Try later or message support.")
		}
		b.sendMenu(ctx, chatID, msg, b.customerMenu(from.ID))
		return
	}
	b.notifyOrderStaff(ctx, o, b.tr("💰 فروش با کیف پول (خودکار تأیید شد)", "💰 Sold from wallet (approved automatically)")+
		"\n"+esc(o.Detail)+"\n"+b.tr("مبلغ: ", "Amount: ")+esc(formatMoney(o.Amount))+fmt.Sprintf("\n<code>%d</code>", o.TgId), nil)
	b.notifyCustomerOrder(ctx, o, true)
	b.maybeQuotaWarn(ctx, o.AdminId)
}

func (b *bot) orderCallback(ctx context.Context, cbID string, chatID, msgID int64, parts []string) {
	if len(parts) < 3 {
		b.answer(ctx, cbID, b.denied())
		return
	}
	id64, _ := strconv.ParseUint(parts[2], 10, 32)
	o, err := loadOrder(uint(id64))
	if err != nil {
		b.answer(ctx, cbID, "")
		return
	}
	if !b.canApprove() && (o.AdminId == 0 || b.self != o.AdminId) {
		b.answer(ctx, cbID, b.denied())
		return
	}
	if o.Status != orderPending {
		b.answer(ctx, cbID, "")
		b.edit(ctx, chatID, msgID, b.tr("قبلاً رسیدگی شده.", "Already handled."), nil)
		return
	}
	if parts[1] == "ok" && !b.canApprove() && b.ownerApproves(b.self) {
		b.answer(ctx, cbID, b.tr("تأیید سفارش‌های شما با مالک است.", "The owner approves your orders."))
		return
	}
	switch parts[1] {
	case "ok":
		if err := b.approveOrder(o); err != nil {
			b.answer(ctx, cbID, "")
			b.edit(ctx, chatID, msgID, b.tr("❌ تأیید نشد: ", "❌ Approve failed: ")+esc(err.Error()), nil)
			return
		}
		b.answer(ctx, cbID, "")
		b.edit(ctx, chatID, msgID, b.tr("✅ تأیید شد.", "✅ Approved.")+"\n"+esc(o.Detail), nil)
		b.notifyCustomerOrder(ctx, o, true)
		b.maybeQuotaWarn(ctx, o.AdminId)
		b.tellOwnerApproval(ctx, o)
	case "no":
		o.Status = orderRejected
		if err := b.saveOrder(o); err == nil && o.WalletPaid && o.Amount > 0 {
			_, _ = walletCredit(o.TgId, o.Amount, walletMemo{kind: walletRefund, order: o.Id, actor: b.self})
		}
		b.answer(ctx, cbID, "")
		b.edit(ctx, chatID, msgID, b.tr("❌ رد شد.", "❌ Rejected."), nil)
		b.notifyCustomerOrder(ctx, o, false)
	default:
		b.answer(ctx, cbID, "")
	}
}

func (b *bot) notifyCustomerOrder(ctx context.Context, o *model.BotOrder, ok bool) {
	if !ok {
		msg := b.tr("❌ درخواست شما رد شد.", "❌ Your request was rejected.")
		if o.WalletPaid && o.Amount > 0 {
			msg += "\n" + b.tr("مبلغ به کیف پول شما برگشت.", "The amount went back to your wallet.")
		}
		b.sendMenu(ctx, o.TgId, msg, b.customerMenu(o.TgId))
		return
	}
	msg := b.tr("✅ درخواست شما تأیید شد.", "✅ Your request was approved.")
	if o.Topic == "wallet" {
		msg = b.tr("✅ کیف پول شما شارژ شد: ", "✅ Your wallet was topped up: ") + "<b>" + esc(formatMoney(o.Amount)) + "</b>\n" +
			b.tr("موجودی: ", "Balance: ") + "<b>" + esc(formatMoney(walletBalance(o.TgId))) + "</b>"
		b.sendMenu(ctx, o.TgId, msg, b.customerMenu(o.TgId))
		return
	}
	if o.ClientId != 0 {
		var c model.Client
		if database.GetDB().First(&c, o.ClientId).Error == nil {
			if link, err := b.subLink(c.Name); err == nil {
				msg += "\n<code>" + esc(link) + "</code>" + b.singBoxLine(link)
			}
		}
		b.sendMenu(ctx, o.TgId, msg, b.customerMenu(o.TgId))
		b.customerServices(ctx, o.TgId, o.TgId)
		return
	}
	b.sendMenu(ctx, o.TgId, msg, b.customerMenu(o.TgId))
}

func (b *bot) orderShop(o *model.BotOrder) shopConfig {
	s := b.loadShopRaw()
	if o != nil && o.AdminId != 0 {
		return s.forAdmin(o.AdminId)
	}
	return s
}

func orderPlan(o *model.BotOrder, s shopConfig) (gb, days int, group string, ok bool) {
	if o.PlanSet {
		return o.PlanGB, o.PlanDays, o.PlanGroup, true
	}
	if p := s.plan(o.PlanID); p != nil {
		return p.GB, p.Days, p.Group, true
	}
	return 0, 0, "", false
}

func (b *bot) approveOrder(o *model.BotOrder) error {
	staff := b.staffBot()
	s := b.orderShop(o)
	switch o.Topic {
	case "buy", "test":
		gb, days, hours, group := 0, 0, 0, ""
		if o.Topic == "test" {
			if hasUsedTrial(o.TgId) {
				return errors.New(b.tr("این مشتری قبلاً اکانت تست گرفته است", "this customer already had a test account"))
			}
			gb, hours, group = s.TrialGB, s.TrialHours, s.TrialGroup
			if gb <= 0 {
				gb = 1
			}
			if hours <= 0 {
				hours = 24
			}
		} else {
			var ok bool
			if gb, days, group, ok = orderPlan(o, s); !ok {
				return errors.New(b.tr("پلن این سفارش دیگر در فروشگاه نیست", "the plan of this order is no longer in the shop"))
			}
		}
		if g := s.refGroup(o.Ref); g != "" {
			group = g
		}
		// A seller limited to a group sells into it, or they could not see
		// what they sold.
		if g := b.sellerGroup(o.AdminId); g != "" {
			group = g
		}
		name := staff.uniqueClientName(o.TgId, o.Hint)
		vol := int64(0)
		if gb > 0 {
			vol = int64(gb) * gib
		}
		charged, err := b.saleQuota(o.AdminId, vol, true)
		if err != nil {
			return err
		}
		if err := staff.createClient(name, group, vol, days, 0, false); err != nil {
			if charged {
				_ = addQuotaSold(o.AdminId, -vol)
			}
			return err
		}
		var c model.Client
		if err := database.GetDB().Where("name = ?", name).First(&c).Error; err != nil {
			return err
		}
		if charged {
			b.markPrepaid(o.AdminId, c.Id)
		}
		if hours > 0 {
			_ = staff.editClient(c.Id, func(cl *model.Client) error {
				cl.Expiry = time.Now().Add(time.Duration(hours) * time.Hour).Unix()
				return nil
			})
		}
		_ = staff.bindClient(c.Id, o.TgId)
		o.ClientId = c.Id
	case "renew":
		c := clientByIDForUser(o.TgId, o.Extra)
		if c == nil {
			return fmt.Errorf("client not found")
		}
		gb, days, _, ok := orderPlan(o, s)
		if !ok {
			return errors.New(b.tr("پلن این سفارش دیگر در فروشگاه نیست", "the plan of this order is no longer in the shop"))
		}
		need := int64(0)
		if gb > 0 {
			need = int64(gb) * gib
		}
		charged := false
		if need > 0 {
			// A client already counted by its traffic uses the limit as it
			// is used; charging the plan as well would count it twice.
			metered, err := service.BotQuotaMetered(database.GetDB(), o.AdminId, c.Id)
			if err != nil {
				return err
			}
			if charged, err = b.saleQuota(o.AdminId, need, !metered); err != nil {
				return err
			}
		}
		undo := func() {
			if charged {
				_ = addQuotaSold(o.AdminId, -need)
			}
		}
		if days > 0 {
			if err := staff.addDays(c.Id, days); err != nil {
				undo()
				return err
			}
		}
		if need > 0 {
			if err := staff.addVolume(c.Id, need); err != nil {
				undo()
				return err
			}
		}
		if charged {
			b.markPrepaid(o.AdminId, c.Id)
		}
		o.ClientId = c.Id
	case "wallet":
		amount := o.Amount
		if amount <= 0 {
			if w := s.wallet(o.PlanID); w != nil {
				amount, _ = parseMoney(nonempty(w.Amount, w.Label))
			}
		}
		if amount <= 0 {
			return errors.New(b.tr("مبلغ شارژ مشخص نیست", "the top-up amount is unknown"))
		}
		o.Amount = amount
		o.Status = orderApproved
		if err := b.saveOrder(o); err != nil {
			return err
		}
		if _, err := walletCredit(o.TgId, amount, walletMemo{kind: walletTopUp, order: o.Id, actor: b.self}); err != nil {
			o.Status = orderPending
			_ = b.saveOrder(o)
			return err
		}
		return nil
	case "bind":
		id64, _ := strconv.ParseUint(o.Extra, 10, 32)
		var c model.Client
		if err := database.GetDB().First(&c, uint(id64)).Error; err != nil {
			return fmt.Errorf("client not found")
		}
		if c.TgId != 0 && c.TgId != o.TgId {
			return fmt.Errorf("already bound")
		}
		if err := staff.bindClient(c.Id, o.TgId); err != nil {
			return err
		}
		o.ClientId = c.Id
	default:
		return fmt.Errorf("unknown order type %q", o.Topic)
	}
	o.Status = orderApproved
	return b.saveOrder(o)
}

// markPrepaid keeps a sold client's traffic out of the admin's limit, since
// its plan volume was already taken from it.
func (b *bot) markPrepaid(adminID int64, clientID uint) {
	if err := service.BotQuotaPrepaid(database.GetDB(), adminID, clientID); err != nil {
		logger.Warning("telegram bot: mark sold client ", clientID, " as prepaid: ", err)
	}
}

func (b *bot) autoTrial(ctx context.Context, chatID int64, from *fromUser) {
	if b.customerNeedsRef(from.ID) {
		b.customerAskRef(ctx, chatID, from, "trial")
		return
	}
	if hasUsedTrial(from.ID) {
		b.sendMenu(ctx, chatID, b.tr(
			"🔑 اکانت تست فقط یک‌بار داده می‌شود. از «خرید اشتراک» استفاده کنید یا به پشتیبانی پیام دهید.",
			"🔑 A test account is given only once. Use Buy a plan or message support."), b.customerMenu(from.ID))
		return
	}
	if len(boundClients(from.ID)) > 0 {
		b.sendMenu(ctx, chatID, b.tr(
			"🛍️ همین حالا سرویس دارید. از «سرویس های من» ببینید.",
			"🛍️ You already have a service. See My services."), b.customerMenu(from.ID))
		return
	}
	ref := ""
	if stored := loadBotRef(from.ID); stored != nil {
		ref = stored.Word
	}
	o := &model.BotOrder{TgId: from.ID, Topic: "test", Status: orderPending, Detail: b.tr("اکانت تست", "Test account"), Ref: ref, AdminId: b.customerAdminID(from.ID), Hint: clientNameHint(from)}
	if err := b.saveOrder(o); err != nil {
		b.sendMenu(ctx, chatID, b.tr("❌ ثبت درخواست ممکن نشد.", "❌ Could not save the request."), b.customerMenu(from.ID))
		return
	}
	if err := b.approveOrder(o); err != nil {
		o.Status = orderRejected
		_ = b.saveOrder(o)
		msg := b.tr("❌ ساخت اکانت تست ممکن نشد.", "❌ Could not create a test account.")
		var qe *quotaError
		if errors.As(err, &qe) {
			msg = b.tr(
				"❌ سهمیه فروشنده تمام است؛ الان اکانت تست داده نمی‌شود. کمی بعد دوباره تلاش کنید یا به پشتیبانی پیام دهید.",
				"❌ The seller's volume quota is used up, so a test account cannot be created now. Try again later or message support.")
		}
		b.sendMenu(ctx, chatID, msg, b.customerMenu(from.ID))
		return
	}
	b.notifyCustomerOrder(ctx, o, true)
	b.maybeQuotaWarn(ctx, o.AdminId)
}

func (b *bot) orderStatusLines(tgID int64) string {
	orders := customerOrders(tgID)
	if len(orders) == 0 {
		return ""
	}
	lines := []string{b.tr("📋 <b>درخواست‌ها</b>", "📋 <b>Requests</b>")}
	for i, o := range orders {
		if i >= 5 {
			break
		}
		st := o.Status
		switch o.Status {
		case orderPending:
			st = b.tr("در انتظار", "pending")
		case orderApproved:
			st = b.tr("تأیید شد", "approved")
		case orderRejected:
			st = b.tr("رد شد", "rejected")
		case orderCancelled:
			st = b.tr("لغو شد", "cancelled")
		}
		line := "• " + st
		if o.Detail != "" {
			line += " — " + o.Detail
		}
		lines = append(lines, esc(line))
	}
	return strings.Join(lines, "\n")
}
