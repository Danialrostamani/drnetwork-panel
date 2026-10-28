package tgbot

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
)

// The client alerts carry what the client has, warn at 95% of the data and
// of the time, go once (also across restarts) and only to the people who
// still want them.
func TestClientAlertsDetailsFinalWarningsAndSwitches(t *testing.T) {
	e := newAccessEnv(t)
	ctx := context.Background()
	db := database.GetDB()
	now := time.Now()
	s1 := e.client("s1")
	s1.Volume, s1.Up, s1.TgId = 100, 96, 9001
	s1.CreatedAt, s1.Expiry = now.Add(-40*24*time.Hour).Unix(), now.Add(48*time.Hour).Unix()
	if err := db.Save(&s1).Error; err != nil {
		t.Fatal(err)
	}
	byChat := func(msgs []sent) map[int64]string {
		out := map[int64]string{}
		for _, m := range msgs {
			out[m.ChatID] += m.Text + "\n"
		}
		return out
	}
	e.b.checkClients(ctx, nil)
	got := byChat(e.got())
	owner := got[ownerID]
	for _, want := range []string{"<b>s1</b>", "96% of the data used (last warning)", "95% of the time gone", "less than 3 days until expiry",
		"Total: 100 B", "Group: Sales", "Seller: ", "Created: "} {
		if !strings.Contains(owner, want) {
			t.Errorf("owner alert lacks %q:\n%s", want, owner)
		}
	}
	if !strings.Contains(got[salesAdmin], "<b>s1</b>") || strings.Contains(got[salesAdmin], "o1") {
		t.Errorf("seller alert:\n%s", got[salesAdmin])
	}
	customer := got[9001]
	if !strings.Contains(customer, "last warning") || strings.Contains(customer, "Seller") || strings.Contains(customer, "less than 3 days") {
		t.Errorf("customer alert:\n%s", customer)
	}

	// Stored: a check with a new map (a restart) sends nothing again.
	n := len(e.got())
	e.b.checkClients(ctx, map[string]bool{})
	if len(e.got()) != n {
		t.Fatalf("sent again: %+v", e.since(n))
	}

	// An administrator mutes them for themselves; the owner turns off those
	// to the owner; the customer mutes theirs with the button.
	e.say(fullAdmin, "/alerts off")
	if personAlertsOn(fullAdmin) {
		t.Fatal("/alerts off did not mute")
	}
	if err := setPref(prefCAOwner, "0"); err != nil {
		t.Fatal(err)
	}
	o1 := e.client("o1")
	o1.Volume, o1.Up = 100, 97
	db.Save(&o1)
	e.press(9001, "u:al:"+e.cid("s1"))
	if personAlertsOn(9001) {
		t.Fatal("the customer's button did not mute")
	}
	n = len(e.got())
	e.b.checkClients(ctx, nil)
	got = byChat(e.since(n))
	if got[fullAdmin] != "" || got[ownerID] != "" {
		t.Fatalf("muted people alerted: %+v", got)
	}
	if !strings.Contains(got[clientsOnly], "<b>o1</b>") {
		t.Fatalf("an admin with Clients missed o1: %+v", got)
	}

	// A renewal starts the term over: the time alert is told again when due.
	var st model.BotClientAlert
	db.First(&st, s1.Id)
	if !strings.Contains(st.Sent, "time95") || !strings.Contains(st.Sent, "vol95") {
		t.Fatalf("stored stages: %q", st.Sent)
	}
	s1 = e.client("s1")
	s1.Expiry = now.Add(30 * 24 * time.Hour).Unix()
	db.Save(&s1)
	e.b.checkClients(ctx, nil)
	db.First(&st, s1.Id)
	if strings.Contains(st.Sent, "time95") || st.Start < now.Unix()-5 {
		t.Fatalf("renewal kept the old term: %+v", st)
	}

	// The card's time bar counts from the new term, not from the creation 40
	// days ago: right after the change, and after the next look too.
	if bar := e.b.expiryBar(s1, now); !strings.Contains(bar, " 0%") {
		t.Fatalf("time bar after a renewal: %q", bar)
	}
	if err := e.b.setExpiryDays(s1.Id, 10); err != nil {
		t.Fatal(err)
	}
	if bar := e.b.expiryBar(e.client("s1"), now); !strings.Contains(bar, " 0%") {
		t.Fatalf("time bar right after a new expiry: %q", bar)
	}
}
