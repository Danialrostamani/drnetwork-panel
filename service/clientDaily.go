package service

import (
	"encoding/json"
	"strconv"
	"sync"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/util/common"

	"gorm.io/gorm"
)

// A client may have a daily limit, of bytes or of time online, that starts
// over every day in the panel's time zone: past it the client is disabled
// until the next day. What each client moves every day is kept too, for the
// last 30 days on its subscription page.

// onlineGap is the longest pause between two signs of a client that still
// counts as online time: the stats job sees traffic every few seconds, a
// node reports every few seconds more.
const onlineGap = 90

// clientDaysKept is how many days of the daily history are kept.
const clientDaysKept = 92

var (
	dayLocMu  sync.Mutex
	dayLoc    *time.Location
	dayLocAt  time.Time
	dayLocTTL = time.Minute
)

// panelLocation is the panel's time zone, read again once a minute.
func panelLocation() *time.Location {
	dayLocMu.Lock()
	defer dayLocMu.Unlock()
	if dayLoc == nil || time.Since(dayLocAt) > dayLocTTL {
		loc, err := (&SettingService{}).GetTimeLocation()
		if err != nil || loc == nil {
			loc = time.Local
		}
		dayLoc, dayLocAt = loc, time.Now()
	}
	return dayLoc
}

// DayKey is the day of t as yyyymmdd in the panel's time zone.
func DayKey(t time.Time) int {
	y, m, d := t.In(panelLocation()).Date()
	return y*10000 + int(m)*100 + d
}

// dayCounters are the column updates that count traffic and time online
// into the client's day, starting the day over when it changed.
func dayCounters(update map[string]interface{}, now int64, bytes int64) {
	key := DayKey(time.Unix(now, 0))
	online := gorm.Expr("CASE WHEN ? - online_at BETWEEN 1 AND ? THEN ? - online_at ELSE 0 END", now, onlineGap, now)
	update["day_used"] = gorm.Expr("CASE WHEN day_key = ? THEN day_used + ? ELSE ? END", key, bytes, bytes)
	update["day_online"] = gorm.Expr("CASE WHEN day_key = ? THEN day_online + (?) ELSE (?) END", key, online, online)
	update["day_key"] = key
}

// addClientDay adds traffic to the client's row of the day.
func addClientDay(tx *gorm.DB, name string, now int64, up, down int64) error {
	if up <= 0 && down <= 0 {
		return nil
	}
	return tx.Exec(`INSERT INTO client_days (client_id, day, up, down)
		SELECT id, ?, ?, ? FROM clients WHERE name = ?
		ON CONFLICT (client_id, day) DO UPDATE SET up = client_days.up + excluded.up, down = client_days.down + excluded.down`,
		DayKey(time.Unix(now, 0)), max(up, 0), max(down, 0), name).Error
}

// dailyOverWhere picks the enabled clients past a daily limit.
const dailyOverWhere = "enable = true AND ((daily_volume > 0 AND day_key = ? AND day_used >= daily_volume) OR (daily_time > 0 AND day_key = ? AND day_online >= daily_time))"

// dailyRollover starts a new day: every client's counters go to zero and the
// clients a daily limit disabled are enabled again, unless their volume or
// expiry is past too. It returns their inbounds and names.
func dailyRollover(tx *gorm.DB, now int64) ([]uint, []string, error) {
	key := DayKey(time.Unix(now, 0))
	var back []model.Client
	if err := tx.Model(model.Client{}).
		Where("day_locked = true AND enable = false AND day_key <> ?", key).
		Where("NOT (volume > 0 AND up+down >= volume) AND NOT (expiry > 0 AND expiry < ?)", now).
		Select("id", "name", "inbounds").Find(&back).Error; err != nil {
		return nil, nil, err
	}
	var inboundIDs []uint
	var names []string
	ids := make([]uint, 0, len(back))
	for _, c := range back {
		ids = append(ids, c.Id)
		names = append(names, c.Name)
		var in []uint
		_ = json.Unmarshal(c.Inbounds, &in)
		inboundIDs = common.UnionUintArray(inboundIDs, in)
	}
	if len(ids) > 0 {
		if err := tx.Model(model.Client{}).Where("id IN ?", ids).Updates(map[string]interface{}{"enable": true, "day_locked": false}).Error; err != nil {
			return nil, nil, err
		}
		changes := make([]model.Changes, 0, len(back))
		for _, c := range back {
			changes = append(changes, model.Changes{DateTime: now, Actor: "DailyLimit", Key: "clients", Action: "enable", Obj: clientNameJSON(c.Name)})
		}
		if err := tx.Create(&changes).Error; err != nil {
			return nil, nil, err
		}
	}
	// A day locked client someone else disabled meanwhile stays disabled.
	if err := tx.Model(model.Client{}).Where("day_key <> ?", key).
		Updates(map[string]interface{}{"day_used": 0, "day_online": 0, "day_key": key, "day_locked": false}).Error; err != nil {
		return nil, nil, err
	}
	return inboundIDs, names, nil
}

// lockDailyOver marks the clients past a daily limit, for DepleteClients to
// disable with the others; the mark lets the new day enable them again.
func lockDailyOver(tx *gorm.DB, now int64) error {
	key := DayKey(time.Unix(now, 0))
	return tx.Model(model.Client{}).Where(dailyOverWhere, key, key).Update("day_locked", true).Error
}

// DayUsage is what a client moved on one day.
type DayUsage struct {
	Day  int   `json:"day"`
	Up   int64 `json:"up"`
	Down int64 `json:"down"`
}

// ClientDays is the client's daily history of the last n days up to today,
// oldest first, a zero row for a day without traffic.
func ClientDays(clientID uint, n int, now time.Time) []DayUsage {
	if n <= 0 {
		return nil
	}
	loc := panelLocation()
	today := now.In(loc)
	first := time.Date(today.Year(), today.Month(), today.Day()-(n-1), 12, 0, 0, 0, loc)
	var rows []model.ClientDay
	database.GetDB().Where("client_id = ? AND day >= ?", clientID, DayKey(first)).Find(&rows)
	by := make(map[int]model.ClientDay, len(rows))
	for _, r := range rows {
		by[r.Day] = r
	}
	out := make([]DayUsage, 0, n)
	for i := 0; i < n; i++ {
		k := DayKey(first.AddDate(0, 0, i))
		r := by[k]
		out = append(out, DayUsage{Day: k, Up: r.Up, Down: r.Down})
	}
	return out
}

// delOldClientDays drops the history past what is kept and of deleted clients.
func delOldClientDays(db *gorm.DB, now time.Time) error {
	oldest := DayKey(now.AddDate(0, 0, -clientDaysKept))
	if err := db.Where("day < ?", oldest).Delete(&model.ClientDay{}).Error; err != nil {
		return err
	}
	return db.Where("client_id NOT IN (?)", db.Model(model.Client{}).Select("id")).Delete(&model.ClientDay{}).Error
}

// DayKeyLabel is a day key as yyyy-mm-dd.
func DayKeyLabel(k int) string {
	s := strconv.Itoa(k)
	if len(s) != 8 {
		return s
	}
	return s[:4] + "-" + s[4:6] + "-" + s[6:]
}
