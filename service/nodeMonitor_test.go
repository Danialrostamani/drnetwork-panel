package service

import (
	"encoding/json"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
)

// resetNodeState gives a test a fresh database and none of the node state the
// package keeps in memory, in UTC.
func resetNodeState(t *testing.T) {
	t.Helper()
	// A probe an earlier action left running must not see the swap.
	WaitNodeActionProbes()
	if err := database.InitDB(filepath.Join(t.TempDir(), "nodes.db")); err != nil {
		t.Fatal(err)
	}
	clear := func() {
		nodeMonitorMu.Lock()
		nodeMonitors = map[uint]*nodeMonitorState{}
		nodeLastPurge = 0
		nodeMonitorOnce = sync.Once{}
		nodeMonitorMu.Unlock()
		nodeStatusMu.Lock()
		nodeStatuses = map[uint]NodeStatus{}
		nodeStatusMu.Unlock()
		DrainNodeEvents()
		invalidateNodeLinkRules()
		nodeLocMu.Lock()
		nodeLoc, nodeLocAt = time.UTC, time.Now().Add(time.Hour)
		nodeLocMu.Unlock()
	}
	clear()
	t.Cleanup(func() {
		WaitNodeActionProbes()
		clear()
		nodeLocMu.Lock()
		nodeLoc, nodeLocAt = nil, time.Time{}
		nodeLocMu.Unlock()
	})
}

func intp(v int) *int    { return &v }
func boolp(v bool) *bool { return &v }

func TestCapCycleEdges(t *testing.T) {
	d := func(y int, m time.Month, day, h int) time.Time { return time.Date(y, m, day, h, 0, 0, 0, time.UTC) }
	cases := []struct {
		now        time.Time
		day        int
		start, end time.Time
	}{
		{d(2025, 2, 15, 12), 31, d(2025, 1, 31, 0), d(2025, 2, 28, 0)},
		{d(2025, 2, 28, 0), 31, d(2025, 2, 28, 0), d(2025, 3, 31, 0)},
		{d(2025, 3, 30, 23), 31, d(2025, 2, 28, 0), d(2025, 3, 31, 0)},
		{d(2025, 3, 31, 0), 31, d(2025, 3, 31, 0), d(2025, 4, 30, 0)},
		{d(2025, 1, 10, 5), 15, d(2024, 12, 15, 0), d(2025, 1, 15, 0)},
		{d(2025, 12, 20, 5), 15, d(2025, 12, 15, 0), d(2026, 1, 15, 0)},
		{d(2025, 3, 1, 0), 1, d(2025, 3, 1, 0), d(2025, 4, 1, 0)},
		{d(2024, 2, 29, 10), 30, d(2024, 2, 29, 0), d(2024, 3, 30, 0)},
		{d(2024, 2, 29, 10), 1, d(2024, 2, 1, 0), d(2024, 3, 1, 0)},
		{d(2025, 6, 5, 1), 0, d(2025, 6, 1, 0), d(2025, 7, 1, 0)},
	}
	for _, c := range cases {
		start, end := capCycle(c.now, c.day)
		if !start.Equal(c.start) || !end.Equal(c.end) {
			t.Errorf("capCycle(%s, %d) = %s – %s, want %s – %s", c.now, c.day, start, end, c.start, c.end)
		}
		if c.now.Before(start) || !c.now.Before(end) {
			t.Errorf("capCycle(%s, %d): now is outside its own cycle", c.now, c.day)
		}
	}
	// Local time: the cycle starts at the local midnight.
	tehran := time.FixedZone("IRST", 3*3600+1800)
	start, _ := capCycle(time.Date(2025, 5, 10, 1, 0, 0, 0, tehran), 10)
	if start.Hour() != 0 || start.Day() != 10 || start.Location() != tehran {
		t.Fatalf("local cycle start = %s", start)
	}
}

func TestCapLevelOf(t *testing.T) {
	for _, c := range []struct {
		used, limit int64
		want        int
	}{{0, 100, 0}, {79, 100, 0}, {80, 100, 80}, {89, 100, 80}, {90, 100, 90}, {99, 100, 90}, {100, 100, 100}, {250, 100, 100}, {50, 0, 0}, {50, -1, 0}} {
		if got := capLevelOf(c.used, c.limit); got != c.want {
			t.Errorf("capLevelOf(%d, %d) = %d, want %d", c.used, c.limit, got, c.want)
		}
	}
	if got := (model.NodeCap{Mode: "up"}).Counted(3, 5); got != 3 {
		t.Errorf("up counts %d", got)
	}
	if got := (model.NodeCap{Mode: "down"}).Counted(3, 5); got != 5 {
		t.Errorf("down counts %d", got)
	}
	if got := (model.NodeCap{}).Counted(3, 5); got != 8 {
		t.Errorf("total counts %d", got)
	}
	for in, want := range map[int]int{-3: 1, 0: 1, 1: 1, 17: 17, 31: 31, 40: 31} {
		if got := (model.NodeCap{Day: in}).ResetDay(); got != want {
			t.Errorf("ResetDay(%d) = %d", in, got)
		}
	}
}

func TestNetDelta(t *testing.T) {
	ifs := func(kv ...interface{}) map[string][2]uint64 {
		m := map[string][2]uint64{}
		for i := 0; i < len(kv); i += 3 {
			m[kv[i].(string)] = [2]uint64{uint64(kv[i+1].(int)), uint64(kv[i+2].(int))}
		}
		return m
	}
	cases := []struct {
		name     string
		base     nodeNetBase
		have     bool
		cur      nodeNetBase
		up, down int64
		ok       bool
	}{
		{"no base", nodeNetBase{}, false, nodeNetBase{Sent: 10, Src: "nic", At: 10}, 0, 0, false},
		{"other source", nodeNetBase{Sent: 1, Src: "net"}, true, nodeNetBase{Sent: 10, Src: "nic", At: 10}, 0, 0, false},
		{"sums", nodeNetBase{Sent: 100, Recv: 200, Src: "net"}, true, nodeNetBase{Sent: 150, Recv: 260, Src: "net", At: 10}, 50, 60, true},
		{"sum went back", nodeNetBase{Sent: 1000, Recv: 1000, Src: "net"}, true, nodeNetBase{Sent: 100, Recv: 1500, Src: "net", At: 10}, 100, 500, true},
		{"per interface", nodeNetBase{Ifs: ifs("eth0", 100, 100, "eth1", 50, 50), Src: "nic", Boot: 1000}, true,
			nodeNetBase{Ifs: ifs("eth0", 150, 130, "eth1", 60, 55), Src: "nic", Boot: 1000, At: 10}, 60, 35, true},
		{"interface gone", nodeNetBase{Ifs: ifs("eth0", 100, 100, "eth1", 5000, 5000), Src: "nic", Boot: 1000}, true,
			nodeNetBase{Ifs: ifs("eth0", 150, 130), Src: "nic", Boot: 1000, At: 10}, 50, 30, true},
		{"new interface", nodeNetBase{Ifs: ifs("eth0", 100, 100), Src: "nic", Boot: 1000}, true,
			nodeNetBase{Ifs: ifs("eth0", 110, 120, "eth9", 900000, 900000), Src: "nic", Boot: 1000, At: 10}, 10, 20, true},
		{"reboot", nodeNetBase{Ifs: ifs("eth0", 100000, 100000), Src: "nic", Boot: 1000}, true,
			nodeNetBase{Ifs: ifs("eth0", 10, 20, "eth1", 5, 5), Src: "nic", Boot: 9000, At: 10}, 15, 25, true},
		{"boot time jitter", nodeNetBase{Ifs: ifs("eth0", 1000, 1000), Src: "nic", Boot: 1000}, true,
			nodeNetBase{Ifs: ifs("eth0", 1100, 1200), Src: "nic", Boot: 1001, At: 10}, 100, 200, true},
		{"jitter is not a reboot for a counter that went on", nodeNetBase{Ifs: ifs("eth0", 1000, 1000), Src: "nic", Boot: 1060}, true,
			nodeNetBase{Ifs: ifs("eth0", 1500, 1000), Src: "nic", Boot: 1000, At: 10}, 500, 0, true},
		{"glitch", nodeNetBase{Sent: 0, Src: "net"}, true, nodeNetBase{Sent: nodeMaxRate*5 + 1, Src: "net", At: 1}, 0, 0, false},
		{"fast but real", nodeNetBase{Sent: 0, Src: "net", At: 0}, true, nodeNetBase{Sent: nodeMaxRate * 10, Src: "net", At: 10}, nodeMaxRate * 10, 0, true},
	}
	for _, c := range cases {
		up, down, ok := netDelta(c.base, c.have, c.cur)
		if up != c.up || down != c.down || ok != c.ok {
			t.Errorf("%s: netDelta = %d, %d, %v; want %d, %d, %v", c.name, up, down, ok, c.up, c.down, c.ok)
		}
	}
}

func TestVersionOlder(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"1.6.3-drnetwork.21", "1.6.3-drnetwork.22", true},
		{"1.6.3-drnetwork.22", "1.6.3-drnetwork.22", false},
		{"1.6.3-drnetwork.23", "1.6.3-drnetwork.22", false},
		{"1.6.3", "1.6.3-drnetwork.22", true},
		{"1.6.3-drnetwork.22", "1.6.3", false},
		{"1.7.0", "1.6.3-drnetwork.22", false},
		{"1.6.9", "1.6.10", true},
		{"1.6.10", "1.6.9", false},
		{"v1.6.3", "1.6.3", false},
	} {
		if got := versionOlder(c.a, c.b); got != c.want {
			t.Errorf("versionOlder(%q, %q) = %v", c.a, c.b, got)
		}
	}
}

func TestNodeWarnings(t *testing.T) {
	now := time.Now().Unix()
	busy := &NodeStatus{State: "online", Cpu: 95, Mem: NodeMem{Current: 95, Total: 100}, Disk: NodeMem{Current: 91, Total: 100},
		Latency: 5000, CertExpiry: now + 3*86400, AppFull: "1.6.3-drnetwork.20"}
	keys := func(ws []NodeWarning) map[string]NodeWarning {
		out := map[string]NodeWarning{}
		for _, w := range ws {
			out[w.Key] = w
		}
		return out
	}
	got := keys(nodeWarnings(&model.Node{}, busy, "1.6.3-drnetwork.22", now))
	for _, k := range []string{"cpu", "mem", "disk", "cert", "version"} {
		if _, ok := got[k]; !ok {
			t.Errorf("default alerts miss %s: %+v", k, got)
		}
	}
	if _, ok := got["ping"]; ok {
		t.Error("the ping alert is off by default")
	}
	if got["cpu"].Limit != 90 || got["version"].Info != "1.6.3-drnetwork.20" || got["cert"].Value < 2.9 || got["cert"].Value > 3.1 {
		t.Errorf("warning details: %+v", got)
	}
	off := model.Node{Alerts: model.NodeAlerts{Cpu: intp(0), Mem: intp(0), Disk: intp(0), Ping: intp(0), CertDays: intp(0), Version: boolp(false)}}
	if ws := nodeWarnings(&off, busy, "1.6.3-drnetwork.22", now); len(ws) != 0 {
		t.Errorf("alerts turned off still warn: %+v", ws)
	}
	custom := model.Node{Alerts: model.NodeAlerts{Cpu: intp(96), Ping: intp(4000)}}
	got = keys(nodeWarnings(&custom, busy, "1.6.3-drnetwork.22", now))
	if _, ok := got["cpu"]; ok {
		t.Error("cpu under a raised limit warned")
	}
	if got["ping"].Limit != 4000 {
		t.Errorf("ping limit: %+v", got)
	}
	for _, state := range []string{"offline", ""} {
		down := *busy
		down.State = state
		if ws := nodeWarnings(&model.Node{}, &down, "1.6.3-drnetwork.22", now); ws != nil {
			t.Errorf("%q node warned: %+v", state, ws)
		}
	}
	stopped := *busy
	stopped.State = "core-stopped"
	if ws := nodeWarnings(&model.Node{}, &stopped, "1.6.3-drnetwork.22", now); len(ws) == 0 {
		t.Error("a reachable node with its core stopped is still measured")
	}
	noDisk := NodeStatus{State: "online", Disk: NodeMem{Current: 5}, AppVersion: "1.6.3"}
	if ws := keys(nodeWarnings(&model.Node{}, &noDisk, "1.6.3-drnetwork.22", now)); len(ws) != 1 || ws["version"].Info != "1.6.3" {
		t.Errorf("node without disk size: %+v", ws)
	}
}

// probeAt is a status as a probe at time at would report it.
func probeAt(at int64, state string, sent, recv uint64) NodeStatus {
	return NodeStatus{State: state, CheckedAt: at, Cpu: 10, Mem: NodeMem{Current: 50, Total: 100}, Disk: NodeMem{Current: 20, Total: 100}, Latency: 40, Online: 3,
		Uptime24: -1, Uptime7d: -1, netSrc: "nic", netSent: sent, netRecv: recv, netIfs: map[string][2]uint64{"eth0": {sent, recv}}, BootTime: 1000}
}

// capDay is a reset day whose cycle did not start in the last hour, so the
// traffic of the last minutes falls in the current cycle.
func capDay() int {
	if u := time.Now().UTC(); u.Day() == 1 && u.Hour() == 0 {
		return 15
	}
	return 1
}

func eventKinds(events []NodeEvent) map[string]NodeEvent {
	out := map[string]NodeEvent{}
	for _, e := range events {
		out[e.Kind+"/"+e.Reason] = e
	}
	return out
}

func TestMonitorRecordsHistoryOutagesTrafficAndCap(t *testing.T) {
	resetNodeState(t)
	db := database.GetDB()
	down := model.Node{Name: "down-node", Enable: true, BaseUrl: "http://a", Token: "t", HideDown: true}
	capped := model.Node{Name: "capped", Enable: true, BaseUrl: "http://b", Token: "t", Cap: model.NodeCap{Limit: 1000, Day: capDay(), Hide: true}}
	for _, n := range []*model.Node{&down, &capped} {
		if err := db.Create(n).Error; err != nil {
			t.Fatal(err)
		}
	}
	nodes := []*model.Node{&down, &capped}
	svc := NodeService{}
	now := time.Now().Unix()
	t0 := now - now%60 - 600 // ten minutes ago, at a minute's start
	round := func(d, c NodeStatus) {
		svc.applyProbes(nodes, map[uint]NodeStatus{down.Id: d, capped.Id: c}, true)
	}

	// Minute 0: two online probes each; the capped node moves 900 bytes.
	round(probeAt(t0+1, "online", 1000, 1000), probeAt(t0+1, "online", 1000, 1000))
	round(probeAt(t0+11, "online", 1100, 1100), probeAt(t0+11, "online", 1300, 1600))
	st := svc.GetStatuses()[capped.Id]
	if st.NetUp != 30 || st.NetDown != 60 {
		t.Fatalf("speed = %d/%d, want 30/60", st.NetUp, st.NetDown)
	}
	if len(DrainNodeEvents()) != 0 {
		t.Fatal("events before anything happened")
	}
	// Minute 1: the down node starts failing. Its first failure is minutes
	// old already, past the grace: its links leave the subscriptions.
	failed := NodeStatus{State: "offline", Error: "timeout", CheckedAt: t0 + 61, Uptime24: -1, Uptime7d: -1}
	round(failed, probeAt(t0+61, "online", 1300, 1600))
	var metrics []model.NodeMetric
	db.Where("node_id = ?", down.Id).Find(&metrics)
	if len(metrics) != 1 || metrics[0].DateTime != t0 || metrics[0].Probes != 2 || metrics[0].Up != 2 || metrics[0].Latency != 40 || metrics[0].Sent != 100 || metrics[0].Online != 3 {
		t.Fatalf("first minute: %+v", metrics)
	}
	var traffic []model.NodeTraffic
	db.Where("node_id = ?", capped.Id).Find(&traffic)
	if len(traffic) != 1 || traffic[0].Up != 300 || traffic[0].Down != 600 || traffic[0].DateTime != localHour(t0+11, time.UTC) {
		t.Fatalf("traffic rows: %+v", traffic)
	}
	st = svc.GetStatuses()[capped.Id]
	if st.Traffic == nil || st.Traffic.TotalUp != 300 || st.Traffic.TotalDown != 600 || st.Traffic.CapUsed != 900 || st.Traffic.CapLimit != 1000 {
		t.Fatalf("traffic summary: %+v", st.Traffic)
	}
	if st.Uptime24 != 100 || st.Uptime7d != 100 {
		t.Fatalf("uptime = %v / %v", st.Uptime24, st.Uptime7d)
	}
	events := eventKinds(DrainNodeEvents())
	if len(events) != 2 || events["cap/"].Level != 90 || events["cap/"].NodeId != capped.Id || events["hidden/down"].NodeId != down.Id {
		t.Fatalf("events: %+v", events)
	}
	if st = svc.GetStatuses()[down.Id]; st.DownSince != t0+61 || st.Hidden != "down" {
		t.Fatalf("down node: since %d hidden %q", st.DownSince, st.Hidden)
	}
	var stored model.Node
	db.First(&stored, capped.Id)
	if len(stored.CapState) == 0 || len(stored.NetBase) == 0 {
		t.Fatalf("cap level / counters not kept: %s / %s", stored.CapState, stored.NetBase)
	}
	var count int64
	db.Model(model.NodeOutage{}).Count(&count)
	if count != 0 {
		t.Fatal("one failed probe is not an outage yet")
	}

	// A second failure opens the outage, back-dated to the first one.
	failed.CheckedAt = t0 + 71
	round(failed, probeAt(t0+71, "online", 1400, 1600))
	var outages []model.NodeOutage
	db.Find(&outages)
	if len(outages) != 1 || outages[0].Start != t0+61 || outages[0].End != 0 || outages[0].State != "offline" || outages[0].Reason != "timeout" {
		t.Fatalf("outage: %+v", outages)
	}
	st = svc.GetStatuses()[down.Id]
	if st.DownSince != t0+61 || st.Hidden != "down" || st.LastOnline != t0+11 {
		t.Fatalf("down node: %+v", st)
	}
	if ev := DrainNodeEvents(); len(ev) != 0 {
		t.Fatalf("events repeated: %+v", ev)
	}

	// Minute 2: the capped node goes over its cap, the down node is back.
	round(probeAt(t0+121, "online", 1200, 1200), probeAt(t0+121, "online", 1500, 1600))
	db.Find(&outages)
	if len(outages) != 1 || outages[0].End != t0+121 {
		t.Fatalf("outage did not end: %+v", outages)
	}
	if st = svc.GetStatuses()[down.Id]; st.Hidden != "" || st.DownSince != 0 {
		t.Fatalf("recovered node: %+v", st)
	}
	// The capped node leaves the subscriptions in the same round.
	events = eventKinds(DrainNodeEvents())
	if _, ok := events["shown/down"]; !ok || events["cap/"].Level != 100 || events["cap/"].Used != 1100 || events["hidden/cap"].NodeId != capped.Id || len(events) != 3 {
		t.Fatalf("events after the minute: %+v", events)
	}
	if st = svc.GetStatuses()[capped.Id]; st.Hidden != "cap" || st.Traffic.CapUsed != 1100 {
		t.Fatalf("capped node: hidden %q traffic %+v", st.Hidden, st.Traffic)
	}
	round(probeAt(t0+131, "online", 1200, 1200), probeAt(t0+131, "online", 1500, 1600))
	if st = svc.GetStatuses()[capped.Id]; st.Hidden != "cap" {
		t.Fatalf("capped node hidden = %q", st.Hidden)
	}
	if ev := DrainNodeEvents(); len(ev) != 0 {
		t.Fatalf("events repeated: %+v", ev)
	}

	// A result older than the one recorded is dropped.
	svc.applyProbes(nodes, map[uint]NodeStatus{down.Id: {State: "offline", CheckedAt: t0 + 100}}, false)
	if st = svc.GetStatuses()[down.Id]; st.State != "online" || st.CheckedAt != t0+131 {
		t.Fatalf("a stale result replaced the status: %+v", st)
	}
	if _, ok := svc.GetStatuses()[capped.Id]; !ok {
		t.Fatal("a probe of one node dropped the others")
	}

	// Maintenance: the probes count neither for nor against the uptime, and
	// open no outage.
	planned := NodeStatus{State: "core-stopped", Maintenance: true, Uptime24: -1, Uptime7d: -1}
	for _, at := range []int64{181, 191, 241} {
		planned.CheckedAt = t0 + at
		round(planned, probeAt(t0+at, "online", 1500, 1600))
	}
	db.Model(model.NodeOutage{}).Count(&count)
	if count != 1 {
		t.Fatalf("maintenance opened an outage: %d", count)
	}
	var m3 model.NodeMetric
	if err := db.Where("node_id = ? AND date_time = ?", down.Id, t0+180).First(&m3).Error; err == nil {
		t.Fatalf("a minute of planned maintenance was recorded: %+v", m3)
	}
	// Uptime: 4 online probes of the 6 that count.
	if st = svc.GetStatuses()[down.Id]; st.Uptime24 < 66.6 || st.Uptime24 > 66.7 {
		t.Fatalf("uptime of the down node = %v", st.Uptime24)
	}

	// Disabled: the next periodic round ends its outage and forgets it.
	for _, at := range []int64{301, 311} {
		failed.CheckedAt = t0 + at
		round(failed, probeAt(t0+at, "online", 1500, 1600))
	}
	db.Model(model.NodeOutage{}).Where("end_at = 0 AND start_at = ?", t0+301).Count(&count)
	if count != 1 {
		t.Fatalf("second outage not open: %d", count)
	}
	svc.applyProbes([]*model.Node{&capped}, map[uint]NodeStatus{capped.Id: probeAt(t0+321, "online", 1500, 1600)}, true)
	db.Model(model.NodeOutage{}).Where("end_at = 0").Count(&count)
	if count != 0 {
		t.Fatal("the outage of a node no longer probed stays open")
	}
	if _, ok := svc.GetStatuses()[down.Id]; ok {
		t.Fatal("a node no longer probed keeps its status")
	}
}

// The links of a node that just went down stay until the grace is over.
func TestHideDownWaitsForTheGrace(t *testing.T) {
	resetNodeState(t)
	n := model.Node{Name: "n", Enable: true, BaseUrl: "http://a", Token: "t", HideDown: true}
	database.GetDB().Create(&n)
	svc := NodeService{}
	now := time.Now().Unix()
	svc.applyProbes([]*model.Node{&n}, map[uint]NodeStatus{n.Id: probeAt(now-nodeHideGrace-60, "online", 0, 0)}, true)
	svc.applyProbes([]*model.Node{&n}, map[uint]NodeStatus{n.Id: {State: "offline", CheckedAt: now - nodeHideGrace + 30}}, true)
	if st := svc.GetStatuses()[n.Id]; st.Hidden != "" {
		t.Fatalf("hidden %q before the grace was over", st.Hidden)
	}
	svc.applyProbes([]*model.Node{&n}, map[uint]NodeStatus{n.Id: {State: "offline", CheckedAt: now}}, true)
	if st := svc.GetStatuses()[n.Id]; st.Hidden != "" {
		t.Fatalf("hidden %q, the node is down for less than the grace", st.Hidden)
	}
	// Turned off: never hidden.
	n.HideDown = false
	svc.applyProbes([]*model.Node{&n}, map[uint]NodeStatus{n.Id: {State: "offline", CheckedAt: now + 1}}, true)
	nodeStatusMu.Lock()
	st := nodeStatuses[n.Id]
	st.DownSince = now - nodeHideGrace - 10
	nodeStatuses[n.Id] = st
	nodeStatusMu.Unlock()
	svc.applyProbes([]*model.Node{&n}, map[uint]NodeStatus{n.Id: {State: "offline", CheckedAt: now + 2}}, true)
	if st := svc.GetStatuses()[n.Id]; st.Hidden != "" {
		t.Fatalf("hidden %q with hiding off", st.Hidden)
	}
	n.HideDown = true
	svc.applyProbes([]*model.Node{&n}, map[uint]NodeStatus{n.Id: {State: "offline", CheckedAt: now + 3}}, true)
	if st := svc.GetStatuses()[n.Id]; st.Hidden != "down" {
		t.Fatalf("not hidden after the grace: %q", st.Hidden)
	}
	if ev := DrainNodeEvents(); len(ev) != 1 || ev[0].Kind != "hidden" {
		t.Fatalf("events: %+v", ev)
	}
}

func TestMonitorRestoresCountersAndCapLevelAfterRestart(t *testing.T) {
	resetNodeState(t)
	db := database.GetDB()
	n := model.Node{Name: "n", Enable: true, BaseUrl: "http://a", Token: "t", Cap: model.NodeCap{Limit: 1000, Day: capDay()}}
	db.Create(&n)
	svc := NodeService{}
	now := time.Now().Unix()
	t0 := now - now%60 - 300
	svc.applyProbes([]*model.Node{&n}, map[uint]NodeStatus{n.Id: probeAt(t0+1, "online", 0, 0)}, true)
	svc.applyProbes([]*model.Node{&n}, map[uint]NodeStatus{n.Id: probeAt(t0+11, "online", 850, 0)}, true)
	svc.applyProbes([]*model.Node{&n}, map[uint]NodeStatus{n.Id: probeAt(t0+61, "online", 850, 0)}, true)
	if ev := DrainNodeEvents(); len(ev) != 1 || ev[0].Level != 80 {
		t.Fatalf("events: %+v", ev)
	}
	// The master restarts: the monitor state is gone, the database has it.
	nodeMonitorMu.Lock()
	nodeMonitors = map[uint]*nodeMonitorState{}
	nodeMonitorMu.Unlock()
	nodeStatusMu.Lock()
	nodeStatuses = map[uint]NodeStatus{}
	nodeStatusMu.Unlock()
	db.First(&n, n.Id)
	// The node moved 50 bytes while the master was away.
	svc.applyProbes([]*model.Node{&n}, map[uint]NodeStatus{n.Id: probeAt(t0+121, "online", 900, 0)}, true)
	svc.applyProbes([]*model.Node{&n}, map[uint]NodeStatus{n.Id: probeAt(t0+181, "online", 900, 0)}, true)
	ev := DrainNodeEvents()
	if len(ev) != 1 || ev[0].Level != 90 || ev[0].Used != 900 {
		t.Fatalf("after the restart: %+v (the 80%% level must not be announced again)", ev)
	}
	var total struct{ Up int64 }
	db.Model(model.NodeTraffic{}).Select("SUM(up) AS up").Where("node_id = ?", n.Id).Scan(&total)
	if total.Up != 900 {
		t.Fatalf("traffic over the restart = %d, want 900", total.Up)
	}
}

func TestPurgeNodeHistoryArchivesOldTraffic(t *testing.T) {
	resetNodeState(t)
	db := database.GetDB()
	n := model.Node{Name: "n", Enable: true, BaseUrl: "http://a", Token: "t"}
	db.Create(&n)
	now := time.Now().Unix()
	old := now - nodeTrafficRetention - 7200
	rows := []interface{}{
		&model.NodeMetric{NodeId: n.Id, DateTime: now - nodeMetricRetention - 60, Probes: 1},
		&model.NodeMetric{NodeId: n.Id, DateTime: now - 60, Probes: 1},
		&model.NodeOutage{NodeId: n.Id, Start: now - nodeOutageRetention - 500, End: now - nodeOutageRetention - 100},
		&model.NodeOutage{NodeId: n.Id, Start: now - 500, End: 0},
		&model.NodeTraffic{NodeId: n.Id, DateTime: 0, Up: 5, Down: 5},
		&model.NodeTraffic{NodeId: n.Id, DateTime: old, Up: 10, Down: 20},
		&model.NodeTraffic{NodeId: n.Id, DateTime: old + 3600, Up: 1, Down: 2},
		&model.NodeTraffic{NodeId: n.Id, DateTime: now - 3600, Up: 100, Down: 200},
		&model.NodeMetric{NodeId: 999, DateTime: now - 60, Probes: 1},
		&model.NodeTraffic{NodeId: 999, DateTime: now - 3600, Up: 1},
		&model.NodeOutage{NodeId: 999, Start: now - 10},
	}
	for _, r := range rows {
		if err := db.Create(r).Error; err != nil {
			t.Fatal(err)
		}
	}
	purgeNodeHistory(now)
	var metrics, outages int64
	db.Model(model.NodeMetric{}).Count(&metrics)
	db.Model(model.NodeOutage{}).Count(&outages)
	if metrics != 1 || outages != 1 {
		t.Fatalf("metrics %d outages %d, want 1 and 1", metrics, outages)
	}
	var traffic []model.NodeTraffic
	db.Order("date_time").Find(&traffic)
	if len(traffic) != 2 || traffic[0].DateTime != 0 || traffic[0].Up != 16 || traffic[0].Down != 27 || traffic[1].Up != 100 {
		t.Fatalf("traffic after purge: %+v", traffic)
	}
	// The total stays whole.
	sum, err := nodeTrafficSummaryOf(&n, now)
	if err != nil || sum.TotalUp != 116 || sum.TotalDown != 227 {
		t.Fatalf("total = %+v, %v", sum, err)
	}
	// Run twice: nothing is archived again.
	purgeNodeHistory(now)
	db.Order("date_time").Find(&traffic)
	if traffic[0].Up != 16 {
		t.Fatalf("second purge archived again: %+v", traffic)
	}
}

func TestFilterNodeLinksHidesDownAndRestrictedNodes(t *testing.T) {
	resetNodeState(t)
	db := database.GetDB()
	vipOnly := model.Node{Name: "vip node", Enable: true, BaseUrl: "http://a", Token: "t", Access: model.NodeAccess{Groups: []string{"VIP"}, Clients: []uint{7}}}
	gone := model.Node{Name: "gone", Enable: true, BaseUrl: "http://b", Token: "t", HideDown: true}
	open := model.Node{Name: "open", Enable: true, BaseUrl: "http://c", Token: "t"}
	for _, n := range []*model.Node{&vipOnly, &gone, &open} {
		db.Create(n)
	}
	nodeStatusMu.Lock()
	nodeStatuses[gone.Id] = NodeStatus{State: "offline", Hidden: "down"}
	nodeStatuses[open.Id] = NodeStatus{State: "online"}
	nodeStatusMu.Unlock()
	invalidateNodeLinkRules()
	links := json.RawMessage(`[{"type":"local","remark":"[vip node] mine","uri":"vless://1"},{"type":"external","remark":"[vip node] a","uri":"vless://2"},` +
		`{"type":"external","remark":"[gone] b","uri":"vless://3"},{"type":"external","remark":"[open] c","uri":"vless://4"},{"type":"external","remark":"other","uri":"vless://5"}]`)
	uris := func(raw json.RawMessage) []string {
		var ls []map[string]string
		_ = json.Unmarshal(raw, &ls)
		var out []string
		for _, l := range ls {
			out = append(out, l["uri"][len("vless://"):])
		}
		return out
	}
	check := func(c model.Client, want string) {
		t.Helper()
		got := ""
		for _, u := range uris(FilterNodeLinks(&c)) {
			got += u
		}
		if got != want {
			t.Errorf("client %d (%q): links %s, want %s", c.Id, c.Group, got, want)
		}
	}
	check(model.Client{Id: 1, Group: "basic", Links: links}, "145")
	check(model.Client{Id: 2, Group: " vip ", Links: links}, "1245")
	check(model.Client{Id: 7, Links: links}, "1245")
	// Back online: its links are served again once the rules refresh.
	nodeStatusMu.Lock()
	nodeStatuses[gone.Id] = NodeStatus{State: "online"}
	nodeStatusMu.Unlock()
	invalidateNodeLinkRules()
	check(model.Client{Id: 1, Group: "basic", Links: links}, "1345")
	// Nothing hidden or restricted: the links are handed back untouched.
	if err := saveNodes("edit", map[string]interface{}{"id": vipOnly.Id, "access": map[string]interface{}{"groups": []string{}, "clients": []uint{}}}); err != nil {
		t.Fatal(err)
	}
	c := model.Client{Id: 1, Links: links}
	if string(FilterNodeLinks(&c)) != string(links) {
		t.Fatal("unrestricted links were rewritten")
	}
	if FilterNodeLinks(nil) != nil || FilterNodeLinks(&model.Client{}) != nil {
		t.Fatal("no client, no links")
	}
}
