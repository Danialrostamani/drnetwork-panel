package tgbot

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
)

func pageClient(t *testing.T, name, group string, tg int64) model.Client {
	t.Helper()
	c := model.Client{Name: name, Group: group, TgId: tg, Enable: true, Inbounds: json.RawMessage(`[]`), Links: json.RawMessage(`[]`), Config: json.RawMessage(`{}`)}
	if err := database.GetDB().Create(&c).Error; err != nil {
		t.Fatal(err)
	}
	return c
}

// The page of a client shows its seller's support, channel and plans.
func TestSubPageInfoIsTheSellers(t *testing.T) {
	b, _ := ownerBot(t, shopConfig{
		Plans: []shopPlan{{ID: "1", Name: "Owner plan", Days: 30, GB: 50, Price: "100"}},
		Desks: map[string]shopDesk{"77": {
			Plans:   []shopPlan{{ID: "1", Name: "Seller plan", Days: 60, GB: 0, Price: "200"}, {ID: "2"}},
			Support: "@sellersupport", Channel: "javascript:alert(1)",
		}},
	})
	sales := pageClient(t, "sales1", "Sales", 0)
	other := pageClient(t, "other1", "", 0)

	got := b.subPageInfo(sales)
	if len(got.Plans) != 1 || got.Plans[0].Name != "Seller plan" || got.Plans[0].Days != 60 {
		t.Fatalf("seller plans: %+v", got.Plans)
	}
	if got.Support != "https://t.me/sellersupport" || got.Channel != "" {
		t.Fatalf("seller links: %q %q", got.Support, got.Channel)
	}
	got = b.subPageInfo(other)
	if len(got.Plans) != 1 || got.Plans[0].Name != "Owner plan" || got.Support != "" {
		t.Fatalf("owner page: %+v", got)
	}
	// No running bot, no bot part.
	if SubPageExtras(&sales).Bot != "" {
		t.Fatal("extras without a running bot")
	}
}

func startArg(prefix, id string) string {
	return prefix + base64.RawURLEncoding.EncodeToString([]byte(id))
}

// The page's buttons open the bot on the subscription: to renew it when it
// is the customer's, to connect it otherwise.
func TestStartFromTheSubPage(t *testing.T) {
	b, got := ownerBot(t, shopConfig{Plans: []shopPlan{{ID: "1", Name: "Gold", Days: 30, GB: 10, Price: "100"}}})
	ctx := context.Background()
	mine := pageClient(t, "mine1", "", buyer)
	free := pageClient(t, "free1", "", 0)
	pageClient(t, "taken1", "", 9999)

	n := len(got())
	b.handle(ctx, privateMessage(buyer, "/start "+startArg(startRenew, mine.Name)))
	found := false
	for _, m := range got()[n:] {
		if m.Markup != nil && hasData(m.Markup.Keyboard, "u:rb:"+itoa(int64(mine.Id))+":1") {
			found = true
		}
	}
	if !found {
		t.Fatal("renew did not offer the plans for that service")
	}

	const stranger = int64(6006)
	b.handle(ctx, privateMessage(stranger, "/start "+startArg(startConnect, free.Name)))
	o := lastOrder(t, stranger)
	if o.Topic != "bind" || o.Extra != itoa(int64(free.Id)) || o.Status != orderPending {
		t.Fatalf("connect order: %+v", o)
	}

	n = len(got())
	b.handle(ctx, privateMessage(stranger, "/start "+startArg(startConnect, "taken1")))
	if !strings.Contains(lastTo(t, got()[n:], stranger), "another account") {
		t.Fatal("a subscription of someone else was not refused")
	}
	n = len(got())
	b.handle(ctx, privateMessage(stranger, "/start "+startArg(startRenew, "nobody")))
	if !strings.Contains(lastTo(t, got()[n:], stranger), "not found") {
		t.Fatal("an unknown subscription was not reported")
	}
}

// The page shows the plan a client was last sold or renewed with.
func TestLastPlanOfAClient(t *testing.T) {
	_, _ = ownerBot(t, shopConfig{Plans: []shopPlan{{ID: "p1", Name: "Gold", Days: 30, GB: 50, Price: "100"}}})
	c := pageClient(t, "gold1", "", 0)
	if LastPlanOf(&c) != nil {
		t.Fatal("a plan without orders")
	}
	db := database.GetDB()
	db.Create(&model.BotOrder{TgId: 5, Topic: "buy", PlanID: "p1", Status: orderApproved, ClientId: c.Id, PlanSet: true, PlanGB: 50, PlanDays: 30})
	db.Create(&model.BotOrder{TgId: 5, Topic: "renew", PlanID: "p1", Status: orderApproved, ClientId: c.Id, PlanSet: true, PlanGB: 60, PlanDays: 31})
	db.Create(&model.BotOrder{TgId: 5, Topic: "renew", PlanID: "p1", Status: orderPending, ClientId: c.Id, PlanSet: true, PlanGB: 70, PlanDays: 32})
	p := LastPlanOf(&c)
	if p == nil || p.Name != "Gold" || p.GB != 60 || p.Days != 31 || p.Kind != "renew" || p.At == 0 {
		t.Fatalf("last plan %+v", p)
	}
}
