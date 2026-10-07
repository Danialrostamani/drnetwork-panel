package service

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/config"
	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/logger"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Node monitoring beyond the probe itself: the per-minute history behind the
// charts and the uptime, the outages, the traffic a node's network interfaces
// move, the threshold warnings and whether a node's links stay out of the
// subscriptions. All of it is fed by the probe results, in applyProbes.

// NodeWarning is a threshold a node is over right now.
type NodeWarning struct {
	// cpu, mem, disk, ping, cert or version.
	Key   string  `json:"key"`
	Value float64 `json:"value"`
	Limit float64 `json:"limit"`
	// The node's version, for a version warning.
	Info string `json:"info,omitempty"`
}

// NodeTrafficSummary is what a node's server moved, as its network interfaces
// count it. Up is what it sent, Down what it received.
type NodeTrafficSummary struct {
	TodayUp   int64 `json:"todayUp"`
	TodayDown int64 `json:"todayDown"`
	MonthUp   int64 `json:"monthUp"`
	MonthDown int64 `json:"monthDown"`
	TotalUp   int64 `json:"totalUp"`
	TotalDown int64 `json:"totalDown"`
	// The monthly cap, when the node has one: what the current cycle counted
	// and when the cycle started and ends.
	CapLimit int64 `json:"capLimit,omitempty"`
	CapUsed  int64 `json:"capUsed,omitempty"`
	CapStart int64 `json:"capStart,omitempty"`
	CapEnd   int64 `json:"capEnd,omitempty"`
}

// NodeEvent is something about a node the Telegram bot announces.
type NodeEvent struct {
	NodeId uint   `json:"nodeId"`
	Name   string `json:"name"`
	// "cap": the cap crossed Level percent. "hidden" / "shown": the node's links
	// left or came back to the subscriptions; Reason is "down" or "cap".
	Kind   string `json:"kind"`
	Level  int    `json:"level,omitempty"`
	Used   int64  `json:"used,omitempty"`
	Limit  int64  `json:"limit,omitempty"`
	Reason string `json:"reason,omitempty"`
	At     int64  `json:"at"`
}

const (
	nodeMetricRetention  = 7 * 24 * 3600
	nodeOutageRetention  = 90 * 24 * 3600
	nodeTrafficRetention = 400 * 24 * 3600
	// Failed probes in a row before an outage is recorded: one lost probe is
	// not an outage.
	nodeOutageMinChecks = 2
	// How often an ongoing outage records that it still lasts.
	nodeOutageCheckEvery = 60
	// Seconds a node has to be down before its links leave the subscriptions.
	nodeHideGrace = 180
	// A counter that moved faster than this (bytes per second, 100 Gbit/s) is
	// a glitch, not traffic.
	nodeMaxRate       = 12_500_000_000
	nodeEventQueueMax = 100
	// How far two readings of a node's boot time may differ and still be one
	// boot. In a container the boot time is worked out from the uptime, and
	// it moves by a second or so between readings.
	nodeBootSlack = 120
)

// nodeNetBase is the last reading of a node's interface counters. It is
// persisted every minute, so traffic a node moves while the master is down is
// still counted once the master is back.
type nodeNetBase struct {
	Sent uint64 `json:"sent"`
	Recv uint64 `json:"recv"`
	// Per interface, sent and received, when the node reports them.
	Ifs  map[string][2]uint64 `json:"ifs,omitempty"`
	Boot int64                `json:"boot"`
	// "nic" (without loopback) or "net" (every interface, older nodes).
	Src string `json:"src"`
	At  int64  `json:"at"`
}

// nodeCapState is the cap alert level a node reached in a cycle.
type nodeCapState struct {
	Cycle int64 `json:"cycle"`
	Level int   `json:"level"`
}

type nodeMonitorState struct {
	base     nodeNetBase
	haveBase bool

	// The minute being counted, and the sums behind its averages.
	minute               int64
	agg                  model.NodeMetric
	sumLat               int64
	sumCpu, sumMem       float64
	sumDisk              float64
	diskN                int
	pending              map[int64]*[2]int64 // hour -> up, down not yet written
	downStreak           int
	firstDown            int64
	outageOpen           bool
	outageChecked        int64
	outageState          string
	outageReason         string
	uptime24, uptime7d   float64
	traffic              *NodeTrafficSummary
	capLevel             int
	capCycle             int64
	hidden               string
	hiddenKnown          bool
	lastFlushHourChecked int64
}

var (
	nodeMonitorMu   sync.Mutex
	nodeMonitors    = map[uint]*nodeMonitorState{}
	nodeMonitorOnce sync.Once
	nodeLastPurge   int64
	nodeLocMu       sync.Mutex
	nodeLoc         *time.Location
	nodeLocAt       time.Time

	nodeEventsMu sync.Mutex
	nodeEvents   []NodeEvent
)

// nodeLocation is the panel's time zone, read at most once a minute.
func nodeLocation() *time.Location {
	nodeLocMu.Lock()
	defer nodeLocMu.Unlock()
	if nodeLoc != nil && time.Since(nodeLocAt) < time.Minute {
		return nodeLoc
	}
	loc, err := (&SettingService{}).GetTimeLocation()
	if err != nil || loc == nil {
		loc = time.Local
	}
	nodeLoc, nodeLocAt = loc, time.Now()
	return loc
}

// localHour is the start of the local hour that contains ts.
func localHour(ts int64, loc *time.Location) int64 {
	t := time.Unix(ts, 0).In(loc)
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, loc).Unix()
}

// localDay is the local midnight that starts the day of ts.
func localDay(ts int64, loc *time.Location) int64 {
	t := time.Unix(ts, 0).In(loc)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc).Unix()
}

// localMonth is the local midnight that starts the month of ts.
func localMonth(ts int64, loc *time.Location) int64 {
	t := time.Unix(ts, 0).In(loc)
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, loc).Unix()
}

// capCycleDay is day of month m of year y, or the month's last day when it is
// shorter. Month and year are normalised first, so month 0 is December of the
// year before.
func capCycleDay(y int, m time.Month, day int, loc *time.Location) time.Time {
	first := time.Date(y, m, 1, 0, 0, 0, 0, loc)
	last := first.AddDate(0, 1, -1).Day()
	if day > last {
		day = last
	}
	return time.Date(first.Year(), first.Month(), day, 0, 0, 0, 0, loc)
}

// capCycle returns the start and the end of the cap cycle that contains now,
// for a cycle that starts on day of the month.
func capCycle(now time.Time, day int) (time.Time, time.Time) {
	if day < 1 {
		day = 1
	}
	loc := now.Location()
	y, m, _ := now.Date()
	start := capCycleDay(y, m, day, loc)
	if now.Before(start) {
		start = capCycleDay(y, m-1, day, loc)
	}
	end := capCycleDay(start.Year(), start.Month()+1, day, loc)
	return start, end
}

// capLevelOf is the alert level a cap usage reached: 0, 80, 90 or 100.
func capLevelOf(used, limit int64) int {
	if limit <= 0 {
		return 0
	}
	switch {
	case used >= limit:
		return 100
	case used*10 >= limit*9:
		return 90
	case used*10 >= limit*8:
		return 80
	}
	return 0
}

// counterDelta is how far one interface counter moved from base to cur. A
// counter that went back started again from zero (a reboot, an interface that
// came back), so all of cur is new.
func counterDelta(base, cur uint64, restarted bool) int64 {
	if restarted || cur < base {
		return int64(cur)
	}
	return int64(cur - base)
}

// netDelta is what a node sent and received since base. Nothing is counted
// without a base from the same kind of counter, or when the numbers are too
// big to be real. Counted per interface when both readings have them: an
// interface that goes away then does not make the sum drop, which would read
// as counters that started again from zero.
func netDelta(base nodeNetBase, haveBase bool, cur nodeNetBase) (up, down int64, ok bool) {
	if !haveBase || base.Src != cur.Src {
		return 0, 0, false
	}
	boots := cur.Boot - base.Boot
	if boots < 0 {
		boots = -boots
	}
	restarted := cur.Boot > 0 && base.Boot > 0 && boots > nodeBootSlack
	if len(cur.Ifs) > 0 && len(base.Ifs) > 0 {
		for name, c := range cur.Ifs {
			b, had := base.Ifs[name]
			if !had && !restarted {
				// New since the reading before: its counters may hold far
				// more than that time, so they count from here on.
				continue
			}
			up += counterDelta(b[0], c[0], restarted)
			down += counterDelta(b[1], c[1], restarted)
		}
	} else {
		up = counterDelta(base.Sent, cur.Sent, restarted)
		down = counterDelta(base.Recv, cur.Recv, restarted)
	}
	elapsed := cur.At - base.At
	if elapsed < 5 {
		elapsed = 5
	}
	if up < 0 || down < 0 || up > nodeMaxRate*elapsed || down > nodeMaxRate*elapsed {
		return 0, 0, false
	}
	return up, down, true
}

// versionParts splits a version such as 1.6.3-drnetwork.21 into its numbers.
func versionParts(v string) []int {
	var parts []int
	for _, f := range strings.FieldsFunc(v, func(r rune) bool { return r < '0' || r > '9' }) {
		n, err := strconv.Atoi(f)
		if err != nil {
			n = 0
		}
		parts = append(parts, n)
	}
	return parts
}

// versionOlder reports whether version a comes before version b. A version
// that stops where the other goes on is the older one: 1.6.3 came before
// 1.6.3-drnetwork.1.
func versionOlder(a, b string) bool {
	pa, pb := versionParts(a), versionParts(b)
	for i := 0; i < len(pa) && i < len(pb); i++ {
		if pa[i] != pb[i] {
			return pa[i] < pb[i]
		}
	}
	return len(pa) < len(pb)
}

// nodeVersion is the release a node runs, as precisely as it reports it.
func nodeVersion(st *NodeStatus) string {
	if st.AppFull != "" {
		return st.AppFull
	}
	return st.AppVersion
}

func percentOf(cur, total int64) float64 {
	if total <= 0 {
		return 0
	}
	return float64(cur) * 100 / float64(total)
}

// nodeWarnings lists the thresholds a reachable node is over right now.
func nodeWarnings(n *model.Node, st *NodeStatus, masterVersion string, now int64) []NodeWarning {
	if st.State != "online" && st.State != "core-stopped" {
		return nil
	}
	var out []NodeWarning
	if lim := n.Alerts.CpuLimit(); lim > 0 && st.Cpu >= float64(lim) {
		out = append(out, NodeWarning{Key: "cpu", Value: st.Cpu, Limit: float64(lim)})
	}
	if lim := n.Alerts.MemLimit(); lim > 0 && st.Mem.Total > 0 {
		if p := percentOf(st.Mem.Current, st.Mem.Total); p >= float64(lim) {
			out = append(out, NodeWarning{Key: "mem", Value: p, Limit: float64(lim)})
		}
	}
	if lim := n.Alerts.DiskLimit(); lim > 0 && st.Disk.Total > 0 {
		if p := percentOf(st.Disk.Current, st.Disk.Total); p >= float64(lim) {
			out = append(out, NodeWarning{Key: "disk", Value: p, Limit: float64(lim)})
		}
	}
	if lim := n.Alerts.PingLimit(); lim > 0 && st.Latency >= int64(lim) {
		out = append(out, NodeWarning{Key: "ping", Value: float64(st.Latency), Limit: float64(lim)})
	}
	if lim := n.Alerts.CertDaysLimit(); lim > 0 && st.CertExpiry > 0 {
		if days := float64(st.CertExpiry-now) / 86400; days <= float64(lim) {
			out = append(out, NodeWarning{Key: "cert", Value: days, Limit: float64(lim)})
		}
	}
	if n.Alerts.VersionCheck() && masterVersion != "" {
		if v := nodeVersion(st); v != "" && versionOlder(v, masterVersion) {
			out = append(out, NodeWarning{Key: "version", Info: v})
		}
	}
	return out
}

func pushNodeEvent(e NodeEvent) {
	nodeEventsMu.Lock()
	defer nodeEventsMu.Unlock()
	nodeEvents = append(nodeEvents, e)
	if over := len(nodeEvents) - nodeEventQueueMax; over > 0 {
		nodeEvents = append([]NodeEvent(nil), nodeEvents[over:]...)
	}
}

// DrainNodeEvents hands over the node events not announced yet.
func DrainNodeEvents() []NodeEvent {
	nodeEventsMu.Lock()
	defer nodeEventsMu.Unlock()
	out := nodeEvents
	nodeEvents = nil
	return out
}

// monitorFor returns the node's monitor state, creating it from what the
// database kept: the last interface counters and the cap alert level.
// Callers hold nodeMonitorMu.
func monitorFor(n *model.Node, now int64, loc *time.Location) *nodeMonitorState {
	if m, ok := nodeMonitors[n.Id]; ok {
		return m
	}
	m := &nodeMonitorState{pending: map[int64]*[2]int64{}, uptime24: -1, uptime7d: -1}
	if len(n.NetBase) > 0 {
		if json.Unmarshal(n.NetBase, &m.base) == nil && m.base.Src != "" {
			m.haveBase = true
		}
	}
	var cs nodeCapState
	if len(n.CapState) > 0 && json.Unmarshal(n.CapState, &cs) == nil && n.Cap.Limit > 0 {
		start, _ := capCycle(time.Unix(now, 0).In(loc), n.Cap.ResetDay())
		if cs.Cycle == start.Unix() {
			m.capLevel, m.capCycle = cs.Level, cs.Cycle
		}
	}
	nodeMonitors[n.Id] = m
	return m
}

// nodeWrites is the database work one round of probes leaves behind.
type nodeWrites struct {
	metrics      []model.NodeMetric
	traffic      []model.NodeTraffic
	bases        map[uint]nodeNetBase
	outageStarts []model.NodeOutage
	outageEnds   map[uint]int64
	outageChecks map[uint]model.NodeOutage
	minuteDone   bool
}

func (w *nodeWrites) empty() bool {
	return len(w.metrics) == 0 && len(w.traffic) == 0 && len(w.bases) == 0 && len(w.outageStarts) == 0 && len(w.outageEnds) == 0 && len(w.outageChecks) == 0
}

// finishMinute moves the minute counted so far, and the traffic waiting for
// its hour, into w.
func (m *nodeMonitorState) finishMinute(id uint, w *nodeWrites) {
	if m.minute != 0 && m.agg.Probes > 0 {
		row := m.agg
		row.NodeId, row.DateTime = id, m.minute
		if row.Up > 0 {
			row.Latency = m.sumLat / int64(row.Up)
			row.Cpu = m.sumCpu / float64(row.Up)
			row.Mem = m.sumMem / float64(row.Up)
		}
		if m.diskN > 0 {
			row.Disk = m.sumDisk / float64(m.diskN)
		}
		w.metrics = append(w.metrics, row)
	}
	for hour, t := range m.pending {
		if t[0] > 0 || t[1] > 0 {
			w.traffic = append(w.traffic, model.NodeTraffic{NodeId: id, DateTime: hour, Up: t[0], Down: t[1]})
		}
	}
	m.pending = map[int64]*[2]int64{}
	if m.haveBase {
		if w.bases == nil {
			w.bases = map[uint]nodeNetBase{}
		}
		w.bases[id] = m.base
	}
	m.agg, m.sumLat, m.sumCpu, m.sumMem, m.sumDisk, m.diskN = model.NodeMetric{}, 0, 0, 0, 0, 0
	w.minuteDone = true
}

// observe folds one probe of node n into its monitor state and fills in what
// the status derives from it. prev is the status the node had before.
func (m *nodeMonitorState) observe(n *model.Node, st *NodeStatus, prev *NodeStatus, masterVersion string, now int64, loc *time.Location, w *nodeWrites) {
	// Live network speed, and the traffic this probe adds.
	var up, down int64
	if st.netSrc != "" {
		cur := nodeNetBase{Sent: st.netSent, Recv: st.netRecv, Ifs: st.netIfs, Boot: st.BootTime, Src: st.netSrc, At: st.CheckedAt}
		if d1, d2, ok := netDelta(m.base, m.haveBase, cur); ok {
			up, down = d1, d2
			if elapsed := cur.At - m.base.At; elapsed > 0 && elapsed <= 120 {
				st.NetUp, st.NetDown = up/elapsed, down/elapsed
			}
		}
		m.base, m.haveBase = cur, true
	}
	if up > 0 || down > 0 {
		hour := localHour(st.CheckedAt, loc)
		p := m.pending[hour]
		if p == nil {
			p = &[2]int64{}
			m.pending[hour] = p
		}
		p[0] += up
		p[1] += down
	}

	// The minute's history. A node held in maintenance is down on purpose:
	// those probes count neither for nor against its uptime.
	minute := st.CheckedAt - st.CheckedAt%60
	if m.minute != minute {
		m.finishMinute(n.Id, w)
		m.minute = minute
	}
	m.agg.Sent += up
	m.agg.Recv += down
	planned := st.State == "core-stopped" && st.Maintenance
	if !planned {
		m.agg.Probes++
	}
	if st.State == "online" {
		m.agg.Up++
		m.sumLat += st.Latency
		m.sumCpu += st.Cpu
		m.sumMem += percentOf(st.Mem.Current, st.Mem.Total)
		if st.Disk.Total > 0 {
			m.sumDisk += percentOf(st.Disk.Current, st.Disk.Total)
			m.diskN++
		}
		if st.Online > m.agg.Online {
			m.agg.Online = st.Online
		}
	}

	// Since when the node is down.
	if st.State == "online" {
		st.DownSince = 0
	} else if prev != nil && prev.State != "online" && prev.State != "" && prev.DownSince > 0 {
		st.DownSince = prev.DownSince
	} else {
		st.DownSince = st.CheckedAt
	}

	// Outages.
	if st.State == "online" || planned {
		if m.outageOpen {
			if w.outageEnds == nil {
				w.outageEnds = map[uint]int64{}
			}
			w.outageEnds[n.Id] = st.CheckedAt
			m.outageOpen = false
		}
		m.downStreak = 0
	} else {
		if m.downStreak == 0 {
			m.firstDown = st.CheckedAt
		}
		m.downStreak++
		reason := st.Error
		if !m.outageOpen && m.downStreak >= nodeOutageMinChecks {
			w.outageStarts = append(w.outageStarts, model.NodeOutage{NodeId: n.Id, Start: m.firstDown, State: st.State, Reason: reason, Checked: now})
			m.outageOpen, m.outageChecked, m.outageState, m.outageReason = true, now, st.State, reason
		} else if m.outageOpen && (now-m.outageChecked >= nodeOutageCheckEvery || st.State != m.outageState || reason != m.outageReason) {
			if w.outageChecks == nil {
				w.outageChecks = map[uint]model.NodeOutage{}
			}
			w.outageChecks[n.Id] = model.NodeOutage{State: st.State, Reason: reason, Checked: now}
			m.outageChecked, m.outageState, m.outageReason = now, st.State, reason
		}
	}

	st.Warnings = nodeWarnings(n, st, masterVersion, now)
}

// settle finishes a status once the round's history is written and summed: it
// takes the uptime and the traffic summary, and decides whether the node's
// links stay out of the subscriptions, while down long enough or over the cap.
func (m *nodeMonitorState) settle(n *model.Node, st *NodeStatus, now int64) {
	hidden := ""
	if n.HideDown && st.State != "online" && st.DownSince > 0 && now-st.DownSince >= nodeHideGrace {
		hidden = "down"
	} else if n.Cap.Hide && n.Cap.Limit > 0 && m.capLevel >= 100 {
		hidden = "cap"
	}
	if m.hiddenKnown && hidden != m.hidden {
		if hidden != "" {
			pushNodeEvent(NodeEvent{NodeId: n.Id, Name: n.Name, Kind: "hidden", Reason: hidden, At: now})
		} else {
			pushNodeEvent(NodeEvent{NodeId: n.Id, Name: n.Name, Kind: "shown", Reason: m.hidden, At: now})
		}
	}
	if hidden != m.hidden {
		invalidateNodeLinkRules()
	}
	m.hidden, m.hiddenKnown = hidden, true
	st.Hidden = hidden
	st.Uptime24, st.Uptime7d, st.Traffic = m.uptime24, m.uptime7d, m.traffic
}

// writeNodeMonitor stores one round's database work in one transaction.
func writeNodeMonitor(w *nodeWrites) error {
	if w.empty() {
		return nil
	}
	return database.GetDB().Transaction(func(tx *gorm.DB) error {
		for id, end := range w.outageEnds {
			if err := tx.Model(model.NodeOutage{}).Where("node_id = ? AND end_at = 0", id).Updates(map[string]interface{}{"end_at": end, "checked": end}).Error; err != nil {
				return err
			}
		}
		for i := range w.outageStarts {
			o := w.outageStarts[i]
			// A master restarted during an outage may still have it open.
			if err := tx.Model(model.NodeOutage{}).Where("node_id = ? AND end_at = 0", o.NodeId).Updates(map[string]interface{}{"end_at": gorm.Expr("CASE WHEN checked > start_at THEN checked ELSE start_at END")}).Error; err != nil {
				return err
			}
			if err := tx.Create(&o).Error; err != nil {
				return err
			}
		}
		for id, o := range w.outageChecks {
			if err := tx.Model(model.NodeOutage{}).Where("node_id = ? AND end_at = 0", id).Updates(map[string]interface{}{"checked": o.Checked, "state": o.State, "reason": o.Reason}).Error; err != nil {
				return err
			}
		}
		if len(w.metrics) > 0 {
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "node_id"}, {Name: "date_time"}},
				DoUpdates: clause.AssignmentColumns([]string{"probes", "up", "latency", "cpu", "mem", "disk", "online", "sent", "recv"}),
			}).CreateInBatches(w.metrics, 100).Error; err != nil {
				return err
			}
		}
		if len(w.traffic) > 0 {
			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "node_id"}, {Name: "date_time"}},
				DoUpdates: clause.Assignments(map[string]interface{}{
					"up":   gorm.Expr("node_traffic.up + excluded.up"),
					"down": gorm.Expr("node_traffic.down + excluded.down"),
				}),
			}).CreateInBatches(w.traffic, 100).Error; err != nil {
				return err
			}
		}
		for id, base := range w.bases {
			raw, err := json.Marshal(base)
			if err != nil {
				return err
			}
			if err := tx.Model(model.Node{}).Where("id = ?", id).Update("net_base", raw).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// closeStaleOutages ends the outages a stopped master left open, at the last
// moment they were seen. Runs once, before the first probes are recorded.
func closeStaleOutages() {
	err := database.GetDB().Model(model.NodeOutage{}).Where("end_at = 0").
		Update("end_at", gorm.Expr("CASE WHEN checked > start_at THEN checked ELSE start_at END")).Error
	if err != nil {
		logger.Warning("nodes: close stale outages: ", err)
	}
}

// applyProbes records a round of probe results: the statuses the panel shows,
// the history, the outages and the traffic. replace drops the statuses of the
// nodes that were not probed (the periodic round probes every enabled node);
// a probe of a few nodes on demand merges into what is there.
func (s *NodeService) applyProbes(nodes []*model.Node, fresh map[uint]NodeStatus, replace bool) {
	nodeMonitorOnce.Do(closeStaleOutages)
	now := time.Now().Unix()
	loc := nodeLocation()
	masterVersion := config.GetFullVersion()

	nodeMonitorMu.Lock()
	defer nodeMonitorMu.Unlock()

	nodeStatusMu.RLock()
	old := nodeStatuses
	nodeStatusMu.RUnlock()

	w := &nodeWrites{}
	observed := make([]*model.Node, 0, len(nodes))
	for _, n := range nodes {
		st, ok := fresh[n.Id]
		if !ok {
			continue
		}
		var prev *NodeStatus
		if p, had := old[n.Id]; had {
			prev = &p
		}
		if prev != nil && st.CheckedAt < prev.CheckedAt {
			// A round that started before the result already recorded.
			fresh[n.Id] = *prev
			continue
		}
		if st.State == "online" {
			st.LastOnline = st.CheckedAt
		} else if prev != nil {
			st.LastOnline = prev.LastOnline
		}
		monitorFor(n, now, loc).observe(n, &st, prev, masterVersion, now, loc, w)
		fresh[n.Id] = st
		observed = append(observed, n)
	}
	if replace {
		for id, m := range nodeMonitors {
			if _, probed := fresh[id]; probed {
				continue
			}
			// Disabled or deleted: its outage ends now and its history stops.
			if m.outageOpen {
				if w.outageEnds == nil {
					w.outageEnds = map[uint]int64{}
				}
				w.outageEnds[id] = now
			}
			if m.hidden != "" {
				invalidateNodeLinkRules()
			}
			delete(nodeMonitors, id)
		}
	}

	// The history first: the statuses carry the uptime and the traffic it
	// sums up.
	if err := writeNodeMonitor(w); err != nil {
		logger.Warning("nodes: record history: ", err)
	}
	if w.minuteDone {
		summarizeNodes(observed, now, loc)
		if hour := now - now%3600; hour != nodeLastPurge {
			nodeLastPurge = hour
			purgeNodeHistory(now)
		}
	}
	for _, n := range observed {
		st := fresh[n.Id]
		nodeMonitors[n.Id].settle(n, &st, now)
		fresh[n.Id] = st
	}

	nodeStatusMu.Lock()
	var next map[uint]NodeStatus
	if replace {
		next = fresh
	} else {
		next = make(map[uint]NodeStatus, len(old)+len(fresh))
		for id, st := range old {
			next[id] = st
		}
		for id, st := range fresh {
			next[id] = st
		}
	}
	nodeStatuses = next
	nodeStatusMu.Unlock()

	db := database.GetDB()
	for id, st := range fresh {
		if prev, ok := old[id]; ok && prev.State == "online" && st.State != "online" && prev.LastOnline > 0 {
			if err := db.Model(model.Node{}).Where("id = ?", id).Update("last_seen", prev.LastOnline).Error; err != nil {
				logger.Warning("nodes: last_seen update failed: ", err)
			}
		}
	}
}

// summarizeNodes refreshes the uptime, the traffic summary and the cap level
// of each node from the database. Callers hold nodeMonitorMu.
func summarizeNodes(nodes []*model.Node, now int64, loc *time.Location) {
	db := database.GetDB()
	type upRow struct {
		NodeId uint
		Up     int64
		Probes int64
	}
	uptime := func(since int64) map[uint]float64 {
		var rows []upRow
		out := map[uint]float64{}
		if err := db.Model(model.NodeMetric{}).Select("node_id, SUM(up) AS up, SUM(probes) AS probes").Where("date_time >= ?", since).Group("node_id").Scan(&rows).Error; err != nil {
			logger.Warning("nodes: uptime: ", err)
			return out
		}
		for _, r := range rows {
			if r.Probes > 0 {
				out[r.NodeId] = float64(r.Up) * 100 / float64(r.Probes)
			}
		}
		return out
	}
	up24, up7 := uptime(now-24*3600), uptime(now-7*24*3600)

	type sumRow struct {
		NodeId uint
		Up     int64
		Down   int64
	}
	sums := func(since int64) map[uint]sumRow {
		var rows []sumRow
		out := map[uint]sumRow{}
		q := db.Model(model.NodeTraffic{}).Select("node_id, SUM(up) AS up, SUM(down) AS down")
		if since > 0 {
			q = q.Where("date_time >= ?", since)
		}
		if err := q.Group("node_id").Scan(&rows).Error; err != nil {
			logger.Warning("nodes: traffic summary: ", err)
			return out
		}
		for _, r := range rows {
			out[r.NodeId] = r
		}
		return out
	}
	today, month, total := sums(localDay(now, loc)), sums(localMonth(now, loc)), sums(0)

	for _, n := range nodes {
		m, ok := nodeMonitors[n.Id]
		if !ok {
			continue
		}
		m.uptime24, m.uptime7d = -1, -1
		if v, ok := up24[n.Id]; ok {
			m.uptime24 = v
		}
		if v, ok := up7[n.Id]; ok {
			m.uptime7d = v
		}
		t := &NodeTrafficSummary{
			TodayUp: today[n.Id].Up, TodayDown: today[n.Id].Down,
			MonthUp: month[n.Id].Up, MonthDown: month[n.Id].Down,
			TotalUp: total[n.Id].Up, TotalDown: total[n.Id].Down,
		}
		if n.Cap.Limit > 0 {
			used, start, end, err := nodeCapUsage(n, now, loc)
			if err != nil {
				logger.Warning("nodes: cap usage of ", n.Name, ": ", err)
			} else {
				t.CapLimit, t.CapUsed, t.CapStart, t.CapEnd = n.Cap.Limit, used, start, end
				m.updateCapLevel(n, used, start, now)
			}
		} else {
			m.capLevel, m.capCycle = 0, 0
		}
		m.traffic = t
	}
}

// nodeCapUsage is what the node's current cap cycle counted so far.
func nodeCapUsage(n *model.Node, now int64, loc *time.Location) (used, start, end int64, err error) {
	s, e := capCycle(time.Unix(now, 0).In(loc), n.Cap.ResetDay())
	var row struct {
		Up   int64
		Down int64
	}
	err = database.GetDB().Model(model.NodeTraffic{}).Select("COALESCE(SUM(up), 0) AS up, COALESCE(SUM(down), 0) AS down").
		Where("node_id = ? AND date_time >= ? AND date_time < ?", n.Id, s.Unix(), e.Unix()).Scan(&row).Error
	return n.Cap.Counted(row.Up, row.Down), s.Unix(), e.Unix(), err
}

// updateCapLevel announces a cap level the node crossed in this cycle, once,
// and keeps the level in the database so a restart does not announce it again.
func (m *nodeMonitorState) updateCapLevel(n *model.Node, used, cycle, now int64) {
	level := capLevelOf(used, n.Cap.Limit)
	if m.capCycle != cycle {
		m.capCycle, m.capLevel = cycle, 0
	}
	if level == m.capLevel {
		return
	}
	if level > m.capLevel {
		pushNodeEvent(NodeEvent{NodeId: n.Id, Name: n.Name, Kind: "cap", Level: level, Used: used, Limit: n.Cap.Limit, At: now})
	}
	m.capLevel = level
	raw, _ := json.Marshal(nodeCapState{Cycle: cycle, Level: level})
	if err := database.GetDB().Model(model.Node{}).Where("id = ?", n.Id).Update("cap_state", raw).Error; err != nil {
		logger.Warning("nodes: save cap level: ", err)
	}
}

// purgeNodeHistory drops history older than it is kept, and the rows of nodes
// that no longer exist. Traffic hours too old to keep one by one are added to
// the node's row at hour zero, so its total stays whole.
func purgeNodeHistory(now int64) {
	db := database.GetDB()
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("date_time < ?", now-nodeMetricRetention).Delete(&model.NodeMetric{}).Error; err != nil {
			return err
		}
		if err := tx.Where("end_at > 0 AND end_at < ?", now-nodeOutageRetention).Delete(&model.NodeOutage{}).Error; err != nil {
			return err
		}
		cutoff := now - nodeTrafficRetention
		if err := tx.Exec(`INSERT INTO node_traffic (node_id, date_time, up, down)
			SELECT node_id, 0, SUM(up), SUM(down) FROM node_traffic WHERE date_time > 0 AND date_time < ? GROUP BY node_id
			ON CONFLICT (node_id, date_time) DO UPDATE SET up = node_traffic.up + excluded.up, down = node_traffic.down + excluded.down`, cutoff).Error; err != nil {
			return err
		}
		if err := tx.Where("date_time > 0 AND date_time < ?", cutoff).Delete(&model.NodeTraffic{}).Error; err != nil {
			return err
		}
		for _, m := range []interface{}{&model.NodeMetric{}, &model.NodeOutage{}, &model.NodeTraffic{}} {
			if err := tx.Where("node_id NOT IN (SELECT id FROM nodes)").Delete(m).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		logger.Warning("nodes: purge history: ", err)
	}
}

// deleteNodeHistory drops everything recorded about a node being deleted.
func deleteNodeHistory(tx *gorm.DB, id uint) error {
	for _, m := range []interface{}{&model.NodeMetric{}, &model.NodeOutage{}, &model.NodeTraffic{}} {
		if err := tx.Where("node_id = ?", id).Delete(m).Error; err != nil {
			return err
		}
	}
	return nil
}

// ---- subscriptions ----

type nodeLinkRule struct {
	hidden bool
	access model.NodeAccess
}

var (
	nodeLinkRulesMu    sync.Mutex
	nodeLinkRulesAt    time.Time
	nodeLinkRulesCache map[string]nodeLinkRule
)

const nodeLinkRulesTTL = 3 * time.Second

func invalidateNodeLinkRules() {
	nodeLinkRulesMu.Lock()
	nodeLinkRulesAt = time.Time{}
	nodeLinkRulesMu.Unlock()
}

// currentNodeLinkRules returns, by node name, the nodes that keep their links
// from some or all clients right now.
func currentNodeLinkRules() map[string]nodeLinkRule {
	nodeLinkRulesMu.Lock()
	defer nodeLinkRulesMu.Unlock()
	if !nodeLinkRulesAt.IsZero() && time.Since(nodeLinkRulesAt) < nodeLinkRulesTTL {
		return nodeLinkRulesCache
	}
	var nodes []model.Node
	if err := database.GetDB().Select("id", "name", "access").Find(&nodes).Error; err != nil {
		logger.Warning("nodes: load link rules: ", err)
		return nodeLinkRulesCache
	}
	nodeStatusMu.RLock()
	rules := map[string]nodeLinkRule{}
	for _, n := range nodes {
		r := nodeLinkRule{hidden: nodeStatuses[n.Id].Hidden != "", access: n.Access}
		if r.hidden || r.access.Restricted() {
			rules[n.Name] = r
		}
	}
	nodeStatusMu.RUnlock()
	nodeLinkRulesCache, nodeLinkRulesAt = rules, time.Now()
	return rules
}

// FilterNodeLinks returns the client's links without those of the nodes that
// do not serve it right now: a node down long enough or over its cap, with
// hiding turned on, and a node whose access list leaves the client out.
func FilterNodeLinks(client *model.Client) json.RawMessage {
	if client == nil || len(client.Links) == 0 {
		return nil
	}
	rules := currentNodeLinkRules()
	if len(rules) == 0 {
		return client.Links
	}
	var links []map[string]interface{}
	if json.Unmarshal(client.Links, &links) != nil {
		return client.Links
	}
	names := make([]string, 0, len(rules))
	for name := range rules {
		names = append(names, name)
	}
	// Longest first, so node "a" never claims the links of node "a b".
	sort.Slice(names, func(i, j int) bool { return len(names[i]) > len(names[j]) })
	kept := make([]map[string]interface{}, 0, len(links))
	dropped := false
	for _, link := range links {
		typ, _ := link["type"].(string)
		remark, _ := link["remark"].(string)
		if typ == "external" {
			drop := false
			for _, name := range names {
				if strings.HasPrefix(remark, nodeLinkPrefix(name)) {
					r := rules[name]
					drop = r.hidden || !r.access.Allows(client.Id, client.Group)
					break
				}
			}
			if drop {
				dropped = true
				continue
			}
		}
		kept = append(kept, link)
	}
	if !dropped {
		return client.Links
	}
	out, err := json.Marshal(kept)
	if err != nil {
		return client.Links
	}
	return out
}
