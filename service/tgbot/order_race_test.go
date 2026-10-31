package tgbot

import (
	"context"
	"testing"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
)

// The web panel decides orders beside the bot. A seller tapping approve while
// an operator approved the same order on the web used to find it still
// pending and deliver it a second time -- here, top up the wallet twice. The
// bot now decides under the panel's lock and reads the order inside it.
func TestBotOrderDecisionWaitsForThePanel(t *testing.T) {
	b, _ := testBot(t)
	staff := b.as(42)
	ctx := context.Background()
	o := &model.BotOrder{TgId: buyer, Topic: "wallet", Status: orderPending, Amount: 100, Detail: "top-up"}
	if err := staff.saveOrder(o); err != nil {
		t.Fatal(err)
	}

	orderDecisionMu.Lock() // the panel is deciding this order
	done := make(chan struct{})
	go func() {
		defer close(done)
		staff.orderCallback(ctx, "cb", 42, 1, []string{"j", "ok", itoa(int64(o.Id))})
	}()
	select {
	case <-done:
		orderDecisionMu.Unlock()
		t.Fatal("the bot decided the order while the panel held it")
	case <-time.After(100 * time.Millisecond):
	}

	// The panel approves it and tops the wallet up.
	if err := database.GetDB().Model(&model.BotOrder{}).Where("id = ?", o.Id).Update("status", orderApproved).Error; err != nil {
		orderDecisionMu.Unlock()
		t.Fatal(err)
	}
	if _, err := walletCredit(buyer, 100, walletMemo{kind: walletTopUp, order: o.Id}); err != nil {
		orderDecisionMu.Unlock()
		t.Fatal(err)
	}
	orderDecisionMu.Unlock()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the bot's decision never ran")
	}
	if got := walletBalance(buyer); got != 100 {
		t.Fatalf("wallet balance %d after one approved top-up of 100: the order was delivered twice", got)
	}
}

// Without anyone else deciding, the bot's approval goes through as before.
func TestBotApprovesAPendingTopUp(t *testing.T) {
	b, _ := testBot(t)
	staff := b.as(42)
	o := &model.BotOrder{TgId: buyer, Topic: "wallet", Status: orderPending, Amount: 250, Detail: "top-up"}
	if err := staff.saveOrder(o); err != nil {
		t.Fatal(err)
	}
	staff.orderCallback(context.Background(), "cb", 42, 1, []string{"j", "ok", itoa(int64(o.Id))})
	stored, err := loadOrder(o.Id)
	if err != nil || stored.Status != orderApproved {
		t.Fatalf("order after approval: %+v %v", stored, err)
	}
	if got := walletBalance(buyer); got != 250 {
		t.Fatalf("wallet balance %d, want 250", got)
	}
}
