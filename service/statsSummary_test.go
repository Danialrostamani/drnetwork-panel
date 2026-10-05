package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
)

// fakeSummaryNode is a node's API as far as statsTotals goes.
type fakeSummaryNode struct {
	srv     *httptest.Server
	mu      sync.Mutex
	queries []url.Values
	mode    string // "ok", "unknown" (a node from before statsTotals), "status", "garbage", "hang"
	status  int
	answer  StatsSummary
}

func newFakeSummaryNode(t *testing.T, answer StatsSummary) *fakeSummaryNode {
	t.Helper()
	f := &fakeSummaryNode{mode: "ok", status: http.StatusBadGateway, answer: answer}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Token") != fakeNodeToken || r.URL.Path != "/app/apiv2/statsTotals" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		f.mu.Lock()
		f.queries = append(f.queries, r.URL.Query())
		mode, status, answer := f.mode, f.status, f.answer
		f.mu.Unlock()
		switch mode {
		case "unknown":
			_, _ = w.Write([]byte(`{"success":false,"msg":"failed: unknown action: statsTotals"}`))
		case "status":
			w.WriteHeader(status)
		case "garbage":
			_, _ = w.Write([]byte(`{"success":true,"obj":"not a summary"}`))
		case "hang":
			select {
			case <-r.Context().Done():
			case <-time.After(10 * time.Second):
			}
		default:
			body, _ := json.Marshal(map[string]any{"success": true, "obj": answer})
			_, _ = w.Write(body)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeSummaryNode) setMode(mode string) {
	f.mu.Lock()
	f.mode = mode
	f.mu.Unlock()
}

func (f *fakeSummaryNode) calls() []url.Values {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]url.Values(nil), f.queries...)
}

func addStat(t *testing.T, resource, tag string, at, up, down int64) {
	t.Helper()
	rows := []model.Stats{
		{DateTime: at, Resource: resource, Tag: tag, Direction: true, Traffic: up},
		{DateTime: at, Resource: resource, Tag: tag, Direction: false, Traffic: down},
	}
	if err := database.GetDB().Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
}

type summaryWorld struct {
	nl, de         *fakeSummaryNode
	nlNode, deNode model.Node
	since, bucket  int64
	slot           int64 // start of the hour the samples are in
}

// setUpSummaryWorld is a master with two nodes. Inbound "local" runs on the
// master, "nl-a" and "nl-b" are replicas of node nl, "de-a" of node de. The
// clients are alice (everywhere) and bob (master only).
func setUpSummaryWorld(t *testing.T, nlAnswer, deAnswer StatsSummary) *summaryWorld {
	t.Helper()
	if err := database.InitDB(filepath.Join(t.TempDir(), "summary.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		nodeStatusMu.Lock()
		nodeStatuses = map[uint]NodeStatus{}
		nodeStatusMu.Unlock()
	})
	w := &summaryWorld{nl: newFakeSummaryNode(t, nlAnswer), de: newFakeSummaryNode(t, deAnswer), bucket: 3600}
	db := database.GetDB()
	w.nlNode = model.Node{Name: "nl", Enable: true, BaseUrl: w.nl.srv.URL, WebPath: "/app/", Token: fakeNodeToken}
	w.deNode = model.Node{Name: "de", Enable: true, BaseUrl: w.de.srv.URL, WebPath: "/app/", Token: fakeNodeToken}
	for _, n := range []*model.Node{&w.nlNode, &w.deNode} {
		if err := db.Create(n).Error; err != nil {
			t.Fatal(err)
		}
	}
	inbounds := []model.Inbound{
		{Type: "vless", Tag: "local", Options: json.RawMessage(`{"listen_port":8443}`)},
		{Type: "vless", Tag: "nl-a", NodeId: &w.nlNode.Id, Options: json.RawMessage(`{"listen_port":443}`)},
		{Type: "trojan", Tag: "nl-b", NodeId: &w.nlNode.Id, Options: json.RawMessage(`{"listen_port":444}`)},
		{Type: "vless", Tag: "de-a", NodeId: &w.deNode.Id, Options: json.RawMessage(`{"listen_port":443}`)},
	}
	if err := db.Create(&inbounds).Error; err != nil {
		t.Fatal(err)
	}
	addClient(t, "alice", "local", "nl-a", "de-a")
	addClient(t, "bob", "local")

	now := time.Now().Unix()
	w.since = now - 24*3600
	w.slot = (now/w.bucket)*w.bucket - w.bucket
	return w
}

// What the master itself counted: samples inside the window, and one far
// outside it.
func (w *summaryWorld) addMasterStats(t *testing.T) {
	t.Helper()
	at := w.slot + 10
	addStat(t, "user", "alice", at, 7, 9)
	addStat(t, "user", "bob", at, 70, 90)
	addStat(t, "inbound", "local", at, 40, 60)
	addStat(t, "outbound", "direct", at, 1, 2)
	addStat(t, "user", "alice", time.Now().Unix()-3*86400-100, 1_000_000, 1_000_000)
}

func totalOf(sum StatsSummary, resource, tag string) (up, down int64, found bool) {
	for _, t := range sum.Totals {
		if t.Resource == resource && t.Tag == tag {
			return t.Up, t.Down, true
		}
	}
	return 0, 0, false
}

func tagsOf(sum StatsSummary, resource string) string {
	out := ""
	for _, t := range sum.Totals {
		if t.Resource == resource {
			out += t.Tag + " "
		}
	}
	return out
}

func pointOf(sum StatsSummary, tag string, at int64) (int64, bool) {
	for _, p := range sum.Series {
		if p.Tag == tag && p.At == at {
			return p.Traffic, true
		}
	}
	return 0, false
}

func wantTotal(t *testing.T, sum StatsSummary, resource, tag string, up, down int64) {
	t.Helper()
	gotUp, gotDown, found := totalOf(sum, resource, tag)
	if !found || gotUp != up || gotDown != down {
		t.Errorf("%s %s = %d/%d (found %v), want %d/%d", resource, tag, gotUp, gotDown, found, up, down)
	}
}

func TestSummaryAddsUpTheWindow(t *testing.T) {
	w := setUpSummaryWorld(t, StatsSummary{}, StatsSummary{})
	w.addMasterStats(t)
	// Other resources and an unknown one are not part of it.
	addStat(t, "service", "whatever", w.slot+10, 5, 5)
	// A second bucket, and a second tag in the first.
	addStat(t, "inbound", "local", w.slot-3*w.bucket+10, 1, 1)
	addStat(t, "inbound", "second", w.slot+20, 2, 3)

	svc := &StatsService{}
	sum, err := svc.GetSummary(w.since, w.bucket)
	if err != nil {
		t.Fatal(err)
	}
	wantTotal(t, *sum, "user", "alice", 7, 9)
	wantTotal(t, *sum, "user", "bob", 70, 90)
	wantTotal(t, *sum, "inbound", "local", 41, 61)
	wantTotal(t, *sum, "inbound", "second", 2, 3)
	wantTotal(t, *sum, "outbound", "direct", 1, 2)
	if _, _, found := totalOf(*sum, "service", "whatever"); found {
		t.Error("a resource that is not shown was summed")
	}
	if got := tagsOf(*sum, "user"); got != "bob alice " {
		t.Errorf("users = %q, want the busiest first", got)
	}
	if got := tagsOf(*sum, "inbound"); got != "local second " {
		t.Errorf("inbounds = %q", got)
	}
	// The totals come grouped by resource in the order the screen lists them.
	order := ""
	for _, tot := range sum.Totals {
		if len(order) == 0 || order[len(order)-1:] != tot.Resource[:1] {
			order += tot.Resource[:1]
		}
	}
	if order != "uio" {
		t.Errorf("resource order = %q, want users, inbounds, outbounds", order)
	}

	// The inbounds' traffic, per tag and bucket.
	if v, ok := pointOf(*sum, "local", w.slot); !ok || v != 100 {
		t.Errorf("local at %d = %d (%v), want 100", w.slot, v, ok)
	}
	if v, ok := pointOf(*sum, "second", w.slot); !ok || v != 5 {
		t.Errorf("second at %d = %d (%v), want 5", w.slot, v, ok)
	}
	if v, ok := pointOf(*sum, "local", w.slot-3*w.bucket); !ok || v != 2 {
		t.Errorf("local three hours earlier = %d (%v), want 2", v, ok)
	}
	for i := 1; i < len(sum.Series); i++ {
		if sum.Series[i-1].At > sum.Series[i].At {
			t.Fatalf("series is not in time order: %+v", sum.Series)
		}
	}
	if sum.Since != w.since || sum.Bucket != w.bucket {
		t.Errorf("window = %d/%d", sum.Since, sum.Bucket)
	}

	// A narrower window leaves out what is older.
	narrow, err := svc.GetSummary(w.slot-1, w.bucket)
	if err != nil {
		t.Fatal(err)
	}
	wantTotal(t, *narrow, "inbound", "local", 40, 60)

	// Nonsense is repaired instead of trusted: the default window is a day,
	// and a bucket cannot be zero.
	def, err := svc.GetSummary(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if def.Bucket != minSummaryBucket || def.Since < time.Now().Unix()-24*3600-5 || def.Since > time.Now().Unix()-24*3600+5 {
		t.Errorf("defaults = since %d bucket %d", def.Since, def.Bucket)
	}
	wantTotal(t, *def, "user", "alice", 7, 9)

	// Nothing counted is an empty answer that still encodes as lists, not null.
	empty, err := svc.GetSummary(time.Now().Unix()+10, 3600)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(empty)
	var generic map[string]json.RawMessage
	_ = json.Unmarshal(raw, &generic)
	if string(generic["totals"]) != "[]" || string(generic["series"]) != "[]" {
		t.Errorf("empty summary = %s", raw)
	}
}

func TestSummaryKeepsTheBusiestTags(t *testing.T) {
	totals := []StatsTotal{
		{"user", "a", 1, 1}, {"user", "b", 5, 5}, {"user", "c", 3, 3}, {"user", "d", 5, 5},
		{"inbound", "x", 1, 0}, {"inbound", "y", 9, 0}, {"inbound", "z", 4, 0},
		{"outbound", "o", 1, 1},
	}
	got := busiest(totals, 2)
	want := "user b user d inbound y inbound z outbound o "
	text := ""
	for _, tot := range got {
		text += tot.Resource + " " + tot.Tag + " "
	}
	if text != want {
		t.Fatalf("busiest = %q, want %q", text, want)
	}
}

// A client served by a node moves its traffic through that node: the master's
// own numbers are short of it until the node's are added.
func TestClusterSummaryAddsWhatTheNodesCounted(t *testing.T) {
	// The nodes also report things that are not the master's to show.
	w := setUpSummaryWorld(t, StatsSummary{}, StatsSummary{})
	w.addMasterStats(t)
	w.nl.answer = StatsSummary{
		Totals: []StatsTotal{
			{"user", "alice", 1000, 2000}, {"user", "bob", 10, 20},
			{"user", "stranger", 5, 5}, // a node-local user the master does not have
			{"inbound", "nl-a", 500, 700},
			{"inbound", "nl-private", 9, 9}, // a node-local inbound
			{"inbound", "de-a", 4, 4},       // a replica of another node
			{"inbound", "local", 3, 3},      // the master's own inbound is not the node's to claim
			{"outbound", "nl-out", 8, 8},
		},
		Series: []StatsPoint{{"nl-a", w.slot, 300}, {"nl-private", w.slot, 999}, {"local", w.slot, 111}},
	}
	w.de.answer = StatsSummary{
		Totals: []StatsTotal{{"user", "alice", 100, 200}, {"inbound", "de-a", 50, 60}},
		Series: []StatsPoint{{"de-a", w.slot, 40}, {"de-a", w.slot - w.bucket, 7}},
	}

	got := (&StatsService{}).GetClusterSummary(w.since, w.bucket)
	if len(got.Missing) != 0 {
		t.Fatalf("missing = %+v", got.Missing)
	}
	wantTotal(t, got.StatsSummary, "user", "alice", 7+1000+100, 9+2000+200)
	wantTotal(t, got.StatsSummary, "user", "bob", 70+10, 90+20)
	wantTotal(t, got.StatsSummary, "inbound", "local", 40, 60)
	wantTotal(t, got.StatsSummary, "inbound", "nl-a", 500, 700)
	wantTotal(t, got.StatsSummary, "inbound", "de-a", 50, 60)
	wantTotal(t, got.StatsSummary, "outbound", "direct", 1, 2)
	for _, private := range []struct{ resource, tag string }{{"user", "stranger"}, {"inbound", "nl-private"}, {"outbound", "nl-out"}} {
		if _, _, found := totalOf(got.StatsSummary, private.resource, private.tag); found {
			t.Errorf("%s %s of a node is in the master's summary", private.resource, private.tag)
		}
	}
	if got := tagsOf(got.StatsSummary, "user"); got != "alice bob " {
		t.Errorf("users = %q", got)
	}
	if got := tagsOf(got.StatsSummary, "inbound"); got != "nl-a de-a local " {
		t.Errorf("inbounds = %q, want the busiest first", got)
	}
	if v, ok := pointOf(got.StatsSummary, "nl-a", w.slot); !ok || v != 300 {
		t.Errorf("nl-a series = %d (%v)", v, ok)
	}
	if v, ok := pointOf(got.StatsSummary, "de-a", w.slot-w.bucket); !ok || v != 7 {
		t.Errorf("de-a earlier series = %d (%v)", v, ok)
	}
	if v, ok := pointOf(got.StatsSummary, "local", w.slot); !ok || v != 100 {
		t.Errorf("a node changed the master's own series: local = %d (%v)", v, ok)
	}
	if _, ok := pointOf(got.StatsSummary, "nl-private", w.slot); ok {
		t.Error("a node-local inbound is in the series")
	}
	for i := 1; i < len(got.Series); i++ {
		prev, cur := got.Series[i-1], got.Series[i]
		if prev.At > cur.At || prev.At == cur.At && prev.Tag > cur.Tag {
			t.Fatalf("series is not in order: %+v", got.Series)
		}
	}

	// Every node is asked once, for the window the screen shows.
	for name, f := range map[string]*fakeSummaryNode{"nl": w.nl, "de": w.de} {
		calls := f.calls()
		if len(calls) != 1 || calls[0].Get("since") != strconv.FormatInt(w.since, 10) || calls[0].Get("bucket") != "3600" {
			t.Errorf("%s was asked %v", name, calls)
		}
	}
}

func TestClusterSummaryWithoutNodesIsTheMastersOwn(t *testing.T) {
	w := setUpSummaryWorld(t, StatsSummary{}, StatsSummary{})
	w.addMasterStats(t)
	if err := database.GetDB().Model(&model.Node{}).Where("1 = 1").Update("enable", false).Error; err != nil {
		t.Fatal(err)
	}
	got := (&StatsService{}).GetClusterSummary(w.since, w.bucket)
	wantTotal(t, got.StatsSummary, "user", "alice", 7, 9)
	wantTotal(t, got.StatsSummary, "inbound", "local", 40, 60)
	if len(got.Missing) != 0 || len(w.nl.calls())+len(w.de.calls()) != 0 {
		t.Fatalf("disabled nodes were asked or reported: %+v, %d calls", got.Missing, len(w.nl.calls())+len(w.de.calls()))
	}

	// And with no node at all.
	if err := database.GetDB().Where("1 = 1").Delete(&model.Node{}).Error; err != nil {
		t.Fatal(err)
	}
	got = (&StatsService{}).GetClusterSummary(w.since, w.bucket)
	wantTotal(t, got.StatsSummary, "user", "bob", 70, 90)
	if len(got.Missing) != 0 {
		t.Fatalf("missing = %+v", got.Missing)
	}
}

func missingText(notes []NodeNote) string {
	out := ""
	for _, n := range notes {
		out += fmt.Sprintf("%s:%s ", n.Name, n.Reason)
	}
	return out
}

// A node that cannot answer does not take the others down with it, and the
// caller is told which numbers are short, and why.
func TestClusterSummaryReportsNodesItCannotAsk(t *testing.T) {
	w := setUpSummaryWorld(t, StatsSummary{}, StatsSummary{})
	w.addMasterStats(t)
	w.de.answer = StatsSummary{Totals: []StatsTotal{{"user", "alice", 100, 200}}}
	svc := &StatsService{}

	// A node from before statsTotals.
	w.nl.setMode("unknown")
	got := svc.GetClusterSummary(w.since, w.bucket)
	if m := missingText(got.Missing); m != "nl:update " {
		t.Fatalf("missing = %q", m)
	}
	wantTotal(t, got.StatsSummary, "user", "alice", 7+100, 9+200)

	// Failing in every other way is a node that cannot be reached.
	for _, mode := range []string{"status", "garbage"} {
		w.nl.setMode(mode)
		got = svc.GetClusterSummary(w.since, w.bucket)
		if m := missingText(got.Missing); m != "nl:unreachable " {
			t.Fatalf("%s: missing = %q", mode, m)
		}
		wantTotal(t, got.StatsSummary, "user", "alice", 7+100, 9+200)
		wantTotal(t, got.StatsSummary, "user", "bob", 70, 90)
	}

	// Both at once, in the order of the nodes.
	w.nl.setMode("unknown")
	w.de.setMode("status")
	got = svc.GetClusterSummary(w.since, w.bucket)
	if m := missingText(got.Missing); m != "nl:update de:unreachable " {
		t.Fatalf("missing = %q", m)
	}
	wantTotal(t, got.StatsSummary, "user", "alice", 7, 9)

	// Back to normal.
	w.nl.setMode("ok")
	w.de.setMode("ok")
	got = svc.GetClusterSummary(w.since, w.bucket)
	if len(got.Missing) != 0 {
		t.Fatalf("missing = %+v", got.Missing)
	}
	wantTotal(t, got.StatsSummary, "user", "alice", 7+100, 9+200)
}

func TestClusterSummaryDoesNotWaitForeverForANode(t *testing.T) {
	old := nodeSummaryTimeout
	nodeSummaryTimeout = 300 * time.Millisecond
	t.Cleanup(func() { nodeSummaryTimeout = old })
	w := setUpSummaryWorld(t, StatsSummary{}, StatsSummary{})
	w.addMasterStats(t)
	w.de.answer = StatsSummary{Totals: []StatsTotal{{"user", "alice", 100, 200}}}
	w.nl.setMode("hang")

	started := time.Now()
	got := (&StatsService{}).GetClusterSummary(w.since, w.bucket)
	if took := time.Since(started); took > 3*time.Second {
		t.Fatalf("took %v with a node that never answers", took)
	}
	if m := missingText(got.Missing); m != "nl:unreachable " {
		t.Fatalf("missing = %q", m)
	}
	wantTotal(t, got.StatsSummary, "user", "alice", 7+100, 9+200)
}

// The probe already knows a node is down; asking it would only make the screen
// wait for the timeout.
func TestClusterSummarySkipsNodesTheProbeFoundDown(t *testing.T) {
	w := setUpSummaryWorld(t, StatsSummary{}, StatsSummary{})
	w.addMasterStats(t)
	w.nl.answer = StatsSummary{Totals: []StatsTotal{{"user", "alice", 1000, 2000}}}
	svc := &StatsService{}
	setState := func(id uint, state string, checkedAt int64) {
		nodeStatusMu.Lock()
		nodeStatuses[id] = NodeStatus{State: state, CheckedAt: checkedAt}
		nodeStatusMu.Unlock()
	}

	setState(w.nlNode.Id, "offline", time.Now().Unix()-5)
	got := svc.GetClusterSummary(w.since, w.bucket)
	if m := missingText(got.Missing); m != "nl:unreachable " || len(w.nl.calls()) != 0 {
		t.Fatalf("missing = %q, nl calls = %d", m, len(w.nl.calls()))
	}
	wantTotal(t, got.StatsSummary, "user", "alice", 7, 9)

	// A stopped core does not stop the node's panel from answering.
	setState(w.nlNode.Id, "core-stopped", time.Now().Unix()-5)
	got = svc.GetClusterSummary(w.since, w.bucket)
	if len(got.Missing) != 0 || len(w.nl.calls()) != 1 {
		t.Fatalf("core-stopped: missing %+v, nl calls %d", got.Missing, len(w.nl.calls()))
	}
	wantTotal(t, got.StatsSummary, "user", "alice", 7+1000, 9+2000)

	// A verdict that is old is checked again.
	setState(w.nlNode.Id, "offline", time.Now().Unix()-3600)
	got = svc.GetClusterSummary(w.since, w.bucket)
	if len(got.Missing) != 0 || len(w.nl.calls()) != 2 {
		t.Fatalf("stale offline: missing %+v, nl calls %d", got.Missing, len(w.nl.calls()))
	}
}
