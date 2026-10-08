package tgbot

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/config"
	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/service"
)

// fakeNodeState is what the fake node reports; tests change it between probes.
type fakeNodeState struct {
	sync.Mutex
	cpu         float64
	memPct      int64
	diskPct     int64
	running     bool
	maintenance bool
	broken      bool
	restarts    int
	backups     int
}

func startFakeNode(t *testing.T, token string) (*httptest.Server, *fakeNodeState) {
	t.Helper()
	st := &fakeNodeState{cpu: 10, memPct: 40, diskPct: 30, running: true}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		st.Lock()
		defer st.Unlock()
		if st.broken {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		if r.Header.Get("Token") != token {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/app/apiv2/status":
			fmt.Fprintf(w, `{"success":true,"obj":{"cpu":%v,"mem":{"current":%d,"total":100},"dsk":{"current":%d,"total":100},`+
				`"sys":{"appVersion":"1.6.3","appFull":%q},"sbd":{"running":%v,"maintenance":%v,"version":"1.12.0"}}}`,
				st.cpu, st.memPct, st.diskPct, config.GetVersion(), st.running, st.maintenance)
		case "/app/apiv2/onlines":
			_, _ = w.Write([]byte(`{"success":true,"obj":{"user":["alice","bob"]}}`))
		case "/app/apiv2/restartSb":
			st.restarts++
			_, _ = w.Write([]byte(`{"success":true,"msg":"restartSb"}`))
		case "/app/apiv2/getdb":
			st.backups++
			_, _ = w.Write(append([]byte("SQLite format 3\x00"), make([]byte, 100)...))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, st
}

func (s *fakeNodeState) set(f func(s *fakeNodeState)) {
	s.Lock()
	defer s.Unlock()
	f(s)
}

// sentSince is the text of the messages sent after the first n.
func sentSince(got func() []sent, n int) []string {
	var out []string
	for _, m := range got()[n:] {
		if m.Method == "sendMessage" {
			out = append(out, m.Text)
		}
	}
	return out
}

func countContaining(texts []string, part string) int {
	n := 0
	for _, t := range texts {
		if strings.Contains(t, part) {
			n++
		}
	}
	return n
}

func TestNodeAlertsCardAndActions(t *testing.T) {
	b, got := testBot(t)
	ctx := context.Background()
	const token = "node-token-123456"
	srv, fake := startFakeNode(t, token)
	node := model.Node{Name: "de-1", Enable: true, BaseUrl: srv.URL, WebPath: "/app/", Token: token, Tags: []string{"germany", "fast"}, Country: "DE"}
	if err := database.GetDB().Create(&node).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// The restart below probes the node again a few seconds later; that
		// probe must not land in the next test's database.
		service.WaitNodeActionProbes()
		// Leave no node status behind for the other tests.
		database.GetDB().Model(model.Node{}).Where("1 = 1").Update("enable", false)
		(&service.NodeService{}).RefreshAll()
		service.DrainNodeEvents()
	})
	probe := func() {
		t.Helper()
		if _, err := (&service.NodeService{}).ProbeNow([]uint{node.Id}); err != nil {
			t.Fatal(err)
		}
	}
	watched := map[uint]*nodeWatch{}
	check := func(times int) []string {
		before := len(got())
		for i := 0; i < times; i++ {
			b.checkNodes(ctx, watched)
		}
		return sentSince(got, before)
	}

	// Healthy: no alert at all.
	probe()
	if msgs := check(10); len(msgs) != 0 {
		t.Fatalf("a healthy node raised alerts: %q", msgs)
	}

	// The disk over its limit is announced after 3 checks, a busy CPU only
	// after 6, and each just once.
	fake.set(func(s *fakeNodeState) { s.cpu, s.diskPct = 97, 95 })
	probe()
	msgs := check(nodeDownAfterChecks)
	if countContaining(msgs, "Disk filling up: 95%") != 1 || countContaining(msgs, "High CPU") != 0 {
		t.Fatalf("after 3 checks: %q", msgs)
	}
	msgs = check(cpuWarnAfterChecks - nodeDownAfterChecks)
	if countContaining(msgs, "High CPU: 97% (limit 90%)") != 1 || countContaining(msgs, "Disk") != 0 {
		t.Fatalf("after 6 checks: %q", msgs)
	}
	if msgs = check(20); len(msgs) != 0 {
		t.Fatalf("a warning was repeated: %q", msgs)
	}

	// Back under the limit: cleared only after it stays there a while.
	fake.set(func(s *fakeNodeState) { s.cpu = 20 })
	probe()
	if msgs = check(warnClearChecks - 1); len(msgs) != 0 {
		t.Fatalf("cleared too early: %q", msgs)
	}
	msgs = check(1)
	if countContaining(msgs, "CPU is back to normal") != 1 || len(msgs) != 1 {
		t.Fatalf("cleared message: %q", msgs)
	}
	// A short dip under the limit does not clear, and coming back over it
	// does not announce it again.
	fake.set(func(s *fakeNodeState) { s.diskPct = 50 })
	probe()
	check(5)
	fake.set(func(s *fakeNodeState) { s.diskPct = 96 })
	probe()
	if msgs = check(10); len(msgs) != 0 {
		t.Fatalf("a flapping disk warning was announced again: %q", msgs)
	}

	// The card shows the new details.
	text, kb := b.objCard(kindByCode("nd"), node.Id)
	for _, want := range []string{"🇩🇪", "🏷 germany, fast", "Disk", "Online: 2", "v" + config.GetVersion(), "Disk 96% ≥ 90%"} {
		if !strings.Contains(text, want) {
			t.Fatalf("card lacks %q:\n%s", want, text)
		}
	}
	if !hasData(kb, fmt.Sprintf("o:nd:rsb:%d", node.Id)) || !hasData(kb, fmt.Sprintf("o:nd:bk:%d", node.Id)) {
		t.Fatalf("card lacks the restart/backup buttons: %v", callbackData(kb))
	}
	if summary := b.nodesText(); !strings.Contains(summary, "👥 2") || !strings.Contains(summary, "Disk 96%") {
		t.Fatalf("nodes summary: %s", summary)
	}
	editable, err := b.objEditable(kindByCode("nd"), node.Id)
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"inboundCount", "clientCount", "syncReport", "tokenSet", "lastSeen"} {
		if _, ok := editable[k]; ok {
			t.Fatalf("editable JSON keeps the server-managed %q", k)
		}
	}

	// Maintenance stops the core on purpose: no down alert.
	fake.set(func(s *fakeNodeState) { s.running, s.maintenance = false, true })
	probe()
	if msgs = check(10); countContaining(msgs, "is down") != 0 {
		t.Fatalf("maintenance raised a down alert: %q", msgs)
	}
	// A core that stopped by itself is an outage.
	fake.set(func(s *fakeNodeState) { s.maintenance = false })
	probe()
	if msgs = check(nodeDownAfterChecks); countContaining(msgs, "Node de-1 is down") != 1 {
		t.Fatalf("stopped core: %q", msgs)
	}
	// Unreachable keeps the alert (sent once) and the warnings as they were.
	fake.set(func(s *fakeNodeState) { s.broken = true })
	probe()
	if msgs = check(10); len(msgs) != 0 {
		t.Fatalf("unreachable node repeated alerts: %q", msgs)
	}
	fake.set(func(s *fakeNodeState) { s.broken, s.running = false, true })
	probe()
	msgs = check(1)
	if countContaining(msgs, "Node de-1 is back online") != 1 || countContaining(msgs, "Disk") != 0 {
		t.Fatalf("recovery: %q", msgs)
	}

	// Buttons: test, restart the core, backup.
	before := len(got())
	b.handle(ctx, callbackFrom(42, fmt.Sprintf("o:nd:probe:%d", node.Id)))
	b.handle(ctx, callbackFrom(42, fmt.Sprintf("o:nd:rsb:%d", node.Id)))
	fake.Lock()
	restarts := fake.restarts
	fake.Unlock()
	if restarts != 1 || countContaining(sentSince(got, before), "core was restarted") != 1 {
		t.Fatalf("restart: %d restarts, %q", restarts, sentSince(got, before))
	}
	b.handle(ctx, callbackFrom(42, fmt.Sprintf("o:nd:bk:%d", node.Id)))
	deadline := time.Now().Add(10 * time.Second)
	for {
		docs := 0
		for _, m := range got() {
			if m.Method == "sendDocument" {
				docs++
			}
		}
		if docs == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no backup was sent: %q", sentSince(got, before))
		}
		time.Sleep(50 * time.Millisecond)
	}
	for _, m := range got()[before:] {
		if strings.Contains(m.Text, "❌") {
			t.Fatalf("an action failed: %s", m.Text)
		}
	}
}

func TestNodeEventTexts(t *testing.T) {
	b, _ := fakeTelegram(t)
	cases := []struct {
		e    service.NodeEvent
		want string
	}{
		{service.NodeEvent{Name: "a<b", Kind: "cap", Level: 80, Used: 80 << 30, Limit: 100 << 30}, "80% of the monthly traffic cap used: 80.00 GiB / 100.00 GiB"},
		{service.NodeEvent{Name: "a<b", Kind: "cap", Level: 100, Used: 101 << 30, Limit: 100 << 30}, "Monthly traffic cap reached: 101.00 GiB / 100.00 GiB"},
		{service.NodeEvent{Name: "a<b", Kind: "hidden", Reason: "down"}, "taken out of the subscriptions because it is down"},
		{service.NodeEvent{Name: "a<b", Kind: "hidden", Reason: "cap"}, "because its monthly cap is reached"},
		{service.NodeEvent{Name: "a<b", Kind: "shown", Reason: "down"}, "back in the subscriptions"},
	}
	for _, c := range cases {
		text := b.nodeEventText(c.e)
		if !strings.Contains(text, c.want) || !strings.Contains(text, "a&lt;b") {
			t.Fatalf("%+v: %q", c.e, text)
		}
	}
	if text := b.nodeEventText(service.NodeEvent{Kind: "other"}); text != "" {
		t.Fatalf("unknown event: %q", text)
	}
	// Each warning has its own text, in both languages.
	for _, lang := range []string{"en", "fa"} {
		b.cfg.Lang = lang
		for _, w := range []service.NodeWarning{{Key: "cpu", Value: 95, Limit: 90}, {Key: "mem", Value: 95, Limit: 90}, {Key: "disk", Value: 95, Limit: 90},
			{Key: "ping", Value: 900, Limit: 500}, {Key: "cert", Value: 3.5, Limit: 7}, {Key: "cert", Value: -1, Limit: 7}, {Key: "version", Info: "1.6.3-drnetwork.20"}} {
			text, cleared := b.nodeWarningText("n", w), b.nodeClearedText("n", w.Key)
			if strings.HasSuffix(text, w.Key) || strings.HasSuffix(cleared, w.Key) || strings.Contains(text, "%!") {
				t.Fatalf("%s %+v: %q / %q", lang, w, text, cleared)
			}
		}
	}
	b.cfg.Lang = "en"
	if text := b.nodeWarningText("n", service.NodeWarning{Key: "cert", Value: 0.2, Limit: 7}); !strings.Contains(text, "expires within a day") {
		t.Fatalf("cert under a day: %q", text)
	}
	if got := uptimeText(99.97); got != "99.9%" {
		t.Fatalf("uptimeText(99.97) = %q", got)
	}
	if got := uptimeText(-1); got != "—" {
		t.Fatalf("uptimeText(-1) = %q", got)
	}
	if flagEmoji("de") != "🇩🇪" || flagEmoji("d") != "" || flagEmoji("1a") != "" {
		t.Fatal("flagEmoji")
	}
	raw, _ := json.Marshal(map[string]interface{}{"tags": []string{"a", "b"}})
	if got := stringList(normalize(json.RawMessage(raw))["tags"]); strings.Join(got, ",") != "a,b" {
		t.Fatalf("stringList = %v", got)
	}
}

// A node going down or coming back, and its links leaving or coming back to
// the subscriptions, are for the owner alone. The other node notices still
// reach every administrator of the nodes, and a bot without an owner sends
// everything to them as before.
func TestNodeUpDownReachOnlyTheOwner(t *testing.T) {
	b, got := testBot(t)
	b.setAccess(buildAccess(7, []int64{42}, nil, nil, nil))
	ctx := context.Background()
	const token = "node-token-654321"
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
	probe := func() {
		t.Helper()
		if _, err := (&service.NodeService{}).ProbeNow([]uint{node.Id}); err != nil {
			t.Fatal(err)
		}
	}
	// recipients lists the chats of the messages sent after the first n that
	// contain part.
	recipients := func(n int, part string) []int64 {
		var out []int64
		for _, m := range got()[n:] {
			if m.Method == "sendMessage" && strings.Contains(m.Text, part) {
				out = append(out, m.ChatID)
			}
		}
		return out
	}
	watched := map[uint]*nodeWatch{}
	checks := func(n int) {
		for i := 0; i < n; i++ {
			b.checkNodes(ctx, watched)
		}
	}
	probe()
	checks(1)

	before := len(got())
	fake.set(func(s *fakeNodeState) { s.broken = true })
	probe()
	checks(nodeDownAfterChecks)
	if r := recipients(before, "Node fi-1 is down"); !reflect.DeepEqual(r, []int64{7}) {
		t.Fatalf("the down alert went to %v", r)
	}
	before = len(got())
	fake.set(func(s *fakeNodeState) { s.broken, s.diskPct = false, 95 })
	probe()
	checks(nodeDownAfterChecks)
	if r := recipients(before, "Node fi-1 is back online"); !reflect.DeepEqual(r, []int64{7}) {
		t.Fatalf("the back-online alert went to %v", r)
	}
	if r := recipients(before, "Disk filling up"); !reflect.DeepEqual(r, []int64{7, 42}) {
		t.Fatalf("the disk warning went to %v", r)
	}

	before = len(got())
	b.announceNodeEvents(ctx, []service.NodeEvent{
		{Name: "fi-1", Kind: "hidden", Reason: "down"},
		{Name: "fi-1", Kind: "cap", Level: 80, Used: 80 << 30, Limit: 100 << 30},
	})
	if r := recipients(before, "taken out of the subscriptions"); !reflect.DeepEqual(r, []int64{7}) {
		t.Fatalf("the links-out notice went to %v", r)
	}
	if r := recipients(before, "monthly traffic cap"); !reflect.DeepEqual(r, []int64{7, 42}) {
		t.Fatalf("the cap notice went to %v", r)
	}

	b.setAccess(buildAccess(0, []int64{42, 43}, nil, nil, nil))
	before = len(got())
	b.announceNodeEvents(ctx, []service.NodeEvent{{Name: "fi-1", Kind: "shown", Reason: "down"}})
	if r := recipients(before, "back in the subscriptions"); !reflect.DeepEqual(r, []int64{42, 43}) {
		t.Fatalf("without an owner the notice went to %v", r)
	}
}
