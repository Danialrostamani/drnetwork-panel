package tgbot

import (
	"errors"
	"fmt"
	"math"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/logger"
	"github.com/Danialrostamani/drnetwork-panel/service"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Volume limits of the bot's administrators.
//
// The owner can give an administrator a volume limit (Admins → ✏️ → 📦 Volume
// limit): the most traffic the clients that count as theirs may consume. What
// is deducted is the traffic the clients really use, not the volume that was
// handed out: creating a client, or giving one more volume, costs nothing, and
// the limit goes down as the traffic of those clients goes up. A traffic reset
// does not give anything back, and neither does deleting a client: what it had
// used stays counted. A client that replaces a deleted one counts from its own
// first byte.
//
// A client counts for an administrator when they created it through the bot
// while they had a limit and, for an administrator limited to a group, when it
// is in that group. The clients an administrator created before their limit
// existed are found in the change history. What a client used before it
// counted is never charged. All of this is kept by service/botquota.go and
// worked out when it is read, from the traffic counters of the clients
// themselves, so there is no balance to drift.
//
// When nothing is left the administrator can still manage their clients, but
// cannot create one or give a client more volume until the owner raises the
// limit. The clients that exist keep working: nothing is cut off.
//
// The owner is never limited, and neither is anybody without a row in the
// bot_quotas table. Every client write of the bot goes through b.save, which
// calls quotaGate, so a screen that forgets the limit cannot get around it.

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

// quotaState is an administrator's limit with what has been used of it.
type quotaState struct {
	model.BotQuota
	// Used is the traffic counted against the limit; Clients is how many
	// clients count for the administrator now.
	Used    int64
	Clients int
}

// Left is the traffic the administrator's clients may still use before the
// limit is reached.
func (q quotaState) Left() int64 {
	if q.Used >= q.Total {
		return 0
	}
	return q.Total - q.Used
}

// ---- the stored limits ----

// loadQuota reads an administrator's limit row; ok is false when they have none.
func loadQuota(id int64) (q model.BotQuota, ok bool, err error) {
	res := database.GetDB().Where("tg_id = ?", id).Limit(1).Find(&q)
	return q, res.Error == nil && res.RowsAffected == 1, res.Error
}

// allQuotas reads every limit row, by administrator.
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
// limit, with nothing used yet, or changes the total of an existing one.
func setQuotaTotal(id, total int64) (model.BotQuota, error) {
	err := database.GetDB().Transaction(func(tx *gorm.DB) error {
		res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.BotQuota{TgId: id})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 1 {
			// A new limit counts from nothing, whatever an earlier one left.
			if err := service.BotQuotaForget(tx, id); err != nil {
				return err
			}
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

// deleteQuota takes the limit away, and with it the list of clients that
// counted for it.
func deleteQuota(id int64) error {
	return database.GetDB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("tg_id = ?", id).Delete(&model.BotQuota{}).Error; err != nil {
			return err
		}
		return service.BotQuotaForget(tx, id)
	})
}

// quotaOf reads an administrator's limit with what has been used of it; ok is
// false when they have none. Reading is also when the clients that count for
// them are brought up to date.
func (b *bot) quotaOf(id int64) (q quotaState, ok bool, err error) {
	row, ok, err := loadQuota(id)
	if err != nil || !ok {
		return quotaState{}, false, err
	}
	b.quotaCatchUp(&row)
	used, live, err := service.BotQuotaUsage(database.GetDB(), id)
	if err != nil {
		return quotaState{}, false, err
	}
	return quotaState{BotQuota: row, Used: used, Clients: live}, true, nil
}

// quotaCatchUp makes sure the clients that count for an administrator are all
// known: those of the group of an administrator limited to one (the owner can
// create clients in it too), and, once, those the administrator created
// through the bot before their limit existed. A failure is logged and leaves
// the count as it was; the next read tries again.
func (b *bot) quotaCatchUp(q *model.BotQuota) {
	db := database.GetDB()
	if a := b.access(); a.isMember(q.TgId) && !a.isOwner(q.TgId) {
		if group := a.roleOf(q.TgId).group; group != "" {
			if err := service.BotQuotaAdoptGroup(db, q.TgId, group); err != nil {
				logger.Warning("telegram bot: find the clients of ", q.TgId, "'s group: ", err)
			}
		}
	}
	if q.Adopted {
		return
	}
	actor := fmt.Sprintf("%s:%d", service.TelegramActor, q.TgId)
	if err := service.BotQuotaAdoptCreated(db, q.TgId, actor); err != nil {
		logger.Warning("telegram bot: find the clients ", q.TgId, " created: ", err)
		return
	}
	if err := db.Model(&model.BotQuota{}).Where("tg_id = ?", q.TgId).Update("adopted", true).Error; err != nil {
		logger.Warning("telegram bot: note the clients ", q.TgId, " created: ", err)
		return
	}
	q.Adopted = true
}

// ---- what a write may do ----

// quotaError is a refusal because of an administrator's volume limit. Its text
// is already in the bot's language, so every screen that shows an error shows
// it right.
type quotaError struct{ msg string }

func (e *quotaError) Error() string { return e.msg }

func (b *bot) errQuotaUsedUp(q quotaState) error {
	used, total := humanBytes(q.Used), humanBytes(q.Total)
	return &quotaError{b.tr(
		fmt.Sprintf("سقف حجم شما تمام شده است (%s از %s مصرف شده). از مالک ربات بخواهید آن را بالا ببرد.", used, total),
		fmt.Sprintf("Your volume limit is used up (%s of %s). Ask the bot owner to raise it.", used, total))}
}

func saturatingAdd(a, b int64) int64 {
	if a > math.MaxInt64-b {
		return math.MaxInt64
	}
	return a + b
}

// usedOf is the traffic a client has used in its current cycle.
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

// givesVolumeTo tells whether taking a client from the state before (nil for a
// client that is being created) to the state after gives it more to use: a new
// client, a bigger volume, a traffic reset of a client that has used some, or
// a client that becomes unlimited or starts to reset itself.
func givesVolumeTo(before *model.Client, after model.Client) bool {
	switch {
	case before == nil:
		return true
	case after.AutoReset && !before.AutoReset:
		return true
	case before.Volume <= 0:
		// Unlimited already: a limit put on it takes volume away.
		return false
	case after.Volume <= 0:
		return true
	}
	usedBefore := usedOf(*before)
	usedAfter := usedBefore
	if after.Up == 0 && after.Down == 0 {
		// A traffic reset: the client starts from nothing again.
		usedAfter = 0
	}
	return usableOf(after.Volume, usedAfter) > usableOf(before.Volume, usedBefore)
}

// givesVolume tells whether a client write gives volume. Writes the bot does
// not know how to judge are refused, so a new kind of write cannot slip past a
// limit.
func (b *bot) givesVolume(act string, payload interface{}) (bool, error) {
	badPayload := errors.New("the volume of this change cannot be worked out")
	switch act {
	case "new":
		if _, ok := payload.(model.Client); !ok {
			return false, badPayload
		}
		return true, nil
	case "addbulk":
		list, ok := payload.([]model.Client)
		if !ok {
			return false, badPayload
		}
		return len(list) > 0, nil
	case "edit":
		c, ok := payload.(*model.Client)
		if !ok || c == nil {
			return false, badPayload
		}
		var before *model.Client
		if stored, err := rawFullClient(c.Id); err == nil {
			before = stored
		}
		return givesVolumeTo(before, *c), nil
	case "editbulk":
		list, ok := payload.([]model.Client)
		if !ok {
			return false, badPayload
		}
		ids := make([]uint, 0, len(list))
		for _, c := range list {
			ids = append(ids, c.Id)
		}
		stored := map[uint]*model.Client{}
		if len(ids) > 0 {
			var rows []model.Client
			if err := database.GetDB().Where("id IN ?", ids).Find(&rows).Error; err != nil {
				return false, err
			}
			for i := range rows {
				stored[rows[i].Id] = &rows[i]
			}
		}
		for _, c := range list {
			if givesVolumeTo(stored[c.Id], c) {
				return true, nil
			}
		}
		return false, nil
	case "del", "delbulk", "attachall":
		return false, nil
	}
	return false, badPayload
}

// quotaGate refuses a client write of an administrator whose limit is used up
// when it would give volume. It lets everything else through, and everything
// while there is volume left: creating a client costs nothing, it is the
// traffic that counts.
func (b *bot) quotaGate(act string, payload interface{}) error {
	if b.owner || b.self <= 0 {
		return nil
	}
	q, ok, err := b.quotaOf(b.self)
	switch {
	case err != nil:
		// A limit that cannot be read is not "no limit".
		return err
	case !ok || q.Left() > 0:
		return nil
	}
	gives, err := b.givesVolume(act, payload)
	switch {
	case err != nil:
		return err
	case gives:
		return b.errQuotaUsedUp(q)
	}
	return nil
}

// myQuota is the limit of the administrator being served; ok is false for the
// owner, for the supervisor's own copy of the bot, and for an administrator who
// has none.
func (b *bot) myQuota() (q quotaState, ok bool) {
	if b.owner || b.self <= 0 {
		return q, false
	}
	q, ok, err := b.quotaOf(b.self)
	if err != nil {
		logger.Warning("telegram bot: read the volume limit of ", b.self, ": ", err)
		return quotaState{}, false
	}
	return q, ok
}

// quotaLine tells an administrator with a limit how much of it is left; it is
// "" for everybody else.
func (b *bot) quotaLine() string {
	q, ok := b.myQuota()
	if !ok {
		return ""
	}
	return "📦 " + b.tr("حجم باقی‌مانده: ", "Remaining volume: ") + "<b>" + humanBytes(q.Left()) + "</b> " + b.tr("از", "of") + " " + humanBytes(q.Total)
}
