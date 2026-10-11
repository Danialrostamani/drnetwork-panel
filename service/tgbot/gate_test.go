package tgbot

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/service"
)

func TestShopMaintenanceAndChannelLock(t *testing.T) {
	e, _ := shopEnv(t)
	svc := &service.ShopService{}
	b := e.b

	// Maintenance answers customers only with its message; administrators
	// go on as before.
	if err := svc.SetSetting("shopMaintenance", "back at 5"); err != nil {
		t.Fatal(err)
	}
	n := len(e.got())
	e.say(buyer, "/start")
	if out := e.since(n); len(out) != 1 || !strings.Contains(out[0].Text, "back at 5") {
		t.Fatalf("maintenance: %+v", out)
	}
	n = len(e.got())
	e.press(buyer, "p:plans")
	if txt := lastText(e.since(n), "editMessageText"); !strings.Contains(txt, "back at 5") {
		t.Fatalf("maintenance button: %q", txt)
	}
	n = len(e.got())
	e.say(42, "/start")
	if txt := lastText(e.since(n), "sendMessage"); strings.Contains(txt, "back at 5") {
		t.Fatalf("an administrator got the maintenance message")
	}
	_ = svc.SetSetting("shopMaintenance", "")

	// The channel lock: a bad value is refused, a customer outside the
	// channel gets the join button and "I joined" checks again.
	if err := svc.SetSetting("shopChannel", "-100123"); err == nil {
		t.Fatal("a private channel without its link was taken")
	}
	if err := svc.SetSetting("shopChannel", "@drnet_ch"); err != nil {
		t.Fatal(err)
	}
	if st := svc.Settings(); st.Channel != "@drnet_ch" || st.ChannelLink != "https://t.me/drnet_ch" {
		t.Fatalf("channel %+v", st)
	}
	fakeAPI.Lock()
	fakeAPI.members = map[int64]bool{}
	fakeAPI.Unlock()
	n = len(e.got())
	e.say(buyer, "/start")
	out := e.since(n)
	if len(out) != 1 || !strings.Contains(out[0].Text, b.tr("عضو کانال", "Join our channel")) {
		t.Fatalf("lock: %+v", out)
	}
	n = len(e.got())
	e.press(buyer, "p:joined")
	if txt := lastText(e.since(n), "answerCallbackQuery"); !strings.Contains(txt, b.tr("هنوز عضو", "not joined")) {
		t.Fatalf("not joined: %q", txt)
	}
	fakeAPI.Lock()
	fakeAPI.members[buyer] = true
	fakeAPI.Unlock()
	n = len(e.got())
	e.press(buyer, "p:joined")
	home, _ := b.shopHome(buyer)
	if txt := lastText(e.since(n), "sendMessage"); txt != home {
		t.Fatalf("joined: %q", txt)
	}
	// A channel the bot cannot read locks nobody out.
	fakeAPI.Lock()
	fakeAPI.members = nil
	fakeAPI.Unlock()
	b.members = &memberCache{}
	n = len(e.got())
	e.say(buyer, "/start")
	if txt := lastText(e.since(n), "sendMessage"); txt != home {
		t.Fatalf("unreadable channel: %q", txt)
	}
}

func TestShopAutoDeleteRemovesOnlyOldShopServices(t *testing.T) {
	e, _ := shopEnv(t)
	db := database.GetDB()
	now := time.Now()
	old := now.Add(-5 * 24 * time.Hour).Unix()
	mk := func(name string, tg int64, expiry int64) model.Client {
		c := seedClient(t, name, "", 0, 0)
		if err := db.Model(&model.Client{}).Where("id = ?", c.Id).Updates(map[string]any{"tg_id": tg, "expiry": expiry}).Error; err != nil {
			t.Fatal(err)
		}
		return c
	}
	sold := mk("u_sold", buyer, old)
	fresh := mk("u_fresh", buyer, now.Add(-24*time.Hour).Unix())
	trial := mk("ut"+itoa(buyer), buyer, old)
	hand := mk("handmade", 0, old)
	for _, c := range []model.Client{sold, fresh} {
		if err := db.Create(&model.ShopOrder{TgId: buyer, Kind: model.OrderBuy, ClientId: c.Id, ClientName: c.Name, Status: model.OrderApproved}).Error; err != nil {
			t.Fatal(err)
		}
	}
	exists := func(id uint) bool {
		var n int64
		db.Model(&model.Client{}).Where("id = ?", id).Count(&n)
		return n == 1
	}

	e.b.root().autoDelete(context.Background(), now)
	if !exists(sold.Id) || !exists(trial.Id) {
		t.Fatal("deleted while the setting is off")
	}
	if err := (&service.ShopService{}).SetSetting("shopAutoDelete", "3"); err != nil {
		t.Fatal(err)
	}
	n := len(e.got())
	e.b.root().autoDelete(context.Background(), now)
	if exists(sold.Id) || exists(trial.Id) {
		t.Fatal("old shop services kept")
	}
	if !exists(fresh.Id) || !exists(hand.Id) {
		t.Fatal("a recent or hand-made client was deleted")
	}
	var told string
	for _, m := range e.since(n) {
		if m.ChatID == fullAdmin {
			told = m.Text
		}
	}
	if !strings.Contains(told, "u_sold") || !strings.Contains(told, "ut"+itoa(buyer)) || strings.Contains(told, "handmade") {
		t.Fatalf("admin told %q", told)
	}
}
