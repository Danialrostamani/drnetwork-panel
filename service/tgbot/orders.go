package tgbot

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
)

const (
	orderPending  = "pending"
	orderApproved = "approved"
	orderRejected = "rejected"
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

func (b *bot) canApprove() bool {
	if b.access().owner != 0 {
		return b.owner
	}
	return !b.restricted()
}

func (b *bot) notifyStaff(ctx context.Context, text string, kb [][]button) {
	staff := b.staffBot()
	for _, id := range b.staffIDs() {
		if id == 0 {
			continue
		}
		staff.sendKeyboard(ctx, id, text, kb)
	}
}

func (b *bot) uniqueClientName(tgID int64) string {
	base := "u" + strconv.FormatInt(tgID, 10)
	name := base
	for i := 2; ; i++ {
		var n int64
		database.GetDB().Model(&model.Client{}).Where("name = ?", name).Count(&n)
		if n == 0 {
			return name
		}
		name = base + "_" + strconv.Itoa(i)
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
	o := &model.BotOrder{
		TgId: from.ID, Topic: topic, PlanID: planID, Extra: extra, Ref: ref,
		Detail: b.orderDetail(topic, planID, extra, from), Status: orderPending,
	}
	if err := b.saveOrder(o); err != nil {
		b.sendMenu(ctx, chatID, b.tr("❌ ثبت درخواست ممکن نشد.", "❌ Could not save the request."), b.customerMenu(from.ID))
		return
	}
	s := b.loadShop()
	if s.hasPayment() && (topic == "buy" || topic == "renew" || topic == "wallet") {
		b.sendPayScreen(ctx, chatID, from, o, s)
		return
	}
	b.finishPendingOrder(ctx, chatID, from, o)
}

func (b *bot) sendPayScreen(ctx context.Context, chatID int64, from *fromUser, o *model.BotOrder, s shopConfig) {
	lines := []string{b.tr("💳 <b>پرداخت</b>", "💳 <b>Payment</b>")}
	if s.PayHint != "" {
		lines = append(lines, esc(s.PayHint))
	}
	if s.PayCard != "" {
		lines = append(lines, b.tr("کارت: ", "Card: ")+"<code>"+esc(s.PayCard)+"</code>")
	}
	if s.PayWallet != "" {
		lines = append(lines, b.tr("کیف پول: ", "Wallet: ")+"<code>"+esc(s.PayWallet)+"</code>")
	}
	if o.Detail != "" {
		lines = append(lines, esc(o.Detail))
	}
	kb := [][]button{
		{{Text: b.tr("📤 ارسال رسید", "📤 Send receipt"), Data: fmt.Sprintf("u:pay:%d", o.Id)}},
		{{Text: b.tr("⏭ بدون رسید", "⏭ Request without receipt"), Data: fmt.Sprintf("u:skip:%d", o.Id)}},
	}
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
	b.notifyStaff(ctx, text, kb)
	if o.ReceiptID != "" {
		staff := b.staffBot()
		for _, id := range b.staffIDs() {
			_ = staff.call(ctx, "sendPhoto", map[string]any{"chat_id": id, "photo": o.ReceiptID}, nil)
			_ = staff.call(ctx, "sendDocument", map[string]any{"chat_id": id, "document": o.ReceiptID}, nil)
		}
	}
	b.sendMenu(ctx, chatID, b.tr(
		"✅ درخواست ثبت شد. وضعیت را در «سرویس های من» ببینید.",
		"✅ Your request was sent. You can see its status under My services."), b.customerMenu(from.ID))
}

func (b *bot) askReceipt(ctx context.Context, chatID int64, from *fromUser, orderID uint) {
	b.pend.set(chatID, &pending{kind: "cu.receipt", chatID: chatID, data: map[string]string{"id": strconv.FormatUint(uint64(orderID), 10)}})
	b.sendMenu(ctx, chatID, b.tr(
		"📷 عکس یا فایل رسید را بفرستید.",
		"📷 Send a photo or file of the receipt."), b.customerMenu(from.ID))
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
	o.ReceiptID = fileID
	_ = b.saveOrder(o)
	b.finishPendingOrder(ctx, chatID, from, o)
}

func (b *bot) skipReceipt(ctx context.Context, chatID int64, from *fromUser, orderID uint) {
	o, err := loadOrder(orderID)
	if err != nil || o.TgId != from.ID {
		return
	}
	b.finishPendingOrder(ctx, chatID, from, o)
}

func (b *bot) orderCallback(ctx context.Context, cbID string, chatID, msgID int64, parts []string) {
	if len(parts) < 3 || !b.canApprove() {
		b.answer(ctx, cbID, b.denied())
		return
	}
	id64, _ := strconv.ParseUint(parts[2], 10, 32)
	o, err := loadOrder(uint(id64))
	if err != nil {
		b.answer(ctx, cbID, "")
		return
	}
	if o.Status != orderPending {
		b.answer(ctx, cbID, "")
		b.edit(ctx, chatID, msgID, b.tr("قبلاً رسیدگی شده.", "Already handled."), nil)
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
	case "no":
		o.Status = orderRejected
		_ = b.saveOrder(o)
		b.answer(ctx, cbID, "")
		b.edit(ctx, chatID, msgID, b.tr("❌ رد شد.", "❌ Rejected."), nil)
		b.notifyCustomerOrder(ctx, o, false)
	default:
		b.answer(ctx, cbID, "")
	}
}

func (b *bot) notifyCustomerOrder(ctx context.Context, o *model.BotOrder, ok bool) {
	if ok {
		msg := b.tr("✅ درخواست شما تأیید شد.", "✅ Your request was approved.")
		if o.ClientId != 0 {
			var c model.Client
			if database.GetDB().First(&c, o.ClientId).Error == nil {
				if link, err := b.subLink(c.Name); err == nil {
					msg += "\n<code>" + esc(link) + "</code>"
				}
			}
			b.sendMenu(ctx, o.TgId, msg, b.customerMenu(o.TgId))
			b.customerServices(ctx, o.TgId, o.TgId, false)
			return
		}
		b.sendMenu(ctx, o.TgId, msg, b.customerMenu(o.TgId))
		return
	}
	b.sendMenu(ctx, o.TgId, b.tr("❌ درخواست شما رد شد.", "❌ Your request was rejected."), b.customerMenu(o.TgId))
}

func (b *bot) approveOrder(o *model.BotOrder) error {
	staff := b.staffBot()
	s := b.loadShop()
	switch o.Topic {
	case "buy", "test":
		gb, days, hours, group := 0, 0, 0, ""
		if o.Topic == "test" {
			gb, hours, group = s.TrialGB, s.TrialHours, s.TrialGroup
			if gb <= 0 {
				gb = 1
			}
			if hours <= 0 {
				hours = 24
			}
		} else if p := s.plan(o.PlanID); p != nil {
			gb, days, group = p.GB, p.Days, p.Group
		}
		if g := s.refGroup(o.Ref); g != "" {
			group = g
		}
		name := staff.uniqueClientName(o.TgId)
		vol := int64(0)
		if gb > 0 {
			vol = int64(gb) * gib
		}
		if err := staff.createClient(name, group, vol, days, 0); err != nil {
			return err
		}
		var c model.Client
		if err := database.GetDB().Where("name = ?", name).First(&c).Error; err != nil {
			return err
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
		if p := s.plan(o.PlanID); p != nil {
			if p.Days > 0 {
				if err := staff.addDays(c.Id, p.Days); err != nil {
					return err
				}
			}
			if p.GB > 0 {
				if err := staff.addVolume(c.Id, int64(p.GB)*gib); err != nil {
					return err
				}
			}
		}
		o.ClientId = c.Id
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
	}
	o.Status = orderApproved
	return b.saveOrder(o)
}

func (b *bot) autoTrial(ctx context.Context, chatID int64, from *fromUser) {
	if b.customerNeedsRef(from.ID) {
		b.customerAskRef(ctx, chatID, from, "trial")
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
	o := &model.BotOrder{TgId: from.ID, Topic: "test", Status: orderPending, Detail: b.tr("اکانت تست", "Test account"), Ref: ref}
	if err := b.saveOrder(o); err != nil {
		b.sendMenu(ctx, chatID, b.tr("❌ ثبت درخواست ممکن نشد.", "❌ Could not save the request."), b.customerMenu(from.ID))
		return
	}
	if err := b.approveOrder(o); err != nil {
		o.Status = orderRejected
		_ = b.saveOrder(o)
		b.sendMenu(ctx, chatID, b.tr("❌ ساخت اکانت تست ممکن نشد.", "❌ Could not create a test account.")+"\n"+esc(err.Error()), b.customerMenu(from.ID))
		return
	}
	b.notifyCustomerOrder(ctx, o, true)
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
		}
		line := "• " + st
		if o.Detail != "" {
			line += " — " + o.Detail
		}
		lines = append(lines, esc(line))
	}
	return strings.Join(lines, "\n")
}
