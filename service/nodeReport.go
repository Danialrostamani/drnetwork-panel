package service

import (
	"sort"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/util/common"
)

// The history behind a node's charts, its outages and its traffic report.

// NodeHistoryPoint is one bucket of a node's history.
type NodeHistoryPoint struct {
	T int64 `json:"t"`
	// Percent of the probes that found the node online; -1 when no probe
	// counts (the node was held in maintenance).
	Uptime  float64 `json:"uptime"`
	Latency int64   `json:"latency"`
	Cpu     float64 `json:"cpu"`
	Mem     float64 `json:"mem"`
	Disk    float64 `json:"disk"`
	Online  int     `json:"online"`
	// Bytes the server sent and received in the bucket.
	Sent int64 `json:"sent"`
	Recv int64 `json:"recv"`
}

// NodeHistory is a node's history over the last hours, in buckets of Bucket
// seconds.
type NodeHistory struct {
	Since  int64              `json:"since"`
	Bucket int64              `json:"bucket"`
	Points []NodeHistoryPoint `json:"points"`
}

const nodeHistoryMaxHours = nodeMetricRetention / 3600

func nodeHistoryBucket(hours int) int64 {
	switch {
	case hours <= 3:
		return 60
	case hours <= 24:
		return 300
	}
	return 1800
}

// GetNodeHistory is the node's history over the last hours.
func (s *NodeService) GetNodeHistory(id uint, hours int) (*NodeHistory, error) {
	if hours < 1 {
		hours = 1
	}
	if hours > nodeHistoryMaxHours {
		hours = nodeHistoryMaxHours
	}
	bucket := nodeHistoryBucket(hours)
	since := time.Now().Unix() - int64(hours)*3600
	since -= since % bucket
	var rows []model.NodeMetric
	if err := database.GetDB().Where("node_id = ? AND date_time >= ?", id, since).Order("date_time").Find(&rows).Error; err != nil {
		return nil, err
	}
	type acc struct {
		probes, up       int
		lat              int64
		cpu, mem, disk   float64
		diskUp, online   int
		sent, recv, from int64
	}
	var order []int64
	sums := map[int64]*acc{}
	for _, r := range rows {
		t := r.DateTime - r.DateTime%bucket
		a := sums[t]
		if a == nil {
			a = &acc{from: t}
			sums[t] = a
			order = append(order, t)
		}
		a.probes += r.Probes
		a.up += r.Up
		a.lat += r.Latency * int64(r.Up)
		a.cpu += r.Cpu * float64(r.Up)
		a.mem += r.Mem * float64(r.Up)
		if r.Disk > 0 {
			a.disk += r.Disk * float64(r.Up)
			a.diskUp += r.Up
		}
		if r.Online > a.online {
			a.online = r.Online
		}
		a.sent += r.Sent
		a.recv += r.Recv
	}
	out := &NodeHistory{Since: since, Bucket: bucket, Points: make([]NodeHistoryPoint, 0, len(order))}
	for _, t := range order {
		a := sums[t]
		p := NodeHistoryPoint{T: t, Uptime: -1, Online: a.online, Sent: a.sent, Recv: a.recv}
		if a.probes > 0 {
			p.Uptime = float64(a.up) * 100 / float64(a.probes)
		}
		if a.up > 0 {
			p.Latency = a.lat / int64(a.up)
			p.Cpu = a.cpu / float64(a.up)
			p.Mem = a.mem / float64(a.up)
		}
		if a.diskUp > 0 {
			p.Disk = a.disk / float64(a.diskUp)
		}
		out.Points = append(out.Points, p)
	}
	return out, nil
}

// NodeOutageReport is a node's latest outages and how much of the last day,
// week and month it was down.
type NodeOutageReport struct {
	Outages []model.NodeOutage `json:"outages"`
	// Percent of the probes that found the node online; -1 without history.
	Uptime24 float64 `json:"uptime24"`
	Uptime7d float64 `json:"uptime7d"`
	// Seconds down, and outages, in the last 24 hours, 7 and 30 days.
	Down24   int64 `json:"down24"`
	Down7d   int64 `json:"down7d"`
	Down30d  int64 `json:"down30d"`
	Count24  int   `json:"count24"`
	Count7d  int   `json:"count7d"`
	Count30d int   `json:"count30d"`
}

const nodeOutagesListed = 100

// outageOverlap is how many seconds of the outage fall after since.
func outageOverlap(o model.NodeOutage, since, now int64) int64 {
	end := o.End
	if end == 0 || end > now {
		end = now
	}
	start := o.Start
	if start < since {
		start = since
	}
	if end <= start {
		return 0
	}
	return end - start
}

func (s *NodeService) GetNodeOutages(id uint) (*NodeOutageReport, error) {
	now := time.Now().Unix()
	db := database.GetDB()
	out := &NodeOutageReport{Outages: []model.NodeOutage{}, Uptime24: -1, Uptime7d: -1}
	if err := db.Where("node_id = ?", id).Order("start_at DESC").Limit(nodeOutagesListed).Find(&out.Outages).Error; err != nil {
		return nil, err
	}
	var recent []model.NodeOutage
	if err := db.Where("node_id = ? AND (end_at = 0 OR end_at > ?)", id, now-30*24*3600).Find(&recent).Error; err != nil {
		return nil, err
	}
	for _, o := range recent {
		if d := outageOverlap(o, now-24*3600, now); d > 0 {
			out.Down24 += d
			out.Count24++
		}
		if d := outageOverlap(o, now-7*24*3600, now); d > 0 {
			out.Down7d += d
			out.Count7d++
		}
		if d := outageOverlap(o, now-30*24*3600, now); d > 0 {
			out.Down30d += d
			out.Count30d++
		}
	}
	for _, w := range []struct {
		since int64
		dst   *float64
	}{{now - 24*3600, &out.Uptime24}, {now - 7*24*3600, &out.Uptime7d}} {
		var row struct {
			Up     int64
			Probes int64
		}
		if err := db.Model(model.NodeMetric{}).Select("COALESCE(SUM(up), 0) AS up, COALESCE(SUM(probes), 0) AS probes").
			Where("node_id = ? AND date_time >= ?", id, w.since).Scan(&row).Error; err != nil {
			return nil, err
		}
		if row.Probes > 0 {
			*w.dst = float64(row.Up) * 100 / float64(row.Probes)
		}
	}
	return out, nil
}

// NodeTrafficPoint is what a node's server moved in one hour, day or month.
type NodeTrafficPoint struct {
	T    int64 `json:"t"`
	Up   int64 `json:"up"`
	Down int64 `json:"down"`
}

// NodeClientTraffic is what one client moved through a node.
type NodeClientTraffic struct {
	Name string `json:"name"`
	Up   int64  `json:"up"`
	Down int64  `json:"down"`
}

// NodeTrafficReport is a node's traffic over a period: what its network
// interfaces moved, and the clients that moved the most through it.
type NodeTrafficReport struct {
	Period  string              `json:"period"`
	Points  []NodeTrafficPoint  `json:"points"`
	Summary NodeTrafficSummary  `json:"summary"`
	Clients []NodeClientTraffic `json:"clients"`
	// Why Clients is empty when the node could not tell: "update",
	// "unreachable" or "disabled".
	ClientsNote string `json:"clientsNote,omitempty"`
}

const nodeTopClients = 10

// nodeTrafficSummaryOf is the traffic summary of one node, fresh from the
// database.
func nodeTrafficSummaryOf(n *model.Node, now int64) (NodeTrafficSummary, error) {
	loc := nodeLocation()
	db := database.GetDB()
	var t NodeTrafficSummary
	for _, w := range []struct {
		since    int64
		up, down *int64
	}{{localDay(now, loc), &t.TodayUp, &t.TodayDown}, {localMonth(now, loc), &t.MonthUp, &t.MonthDown}, {0, &t.TotalUp, &t.TotalDown}} {
		var row struct {
			Up   int64
			Down int64
		}
		if err := db.Model(model.NodeTraffic{}).Select("COALESCE(SUM(up), 0) AS up, COALESCE(SUM(down), 0) AS down").
			Where("node_id = ? AND date_time >= ?", n.Id, w.since).Scan(&row).Error; err != nil {
			return t, err
		}
		*w.up, *w.down = row.Up, row.Down
	}
	if n.Cap.Limit > 0 {
		used, start, end, err := nodeCapUsage(n, now, loc)
		if err != nil {
			return t, err
		}
		t.CapLimit, t.CapUsed, t.CapStart, t.CapEnd = n.Cap.Limit, used, start, end
	}
	return t, nil
}

// nodeTrafficSlots are the starts of the hours, days or months of a period,
// oldest first.
func nodeTrafficSlots(period string, now time.Time) ([]int64, func(int64) int64, error) {
	loc := now.Location()
	var slots []int64
	switch period {
	case "24h":
		first := localHour(now.Unix(), loc) - 23*3600
		for t := first; t <= now.Unix(); t += 3600 {
			slots = append(slots, t)
		}
		return slots, func(ts int64) int64 { return localHour(ts, loc) }, nil
	case "7d", "30d":
		days := 7
		if period == "30d" {
			days = 30
		}
		day := time.Unix(localDay(now.Unix(), loc), 0).In(loc).AddDate(0, 0, -(days - 1))
		for i := 0; i < days; i++ {
			slots = append(slots, day.AddDate(0, 0, i).Unix())
		}
		return slots, func(ts int64) int64 { return localDay(ts, loc) }, nil
	case "12m":
		month := time.Unix(localMonth(now.Unix(), loc), 0).In(loc).AddDate(0, -11, 0)
		for i := 0; i < 12; i++ {
			slots = append(slots, month.AddDate(0, i, 0).Unix())
		}
		return slots, func(ts int64) int64 { return localMonth(ts, loc) }, nil
	}
	return nil, nil, common.NewError("unknown period")
}

// GetTrafficReport is the node's traffic over the period: 24h, 7d, 30d or 12m.
func (s *NodeSyncService) GetTrafficReport(id uint, period string) (*NodeTrafficReport, error) {
	var node model.Node
	if err := database.GetDB().First(&node, id).Error; err != nil {
		return nil, common.NewError("node not found")
	}
	now := time.Now().In(nodeLocation())
	slots, slotOf, err := nodeTrafficSlots(period, now)
	if err != nil {
		return nil, err
	}
	var rows []model.NodeTraffic
	if err := database.GetDB().Where("node_id = ? AND date_time >= ?", id, slots[0]).Find(&rows).Error; err != nil {
		return nil, err
	}
	sums := make(map[int64]*NodeTrafficPoint, len(slots))
	out := &NodeTrafficReport{Period: period, Points: make([]NodeTrafficPoint, len(slots)), Clients: []NodeClientTraffic{}}
	for i, t := range slots {
		out.Points[i] = NodeTrafficPoint{T: t}
		sums[t] = &out.Points[i]
	}
	for _, r := range rows {
		if p := sums[slotOf(r.DateTime)]; p != nil {
			p.Up += r.Up
			p.Down += r.Down
		}
	}
	if out.Summary, err = nodeTrafficSummaryOf(&node, now.Unix()); err != nil {
		return nil, err
	}

	// The clients that moved the most, as the node's own stats count them.
	switch {
	case !node.Enable:
		out.ClientsNote = "disabled"
		return out, nil
	case nodeKnownOffline(node.Id):
		out.ClientsNote = NodeUnreachable
		return out, nil
	}
	window := now.Unix() - slots[0]
	sum, note := askNodeSummary(&node, slots[0], window)
	if sum == nil {
		out.ClientsNote = note
		return out, nil
	}
	var names []string
	if err := database.GetDB().Model(model.Client{}).Pluck("name", &names).Error; err != nil {
		return nil, err
	}
	ours := make(map[string]bool, len(names))
	for _, n := range names {
		ours[n] = true
	}
	for _, t := range sum.Totals {
		if t.Resource == "user" && ours[t.Tag] && t.Up+t.Down > 0 {
			out.Clients = append(out.Clients, NodeClientTraffic{Name: t.Tag, Up: t.Up, Down: t.Down})
		}
	}
	sort.Slice(out.Clients, func(i, j int) bool {
		a, b := out.Clients[i], out.Clients[j]
		if a.Up+a.Down != b.Up+b.Down {
			return a.Up+a.Down > b.Up+b.Down
		}
		return a.Name < b.Name
	})
	if len(out.Clients) > nodeTopClients {
		out.Clients = out.Clients[:nodeTopClients]
	}
	return out, nil
}
