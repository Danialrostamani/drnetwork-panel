package tgbot

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/service"
)

func renewClient(t *testing.T, name string, tg int64, enable bool, f func(c *model.Client)) model.Client {
	t.Helper()
	c := model.Client{Name: name, Enable: enable, TgId: tg, Config: json.RawMessage(`{}`), Inbounds: json.RawMessage(`[]`), Links: json.RawMessage(`[]`)}
	f(&c)
	if err := database.GetDB().Create(&c).Error; err != nil {
		t.Fatal(err)
	}
	return c
}

func TestRenewalRemindersReachTheCustomerOnce(t *testing.T) {
	b, got := ownerBot(t, shopConfig{})
	ctx := context.Background()
	now := time.Now()
	const gb = int64(1 << 30)
	soon := renewClient(t, "soon", 6001, true, func(c *model.Client) { c.Expiry = now.Add(50 * time.Hour).Unix() })
	low := renewClient(t, "low", 6002, true, func(c *model.Client) { c.Volume, c.Up = 20*gb, 19*gb })
	renewClient(t, "old", 6003, false, func(c *model.Client) { c.Expiry = now.Add(-240 * time.Hour).Unix() })
	ended := renewClient(t, "ended", 6004, true, func(c *model.Client) { c.Volume, c.Down = 5*gb, 5*gb })
	far := renewClient(t, "far", 6005, true, func(c *model.Client) { c.Expiry = now.Add(500 * time.Hour).Unix() })
	renewClient(t, "mine", boss, true, func(c *model.Client) { c.Expiry = now.Add(time.Hour).Unix() })

	b.renewWatch(ctx, now)
	msgs := got()
	if m := sentTo(msgs, 6001); len(m) != 1 || !strings.Contains(m[0].Text, "Renewal reminder") || !strings.Contains(m[0].Text, "left until it expires") ||
		m[0].Markup == nil || !hasData(m[0].Markup.Keyboard, fmt.Sprintf("u:ren:%d", soon.Id)) {
		t.Fatalf("soon: %+v", m)
	}
	if m := sentTo(msgs, 6002); len(m) != 1 || !strings.Contains(m[0].Text, "95% of the data is used") || !hasData(m[0].Markup.Keyboard, fmt.Sprintf("u:ren:%d", low.Id)) {
		t.Fatalf("low: %+v", m)
	}
	if m := sentTo(msgs, 6004); len(m) != 1 || !strings.Contains(m[0].Text, "has ended") || !strings.Contains(m[0].Text, "data is used up") || !hasData(m[0].Markup.Keyboard, fmt.Sprintf("u:ren:%d", ended.Id)) {
		t.Fatalf("ended: %+v", m)
	}
	for _, id := range []int64{6003, 6005, boss} {
		if m := sentTo(msgs, id); len(m) != 0 {
			t.Fatalf("%d was reminded: %+v", id, m)
		}
	}

	// Told once; the notes are in the database, so a restart changes nothing.
	before := len(got())
	b.renewWatch(ctx, now.Add(10*time.Minute))
	b.renewWatch(ctx, now.Add(20*time.Minute))
	if n := len(got()) - before; n != 0 {
		t.Fatalf("reminded again: %+v", got()[before:])
	}
	// The ended client is off now and stays told.
	database.GetDB().Model(&model.Client{}).Where("id = ?", ended.Id).Update("enable", false)
	b.renewWatch(ctx, now.Add(30*time.Minute))
	if n := len(got()) - before; n != 0 {
		t.Fatalf("an ended client was told again: %+v", got()[before:])
	}

	// A renewal starts a new term: the next one is told again.
	database.GetDB().Model(&model.Client{}).Where("id = ?", soon.Id).Update("expiry", now.Add(400*time.Hour).Unix())
	b.renewWatch(ctx, now)
	if m := sentTo(got()[before:], 6001); len(m) != 0 {
		t.Fatalf("renewed client reminded: %+v", m)
	}
	database.GetDB().Model(&model.Client{}).Where("id = ?", soon.Id).Update("expiry", now.Add(20*time.Hour).Unix())
	b.renewWatch(ctx, now)
	if m := sentTo(got()[before:], 6001); len(m) != 1 {
		t.Fatalf("next term not reminded: %+v", m)
	}

	// The owner sets the days and can turn the reminders off.
	b.handle(ctx, callbackFrom(boss, "q:al"))
	if !strings.Contains(lastEdit(t, got), "Renewal reminder") {
		t.Fatalf("alerts screen: %s", lastEdit(t, got))
	}
	b.handle(ctx, callbackFrom(boss, "q:ald:30"))
	if prefInt(prefRenewDays, 3) != 30 {
		t.Fatal("days not set")
	}
	b.handle(ctx, callbackFrom(boss, "q:alt:"+prefRenew))
	if prefOn(prefRenew) {
		t.Fatal("reminders not switched off")
	}
	before = len(got())
	b.renewWatch(ctx, now)
	if m := sentTo(got()[before:], 6005); len(m) != 0 {
		t.Fatalf("reminded while off: %+v", m)
	}
	b.handle(ctx, callbackFrom(boss, "q:alt:"+prefRenew))
	b.renewWatch(ctx, now)
	if m := sentTo(got()[before:], 6005); len(m) != 1 || !strings.Contains(m[0].Text, "far") {
		t.Fatalf("30 days ahead not reminded: %+v", m)
	}
	_ = far

	// While they are on, the general client alerts do not message customers.
	before = len(got())
	b.checkClients(ctx, map[string]bool{})
	if m := sentTo(got()[before:], 6002); len(m) != 0 {
		t.Fatalf("customer got the general alert too: %+v", m)
	}
	// The button opens the renewal of that client.
	before = len(got())
	b.handle(ctx, callbackFrom(6002, fmt.Sprintf("u:ren:%d", low.Id)))
	if m := sentTo(got()[before:], 6002); len(m) == 0 {
		t.Fatal("renew button did nothing")
	}
}

func TestNodeConnectionTestAndOutageHistory(t *testing.T) {
	b, got := testBot(t)
	ctx := context.Background()
	const token = "node-token-abcdef"
	srv, fake := startFakeNode(t, token)
	node := model.Node{Name: "fi-1", Enable: true, BaseUrl: srv.URL, WebPath: "/app/", Token: token}
	if err := database.GetDB().Create(&node).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		service.WaitNodeActionProbes()
		database.GetDB().Model(model.Node{}).Where("1 = 1").Update("enable", false)
		(&service.NodeService{}).RefreshAll()
		service.DrainNodeEvents()
	})
	_, kb := b.objCard(kindByCode("nd"), node.Id)
	if !hasData(kb, fmt.Sprintf("o:nd:ntest:%d", node.Id)) || !hasData(kb, fmt.Sprintf("o:nd:otg:%d", node.Id)) {
		t.Fatalf("card lacks the test/outage buttons: %v", callbackData(kb))
	}
	before := len(got())
	b.handle(ctx, callbackFrom(42, fmt.Sprintf("o:nd:ntest:%d", node.Id)))
	msgs := sentSince(got, before)
	if countContaining(msgs, "Reachable; the node panel and the core are running") != 1 || countContaining(msgs, "Response:") != 1 {
		t.Fatalf("test: %q", msgs)
	}
	fake.set(func(s *fakeNodeState) { s.broken = true })
	before = len(got())
	b.handle(ctx, callbackFrom(42, fmt.Sprintf("o:nd:ntest:%d", node.Id)))
	if msgs = sentSince(got, before); countContaining(msgs, "Could not reach the node") != 1 {
		t.Fatalf("broken test: %q", msgs)
	}

	b.handle(ctx, callbackFrom(42, fmt.Sprintf("o:nd:otg:%d", node.Id)))
	if text := lastEdit(t, got); !strings.Contains(text, "No outage recorded") {
		t.Fatalf("empty history: %s", text)
	}
	now := time.Now().Unix()
	database.GetDB().Create(&model.NodeOutage{NodeId: node.Id, Start: now - 3*3600, End: now - 3600, State: "offline", Reason: "dial tcp: i/o timeout", Checked: now - 3600})
	database.GetDB().Create(&model.NodeOutage{NodeId: node.Id, Start: now - 600, State: "core-stopped", Reason: "core stopped", Checked: now})
	b.handle(ctx, callbackFrom(42, fmt.Sprintf("o:nd:otg:%d", node.Id)))
	text := lastEdit(t, got)
	for _, want := range []string{"Outages — fi-1", "24 hours: 2 outages", "i/o timeout", "ongoing", "🟠", "<b>2 h 0 min</b>"} {
		if !strings.Contains(text, want) {
			t.Fatalf("history lacks %q:\n%s", want, text)
		}
	}
}

func TestOutboundsFromLinksAndSubscriptions(t *testing.T) {
	b, got := testBot(t)
	ctx := context.Background()
	sub := base64.StdEncoding.EncodeToString([]byte("vless://11111111-2222-3333-4444-555555555555@1.2.3.4:443?security=tls&sni=a.example&type=tcp#My-VLESS\ntrojan://secret@9.9.9.9:443?security=tls&sni=b.example"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(sub)) }))
	defer srv.Close()

	b.handle(ctx, callbackFrom(42, "o:out:ls:0"))
	if !strings.Contains(fmt.Sprint(got()[len(got())-1].Markup.Keyboard), "o:out:lnk:0") {
		t.Fatal("no link button on the outbounds list")
	}
	b.handle(ctx, callbackFrom(42, "o:out:lnk:0"))
	if p := b.pend.get(42); p == nil || p.kind != "obj.links" {
		t.Fatalf("no prompt: %+v", p)
	}
	// Nothing usable keeps the prompt open.
	b.handle(ctx, privateMessage(42, "hello"))
	everything := func() string {
		return strings.Join(append(texts(got(), "sendMessage"), texts(got(), "editMessageText")...), "\n")
	}
	if p := b.pend.get(42); p == nil || !strings.Contains(everything(), "No outbound could be made") || !strings.Contains(everything(), "not a link") {
		t.Fatalf("bad input: %v\n%s", p, everything())
	}
	b.handle(ctx, privateMessage(42, "vless://11111111-2222-3333-4444-555555555555@5.6.7.8:443?security=reality&sni=c.example&pbk=abc&sid=01&type=tcp#My-VLESS\n"+srv.URL+"/sub\nnot-a-link"))
	var tags []string
	database.GetDB().Model(model.Outbound{}).Order("id").Pluck("tag", &tags)
	want := []string{"direct", "My-VLESS", "My-VLESS-2", "trojan-9.9.9.9"}
	if strings.Join(tags, ",") != strings.Join(want, ",") {
		t.Fatalf("tags = %v, want %v", tags, want)
	}
	if b.pend.get(42) != nil {
		t.Fatal("prompt still open after the import")
	}
	all := everything()
	if !strings.Contains(all, "3 outbounds added") || !strings.Contains(all, "not-a-link") {
		t.Fatalf("report: %s", all)
	}
}
