package service

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"

	"gorm.io/gorm"
)

// TestDailyLimits counts a day's traffic and time online, disables a client
// past its daily volume or time, and enables it again the next day, unless
// its whole volume is used up too.
func TestDailyLimits(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "daily.db")); err != nil {
		t.Fatal(err)
	}
	db := database.GetDB()
	mk := func(name string, c model.Client) model.Client {
		c.Name, c.Enable = name, true
		c.Config, c.Inbounds, c.Links = json.RawMessage(`{}`), json.RawMessage(`[]`), json.RawMessage(`[]`)
		if err := db.Create(&c).Error; err != nil {
			t.Fatal(err)
		}
		return c
	}
	vol := mk("vol", model.Client{DailyVolume: 100})
	tim := mk("tim", model.Client{DailyTime: 20})
	both := mk("both", model.Client{DailyVolume: 100, Volume: 120})
	free := mk("free", model.Client{})
	now := time.Now().Unix()
	move := func(name string, at, bytes int64) {
		t.Helper()
		update := map[string]interface{}{"online_at": at}
		dayCounters(update, at, bytes)
		if bytes > 0 {
			update["up"] = gorm.Expr("up + ?", bytes)
		}
		if err := db.Model(model.Client{}).Where("name = ?", name).Updates(update).Error; err != nil {
			t.Fatal(err)
		}
		if err := addClientDay(db, name, at, bytes, 0); err != nil {
			t.Fatal(err)
		}
	}
	get := func(id uint) model.Client {
		var c model.Client
		db.First(&c, id)
		return c
	}
	move("vol", now, 60)
	move("vol", now, 60)
	move("tim", now-30, 1)
	move("tim", now, 1)
	move("both", now, 130)
	move("free", now, 1000)
	if c := get(vol.Id); c.DayUsed != 120 || c.DayKey != DayKey(time.Unix(now, 0)) {
		t.Fatalf("vol counters: %+v", c)
	}
	if c := get(tim.Id); c.DayOnline != 30 {
		t.Fatalf("tim online: %d", c.DayOnline)
	}
	cs := new(ClientService)
	if !cs.HasDepleted() {
		t.Fatal("quick check missed the daily limits")
	}
	if _, users, err := cs.DepleteClients(); err != nil || len(users) != 3 {
		t.Fatalf("deplete: %v %v", users, err)
	}
	for _, id := range []uint{vol.Id, tim.Id, both.Id} {
		if c := get(id); c.Enable || !c.DayLocked {
			t.Fatalf("%s not locked: %+v", c.Name, c)
		}
	}
	if c := get(free.Id); !c.Enable {
		t.Fatal("a client without daily limits was disabled")
	}
	// The next day.
	yesterday := DayKey(time.Unix(now-86400, 0))
	db.Model(model.Client{}).Where("1 = 1").Update("day_key", yesterday)
	if _, _, err := cs.DepleteClients(); err != nil {
		t.Fatal(err)
	}
	for _, id := range []uint{vol.Id, tim.Id} {
		if c := get(id); !c.Enable || c.DayLocked || c.DayUsed != 0 || c.DayOnline != 0 {
			t.Fatalf("%s not back: %+v", c.Name, c)
		}
	}
	if c := get(both.Id); c.Enable {
		t.Fatal("a client past its whole volume came back with the day")
	}
	days := ClientDays(free.Id, 30, time.Now())
	if len(days) != 30 || days[29].Day != DayKey(time.Now()) || days[29].Up != 1000 || days[0].Up != 0 {
		t.Fatalf("days: %+v", days[len(days)-2:])
	}

	// Enabling a locked client by hand lifts the lock.
	move("vol", now, 200)
	if _, _, err := cs.DepleteClients(); err != nil {
		t.Fatal(err)
	}
	c := get(vol.Id)
	c.Enable = true
	if err := db.Transaction(func(tx *gorm.DB) error { cs.preserveServerOwnedFields(tx, &c); return nil }); err != nil {
		t.Fatal(err)
	}
	if c.DayLocked || c.DayUsed != 200 {
		t.Fatalf("by hand: %+v", c)
	}
}
