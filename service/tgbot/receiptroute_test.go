package tgbot

import (
	"context"
	"testing"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
)

// approveButtons tells who of the chats got the order's Approve button.
func approveButtons(msgs []sent, o model.BotOrder, chats ...int64) map[int64]bool {
	out := map[int64]bool{}
	for _, c := range chats {
		for _, m := range sentTo(msgs, c) {
			if m.Markup != nil && hasData(m.Markup.Keyboard, "j:ok:"+itoa(int64(o.Id))) {
				out[c] = true
			}
		}
	}
	return out
}

// The owner picks, per seller, who gets the seller's receipts to approve.
func TestReceiptRouting(t *testing.T) {
	b, got := ownerBot(t, shopConfig{PayCard: "6037", Plans: []shopPlan{{ID: "1", Name: "Gold", Days: 30, GB: 10, Price: "100"}}})
	// This tests delegation to an ordinary, section-limited approver.
	// Trusted full partners can handle every order independently of routing.
	cfg := b.cfg
	cfg.Perms = map[int64]sectionSet{helper: {"clients": true}}
	useAccess(b, cfg)
	ctx := context.Background()
	b.handle(ctx, callbackFrom(boss, "q:cas:5001:77"))
	route := func(to int64) {
		t.Helper()
		if _, err := patchAdminMeta(seller, func(m *model.BotAdminMeta) { m.ReceiptTo = to }); err != nil {
			t.Fatal(err)
		}
	}
	order := func() (model.BotOrder, map[int64]bool) {
		t.Helper()
		b.handle(ctx, callbackFrom(buyer, "u:buy:1"))
		o := lastOrder(t, buyer)
		if o.AdminId != seller {
			t.Fatalf("order goes to %d", o.AdminId)
		}
		n := len(got())
		b.handle(ctx, photoMessage(buyer, "r", false))
		return o, approveButtons(got()[n:], o, boss, seller, helper)
	}

	// Default: the seller and the owner.
	o, who := order()
	if !who[seller] || !who[boss] || who[helper] {
		t.Fatalf("default route: %v", who)
	}
	b.handle(ctx, callbackFrom(helper, "j:no:"+itoa(int64(o.Id))))
	if lastOrder(t, buyer).Status != orderPending {
		t.Fatal("an administrator the order was not routed to handled it")
	}
	b.handle(ctx, callbackFrom(seller, "j:no:"+itoa(int64(o.Id))))

	// The seller alone.
	route(receiptToSelf)
	o, who = order()
	if !who[seller] || who[boss] || who[helper] {
		t.Fatalf("seller-only route: %v", who)
	}
	b.handle(ctx, callbackFrom(seller, "j:ok:"+itoa(int64(o.Id))))
	if lastOrder(t, buyer).Status != orderApproved {
		t.Fatal("the seller could not approve")
	}

	// Another administrator: they alone get it, and approving it delivers the
	// seller's sale, in the seller's group.
	route(helper)
	o, who = order()
	if !who[helper] || who[boss] || who[seller] {
		t.Fatalf("helper route: %v", who)
	}
	b.handle(ctx, callbackFrom(helper, "j:ok:"+itoa(int64(o.Id))))
	done := lastOrder(t, buyer)
	if done.Status != orderApproved || done.AdminId != seller || done.ClientId == 0 {
		t.Fatalf("routed approval: %+v", done)
	}
	var c model.Client
	if err := database.GetDB().First(&c, done.ClientId).Error; err != nil || c.Group != "Sales" {
		t.Fatalf("client %+v, %v", c, err)
	}

	// Owner-only wins over the route.
	b.handle(ctx, callbackFrom(boss, "q:ao:77"))
	o, who = order()
	if !who[boss] || who[helper] || who[seller] {
		t.Fatalf("owner-only route: %v", who)
	}
	b.handle(ctx, callbackFrom(helper, "j:ok:"+itoa(int64(o.Id))))
	if lastOrder(t, buyer).Status != orderPending {
		t.Fatal("the helper approved an owner-only order")
	}

	// A route to somebody who left the bot falls back to the default.
	b.handle(ctx, callbackFrom(boss, "q:ao:77"))
	route(4242)
	if a, _ := b.orderRoute(seller); len(a) != 2 || a[0] != seller || a[1] != boss {
		t.Fatalf("stale route: %v", a)
	}
}
