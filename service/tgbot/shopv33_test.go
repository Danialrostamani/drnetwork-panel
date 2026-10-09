package tgbot

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/service"
)

func photoWithUID(from int64, fileID, uid string) update {
	var u update
	raw := `{"update_id":4,"message":{"message_id":6,"photo":[{"file_id":"small","file_unique_id":"small-u"},{"file_id":` + mustJSON(fileID) +
		`,"file_unique_id":` + mustJSON(uid) + `}],"chat":{"id":` + itoa(from) + `,"type":"private"},"from":{"id":` + itoa(from) + `,"first_name":"Buyer"}}}`
	_ = json.Unmarshal([]byte(raw), &u)
	return u
}

func TestShopGiftCodeAndReceiptReuse(t *testing.T) {
	e, p := shopEnv(t)
	s := &service.ShopService{}
	pid := itoa(int64(p.Id))
	if err := s.SaveDiscount(&model.ShopDiscount{Code: "GIFT7", Kind: model.CodeGift, Amount: 700, Enable: true}); err != nil {
		t.Fatal(err)
	}
	e.say(buyer, "/start")
	e.press(buyer, "p:gift")
	if txt := e.runSay(buyer, "gift7"); !strings.Contains(txt, "700 T was added to your wallet") {
		t.Fatalf("gift: %s", txt)
	}
	e.press(buyer, "p:gift")
	if txt := e.runSay(buyer, "GIFT7"); !strings.Contains(txt, "already used") || s.Balance(buyer) != 700 {
		t.Fatalf("second gift: %s, balance %d", txt, s.Balance(buyer))
	}
	// A gift code typed where a discount goes is redeemed too, once.
	e.press(referrer, "p:code:"+pid+":0")
	if txt := e.runSay(referrer, "gift7"); !strings.Contains(txt, "was added to your wallet") || s.Balance(referrer) != 700 {
		t.Fatalf("gift as a discount: %s", txt)
	}

	// One receipt pays for one order.
	e.press(buyer, "p:card:"+pid+":0")
	first := lastOrder(t)
	e.b.handle(context.Background(), photoWithUID(buyer, "file-a", "uniq-1"))
	if o, _ := s.Order(first.Id); o.Receipt != "photo:file-a" || o.ReceiptKey != "photo:uniq-1" {
		t.Fatalf("first receipt: %+v", o)
	}
	e.press(referrer, "p:card:"+pid+":0")
	second := lastOrder(t)
	n := len(e.got())
	e.b.handle(context.Background(), photoWithUID(referrer, "file-b", "uniq-1"))
	if txt := e.allText(n); !strings.Contains(txt, "already sent for another order") {
		t.Fatalf("reused receipt: %s", txt)
	}
	if o, _ := s.Order(second.Id); o.Receipt != "" {
		t.Fatalf("the reused receipt was kept: %+v", o)
	}
	// Still waiting: the right receipt is taken.
	e.b.handle(context.Background(), photoWithUID(referrer, "file-c", "uniq-2"))
	if o, _ := s.Order(second.Id); o.Receipt != "photo:file-c" {
		t.Fatalf("second receipt: %+v", o)
	}
}

func TestShopUniqueAmountGoesToTheWallet(t *testing.T) {
	e, p := shopEnv(t)
	s := &service.ShopService{}
	if err := s.SetSetting("shopUniqueAmount", "9"); err != nil {
		t.Fatal(err)
	}
	e.say(buyer, "/start")
	e.press(buyer, "p:card:"+itoa(int64(p.Id))+":0")
	o := lastOrder(t)
	if o.Extra < 1 || o.Extra > 9 {
		t.Fatalf("extra %d", o.Extra)
	}
	if txt := lastText(e.got(), "sendMessage"); !strings.Contains(txt, fmt.Sprintf("1,%03d T", o.Extra)) || !strings.Contains(txt, "6037-0000 Ali") {
		t.Fatalf("payment: %s", txt)
	}
	e.press(fullAdmin, fmt.Sprintf("q:ok:%d", o.Id))
	if got := lastOrder(t); got.Status != model.OrderApproved || s.Balance(buyer) != o.Extra {
		t.Fatalf("order %+v, wallet %d", got, s.Balance(buyer))
	}
}

func TestShopAutoRenewFromWallet(t *testing.T) {
	e, p := shopEnv(t)
	s := &service.ShopService{}
	pid := itoa(int64(p.Id))
	ctx := context.Background()
	e.say(buyer, "/start")
	if _, err := s.AddFunds(nil, buyer, 2500, "admin", 0, fullAdmin); err != nil {
		t.Fatal(err)
	}
	e.press(buyer, "p:walY:"+pid+":0")
	bought := lastOrder(t)
	c := e.b.findClientByName(bought.ClientName)
	if c == nil || c.TgId != buyer {
		t.Fatalf("purchase: %+v", bought)
	}
	// Off in the shop: no button, and pressing does nothing.
	if row := e.b.autoRenewRow(*c); row != nil {
		t.Fatalf("button while off: %+v", row)
	}
	if err := s.SetSetting("shopAutoRenew", "true"); err != nil {
		t.Fatal(err)
	}
	if row := e.b.autoRenewRow(*c); len(row) != 1 || !strings.Contains(row[0].Text, "off") {
		t.Fatalf("button: %+v", row)
	}
	// Somebody else cannot turn it on.
	e.press(referrer, fmt.Sprintf("p:ar:%d", c.Id))
	if _, ok := s.AutoRenewOf(c.Id); ok {
		t.Fatal("a stranger turned it on")
	}
	if txt := e.runPress(buyer, fmt.Sprintf("p:ar:%d", c.Id)); !strings.Contains(txt, "Auto-renewal of "+c.Name+" is on") {
		t.Fatalf("turning on: %s", txt)
	}
	// Not due yet: nothing happens.
	e.b.root().autoRenew(ctx, time.Now())
	if lastOrder(t).Id != bought.Id {
		t.Fatal("renewed a service that was not running out")
	}
	full, _ := rawFullClient(c.Id)
	full.Expiry = time.Now().Add(time.Hour).Unix()
	database.GetDB().Save(full)
	e.b.root().autoRenew(ctx, time.Now())
	renewal := lastOrder(t)
	if renewal.Id == bought.Id || !renewal.Auto || renewal.Status != model.OrderApproved || renewal.Kind != model.OrderRenew || renewal.ClientId != c.Id {
		t.Fatalf("renewal: %+v", renewal)
	}
	full, _ = rawFullClient(c.Id)
	if full.Expiry < time.Now().Add(30*24*time.Hour).Unix() || s.Balance(buyer) != 500 {
		t.Fatalf("after renewal: expiry %d, wallet %d", full.Expiry, s.Balance(buyer))
	}
	if txt := lastText(e.got(), "sendMessage"); !strings.Contains(txt, "Volume") && !strings.Contains(txt, c.Name) {
		t.Fatalf("customer view: %s", txt)
	}
	// Right after, it is not renewed again even when due.
	full.Expiry = time.Now().Add(time.Hour).Unix()
	database.GetDB().Save(full)
	e.b.root().autoRenew(ctx, time.Now())
	if lastOrder(t).Id != renewal.Id {
		t.Fatal("renewed twice in a row")
	}
	// Later, with too little money, the customer hears once a day; the
	// third day the renewal turns off.
	later := time.Now().Add(13 * time.Hour)
	for day := 0; day < 3; day++ {
		at := later.Add(time.Duration(day) * 25 * time.Hour)
		n := len(e.got())
		e.b.root().autoRenew(ctx, at)
		e.b.root().autoRenew(ctx, at.Add(time.Hour))
		var told []sent
		for _, m := range e.since(n) {
			if m.ChatID == buyer && strings.Contains(m.Text, "was not renewed automatically") {
				told = append(told, m)
			}
		}
		if len(told) != 1 {
			t.Fatalf("day %d: told %d times: %s", day, len(told), e.allText(n))
		}
		if day == 2 && !strings.Contains(told[0].Text, "is off now") {
			t.Fatalf("third failure: %s", told[0].Text)
		}
	}
	if _, err := s.AutoRenewOf(c.Id); !err || lastOrder(t).Id != renewal.Id || s.Balance(buyer) != 500 {
		t.Fatal("money was taken without a renewal")
	}
	if rows, _ := s.AutoRenews(); len(rows) != 0 {
		t.Fatalf("still on after three failures: %+v", rows)
	}
	// A service that changes hands stops renewing for the old owner.
	if _, err := s.SetAutoRenew(c.Id, buyer, true); err != nil {
		t.Fatal(err)
	}
	database.GetDB().Model(&model.Client{}).Where("id = ?", c.Id).Update("tg_id", referrer)
	e.b.root().autoRenew(ctx, later.Add(100*time.Hour))
	if rows, _ := s.AutoRenews(); len(rows) != 0 {
		t.Fatalf("renewal kept for the old owner: %+v", rows)
	}
}
