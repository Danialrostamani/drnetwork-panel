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

func hasUsedTrial(tgID int64) bool {
	var n int64
	database.GetDB().Model(&model.BotOrder{}).
		Where("tg_id = ? AND topic = ? AND status = ?", tgID, "test", orderApproved).
		Count(&n)
	return n > 0
}

func (b *bot) orderAdminFilter(adminID int64) (owner bool, id int64) {
	a := b.access()
	if b.owner || (adminID != 0 && a.isOwner(adminID)) {
		return true, adminID
	}
	return false, adminID
}

func listOrders(adminID int64, owner bool, onlyPending bool, limit int) []model.BotOrder {
	q := database.GetDB().Model(&model.BotOrder{})
	if !owner {
		q = q.Where("admin_id = ?", adminID)
	} else if adminID != 0 {
		q = q.Where("admin_id = ?", adminID)
	}
	if onlyPending {
		q = q.Where("status = ?", orderPending)
	}
	var out []model.BotOrder
	q.Order("id desc").Limit(limit).Find(&out)
	return out
}

func countOrders(adminID int64, owner bool, status string, since int64) int64 {
	q := database.GetDB().Model(&model.BotOrder{})
	if !owner {
		q = q.Where("admin_id = ?", adminID)
	} else if adminID != 0 {
		q = q.Where("admin_id = ?", adminID)
	}
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if since > 0 {
		q = q.Where("created_at >= ?", since)
	}
	var n int64
	q.Count(&n)
	return n
}

func (b *bot) orderStatusLabel(st string) string {
	switch st {
	case orderPending:
		return b.tr("در انتظار", "pending")
	case orderApproved:
		return b.tr("تأیید شد", "approved")
	case orderRejected:
		return b.tr("رد شد", "rejected")
	case orderCancelled:
		return b.tr("لغو شد", "cancelled")
	}
	return st
}

func (b *bot) orderTopicLabel(topic string) string {
	switch topic {
	case "buy":
		return b.tr("خرید", "buy")
	case "renew":
		return b.tr("تمدید", "renew")
	case "test":
		return b.tr("تست", "trial")
	case "wallet":
		return b.tr("کیف پول", "wallet")
	case "bind":
		return b.tr("اتصال لینک", "bind")
	}
	return topic
}

func (b *bot) orderQueueScreen(onlyPending bool) (string, [][]button) {
	owner, adminID := b.orderAdminFilter(b.self)
	orders := listOrders(adminID, owner, onlyPending, 20)
	title := b.tr("📋 <b>سفارش‌ها</b>", "📋 <b>Orders</b>")
	if onlyPending {
		title = b.tr("📋 <b>سفارش‌های در انتظار</b>", "📋 <b>Pending orders</b>")
	}
	lines := []string{title}
	if len(orders) == 0 {
		lines = append(lines, b.tr("موردی نیست.", "None."))
	}
	var kb [][]button
	for _, o := range orders {
		detail := o.Detail
		if detail == "" {
			detail = b.orderTopicLabel(o.Topic)
		}
		line := fmt.Sprintf("#%d · %s · %s", o.Id, b.orderStatusLabel(o.Status), detail)
		if o.TgId != 0 {
			line += fmt.Sprintf(" · <code>%d</code>", o.TgId)
		}
		lines = append(lines, esc(line))
		if o.Status == orderPending {
			kb = append(kb, []button{
				{Text: fmt.Sprintf("✅ #%d", o.Id), Data: fmt.Sprintf("j:ok:%d", o.Id)},
				{Text: fmt.Sprintf("❌ #%d", o.Id), Data: fmt.Sprintf("j:no:%d", o.Id)},
			})
		}
	}
	filter := []button{
		{Text: b.tr("در انتظار", "Pending"), Data: "k:ordsP"},
		{Text: b.tr("همه", "All"), Data: "k:ords"},
	}
	kb = append(kb, filter)
	kb = append(kb, []button{{Text: b.tr("📊 گزارش فروش", "📊 Sales report"), Data: "k:sales"}})
	kb = append(kb, []button{{Text: b.tr("🔙 فروشگاه", "🔙 Shop"), Data: "k:edit"}})
	return strings.Join(lines, "\n"), kb
}

func (b *bot) salesReportScreen() (string, [][]button) {
	owner, adminID := b.orderAdminFilter(b.self)
	now := time.Now()
	if b.loc != nil {
		now = now.In(b.loc)
	}
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Unix()
	lines := []string{b.tr("📊 <b>گزارش فروش</b>", "📊 <b>Sales report</b>")}
	if owner && adminID == b.access().owner {
		// Owner looking at the whole shop: one block per limited admin plus totals.
		lines = append(lines, b.salesBlock(0, true, start))
		for _, id := range b.access().order {
			if b.access().isOwner(id) {
				continue
			}
			lines = append(lines, "", b.salesBlock(id, true, start))
		}
	} else {
		lines = append(lines, b.salesBlock(adminID, owner, start))
	}
	kb := [][]button{
		{{Text: b.tr("📋 سفارش‌ها", "📋 Orders"), Data: "k:ordsP"}},
		{{Text: b.tr("🔙 فروشگاه", "🔙 Shop"), Data: "k:edit"}},
	}
	return strings.Join(lines, "\n"), kb
}

func (b *bot) salesBlock(adminID int64, owner bool, start int64) string {
	pending := countOrders(adminID, owner && adminID == 0, orderPending, 0)
	today := countOrders(adminID, owner && adminID == 0, orderApproved, start)
	who := b.tr("کل فروشگاه", "Whole shop")
	if adminID != 0 {
		who = fmt.Sprintf("%d", adminID)
		if adminID == b.self && !b.owner {
			who = b.tr("شما", "You")
		}
	}
	lines := []string{"👤 " + esc(who)}
	lines = append(lines,
		b.tr("سفارش امروز (تأییدشده): ", "Approved today: ")+strconv.FormatInt(today, 10),
		b.tr("در انتظار: ", "Pending: ")+strconv.FormatInt(pending, 10),
	)
	if adminID != 0 {
		if q, ok, err := b.quotaOf(adminID); err == nil && ok {
			pct := 0
			if q.Total > 0 {
				pct = int(q.Used * 100 / q.Total)
				if pct > 100 {
					pct = 100
				}
			}
			lines = append(lines,
				b.tr("فروخته‌شده: ", "Sold: ")+humanBytes(q.Sold),
				b.tr("باقی‌مانده: ", "Left: ")+humanBytes(q.Left())+" / "+humanBytes(q.Total)+fmt.Sprintf(" (%d%%)", pct),
			)
		} else {
			lines = append(lines, b.tr("محدودیت حجم: ندارد", "Volume limit: none"))
		}
	}
	return strings.Join(lines, "\n")
}

func (b *bot) customerCancel(ctx context.Context, chatID int64, from *fromUser, orderID uint) {
	o, err := loadOrder(orderID)
	if err != nil || o.TgId != from.ID {
		b.sendMenu(ctx, chatID, b.tr("❌ سفارش پیدا نشد.", "❌ Order not found."), b.customerMenu(from.ID))
		return
	}
	if o.Status != orderPending {
		b.sendMenu(ctx, chatID, b.tr("این سفارش دیگر در انتظار نیست.", "That order is no longer pending."), b.customerMenu(from.ID))
		return
	}
	o.Status = orderCancelled
	if err := b.saveOrder(o); err != nil {
		b.sendMenu(ctx, chatID, b.tr("❌ لغو ممکن نشد.", "❌ Could not cancel."), b.customerMenu(from.ID))
		return
	}
	if o.WalletPaid && o.Amount > 0 {
		_, _ = walletCredit(o.TgId, o.Amount)
	}
	// A receipt sent after this must not reach staff for a cancelled order.
	if p := b.pend.get(chatID); p != nil && p.kind == "cu.receipt" && p.data != nil && p.data["id"] == strconv.FormatUint(uint64(o.Id), 10) {
		b.pend.clear(chatID)
	}
	b.notifyOrderStaff(ctx, o, b.tr("❌ مشتری سفارش را لغو کرد.", "❌ The customer cancelled the order.")+"\n"+esc(o.Detail)+fmt.Sprintf("\n<code>%d</code>", o.TgId), nil)
	b.sendMenu(ctx, chatID, b.tr("✅ سفارش لغو شد.", "✅ Order cancelled."), b.customerMenu(from.ID))
}

func (b *bot) maybeQuotaWarn(ctx context.Context, adminID int64) {
	if adminID == 0 {
		return
	}
	q, ok, err := b.quotaOf(adminID)
	if err != nil || !ok || q.Total <= 0 {
		return
	}
	pct := int(q.Used * 100 / q.Total)
	if pct > 100 {
		pct = 100
	}
	if pct < 80 {
		if q.Warn80 {
			_ = database.GetDB().Model(&model.BotQuota{}).Where("tg_id = ?", adminID).Update("warn80", false).Error
		}
		return
	}
	if q.Warn80 {
		return
	}
	if err := database.GetDB().Model(&model.BotQuota{}).Where("tg_id = ?", adminID).Update("warn80", true).Error; err != nil {
		return
	}
	owner := b.access().owner
	if owner == 0 {
		return
	}
	text := b.tr(
		fmt.Sprintf("⚠️ سهمیه ادمین <code>%d</code> به %d٪ رسیده است (%s از %s).", adminID, pct, humanBytes(q.Used), humanBytes(q.Total)),
		fmt.Sprintf("⚠️ Admin <code>%d</code> has reached %d%% of their quota (%s of %s).", adminID, pct, humanBytes(q.Used), humanBytes(q.Total)),
	)
	b.notifyIDs(ctx, []int64{owner}, text, nil)
}
