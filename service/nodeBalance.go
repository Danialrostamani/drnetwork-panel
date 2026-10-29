package service

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/logger"
	"gorm.io/gorm/clause"
)

// Smart mode is opt-in. Existing percentage mode and non-node links keep their
// behaviour. Mbps is operator-supplied measured capacity, never NIC link speed.
const smartFreshSeconds int64 = 120

type smartSample struct {
	At, HealthySince, BusySince, DownSince int64
	HealthyChecks, DownChecks              int
	CPU, Memory, Rate                      float64
}

var smartSamples = struct {
	sync.Mutex
	values map[uint]smartSample
}{values: map[uint]smartSample{}}
var smartAssignmentMu sync.Mutex

type smartNode struct {
	ID          uint
	Entry       SubNodeRouteEntry
	Base, Free  float64
	Keep, Admit bool
	BusySince   int64
	Reason      string
}

func bounded(v, low, high float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return low
	}
	return math.Max(low, math.Min(high, v))
}

func observeSmartNodes(fresh map[uint]NodeStatus) {
	var cfg SubNodeRoute
	raw, _ := (&SettingService{}).getString("subNodeRoute")
	_ = json.Unmarshal([]byte(raw), &cfg)
	policies := map[uint]SubNodeRouteEntry{}
	for _, e := range cfg.Entries {
		policies[e.Id] = e
	}
	smartSamples.Lock()
	defer smartSamples.Unlock()
	for id, st := range fresh {
		s := smartSamples.values[id]
		if st.CheckedAt <= s.At {
			continue
		}
		gap := s.At == 0 || st.CheckedAt-s.At > smartFreshSeconds
		if st.State != "online" || st.Maintenance {
			if s.DownSince == 0 || gap {
				s.DownSince = st.CheckedAt
				s.DownChecks = 0
			}
			s.DownChecks++
			s.HealthySince, s.HealthyChecks = 0, 0
			// A down node is not "busy"; a stale timestamp would let sustained-load
			// migration fire instantly once the node recovers.
			s.BusySince = 0
		} else {
			if gap {
				s.BusySince = 0
			}
			if s.HealthySince == 0 || gap {
				s.HealthySince = st.CheckedAt
				s.HealthyChecks = 0
			}
			s.HealthyChecks++
			s.DownSince, s.DownChecks = 0, 0
			mem := 0.0
			if st.Mem.Total > 0 {
				mem = float64(st.Mem.Current) / float64(st.Mem.Total) * 100
			}
			rate := math.Max(float64(st.NetUp), float64(st.NetDown)) * 8 / 1e6
			if gap || s.At == 0 {
				s.CPU, s.Memory, s.Rate = st.Cpu, mem, rate
			} else {
				// Time-based EWMA, about a one-minute response, independent of polling frequency.
				a := 1 - math.Exp(-float64(st.CheckedAt-s.At)/60)
				s.CPU += a * (st.Cpu - s.CPU)
				s.Memory += a * (mem - s.Memory)
				s.Rate += a * (rate - s.Rate)
			}
			e := policies[id]
			busy := s.CPU >= 90 || s.Memory >= 90 || (e.BandwidthMbps > 0 && s.Rate >= float64(e.BandwidthMbps)*.9) || (e.MaxUsers > 0 && st.Online >= e.MaxUsers)
			if busy {
				if s.BusySince == 0 {
					s.BusySince = st.CheckedAt
				}
			} else {
				s.BusySince = 0
			}
		}
		s.At = st.CheckedAt
		smartSamples.values[id] = s
	}
}

func measureSmartNode(n smartNode, node model.Node, st NodeStatus, now int64) smartNode {
	n.Keep, n.Admit = false, false
	n.Reason = "unavailable"
	if !node.Enable {
		n.Reason = "disabled"
		return n
	}
	if st.CheckedAt <= 0 || now-st.CheckedAt > smartFreshSeconds || st.CheckedAt > now+30 {
		n.Reason = "stale"
		return n
	}
	if st.Maintenance || st.Hidden != "" {
		return n
	}
	if st.Traffic != nil && st.Traffic.CapLimit > 0 && st.Traffic.CapUsed >= st.Traffic.CapLimit {
		n.Reason = "traffic-cap"
		return n
	}
	smartSamples.Lock()
	s := smartSamples.values[n.ID]
	smartSamples.Unlock()
	if st.State != "online" {
		// One failed probe stops admissions, but keeps an existing choice for a
		// short grace period. A confirmed failure moves it at the next fetch.
		n.Keep = s.DownChecks == 1 && now-s.DownSince < 30
		return n
	}
	n.Keep = true
	n.Reason = "warming-up"
	if s.At != st.CheckedAt || s.HealthyChecks < 2 {
		return n
	}
	cores := math.Max(1, float64(st.CpuCount))
	capacity := cores
	if st.Mem.Total > 0 {
		// A conservative baseline, not a benchmark: two GiB per CPU unit.
		capacity = math.Min(capacity, math.Max(.25, float64(st.Mem.Total)/(2*1024*1024*1024)))
	}
	if n.Entry.Capacity > 0 {
		capacity = n.Entry.Capacity
	}
	n.Base = capacity * math.Max(1, float64(n.Entry.Weight))
	load := math.Max(bounded(s.CPU/100, 0, 1), bounded(s.Memory/100, 0, 1))
	if n.Entry.BandwidthMbps > 0 {
		load = math.Max(load, bounded(s.Rate/float64(n.Entry.BandwidthMbps), 0, 1))
	}
	if n.Entry.MaxUsers > 0 {
		load = math.Max(load, float64(st.Online)/float64(n.Entry.MaxUsers))
	}
	n.BusySince = s.BusySince
	// Near a monthly quota, reduce only new admissions rather than move all users.
	quota := 1.0
	if st.Traffic != nil && st.Traffic.CapLimit > 0 {
		quota = bounded(1-float64(st.Traffic.CapUsed)/float64(st.Traffic.CapLimit), .05, 1)
	}
	// Recovered nodes enter gradually over five minutes; existing choices do not
	// move back simply because a former preferred node recovered.
	warm := bounded(float64(now-s.HealthySince)/300, .1, 1)
	n.Free = n.Base * bounded(1-load, .01, 1) * quota * warm
	n.Admit = !n.Entry.Drain && load < .9
	n.Reason = "ready"
	if n.Entry.Drain {
		n.Reason = "draining"
	} else if !n.Admit {
		n.Reason = "busy"
	}
	return n
}

// Weighted rendezvous: independent node scores, no list-position dependence.
// Removing a node redistributes only its users; every user's backup is distinct.
func smartRank(client uint, node uint, weight float64, salt string) float64 {
	var b [16]byte
	binary.BigEndian.PutUint64(b[:8], uint64(client))
	binary.BigEndian.PutUint64(b[8:], uint64(node))
	h := sha256.Sum256(append(b[:], []byte(salt)...))
	u := (float64(binary.BigEndian.Uint64(h[:8])>>11) + 1) / (float64(uint64(1)<<53) + 1)
	return -math.Log(u) / math.Max(weight, .000001)
}

func smartOrder(client uint, plan nodeRoutePlan, available map[string]bool, now int64) []string {
	type ranked struct {
		name  string
		tier  int
		score float64
	}
	list := []ranked{}
	for name, n := range plan.candidates {
		if !available[name] || !n.Keep {
			continue
		}
		// Healthy positive-weight nodes first. Zero-share nodes are emergency
		// capacity only; draining/warming nodes retain existing users only.
		if !n.Admit {
			continue
		}
		tier := 0
		if n.Entry.Weight == 0 {
			tier = 1
		}
		list = append(list, ranked{name, tier, smartRank(client, n.ID, n.Free, "admit")})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].tier != list[j].tier {
			return list[i].tier < list[j].tier
		}
		if list[i].score != list[j].score {
			return list[i].score < list[j].score
		}
		return list[i].name < list[j].name
	})
	primary := ""
	smartAssignmentMu.Lock()
	defer smartAssignmentMu.Unlock()
	db := database.GetDB()
	var saved model.NodeAssignment
	have := db != nil && client > 0 && db.Where("client_id = ?", client).Find(&saved).Error == nil && saved.NodeId != 0
	reason := "capacity"
	if have {
		reason = "unavailable"
		for name, n := range plan.candidates {
			if n.ID != saved.NodeId || !available[name] || !n.Keep {
				continue
			}
			primary = name
			// Sustained CPU/memory pressure may move only a tenth of users per
			// minute, and never twice within ten minutes. No single-sample churn.
			if !n.Entry.Drain && n.BusySince > 0 && now-n.BusySince >= 120 && now-saved.UpdatedAt >= 600 &&
				smartRank(client, n.ID, 1, time.Unix(now, 0).UTC().Format("200601021504")) < -math.Log(.9) {
				for _, r := range list {
					if r.name != name && r.tier == 0 {
						primary = r.name
						reason = "sustained-load"
						break
					}
				}
			}
			break
		}
	}
	if primary == "" && len(list) > 0 {
		primary = list[0].name
	}
	if primary != "" && db != nil && client > 0 && (!have || saved.NodeId != plan.candidates[primary].ID) {
		next := model.NodeAssignment{ClientId: client, NodeId: plan.candidates[primary].ID, UpdatedAt: now, Reason: reason}
		if err := db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "client_id"}}, DoUpdates: clause.AssignmentColumns([]string{"node_id", "updated_at", "reason"})}).Create(&next).Error; err != nil {
			logger.Warning("nodes: persist assignment: ", err)
		}
	}
	out := []string{}
	if primary != "" {
		out = append(out, primary)
	}
	for _, r := range list {
		if r.name != primary {
			out = append(out, r.name)
		}
	}
	return out
}

func orderBySmartRoute(links []map[string]interface{}, names map[int]string, client uint, plan nodeRoutePlan) bool {
	available := map[string]bool{}
	for _, name := range names {
		available[name] = true
	}
	order := smartOrder(client, plan, available, time.Now().Unix())
	ranks := map[string]int{}
	for i, name := range order {
		ranks[name] = i
	}
	slots := []int{}
	for i := range names {
		slots = append(slots, i)
	}
	sort.Ints(slots)
	source := append([]int(nil), slots...)
	rank := func(i int) int {
		if r, ok := ranks[names[i]]; ok {
			return r
		}
		return len(order) + 1
	}
	sort.SliceStable(source, func(i, j int) bool { return rank(source[i]) < rank(source[j]) })
	picked := make([]map[string]interface{}, len(source))
	moved := false
	for i, from := range source {
		picked[i] = links[from]
		moved = moved || from != slots[i]
	}
	for i, at := range slots {
		links[at] = picked[i]
	}
	return moved
}

// SmartPrimaryNode is the persisted, still-eligible choice, not a new allocation.
// JSON generation uses it after FilterNodeLinks has made exactly one decision.
func SmartPrimaryNode(client uint) string {
	currentNodeLinkRules()
	nodeLinkRulesMu.Lock()
	plan := nodeRoutePlanCached
	nodeLinkRulesMu.Unlock()
	if !plan.enable || !plan.smart || database.GetDB() == nil {
		return ""
	}
	var a model.NodeAssignment
	if database.GetDB().Where("client_id = ?", client).Find(&a).Error != nil {
		return ""
	}
	for name, n := range plan.candidates {
		if n.ID == a.NodeId && n.Keep {
			return name
		}
	}
	return ""
}

// SmartRoutingEnabled is deliberately separate from the saved percentage mode.
func SmartRoutingEnabled() bool {
	raw, _ := (&SettingService{}).getString("subNodeRoute")
	var cfg SubNodeRoute
	return json.Unmarshal([]byte(raw), &cfg) == nil && cfg.Enable && cfg.Smart
}

// SmartBackupNodes excludes stale, down, disabled, draining and busy nodes
// from the generated backup group, even if their manual links remain visible.
func SmartBackupNodes() map[string]bool {
	currentNodeLinkRules()
	nodeLinkRulesMu.Lock()
	defer nodeLinkRulesMu.Unlock()
	out := map[string]bool{}
	for name, n := range nodeRoutePlanCached.candidates {
		if n.Admit {
			out[name] = true
		}
	}
	return out
}

type NodeBalanceStatus struct {
	State    string  `json:"state"`
	Capacity float64 `json:"capacity"`
	Free     float64 `json:"free"`
	Assigned int64   `json:"assigned"`
}

func decorateSmartStatuses(statuses map[uint]NodeStatus) {
	if database.GetDB() == nil {
		return
	}
	currentNodeLinkRules()
	nodeLinkRulesMu.Lock()
	plan := nodeRoutePlanCached
	nodeLinkRulesMu.Unlock()
	if !plan.enable || !plan.smart {
		return
	}
	type row struct {
		NodeId uint
		Count  int64
	}
	var counts []row
	if db := database.GetDB(); db != nil {
		db.Model(&model.NodeAssignment{}).Select("node_assignments.node_id, count(*) AS count").
			Joins("JOIN clients ON clients.id = node_assignments.client_id AND clients.enable = ?", true).
			Group("node_assignments.node_id").Scan(&counts)
	}
	byID := map[uint]int64{}
	for _, r := range counts {
		byID[r.NodeId] = r.Count
	}
	for _, n := range plan.candidates {
		st, ok := statuses[n.ID]
		if !ok {
			continue
		}
		st.Balance = &NodeBalanceStatus{State: n.Reason, Capacity: n.Base, Free: n.Free, Assigned: byID[n.ID]}
		statuses[n.ID] = st
	}
}
