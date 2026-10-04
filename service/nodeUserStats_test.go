package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
)

// fakeStatsNode is a node whose api/stats answers every question with the same
// buckets, laid over whatever window it was asked for.
type fakeStatsNode struct {
	srv     *httptest.Server
	mu      sync.Mutex
	queries []map[string]string
	status  int
	buckets map[string][]int64
}

const fakeNodeToken = "node-token"

func newFakeStatsNode(t *testing.T, buckets map[string][]int64) *fakeStatsNode {
	t.Helper()
	f := &fakeStatsNode{status: http.StatusOK, buckets: buckets}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Token") != fakeNodeToken || r.URL.Path != "/app/apiv2/stats" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		q := map[string]string{}
		for k := range r.URL.Query() {
			q[k] = r.URL.Query().Get(k)
		}
		f.mu.Lock()
		f.queries = append(f.queries, q)
		status := f.status
		f.mu.Unlock()
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		start, _ := strconv.ParseInt(q["start"], 10, 64)
		end, _ := strconv.ParseInt(q["end"], 10, 64)
		const numBuckets = 6
		body, _ := json.Marshal(map[string]interface{}{"success": true, "obj": map[string]interface{}{
			"stats": f.buckets, "startTime": start, "bucketSpan": (end - start) / numBuckets, "numBuckets": numBuckets,
		}})
		_, _ = w.Write(body)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeStatsNode) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.queries)
}

func (f *fakeStatsNode) last() map[string]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.queries) == 0 {
		return nil
	}
	return f.queries[len(f.queries)-1]
}

func (f *fakeStatsNode) fail(status int) {
	f.mu.Lock()
	f.status = status
	f.mu.Unlock()
}

type userChart struct {
	Stats      map[string][]int64 `json:"stats"`
	StartTime  int64              `json:"startTime"`
	BucketSpan int64              `json:"bucketSpan"`
	NumBuckets int                `json:"numBuckets"`
}

func (c userChart) totals() (up, down int64) {
	for _, pair := range c.Stats {
		up += pair[0]
		down += pair[1]
	}
	return up, down
}

func chartOf(t *testing.T, svc *StatsService, user string, limit int, start, end int64) userChart {
	t.Helper()
	got, err := svc.GetStats("user", user, limit, start, end)
	if err != nil {
		t.Fatalf("GetStats(user %s): %v", user, err)
	}
	raw, _ := json.Marshal(got)
	var chart userChart
	if err := json.Unmarshal(raw, &chart); err != nil {
		t.Fatal(err)
	}
	return chart
}

type userChartWorld struct {
	nl, de *fakeStatsNode
	nlNode model.Node
	deNode model.Node
}

// setUpUserChartWorld builds a master with two nodes. Inbounds: "local" runs on
// the master, "nl-a" and "nl-b" are replicas of two inbounds on node nl, "de-a"
// is a replica on node de.
func setUpUserChartWorld(t *testing.T, nlBuckets, deBuckets map[string][]int64) *userChartWorld {
	t.Helper()
	if err := database.InitDB(filepath.Join(t.TempDir(), "userchart.db")); err != nil {
		t.Fatal(err)
	}
	w := &userChartWorld{nl: newFakeStatsNode(t, nlBuckets), de: newFakeStatsNode(t, deBuckets)}
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
	return w
}

func addClient(t *testing.T, name string, inboundTags ...string) {
	t.Helper()
	db := database.GetDB()
	var ids []uint
	for _, tag := range inboundTags {
		var in model.Inbound
		if err := db.Where("tag = ?", tag).First(&in).Error; err != nil {
			t.Fatal(err)
		}
		ids = append(ids, in.Id)
	}
	raw, _ := json.Marshal(ids)
	if err := db.Create(&model.Client{Name: name, Enable: true, Inbounds: raw, Config: json.RawMessage(`{}`)}).Error; err != nil {
		t.Fatal(err)
	}
}

func addUserTraffic(t *testing.T, user string, at int64, up, down int64) {
	t.Helper()
	rows := []model.Stats{
		{DateTime: at, Resource: "user", Tag: user, Direction: true, Traffic: up},
		{DateTime: at, Resource: "user", Tag: user, Direction: false, Traffic: down},
	}
	if err := database.GetDB().Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
}

// What a client moves through an inbound hosted on a node is counted on that
// node, so its usage chart on the master has to be assembled from there too.
func TestUserChartAddsTheTrafficCountedOnTheNodes(t *testing.T) {
	// Sixths of the window: the first bucket, and the last one.
	w := setUpUserChartWorld(t,
		map[string][]int64{"0": {1000, 2000}, "5": {10, 20}},
		map[string][]int64{"5": {100000, 200000}})
	addClient(t, "alice", "local", "nl-a", "nl-b", "de-a")
	addClient(t, "bob", "local")
	addClient(t, "carol", "nl-a")
	now := time.Now().Unix()
	addUserTraffic(t, "alice", now-30, 7, 9)
	addUserTraffic(t, "bob", now-30, 70, 90)

	svc := &StatsService{}
	chart := chartOf(t, svc, "alice", 1, 0, 0)
	up, down := chart.totals()
	if up != 7+1000+10+100000 || down != 9+2000+20+200000 {
		t.Fatalf("alice totals = %d/%d, chart %+v", up, down, chart)
	}
	// The same node holds two of her inbounds yet is asked once, for her name.
	if w.nl.calls() != 1 || w.de.calls() != 1 {
		t.Fatalf("node calls: nl=%d de=%d, want 1 each", w.nl.calls(), w.de.calls())
	}
	q := w.nl.last()
	if q["resource"] != "user" || q["tag"] != "alice" || q["limit"] != "1" || q["start"] == "" || q["end"] == "" {
		t.Fatalf("query sent to the node = %v", q)
	}
	start, _ := strconv.ParseInt(q["start"], 10, 64)
	end, _ := strconv.ParseInt(q["end"], 10, 64)
	if start != chart.StartTime || end-start != 3600 {
		t.Fatalf("window sent %d..%d, chart starts at %d", start, end, chart.StartTime)
	}

	// Samples keep their place in time: the first sixth of the hour stays near
	// the left edge and the last sixth near the right, in the master's buckets.
	var early, late int64 = -1, -1
	for key, pair := range chart.Stats {
		idx, _ := strconv.ParseInt(key, 10, 64)
		if pair[0] == 1000 {
			early = idx
		}
		if pair[0] == 100000+10 || pair[0] == 10 {
			late = idx
		}
	}
	n := int64(chart.NumBuckets)
	if early < 0 || early > n/6 {
		t.Fatalf("first-sixth sample landed in bucket %d of %d", early, n)
	}
	if late < n*5/6 {
		t.Fatalf("last-sixth sample landed in bucket %d of %d", late, n)
	}

	// Clients that never touch a node are answered from the master alone.
	nl, de := w.nl.calls(), w.de.calls()
	local := chartOf(t, svc, "bob", 1, 0, 0)
	if up, down := local.totals(); up != 70 || down != 90 {
		t.Fatalf("bob totals = %d/%d", up, down)
	}
	if w.nl.calls() != nl || w.de.calls() != de {
		t.Fatal("a client on master inbounds only called a node")
	}

	// Someone who is served by one node only asks that node.
	carol := chartOf(t, svc, "carol", 1, 0, 0)
	if up, down := carol.totals(); up != 1000+10 || down != 2000+20 {
		t.Fatalf("carol totals = %d/%d", up, down)
	}
	if w.nl.calls() != nl+1 || w.de.calls() != de {
		t.Fatalf("carol should ask nl only: nl=%d de=%d", w.nl.calls(), w.de.calls())
	}
}

func TestUserChartForwardsACustomRangeToTheNodes(t *testing.T) {
	w := setUpUserChartWorld(t, map[string][]int64{"2": {5, 6}}, nil)
	addClient(t, "alice", "nl-a")
	now := time.Now().Unix()
	from, to := now-86400, now-43200

	chart := chartOf(t, &StatsService{}, "alice", 0, from, to)
	q := w.nl.last()
	if q["start"] != strconv.FormatInt(from, 10) || q["end"] != strconv.FormatInt(to, 10) {
		t.Fatalf("range sent to the node = %v, want %d..%d", q, from, to)
	}
	if chart.StartTime != from {
		t.Fatalf("chart starts at %d, want %d", chart.StartTime, from)
	}
	if up, down := chart.totals(); up != 5 || down != 6 {
		t.Fatalf("totals = %d/%d", up, down)
	}
}

// A node that is down must not take the master's own numbers with it.
func TestUserChartSurvivesANodeThatCannotAnswer(t *testing.T) {
	w := setUpUserChartWorld(t, map[string][]int64{"5": {10, 20}}, map[string][]int64{"5": {100, 200}})
	addClient(t, "alice", "local", "nl-a", "de-a")
	addUserTraffic(t, "alice", time.Now().Unix()-30, 7, 9)
	svc := &StatsService{}

	w.nl.fail(http.StatusBadGateway)
	chart := chartOf(t, svc, "alice", 1, 0, 0)
	if up, down := chart.totals(); up != 7+100 || down != 9+200 {
		t.Fatalf("with nl down: %d/%d", up, down)
	}

	// Disabled nodes are left out too.
	if err := database.GetDB().Model(&model.Node{}).Where("id = ?", w.deNode.Id).Update("enable", false).Error; err != nil {
		t.Fatal(err)
	}
	de := w.de.calls()
	chart = chartOf(t, svc, "alice", 1, 0, 0)
	if up, down := chart.totals(); up != 7 || down != 9 {
		t.Fatalf("with both nodes unavailable: %d/%d", up, down)
	}
	if w.de.calls() != de {
		t.Fatal("a disabled node was called")
	}
}

// With nothing recorded on the master, an unreachable node is the reason the
// chart has no data and the operator is told so instead of getting a blank.
func TestUserChartSaysWhyWhenNoNodeAnswers(t *testing.T) {
	w := setUpUserChartWorld(t, nil, map[string][]int64{"5": {100, 200}})
	addClient(t, "alice", "nl-a")
	addClient(t, "dave", "nl-a", "de-a")
	svc := &StatsService{}

	w.nl.fail(http.StatusBadGateway)
	if _, err := svc.GetStats("user", "alice", 1, 0, 0); err == nil {
		t.Fatal("expected an error when the only node is down")
	}
	// One node answering is enough to draw a chart.
	chart := chartOf(t, svc, "dave", 1, 0, 0)
	if up, down := chart.totals(); up != 100 || down != 200 {
		t.Fatalf("dave totals = %d/%d", up, down)
	}

	// A node that is reachable but has nothing to show is an empty chart, not a failure.
	w.nl.fail(http.StatusOK)
	empty := chartOf(t, svc, "alice", 1, 0, 0)
	if up, down := empty.totals(); up != 0 || down != 0 {
		t.Fatalf("empty chart totals = %d/%d", up, down)
	}

	// A disabled node is a reason as well.
	if err := database.GetDB().Model(&model.Node{}).Where("id = ?", w.nlNode.Id).Update("enable", false).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetStats("user", "alice", 1, 0, 0); err == nil {
		t.Fatal("expected an error when the only node is disabled")
	}
}

// Unknown names, clients without inbounds and the other resources never reach
// a node.
func TestOnlyUserChartsOfNodeClientsAskTheNodes(t *testing.T) {
	w := setUpUserChartWorld(t, map[string][]int64{"5": {1, 2}}, map[string][]int64{"5": {3, 4}})
	addClient(t, "alice", "nl-a")
	addClient(t, "idle")
	svc := &StatsService{}

	if up, down := chartOf(t, svc, "nobody", 1, 0, 0).totals(); up != 0 || down != 0 {
		t.Fatalf("unknown user totals = %d/%d", up, down)
	}
	if up, down := chartOf(t, svc, "idle", 1, 0, 0).totals(); up != 0 || down != 0 {
		t.Fatalf("idle user totals = %d/%d", up, down)
	}
	if _, err := svc.GetStats("user", "", 1, 0, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetStats("outbound", "alice", 1, 0, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetStats("endpoint", "alice", 1, 0, 0); err != nil {
		t.Fatal(err)
	}
	if w.nl.calls() != 0 || w.de.calls() != 0 {
		t.Fatalf("nodes were called: nl=%d de=%d", w.nl.calls(), w.de.calls())
	}
}

// Whatever a node answers, the chart stays inside its window.
func TestNodeChartRowsStayInsideTheWindow(t *testing.T) {
	raw := json.RawMessage(`{"stats":{"0":[1,2],"1":[3,4],"x":[9,9],"-1":[9,9],"2":[5],"3":[0,0],"99":[7,8]},"startTime":1000,"bucketSpan":10,"numBuckets":6}`)
	rows, err := nodeChartRows(raw, "alice", 1000, 1060)
	if err != nil {
		t.Fatal(err)
	}
	var up, down int64
	for _, r := range rows {
		if r.DateTime <= 1000 || r.DateTime > 1060 || r.Resource != "user" || r.Tag != "alice" {
			t.Fatalf("row outside the window: %+v", r)
		}
		if r.Direction {
			up += r.Traffic
		} else {
			down += r.Traffic
		}
	}
	// Garbage keys, short pairs and zero traffic are dropped; a bucket past the
	// end is pulled back to the last one.
	if up != 1+3+7 || down != 2+4+8 {
		t.Fatalf("up=%d down=%d from %+v", up, down, rows)
	}
	if _, err := nodeChartRows(json.RawMessage(`[1,2]`), "alice", 1000, 1060); err == nil {
		t.Fatal("expected an error for an answer that is not a chart")
	}
}
