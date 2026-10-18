package tgbot

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database/model"

	"gorm.io/gorm/clause"
)

// Renewal reminders: each customer bound to a client hears, once per term,
// that its time or data is about to run out and once more when it has, with a
// button that renews that very client. They do not depend on the alert
// setting of the web panel, and what was sent is kept in the database, so a
// restart does not send it again.
const (
	prefRenew     = "rm.on"   // reminders on
	prefRenewDays = "rm.days" // how many days before the expiry
	// renewLowPercent of the volume used is "about to run out".
	renewLowPercent = 90
	// renewPerRound caps the reminders of one round; the rest go in the next.
	renewPerRound = 40
)

var renewDayChoices = []int{1, 2, 3, 5, 7}

// renewStage is where a client stands for the reminders.
type renewStage struct {
	ended, timeOver, volOver bool
	soon, low                bool
}

func renewStageOf(c model.Client, now time.Time, days int) renewStage {
	var s renewStage
	s.volOver = c.Volume > 0 && c.Up+c.Down >= c.Volume
	s.timeOver = c.Expiry > 0 && c.Expiry <= now.Unix()
	s.ended = s.volOver || s.timeOver
	if s.ended || !c.Enable {
		return s
	}
	s.soon = c.Expiry > 0 && time.Unix(c.Expiry, 0).Sub(now) <= time.Duration(days)*24*time.Hour
	s.low = c.Volume > 0 && (usagePercent(c) >= renewLowPercent || c.Volume-c.Up-c.Down <= lowVolumeLeft)
	return s
}

// renewWatch sends the reminders that are due.
func (b *bot) renewWatch(ctx context.Context, now time.Time) {
	db := ownerDB()
	if db == nil || !prefOn(prefRenew) {
		return
	}
	days := prefInt(prefRenewDays, 3)
	var rows []model.BotRenewNote
	if err := db.Find(&rows).Error; err != nil {
		return
	}
	notes := make(map[uint]model.BotRenewNote, len(rows))
	for _, n := range rows {
		notes[n.ClientId] = n
	}
	sent := 0
	for _, c := range allClients() {
		if c.TgId == 0 || b.isAdmin(c.TgId) || customerBlocked(c.TgId) {
			continue
		}
		st := renewStageOf(c, now, days)
		old, seen := notes[c.Id]
		n := old
		if !seen || n.Expiry != c.Expiry || n.Volume != c.Volume {
			// A new term (or a client never looked at): nothing told yet.
			n = model.BotRenewNote{ClientId: c.Id, Expiry: c.Expiry, Volume: c.Volume}
		}
		// What no longer holds is forgotten, so it is told again next time.
		n.Soon = n.Soon && st.soon
		n.Low = n.Low && st.low
		n.Ended = n.Ended && st.ended
		var tell renewStage
		switch {
		case st.ended && !n.Ended:
			n.Ended = true
			// A client that was already off before the bot first looked at
			// it heard about it long ago.
			if seen || c.Enable {
				tell = st
			}
		case !st.ended:
			if st.soon && !n.Soon {
				n.Soon, tell.soon = true, true
			}
			if st.low && !n.Low {
				n.Low, tell.low = true, true
			}
		}
		if tell.ended || tell.soon || tell.low {
			if sent >= renewPerRound {
				continue
			}
			sent++
			text, kb := b.renewReminder(c, tell, now)
			b.sendKeyboard(ctx, c.TgId, text, kb)
			n.At = now.Unix()
		}
		if !seen || n != old {
			db.Clauses(clause.OnConflict{UpdateAll: true}).Create(&n)
		}
	}
}

// renewReminder is the message a customer gets about one client.
func (b *bot) renewReminder(c model.Client, s renewStage, now time.Time) (string, [][]button) {
	name := esc(c.Name)
	var lines []string
	if s.ended {
		lines = append(lines, "⛔ <b>"+b.tr("اشتراک شما به پایان رسید", "Your subscription has ended")+"</b> — <b>"+name+"</b>")
		switch {
		case s.timeOver && s.volOver:
			lines = append(lines, b.tr("زمان و حجم اشتراک تمام شده است.", "Both its time and its data are used up."))
		case s.volOver:
			lines = append(lines, b.tr("حجم اشتراک تمام شده است.", "Its data is used up."))
		default:
			lines = append(lines, b.tr("زمان اشتراک تمام شده است.", "Its time is over."))
		}
		lines = append(lines, "", b.tr("برای وصل شدن دوباره، اشتراک را تمدید کنید 👇", "Renew it to connect again 👇"))
	} else {
		lines = append(lines, "⏳ <b>"+b.tr("یادآوری تمدید", "Renewal reminder")+"</b> — <b>"+name+"</b>")
		if s.soon {
			left := time.Unix(c.Expiry, 0).Sub(now)
			lines = append(lines, "⏰ "+b.tr(
				fmt.Sprintf("%s تا پایان زمان اشتراک مانده است (%s).", b.humanDuration(left), b.stamp(c.Expiry)),
				fmt.Sprintf("%s left until it expires (%s).", b.humanDuration(left), b.stamp(c.Expiry))))
		}
		if s.low {
			left := max(c.Volume-c.Up-c.Down, 0)
			p := usagePercent(c)
			lines = append(lines, "📊 "+b.tr(
				fmt.Sprintf("%d٪ حجم مصرف شده؛ %s باقی مانده است.", p, humanBytes(left)),
				fmt.Sprintf("%d%% of the data is used; %s left.", p, humanBytes(left))))
		}
		lines = append(lines, "", b.tr("برای اینکه سرویس قطع نشود، همین حالا تمدید کنید 👇", "Renew now so the service does not stop 👇"))
	}
	kb := [][]button{{{Text: b.tr("🔄 تمدید همین اشتراک", "🔄 Renew this subscription"), Data: "u:ren:" + strconv.FormatUint(uint64(c.Id), 10)}}}
	return strings.Join(lines, "\n"), kb
}

// renewLines describe the reminders on the owner's alerts screen.
func (b *bot) renewLines() []string {
	d := prefInt(prefRenewDays, 3)
	return []string{b.tr(
		fmt.Sprintf("یادآوری تمدید: %d روز مانده به پایان زمان، وقتی ٪%d حجم مصرف شد و وقتی اشتراک تمام شد، برای خود مشتری (با دکمهٔ تمدید) فرستاده می‌شود.", d, renewLowPercent),
		fmt.Sprintf("Renewal reminder: sent to the customer (with a renew button) %d days before the expiry, at %d%% of the data, and when it ends.", d, renewLowPercent))}
}
