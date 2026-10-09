package tgbot

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/logger"
	"github.com/Danialrostamani/drnetwork-panel/service"
)

// Auto-renewal: a customer turns it on from a service's card, and the watch
// loop renews the service with its last plan, from the wallet, when it is
// about to run out.

const (
	autoRenewEvery = 5 * time.Minute
	// autoRenewGap keeps a service from being renewed twice in a row.
	autoRenewGap = 12 * time.Hour
	// A failure is told once a day; after autoRenewMaxFails of them the
	// renewal turns itself off.
	autoRenewFailGap  = 24 * time.Hour
	autoRenewMaxFails = 3
)

// autoRenewRow is the auto-renewal button of a bound client's card, nil when
// the shop does not offer it.
func (b *bot) autoRenewRow(c model.Client) []button {
	if c.TgId == 0 {
		return nil
	}
	st := b.shopSvc().Settings()
	if !st.Enable || !st.AutoRenew {
		return nil
	}
	label := b.tr("🔁 تمدید خودکار: خاموش", "🔁 Auto-renew: off")
	if r, ok := b.shopSvc().AutoRenewOf(c.Id); ok && r.Enable && r.TgId == c.TgId {
		label = b.tr("🔁 تمدید خودکار: روشن ✅", "🔁 Auto-renew: on ✅")
	}
	return []button{{Text: label, Data: fmt.Sprintf("p:ar:%d", c.Id)}}
}

// toggleAutoRenew turns a customer's auto-renewal of a service on or off.
func (b *bot) toggleAutoRenew(ctx context.Context, cbID string, chatID, msgID, from int64, id uint) {
	t := b.tr
	svc := b.shopSvc()
	if !svc.Settings().AutoRenew {
		b.answer(ctx, cbID, t("تمدید خودکار فعال نیست.", "Auto-renewal is not offered."))
		return
	}
	var c *model.Client
	for _, bc := range boundClients(from) {
		if bc.Id == id {
			c = &bc
			break
		}
	}
	if c == nil {
		b.answer(ctx, cbID, t("این سرویس پیدا نشد.", "That service was not found."))
		return
	}
	on := false
	if r, ok := svc.AutoRenewOf(c.Id); ok && r.Enable && r.TgId == from {
		on = true
	}
	p, err := svc.SetAutoRenew(c.Id, from, !on)
	switch {
	case errors.Is(err, service.ErrNoRenewPlan):
		b.answer(ctx, cbID, "")
		b.sendKeyboard(ctx, chatID, t("❌ پلنی برای تمدید خودکار این سرویس پیدا نشد؛ یک بار آن را از فروشگاه تمدید کنید.", "❌ There is no plan to renew this service with; renew it once from the shop."),
			[][]button{{{Text: t("🔄 تمدید", "🔄 Renew"), Data: fmt.Sprintf("p:plans:%d", c.Id)}}, b.homeRow()})
		return
	case err != nil:
		b.answer(ctx, cbID, b.errText(err))
		return
	}
	text, kb := b.userView(*c)
	if on {
		b.answer(ctx, cbID, t("تمدید خودکار خاموش شد.", "Auto-renewal is off."))
		b.edit(ctx, chatID, msgID, text, kb)
		return
	}
	b.answer(ctx, cbID, t("تمدید خودکار روشن شد.", "Auto-renewal is on."))
	b.edit(ctx, chatID, msgID, text, kb)
	_, paid := service.Price(p.Price, b.shopperOf(from).percent, 0)
	b.sendKeyboard(ctx, chatID, fmt.Sprintf(t("🔁 تمدید خودکار %s روشن شد.\nوقتی یک روز به پایان آن مانده باشد یا ۹۵٪ حجمش مصرف شده باشد، با پلن «%s» به مبلغ %s از کیف پول تمدید می‌شود.\nموجودی فعلی: %s",
		"🔁 Auto-renewal of %s is on.\nWith a day left or 95%% of its volume used, it is renewed with “%s” for %s from your wallet.\nBalance now: %s"),
		esc(c.Name), esc(p.Name), b.money(paid), b.money(svc.Balance(from))),
		[][]button{{{Text: t("➕ شارژ کیف پول", "➕ Top up"), Data: "p:topup"}}, b.homeRow()})
}

// autoRenew renews the services with auto-renewal on that are about to run
// out.
func (b *bot) autoRenew(ctx context.Context, now time.Time) {
	svc := b.shopSvc()
	st := svc.Settings()
	if !st.Enable || !st.AutoRenew {
		return
	}
	rows, err := svc.AutoRenews()
	if err != nil || len(rows) == 0 {
		return
	}
	clients := map[uint]model.Client{}
	for _, c := range allClients() {
		clients[c.Id] = c
	}
	if len(clients) == 0 {
		// The clients could not be read: nothing is known to be gone.
		return
	}
	for _, r := range rows {
		if ctx.Err() != nil {
			return
		}
		c, ok := clients[r.ClientId]
		if !ok {
			svc.DropAutoRenew(r.ClientId)
			continue
		}
		if c.TgId != r.TgId {
			// The service has another owner now, who did not ask for it.
			_, _ = svc.SetAutoRenew(r.ClientId, r.TgId, false)
			continue
		}
		if u := svc.User(r.TgId); u != nil && u.Blocked {
			continue
		}
		if r.LastAt > 0 && now.Sub(time.Unix(r.LastAt, 0)) < autoRenewGap {
			continue
		}
		if !service.AutoRenewDue(&c, now.Unix()) {
			continue
		}
		over := (c.Volume > 0 && c.Up+c.Down >= c.Volume) || (c.Expiry > 0 && c.Expiry <= now.Unix())
		if !c.Enable && !over {
			// Turned off by an administrator, not run out.
			continue
		}
		b.autoRenewOne(ctx, r, &c, now)
	}
}

// autoRenewOne renews one service from its owner's wallet.
func (b *bot) autoRenewOne(ctx context.Context, r model.ShopAutoRenew, c *model.Client, now time.Time) {
	t := b.tr
	svc := b.shopSvc()
	p, err := svc.RenewPlanOf(c.Id)
	if err != nil {
		b.autoRenewFailed(ctx, r, c, now, t("پلن آن دیگر فروخته نمی‌شود", "its plan is no longer sold"), false)
		return
	}
	sh := b.shopperOf(r.TgId)
	discount, paid := service.Price(p.Price, sh.percent, 0)
	lowFunds := fmt.Sprintf(t("موجودی کیف پول (%s) به مبلغ تمدید (%s) نمی‌رسد", "the wallet (%s) does not cover the renewal (%s)"), b.money(svc.Balance(r.TgId)), b.money(paid))
	if svc.Balance(r.TgId) < paid {
		b.autoRenewFailed(ctx, r, c, now, lowFunds, true)
		return
	}
	o := &model.ShopOrder{TgId: r.TgId, TgName: b.shopName(r.TgId), Kind: model.OrderRenew, PlanId: p.Id, PlanName: p.Name, Volume: p.Volume, Days: p.Days,
		Amount: p.Price, Discount: discount, Paid: paid, Method: model.PayWallet, Reseller: sh.reseller, ClientId: c.Id, ClientName: c.Name, Group: c.Group, Auto: true}
	if err := svc.CreateOrder(o); err != nil {
		logger.Warning("telegram bot: auto-renew ", c.Name, ": ", err)
		return
	}
	if _, err := svc.AddFunds(nil, r.TgId, -o.Paid, "order", o.Id, 0); err != nil {
		_, _ = svc.Decide(o.Id, model.OrderCanceled, 0)
		if errors.Is(err, service.ErrNoFunds) {
			b.autoRenewFailed(ctx, r, c, now, lowFunds, true)
		} else {
			logger.Warning("telegram bot: auto-renew ", c.Name, ": ", err)
		}
		return
	}
	if _, err := svc.Decide(o.Id, model.OrderApproved, 0); err != nil {
		_, _ = svc.AddFunds(nil, r.TgId, o.Paid, "refund", o.Id, 0)
		logger.Warning("telegram bot: auto-renew ", c.Name, ": ", err)
		return
	}
	if err := b.deliver(ctx, o); err != nil {
		// Nothing was renewed: the money goes back.
		_, _ = svc.AddFunds(nil, r.TgId, o.Paid, "refund", o.Id, 0)
		database.GetDB().Model(&model.ShopOrder{}).Where("id = ?", o.Id).Update("status", model.OrderCanceled)
		logger.Warning("telegram bot: auto-renew ", c.Name, ": ", err)
		b.autoRenewFailed(ctx, r, c, now, b.errText(err), false)
		return
	}
	svc.AutoRenewed(c.Id, p.Id, now.Unix())
}

// autoRenewFailed tells the owner, at most once a day, why a service was not
// renewed. After autoRenewMaxFails such days the renewal turns off.
func (b *bot) autoRenewFailed(ctx context.Context, r model.ShopAutoRenew, c *model.Client, now time.Time, why string, topup bool) {
	if r.LastFailAt > 0 && now.Sub(time.Unix(r.LastFailAt, 0)) < autoRenewFailGap {
		return
	}
	t := b.tr
	svc := b.shopSvc()
	svc.AutoRenewed(c.Id, 0, now.Unix())
	msg := fmt.Sprintf(t("⚠️ سرویس %s رو به پایان است ولی خودکار تمدید نشد: %s.", "⚠️ %s is running out but was not renewed automatically: %s."), esc(c.Name), esc(why))
	if r.Fails+1 >= autoRenewMaxFails {
		_, _ = svc.SetAutoRenew(c.Id, r.TgId, false)
		msg += "\n" + t("🔕 تمدید خودکار این سرویس خاموش شد؛ می‌توانید دوباره روشنش کنید.", "🔕 Auto-renewal of this service is off now; you can turn it on again.")
	}
	var kb [][]button
	if topup {
		kb = append(kb, []button{{Text: t("➕ شارژ کیف پول", "➕ Top up"), Data: "p:topup"}})
	}
	kb = append(kb, []button{{Text: t("🔄 تمدید", "🔄 Renew"), Data: fmt.Sprintf("p:plans:%d", c.Id)}}, b.homeRow())
	b.sendKeyboard(ctx, r.TgId, msg, kb)
}
