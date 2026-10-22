package tgbot

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
)

// The panel's members page blocks, moves and credits a customer the way the
// owner does from Telegram, and refuses to message anybody while the bot is off.
func TestPanelMemberActions(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "bot.db")); err != nil {
		t.Fatal(err)
	}
	if err := PanelSetBlocked(5, true); err != nil {
		t.Fatal(err)
	}
	if !customerBlocked(5) {
		t.Fatal("the customer was not blocked")
	}
	if err := PanelSetBlocked(5, false); err != nil || customerBlocked(5) {
		t.Fatal("the customer was not unblocked: ", err)
	}
	if err := PanelSetSeller(5, 0); err != nil {
		t.Fatal(err)
	}
	if err := PanelSetSeller(5, 777); err == nil {
		t.Fatal("somebody who is not a bot administrator was made the seller")
	}
	bal, err := PanelWallet(context.Background(), 5, 300, "gift", false)
	if err != nil || bal != 300 {
		t.Fatalf("balance = %d, %v", bal, err)
	}
	if bal, err = PanelWallet(context.Background(), 5, -500, "", false); err != nil || bal != 0 {
		t.Fatalf("balance = %d, %v", bal, err)
	}
	var txs []model.BotWalletTx
	database.GetDB().Where("tg_id = ?", 5).Order("id").Find(&txs)
	if len(txs) != 2 || txs[0].Kind != walletManual || txs[0].Note != "gift" || txs[1].Delta != -300 {
		t.Fatalf("history = %+v", txs)
	}
	if _, err := PanelMessage([]int64{5}, "hi"); err == nil {
		t.Fatal("a message went out with the bot off")
	}
	if _, err := PanelMessage([]int64{5}, "  "); err == nil {
		t.Fatal("an empty message was accepted")
	}
	if PanelSetBlocked(0, true) == nil || PanelSetSeller(-1, 0) == nil {
		t.Fatal("a bad id was accepted")
	}
}
