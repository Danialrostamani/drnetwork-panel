package tgbot

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/service"
)

func TestCustomerProfileActions(t *testing.T) {
	e, _ := shopEnv(t)
	s := &service.ShopService{}
	e.say(buyer, "/start")
	e.press(buyer, "p:trial")
	c := boundClients(buyer)[0]

	e.press(42, "q:usr")
	n := len(e.got())
	e.say(42, itoa(buyer))
	got := e.since(n)
	if txt := lastText(got, "sendMessage", "editMessageText"); !strings.Contains(txt, "<code>5000</code>") || !strings.Contains(txt, c.Name) {
		t.Fatalf("profile: %q", txt)
	}
	for _, d := range []string{"q:uw:5000", "q:ub:5000", "q:ug:5000", "q:um:5000"} {
		if !has(inlineData(got), d) {
			t.Fatalf("profile lacks %s: %v", d, inlineData(got))
		}
	}

	// The wallet, a block and back.
	e.press(42, "q:uw:5000")
	e.say(42, "250 outage")
	if s.Balance(buyer) != 250 {
		t.Fatalf("balance %d", s.Balance(buyer))
	}
	e.press(42, "q:ub:5000")
	if u := s.User(buyer); u == nil || !u.Blocked {
		t.Fatal("not blocked")
	}
	e.press(42, "q:ub:5000")
	if u := s.User(buyer); u.Blocked {
		t.Fatal("still blocked")
	}

	// A gift of volume and days to the one service, and a message.
	e.press(42, "q:ug:5000")
	e.say(42, "2 3")
	after := e.b.findClientByName(c.Name)
	if after.Volume != c.Volume+2*gib || after.Expiry != c.Expiry+3*86400 {
		t.Fatalf("gift: volume %d→%d expiry %d→%d", c.Volume, after.Volume, c.Expiry, after.Expiry)
	}
	n = len(e.got())
	e.press(42, "q:um:5000")
	e.say(42, "Hello there")
	if txt := lastText(to(e.since(n), buyer), "sendMessage"); !strings.Contains(txt, "Hello there") {
		t.Fatalf("message: %q", txt)
	}

	// Customers have none of it.
	n = len(e.got())
	e.press(referrer, "q:u:5000")
	if strings.Contains(lastText(e.since(n), "sendMessage", "editMessageText"), "<code>5000</code>") {
		t.Fatal("a customer saw a profile")
	}
	if users, _ := s.FindUsers("nobody-like-this", 5); len(users) != 0 {
		t.Fatalf("found %v", users)
	}
}

func TestBroadcastAudiences(t *testing.T) {
	e, gold := shopEnv(t)
	s := &service.ShopService{}
	broadcastGap = 0
	e.say(buyer, "/start")
	e.say(referrer, "/start")
	_, _ = s.AddFunds(nil, buyer, 1000, "admin", 0, 0)
	e.press(buyer, fmt.Sprintf("p:walY:%d:0", gold.Id))
	c := boundClients(buyer)[0]

	// The buyer's service is on node DE and in group vip.
	node := model.Node{Name: "DE"}
	if err := database.GetDB().Create(&node).Error; err != nil {
		t.Fatal(err)
	}
	in := model.Inbound{Tag: "de-in", Type: "vless", NodeId: &node.Id}
	if err := database.GetDB().Create(&in).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().Model(&model.Client{}).Where("id = ?", c.Id).Updates(map[string]any{"group": "vip", "inbounds": []byte(fmt.Sprintf("[%d]", in.Id))}).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for aud, want := range map[string][]int64{
		"":                              {buyer, referrer},
		service.AudienceBuyers:          {buyer},
		service.AudienceNever:           {referrer},
		service.AudienceIdle30:          {referrer},
		service.AudienceActive:          {buyer},
		service.AudienceExpired:         nil,
		"group:VIP":                     {buyer},
		fmt.Sprintf("node:%d", node.Id): {buyer},
		"node:999":                      nil,
	} {
		ids, err := s.AudienceTargets(aud, now)
		if err != nil {
			t.Fatal(aud, err)
		}
		if fmt.Sprint(sorted(ids)) != fmt.Sprint(sorted(want)) {
			t.Errorf("%q reaches %v, want %v", aud, ids, want)
		}
	}

	// Through the bot: preview, audience, count, send.
	e.press(42, "q:bc")
	n := len(e.got())
	e.say(42, "Promo for buyers")
	if !has(inlineData(e.since(n)), "q:bca:buyers") {
		t.Fatalf("no audiences: %+v", e.since(n))
	}
	n = len(e.got())
	e.press(42, "q:bca:buyers")
	if !has(inlineData(e.since(n)), "q:bcy:buyers") {
		t.Fatalf("no send button: %+v", e.since(n))
	}
	e.press(42, "q:bcy:buyers")
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && !strings.Contains(lastText(to(e.got(), buyer), "sendMessage"), "Promo for buyers") {
		time.Sleep(20 * time.Millisecond)
	}
	if !strings.Contains(lastText(to(e.got(), buyer), "sendMessage"), "Promo for buyers") {
		t.Fatal("the buyer did not get it")
	}
	if strings.Contains(lastText(to(e.got(), referrer), "sendMessage"), "Promo for buyers") {
		t.Fatal("the referrer got it")
	}
}

func sorted(ids []int64) []int64 {
	out := append([]int64(nil), ids...)
	for i := range out {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}
