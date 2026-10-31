package tgbot

import (
	"testing"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
)

// A renewal used to add the plan's volume and days to a client that had run
// out, but leave the client switched off, so its link stayed dead after the
// customer paid. It is switched back on once it has both time and volume
// again; a client switched off by hand stays off.
func TestRenewalSwitchesARunOutClientBackOn(t *testing.T) {
	b, _ := shopBot(t, shopConfig{PayCard: "6037", Plans: []shopPlan{{ID: "1", Name: "Gold", Days: 30, GB: 40, Price: "100"}}})
	db := database.GetDB()
	past := time.Now().Add(-time.Hour).Unix()
	future := time.Now().Add(10 * 24 * time.Hour).Unix()

	seed := func(name string, enable bool, volume, used, expiry int64) model.Client {
		t.Helper()
		c := model.Client{Name: name, TgId: 99, Volume: volume, Up: used, Expiry: expiry, Inbounds: []byte("[]"), Links: []byte("[]")}
		if err := db.Create(&c).Error; err != nil {
			t.Fatal(err)
		}
		// Written separately: a false bool is a zero value GORM may skip.
		if err := db.Model(&model.Client{}).Where("id = ?", c.Id).Update("enable", enable).Error; err != nil {
			t.Fatal(err)
		}
		return c
	}
	renew := func(c model.Client, gb, days int) model.Client {
		t.Helper()
		o := &model.BotOrder{TgId: 99, Topic: "renew", PlanSet: true, PlanGB: gb, PlanDays: days, Extra: itoa(int64(c.Id)), Status: orderPending}
		if err := b.saveOrder(o); err != nil {
			t.Fatal(err)
		}
		if err := b.approveOrder(o); err != nil {
			t.Fatalf("approve renew of %s: %v", c.Name, err)
		}
		var fresh model.Client
		if err := db.First(&fresh, c.Id).Error; err != nil {
			t.Fatal(err)
		}
		return fresh
	}

	// Out of time and of volume, switched off by the depletion job.
	got := renew(seed("ranout", false, 10*gib, 10*gib, past), 40, 30)
	if !got.Enable {
		t.Errorf("renewed client is still disabled")
	}
	if got.Volume != 50*gib || got.Expiry <= time.Now().Unix() {
		t.Errorf("renewal did not add the plan: volume %d, expiry %d", got.Volume, got.Expiry)
	}

	// Switched off by hand with time and volume left: a renewal adds the
	// plan but does not overrule the administrator.
	got = renew(seed("byhand", false, 10*gib, 1*gib, future), 40, 30)
	if got.Enable {
		t.Errorf("a client disabled by hand was switched on by a renewal")
	}

	// Days alone do not help a client that is out of volume: it stays off.
	got = renew(seed("novol", false, 10*gib, 10*gib, future), 0, 30)
	if got.Enable {
		t.Errorf("a client still out of volume was switched on")
	}

	// An administrator topping up a client that ran out of volume brings it
	// back too.
	c := seed("topup", false, 10*gib, 10*gib, future)
	if err := b.addVolume(c.Id, 10*gib); err != nil {
		t.Fatal(err)
	}
	var fresh model.Client
	db.First(&fresh, c.Id)
	if !fresh.Enable || fresh.Volume != 20*gib {
		t.Errorf("+10 GB on a run-out client: enable %v, volume %d", fresh.Enable, fresh.Volume)
	}
}
