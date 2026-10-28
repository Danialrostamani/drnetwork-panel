package service

import (
	"encoding/json"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
)

func resetSmart(t *testing.T) {
	t.Helper()
	resetNodeState(t)
	smartSamples.Lock()
	smartSamples.values = map[uint]smartSample{}
	smartSamples.Unlock()
	t.Cleanup(func() { smartSamples.Lock(); smartSamples.values = map[uint]smartSample{}; smartSamples.Unlock() })
}
func readySmart(id uint, weight float64) smartNode {
	return smartNode{ID: id, Entry: SubNodeRouteEntry{Id: id, Weight: 100}, Base: weight, Free: weight, Keep: true, Admit: true, Reason: "ready"}
}
func smartPlan() nodeRoutePlan {
	return nodeRoutePlan{enable: true, smart: true, names: []string{"a", "b", "c"}, candidates: map[string]smartNode{
		"a": readySmart(1, 4), "b": readySmart(2, 2), "c": readySmart(3, 1),
	}}
}
func TestSmartRendezvousSpreadAndMinimalMovement(t *testing.T) {
	plan := smartPlan()
	counts := map[string]int{}
	changed := 0
	rank := func(client uint, exclude string) string {
		name := ""
		score := math.Inf(1)
		for k, n := range plan.candidates {
			if k == exclude {
				continue
			}
			r := smartRank(client, n.ID, n.Free, "admit")
			if r < score {
				name, score = k, r
			}
		}
		return name
	}
	for id := uint(1); id <= 10000; id++ {
		first := rank(id, "")
		counts[first]++
		if first != "a" && rank(id, "a") != first {
			changed++
		}
	}
	if counts["a"] < 5400 || counts["a"] > 6000 || counts["b"] < 2600 || counts["b"] > 3100 || counts["c"] < 1200 || counts["c"] > 1600 || changed != 0 {
		t.Fatalf("spread %v unaffected users moved %d", counts, changed)
	}
}
func TestSmartAssignmentStableAndDistributedFailure(t *testing.T) {
	resetSmart(t)
	plan := smartPlan()
	all := map[string]bool{"a": true, "b": true, "c": true}
	now := time.Now().Unix()
	victims := []uint{}
	for id := uint(1); id <= 300; id++ {
		order := smartOrder(id, plan, all, now)
		if order[0] == "a" {
			victims = append(victims, id)
		}
		other := plan
		other.candidates = map[string]smartNode{"a": readySmart(1, 1), "b": readySmart(2, 100), "c": readySmart(3, 100)}
		if got := smartOrder(id, other, all, now+10); got[0] != order[0] {
			t.Fatalf("fluctuation moved %d", id)
		}
	}
	// Simulate process state disappearing. Persistent assignments survive.
	counts := map[string]int{}
	failed := plan.candidates["a"]
	failed.Keep = false
	failed.Admit = false
	plan.candidates["a"] = failed
	for _, id := range victims {
		order := smartOrder(id, plan, all, now+30)
		counts[order[0]]++
	}
	if counts["a"] != 0 || counts["b"] == 0 || counts["c"] == 0 {
		t.Fatalf("failed-node users not spread: %v", counts)
	}
	// A recovered node doesn't summon all old users back.
	plan.candidates["a"] = readySmart(1, 10000)
	for _, id := range victims {
		if got := smartOrder(id, plan, all, now+40); got[0] == "a" {
			t.Fatal("mass failback")
		}
	}
}
func TestSmartDrainAndEligibility(t *testing.T) {
	resetSmart(t)
	plan := smartPlan()
	now := time.Now().Unix()
	all := map[string]bool{"a": true, "b": true, "c": true}
	order := smartOrder(1, plan, all, now)
	saved := order[0]
	n := plan.candidates[saved]
	n.Admit = false
	n.Entry.Drain = true
	plan.candidates[saved] = n
	if got := smartOrder(1, plan, all, now+1); got[0] != saved {
		t.Fatal("drain evicted current user")
	}
	for id := uint(2); id < 30; id++ {
		if got := smartOrder(id, plan, all, now); got[0] == saved {
			t.Fatal("new admission to drained node")
		}
	}
	delete(all, saved)
	if got := smartOrder(1, plan, all, now+2); got[0] == saved {
		t.Fatal("kept disallowed node")
	}
	for name, n := range plan.candidates {
		n.Keep = false
		n.Admit = false
		plan.candidates[name] = n
	}
	if got := smartOrder(100, plan, all, now); len(got) != 0 {
		t.Fatal("unhealthy fallback admitted")
	}
}
func TestSmartZeroShareOnlyEmergency(t *testing.T) {
	resetSmart(t)
	p := smartPlan()
	c := p.candidates["c"]
	c.Entry.Weight = 0
	c.Free = 1e9
	p.candidates["c"] = c
	all := map[string]bool{"a": true, "b": true, "c": true}
	now := time.Now().Unix()
	for id := uint(1); id < 40; id++ {
		if got := smartOrder(id, p, all, now); got[0] == "c" {
			t.Fatal("backup took normal user")
		}
	}
	if got := smartOrder(99, p, map[string]bool{"c": true}, now); got[0] != "c" {
		t.Fatal("emergency not used")
	}
}
func TestSmartHardwareLoadStalenessAndWarmup(t *testing.T) {
	resetSmart(t)
	now := time.Now().Unix()
	node := model.Node{Id: 1, Enable: true, Name: "a"}
	st := NodeStatus{State: "online", CheckedAt: now - 20, CpuCount: 8, Cpu: 20, Mem: NodeMem{Total: 8 << 30, Current: 2 << 30}}
	n := smartNode{ID: 1, Entry: SubNodeRouteEntry{Id: 1, Weight: 100}}
	observeSmartNodes(map[uint]NodeStatus{1: st})
	if got := measureSmartNode(n, node, st, now-20); got.Admit {
		t.Fatal("single probe admitted")
	}
	st.CheckedAt = now
	observeSmartNodes(map[uint]NodeStatus{1: st})
	got := measureSmartNode(n, node, st, now)
	if !got.Admit || got.Base != 400 || got.Free <= 0 {
		t.Fatalf("hardware baseline %+v", got)
	}
	n.Entry.Capacity = 12
	n.Entry.BandwidthMbps = 100
	netSt := st
	netSt.CheckedAt = now + 10
	netSt.NetDown = 20_000_000
	// sustained bandwidth saturation, time-based smoothing
	for i := 1; i <= 15; i++ {
		netSt.CheckedAt = now + int64(i*10)
		observeSmartNodes(map[uint]NodeStatus{1: netSt})
	}
	got = measureSmartNode(n, node, netSt, netSt.CheckedAt)
	if got.Base != 1200 || got.Admit {
		t.Fatalf("bandwidth cap not respected %+v", got)
	}
	netSt.CheckedAt = now - 121
	if got := measureSmartNode(n, node, netSt, now); got.Keep || got.Admit || got.Reason != "stale" {
		t.Fatalf("stale eligible %+v", got)
	}
	st.Traffic = &NodeTrafficSummary{CapLimit: 100, CapUsed: 100}
	if got := measureSmartNode(n, node, st, now); got.Keep {
		t.Fatal("exhausted traffic admitted")
	}
}
func TestSmartFailureRequiresConfirmation(t *testing.T) {
	resetSmart(t)
	now := time.Now().Unix()
	node := model.Node{Id: 1, Enable: true}
	n := smartNode{ID: 1, Entry: SubNodeRouteEntry{Id: 1, Weight: 100}}
	st := NodeStatus{State: "offline", CheckedAt: now}
	observeSmartNodes(map[uint]NodeStatus{1: st})
	if got := measureSmartNode(n, node, st, now); !got.Keep || got.Admit {
		t.Fatal("bad single-failure grace")
	}
	st.CheckedAt = now + 5
	observeSmartNodes(map[uint]NodeStatus{1: st})
	if got := measureSmartNode(n, node, st, now+5); got.Keep {
		t.Fatal("confirmed down retained")
	}
}
func TestSmartAssignmentConcurrent(t *testing.T) {
	resetSmart(t)
	p := smartPlan()
	all := map[string]bool{"a": true, "b": true, "c": true}
	now := time.Now().Unix()
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); smartOrder(5, p, all, now) }()
	}
	wg.Wait()
	var count int64
	database.GetDB().Model(&model.NodeAssignment{}).Where("client_id = ?", 5).Count(&count)
	if count != 1 {
		t.Fatalf("duplicate assignment rows %d", count)
	}
}
func TestSmartRouteValidation(t *testing.T) {
	for _, e := range []SubNodeRouteEntry{{Id: 1, Capacity: -1}, {Id: 1, BandwidthMbps: -1}, {Id: 1, MaxUsers: 1000001}} {
		b, _ := json.Marshal(SubNodeRoute{Enable: true, Smart: true, Entries: []SubNodeRouteEntry{e}})
		if validateSubNodeRoute(string(b)) == nil {
			t.Fatal("invalid capacity accepted")
		}
	}
	if validateSubNodeRoute(`{"enable":true,"smart":true,"entries":[{"id":1,"weight":100,"capacity":2.5,"bandwidthMbps":500,"maxUsers":200,"drain":true}]}`) != nil {
		t.Fatal("valid smart settings rejected")
	}
}

func TestSmartFilterMatchesSavedPrimaryAndPreservesOtherLinks(t *testing.T) {
	resetSmart(t)
	db := database.GetDB()
	now := time.Now().Unix()
	a := model.Node{Name: "a", Enable: true}
	b := model.Node{Name: "b", Enable: true}
	if err := db.Create(&a).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&b).Error; err != nil {
		t.Fatal(err)
	}
	cfg := SubNodeRoute{Enable: true, Smart: true, Entries: []SubNodeRouteEntry{{Id: a.Id, Weight: 100}, {Id: b.Id, Weight: 100}}}
	raw, _ := json.Marshal(cfg)
	db.Create(&model.Setting{Key: "subNodeRoute", Value: string(raw)})
	fresh := map[uint]NodeStatus{}
	for _, id := range []uint{a.Id, b.Id} {
		fresh[id] = NodeStatus{State: "online", CpuCount: 4, Mem: NodeMem{Total: 8 << 30, Current: 1 << 30}, CheckedAt: now - 20}
	}
	observeSmartNodes(fresh)
	for id, st := range fresh {
		st.CheckedAt = now
		fresh[id] = st
	}
	observeSmartNodes(fresh)
	nodeStatusMu.Lock()
	nodeStatuses = fresh
	nodeStatusMu.Unlock()
	invalidateNodeLinkRules()
	links := json.RawMessage(`[{"type":"external","remark":"[a] one","uri":"a"},{"type":"local","uri":"local"},{"type":"external","remark":"[b] two","uri":"b"},{"type":"external","remark":"manual","uri":"manual"}]`)
	for id := uint(1); id < 15; id++ {
		var out []map[string]interface{}
		if err := json.Unmarshal(FilterNodeLinks(&model.Client{Id: id, Links: links}), &out); err != nil {
			t.Fatal(err)
		}
		primary := SmartPrimaryNode(id)
		if primary == "" || out[0]["uri"] != primary || out[1]["uri"] != "local" || out[3]["uri"] != "manual" {
			t.Fatalf("primary %s links %+v", primary, out)
		}
	}
	before := (&NodeService{}).GetBalanceStatuses()
	if before[a.Id].Balance == nil || before[b.Id].Balance == nil {
		t.Fatal("missing operator eligibility status")
	}
	// Restrict a node after assignment: no returned primary may bypass access.
	a.Access = model.NodeAccess{Groups: []string{"VIP"}}
	if err := db.Save(&a).Error; err != nil {
		t.Fatal(err)
	}
	invalidateNodeLinkRules()
	_ = FilterNodeLinks(&model.Client{Id: 1, Links: links, Group: "ordinary"})
	if got := SmartPrimaryNode(1); got != "b" {
		t.Fatalf("restricted primary %s", got)
	}
}

func TestSmartSustainedOverloadDoesNotEvictEveryone(t *testing.T) {
	resetSmart(t)
	p := smartPlan()
	now := int64(1800000000)
	all := map[string]bool{"a": true, "b": true, "c": true}
	for id := uint(1); id <= 100; id++ {
		database.GetDB().Create(&model.NodeAssignment{ClientId: id, NodeId: 1, UpdatedAt: now - 1000})
	}
	n := p.candidates["a"]
	n.Admit = false
	n.BusySince = now - 200
	p.candidates["a"] = n
	moved := 0
	for id := uint(1); id <= 100; id++ {
		if smartOrder(id, p, all, now)[0] != "a" {
			moved++
		}
	}
	if moved == 0 || moved > 25 {
		t.Fatalf("overload movement %d", moved)
	}
	n.Entry.Drain = true
	p.candidates["a"] = n
	database.GetDB().Create(&model.NodeAssignment{ClientId: 200, NodeId: 1, UpdatedAt: now - 1000})
	if smartOrder(200, p, all, now)[0] != "a" {
		t.Fatal("draining user evicted by load")
	}

}
