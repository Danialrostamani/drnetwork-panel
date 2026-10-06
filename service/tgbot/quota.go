package tgbot

import (
	"errors"
	"fmt"
	"math"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/logger"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Volume limits of the bot's administrators.
//
// The owner can give an administrator a total volume (Admins → ✏️ → 📦 Volume
// limit). From then on every byte of volume that administrator hands out to
// clients through the bot is deducted from it: the volume of a new client, "+10
// GB", a raised volume, a traffic reset that gives the volume back. When the
// total is used up they can still manage their clients, only not give out more
// volume until the owner raises the total.
//
// What a change costs is the increase of what the client can still use (its
// volume less its traffic so far), so renaming, enabling, extending a client or
// lowering its volume costs nothing, and a client the administrator did not
// create costs them only what they add to it. Nothing is given back
// automatically: deleting a client or lowering its volume does not credit the
// administrator, because the bot cannot tell who handed out which volume; the
// owner tops the total up when they want to.
//
// A limited administrator also cannot make a client unlimited or switch on auto
// reset, either of which would hand out volume without bound.
//
// The owner is never limited, and neither is anybody without a row in the
// bot_quotas table. Every client write of the bot goes through b.save, which
// calls chargeVolume, so a screen that forgets the limit cannot get around it.

// quotaCap keeps a total, and every sum made with one, far from overflowing.
const quotaCap = int64(1e9) * gib

func clampQuota(n int64) int64 {
	switch {
	case n < 0:
		return 0
	case n > quotaCap:
		return quotaCap
	}
	return n
}

// quotaLeft is what an administrator may still hand out.
func quotaLeft(q model.BotQuota) int64 {
	if q.Granted >= q.Total {
		return 0
	}
	return q.Total - q.Granted
}

// ---- the stored limits ----

// loadQuota reads an administrator's limit; ok is false when they have none.
func loadQuota(id int64) (q model.BotQuota, ok bool, err error) {
	res := database.GetDB().Where("tg_id = ?", id).Limit(1).Find(&q)
	return q, res.Error == nil && res.RowsAffected == 1, res.Error
}

// allQuotas reads every limit, by administrator.
func allQuotas() map[int64]model.BotQuota {
	out := map[int64]model.BotQuota{}
	var rows []model.BotQuota
	if err := database.GetDB().Find(&rows).Error; err != nil {
		logger.Warning("telegram bot: load volume limits: ", err)
		return out
	}
	for _, q := range rows {
		out[q.TgId] = q
	}
	return out
}

// setQuotaTotal gives an administrator a limit of total bytes: it creates the
// limit, with nothing handed out yet, or changes the total of an existing one.
func setQuotaTotal(id, total int64) (model.BotQuota, error) {
	err := database.GetDB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.BotQuota{TgId: id}).Error; err != nil {
			return err
		}
		return tx.Model(&model.BotQuota{}).Where("tg_id = ?", id).Update("total", clampQuota(total)).Error
	})
	if err != nil {
		return model.BotQuota{}, err
	}
	q, _, err := loadQuota(id)
	return q, err
}

// shiftQuotaTotal adds delta, which may be negative, to the total of an
// existing limit; found is false when the administrator has none.
func shiftQuotaTotal(id, delta int64) (q model.BotQuota, found bool, err error) {
	res := database.GetDB().Model(&model.BotQuota{}).Where("tg_id = ?", id).
		Update("total", gorm.Expr("MIN(MAX(total + ?, 0), ?)", delta, quotaCap))
	if res.Error != nil || res.RowsAffected == 0 {
		return q, false, res.Error
	}
	return loadQuota(id)
}

// resetQuotaGranted starts the count of what has been handed out from zero.
func resetQuotaGranted(id int64) (q model.BotQuota, found bool, err error) {
	res := database.GetDB().Model(&model.BotQuota{}).Where("tg_id = ?", id).Update("granted", 0)
	if res.Error != nil || res.RowsAffected == 0 {
		return q, false, res.Error
	}
	return loadQuota(id)
}

// deleteQuota takes the limit away.
func deleteQuota(id int64) error {
	return database.GetDB().Where("tg_id = ?", id).Delete(&model.BotQuota{}).Error
}

var errQuotaShort = errors.New("not enough volume left")

// reserveQuota takes n bytes from an administrator's limit in one statement, so
// two requests at once cannot both take the last of it. reserved is false, with
// no error, when there is no limit any more; errQuotaShort comes with what is
// left when n does not fit.
func reserveQuota(id, n int64) (reserved bool, left int64, err error) {
	res := database.GetDB().Model(&model.BotQuota{}).
		Where("tg_id = ? AND total - granted >= ?", id, n).
		Update("granted", gorm.Expr("granted + ?", n))
	if res.Error != nil {
		return false, 0, res.Error
	}
	if res.RowsAffected == 1 {
		return true, 0, nil
	}
	q, ok, err := loadQuota(id)
	switch {
	case err != nil:
		return false, 0, err
	case !ok:
		return false, 0, nil
	}
	return false, quotaLeft(q), errQuotaShort
}

// releaseQuota gives back what reserveQuota took, for a write that then failed.
func releaseQuota(id, n int64) {
	err := database.GetDB().Model(&model.BotQuota{}).Where("tg_id = ?", id).
		Update("granted", gorm.Expr("MAX(granted - ?, 0)", n)).Error
	if err != nil {
		logger.Warning("telegram bot: give back volume of ", id, ": ", err)
	}
}

// ---- what a write costs ----

// quotaError is a refusal because of an administrator's volume limit. Its text
// is already in the bot's language, so every screen that shows an error shows
// it right.
type quotaError struct{ msg string }

func (e *quotaError) Error() string { return e.msg }

func (b *bot) errQuotaLow(need, left int64) error {
	return &quotaError{b.tr(
		fmt.Sprintf("حجم باقی‌مانده کافی نیست: %s لازم است و %s دارید. از مالک ربات بخواهید سقف شما را بالا ببرد.", humanBytes(need), humanBytes(left)),
		fmt.Sprintf("Not enough volume left: this needs %s and you have %s. Ask the bot owner to raise your limit.", humanBytes(need), humanBytes(left)))}
}

func (b *bot) errQuotaUnlimited() error {
	return &quotaError{b.tr(
		"شما سقف حجم دارید، پس کلاینت نامحدود مجاز نیست؛ یک حجم مشخص بدهید.",
		"You have a volume limit, so a client cannot be unlimited; give it a volume.")}
}

func (b *bot) errQuotaAutoReset() error {
	return &quotaError{b.tr(
		"شما سقف حجم دارید، پس ریست خودکار مجاز نیست: هر دوره حجم را دوباره می‌دهد.",
		"You have a volume limit, so auto reset is not allowed: it would hand the volume out again every period.")}
}

func saturatingAdd(a, b int64) int64 {
	if a > math.MaxInt64-b {
		return math.MaxInt64
	}
	return a + b
}

// usedOf is the traffic a client has used.
func usedOf(c model.Client) int64 {
	up, down := c.Up, c.Down
	if up < 0 {
		up = 0
	}
	if down < 0 {
		down = 0
	}
	return saturatingAdd(up, down)
}

// usableOf is the volume a client can still use, for a volume that is not
// unlimited.
func usableOf(volume, used int64) int64 {
	if volume <= used {
		return 0
	}
	return volume - used
}

// clientCost is what taking a client from the state before (nil for a client
// that is being created) to the state after costs the administrator: the
// increase of the volume the client can still use.
func (b *bot) clientCost(before *model.Client, after model.Client) (int64, error) {
	if after.AutoReset && (before == nil || !before.AutoReset) {
		return 0, b.errQuotaAutoReset()
	}
	unlimitedBefore := before != nil && before.Volume <= 0
	if after.Volume <= 0 {
		if before == nil || !unlimitedBefore {
			return 0, b.errQuotaUnlimited()
		}
		return 0, nil
	}
	if unlimitedBefore {
		// Putting a limit on an unlimited client takes nothing away from the owner.
		return 0, nil
	}
	var usedBefore, usableBefore int64
	if before != nil {
		usedBefore = usedOf(*before)
		usableBefore = usableOf(before.Volume, usedBefore)
	}
	usedAfter := usedBefore
	if before != nil && after.Up == 0 && after.Down == 0 {
		// A traffic reset: the client starts from nothing again.
		usedAfter = 0
	}
	usableAfter := usableOf(after.Volume, usedAfter)
	if usableAfter <= usableBefore {
		return 0, nil
	}
	return usableAfter - usableBefore, nil
}

// volumeCost is what a client write costs the administrator. Writes the bot
// does not know how to price are refused, so a new kind of write cannot slip
// past a limit.
func (b *bot) volumeCost(act string, payload interface{}) (int64, error) {
	badPayload := errors.New("the volume of this change cannot be worked out")
	switch act {
	case "new":
		c, ok := payload.(model.Client)
		if !ok {
			return 0, badPayload
		}
		return b.clientCost(nil, c)
	case "addbulk":
		list, ok := payload.([]model.Client)
		if !ok {
			return 0, badPayload
		}
		var total int64
		for _, c := range list {
			cost, err := b.clientCost(nil, c)
			if err != nil {
				return 0, err
			}
			total = saturatingAdd(total, cost)
		}
		return total, nil
	case "edit":
		c, ok := payload.(*model.Client)
		if !ok || c == nil {
			return 0, badPayload
		}
		var before *model.Client
		if stored, err := rawFullClient(c.Id); err == nil {
			before = stored
		}
		return b.clientCost(before, *c)
	case "editbulk":
		list, ok := payload.([]model.Client)
		if !ok {
			return 0, badPayload
		}
		ids := make([]uint, 0, len(list))
		for _, c := range list {
			ids = append(ids, c.Id)
		}
		stored := map[uint]*model.Client{}
		if len(ids) > 0 {
			var rows []model.Client
			if err := database.GetDB().Where("id IN ?", ids).Find(&rows).Error; err != nil {
				return 0, err
			}
			for i := range rows {
				stored[rows[i].Id] = &rows[i]
			}
		}
		var total int64
		for _, c := range list {
			cost, err := b.clientCost(stored[c.Id], c)
			if err != nil {
				return 0, err
			}
			total = saturatingAdd(total, cost)
		}
		return total, nil
	case "del", "delbulk", "attachall":
		// Nothing is given out, and nothing is given back.
		return 0, nil
	}
	return 0, badPayload
}

// myQuota is the limit of the administrator being served; ok is false for the
// owner, for the supervisor's own copy of the bot, and for an administrator who
// has none.
func (b *bot) myQuota() (q model.BotQuota, ok bool) {
	if b.owner || b.self <= 0 {
		return q, false
	}
	q, ok, err := loadQuota(b.self)
	if err != nil {
		logger.Warning("telegram bot: read the volume limit of ", b.self, ": ", err)
		return q, false
	}
	return q, ok
}

// quotaCharge is volume taken from an administrator's limit for a write.
type quotaCharge struct{ id, n int64 }

// refund gives the volume back when the write it was taken for failed.
func (c *quotaCharge) refund() {
	if c != nil {
		releaseQuota(c.id, c.n)
	}
}

// chargeVolume prices a client write and takes that volume from the limit of
// the administrator being served, or refuses the write. A nil charge means
// there is nothing to give back later.
func (b *bot) chargeVolume(act string, payload interface{}) (*quotaCharge, error) {
	if b.owner || b.self <= 0 {
		return nil, nil
	}
	// A limit that cannot be read is not "no limit".
	if _, ok, err := loadQuota(b.self); err != nil {
		return nil, err
	} else if !ok {
		return nil, nil
	}
	cost, err := b.volumeCost(act, payload)
	if err != nil {
		return nil, err
	}
	if cost <= 0 {
		return nil, nil
	}
	reserved, left, err := reserveQuota(b.self, cost)
	switch {
	case errors.Is(err, errQuotaShort):
		return nil, b.errQuotaLow(cost, left)
	case err != nil:
		return nil, err
	case !reserved:
		return nil, nil
	}
	return &quotaCharge{id: b.self, n: cost}, nil
}

// checkNewVolume tells at once whether the administrator could create a client
// with this volume, so the new-client wizard can refuse at the volume step
// instead of after the last question.
func (b *bot) checkNewVolume(volume int64) error {
	q, ok := b.myQuota()
	if !ok {
		return nil
	}
	cost, err := b.clientCost(nil, model.Client{Volume: volume})
	if err != nil {
		return err
	}
	if left := quotaLeft(q); cost > left {
		return b.errQuotaLow(cost, left)
	}
	return nil
}

// quotaLine tells an administrator with a limit how much they can still hand
// out; it is "" for everybody else.
func (b *bot) quotaLine() string {
	q, ok := b.myQuota()
	if !ok {
		return ""
	}
	return "📦 " + b.tr("حجم قابل واگذاری: ", "Volume you can still give: ") + "<b>" + humanBytes(quotaLeft(q)) + "</b> " + b.tr("از", "of") + " " + humanBytes(q.Total)
}
