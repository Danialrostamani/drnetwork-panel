package tgbot

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/service"
)

// The panel lists orders of every status and corrects them: a rejected order
// approved later is delivered, an approved one rejected keeps its account,
// wallet payments follow the status, and a settled order stays as it is.
func TestPanelEditsOrdersOfAnyStatus(t *testing.T) {
	b, _ := testBot(t)
	setShop(t, shopConfig{Plans: []shopPlan{{ID: "1", Name: "Gold", Days: 30, GB: 1, Price: "100"}}})
	raw, _ := json.Marshal(map[string]string{"tgBotOwner": "1000"})
	if err := (&service.SettingService{}).Save(database.GetDB(), raw); err != nil {
		t.Fatal(err)
	}
	cs := b.configService
	ctx := context.Background()
	o := &model.BotOrder{TgId: buyer, Topic: "buy", PlanID: "1", PlanSet: true, PlanGB: 1, PlanDays: 30, Status: orderPending, Detail: "Gold", Amount: 100, Hint: "zed"}
	if err := b.saveOrder(o); err != nil {
		t.Fatal(err)
	}
	edit := func(e PanelOrderEdit) (PanelOrderRow, error) {
		e.Id = o.Id
		return PanelEditOrder(ctx, cs, "admin", e)
	}
	if r, err := edit(PanelOrderEdit{Status: orderRejected}); err != nil || r.Status != orderRejected {
		t.Fatalf("reject: %+v %v", r, err)
	}
	amount, detail := int64(150), " Gold, corrected "
	if r, err := edit(PanelOrderEdit{Amount: &amount, Detail: &detail}); err != nil || r.Amount != 150 || r.Detail != "Gold, corrected" || r.Status != orderRejected {
		t.Fatalf("edit: %+v %v", r, err)
	}
	r, err := edit(PanelOrderEdit{Status: orderApproved})
	if err != nil || r.Status != orderApproved || r.ClientId == 0 || r.ClientName == "" {
		t.Fatalf("approve: %+v %v", r, err)
	}
	if _, err := edit(PanelOrderEdit{Status: orderApproved}); err != nil {
		t.Fatalf("same status again: %v", err)
	}
	if r, err := edit(PanelOrderEdit{Status: orderRejected}); err != nil || r.Status != orderRejected {
		t.Fatalf("un-approve: %+v %v", r, err)
	}
	var clients int64
	database.GetDB().Model(&model.Client{}).Where("id = ?", r.ClientId).Count(&clients)
	if clients != 1 {
		t.Fatal("rejecting an approved order deleted its account")
	}

	if _, err := walletCredit(buyer, 500, walletMemo{kind: walletManual}); err != nil {
		t.Fatal(err)
	}
	w := &model.BotOrder{TgId: buyer, Topic: "renew", Extra: itoa(int64(r.ClientId)), PlanSet: true, PlanGB: 1, PlanDays: 30, Status: orderRejected, Amount: 100, WalletPaid: true}
	if err := b.saveOrder(w); err != nil {
		t.Fatal(err)
	}
	database.GetDB().Model(&model.Client{}).Where("id = ?", r.ClientId).Update("tg_id", buyer)
	if r, err := PanelEditOrder(ctx, cs, "admin", PanelOrderEdit{Id: w.Id, Status: orderApproved}); err != nil || r.Status != orderApproved || walletBalance(buyer) != 400 {
		t.Fatalf("wallet re-approve: %+v %v balance %d", r, err, walletBalance(buyer))
	}
	more := int64(1)
	if _, err := PanelEditOrder(ctx, cs, "admin", PanelOrderEdit{Id: w.Id, Amount: &more}); err == nil {
		t.Fatal("changed the amount the wallet was charged with")
	}
	if _, err := PanelEditOrder(ctx, cs, "admin", PanelOrderEdit{Id: w.Id, Status: orderRejected}); err != nil || walletBalance(buyer) != 500 {
		t.Fatalf("wallet refund: %v balance %d", err, walletBalance(buyer))
	}

	database.GetDB().Model(&model.BotOrder{}).Where("id = ?", w.Id).Update("settlement_id", 3)
	if _, err := PanelEditOrder(ctx, cs, "admin", PanelOrderEdit{Id: w.Id, Status: orderApproved}); err != errOrderSettled {
		t.Fatalf("settled order changed: %v", err)
	}
	note := "kept"
	if r, err := PanelEditOrder(ctx, cs, "admin", PanelOrderEdit{Id: w.Id, Detail: &note}); err != nil || r.Detail != "kept" || !r.Settled {
		t.Fatalf("settled detail: %+v %v", r, err)
	}
	stranger := int64(4242)
	if _, err := edit(PanelOrderEdit{Seller: &stranger}); err == nil {
		t.Fatal("gave the order to somebody who is no seller")
	}

	page, err := PanelOrders(PanelOrderQuery{Status: orderRejected, Seller: -1})
	if err != nil || page.Total != 2 || len(page.Rows) != 2 || page.Counts[orderRejected] != 2 {
		t.Fatalf("list: %+v %v", page, err)
	}
	if page, _ := PanelOrders(PanelOrderQuery{Seller: -1, Search: "corrected"}); page.Total != 1 || page.Rows[0].Id != o.Id {
		t.Fatalf("search: %+v", page)
	}
	if page, _ := PanelOrders(PanelOrderQuery{Seller: -1, Search: "#" + itoa(int64(w.Id))}); page.Total != 1 || page.Rows[0].Id != w.Id {
		t.Fatalf("search by number: %+v", page)
	}
	if _, err := PanelOrders(PanelOrderQuery{Status: "bogus", Seller: -1}); err == nil {
		t.Fatal("unknown status listed")
	}
}
