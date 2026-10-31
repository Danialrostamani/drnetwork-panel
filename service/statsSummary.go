package service

import (
	"encoding/json"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/logger"
)

// A summary of the traffic counted in a window, the numbers behind the
// Telegram bot's Stats screen.
//
// A node counts what its own core carries, so a client served by a node has
// that traffic in the node's stats table and not in the master's. A node
// therefore answers the same question the master asks itself (action
// statsTotals of its API), and the master adds the answers up.

// StatsTotal is what one user, inbound or outbound moved in the window.
type StatsTotal struct {
	Resource string `json:"resource"`
	Tag      string `json:"tag"`
	Up       int64  `json:"up"`
	Down     int64  `json:"down"`
}

// StatsPoint is what one inbound moved in one bucket of the window; At is the
// start of the bucket, in seconds since the epoch.
type StatsPoint struct {
	Tag     string `json:"tag"`
	At      int64  `json:"t"`
	Traffic int64  `json:"traffic"`
}

// StatsSummary is the answer of a node's statsTotals and of the master's own
// stats alike.
type StatsSummary struct {
	Since  int64        `json:"since"`
	Bucket int64        `json:"bucket"`
	Totals []StatsTotal `json:"totals"`
	Series []StatsPoint `json:"series"`
}

const (
	// statsTotalsAction is the node API action that returns a StatsSummary.
	statsTotalsAction = "statsTotals"

	// A summary carries the busiest summaryTopN tags of each resource and at
	// most summaryMaxSeries points, which keeps the answer of a node with
	// thousands of clients well inside what the master accepts.
	summaryTopN      = 2000
	summaryMaxSeries = 20000
	minSummaryBucket = 60

	// A node the probe found down less than this long ago is not asked.
	nodeSummarySkipOffline = 60 * time.Second
)

// nodeSummaryTimeout bounds how long one unreachable node can hold the bot
// back; the nodes are asked at the same time.
var nodeSummaryTimeout = 5 * time.Second

var summaryResources = []string{"user", "inbound", "outbound"}

// GetSummary adds up what the stats table counted after since: per user,
// inbound and outbound, and per bucket seconds for the inbounds. This is what
// a node answers to statsTotals.
func (s *StatsService) GetSummary(since, bucket int64) (*StatsSummary, error) {
	if bucket < minSummaryBucket {
		bucket = minSummaryBucket
	}
	if since <= 0 {
		since = time.Now().Unix() - 24*3600
	}
	db := database.GetDB()
	var rows []struct {
		Resource  string
		Tag       string
		Direction bool
		Total     int64
	}
	err := db.Model(model.Stats{}).
		Select("resource, tag, direction, SUM(traffic) AS total").
		Where("date_time > ? AND resource IN ?", since, summaryResources).
		Group("resource, tag, direction").Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	type key struct{ resource, tag string }
	sums := map[key]*StatsTotal{}
	for _, r := range rows {
		k := key{r.Resource, r.Tag}
		t := sums[k]
		if t == nil {
			t = &StatsTotal{Resource: r.Resource, Tag: r.Tag}
			sums[k] = t
		}
		if r.Direction {
			t.Up += r.Total
		} else {
			t.Down += r.Total
		}
	}
	sum := &StatsSummary{Since: since, Bucket: bucket, Totals: []StatsTotal{}, Series: []StatsPoint{}}
	for _, t := range sums {
		sum.Totals = append(sum.Totals, *t)
	}
	sum.Totals = busiest(sum.Totals, summaryTopN)

	var points []struct {
		Tag   string
		B     int64
		Total int64
	}
	// Newest first, so that if the limit ever cuts, it cuts the oldest points.
	err = db.Raw("SELECT tag, (date_time / ?) * ? AS b, SUM(traffic) AS total FROM stats WHERE resource = 'inbound' AND date_time > ? GROUP BY tag, b ORDER BY b DESC, tag LIMIT ?",
		bucket, bucket, since, summaryMaxSeries).Scan(&points).Error
	if err != nil {
		return nil, err
	}
	for _, p := range points {
		sum.Series = append(sum.Series, StatsPoint{Tag: p.Tag, At: p.B, Traffic: p.Total})
	}
	sortSeries(sum.Series)
	return sum, nil
}

// sortSeries puts points in time order, and by tag within a bucket.
func sortSeries(points []StatsPoint) {
	sort.Slice(points, func(i, j int) bool {
		if points[i].At != points[j].At {
			return points[i].At < points[j].At
		}
		return points[i].Tag < points[j].Tag
	})
}

// busiest orders totals resource by resource, the busiest tag first, and keeps
// at most limit of each.
func busiest(totals []StatsTotal, limit int) []StatsTotal {
	rank := map[string]int{}
	for i, r := range summaryResources {
		rank[r] = i
	}
	sort.Slice(totals, func(i, j int) bool {
		a, b := totals[i], totals[j]
		if a.Resource != b.Resource {
			return rank[a.Resource] < rank[b.Resource]
		}
		if at, bt := a.Up+a.Down, b.Up+b.Down; at != bt {
			return at > bt
		}
		return a.Tag < b.Tag
	})
	out := make([]StatsTotal, 0, len(totals))
	kept := map[string]int{}
	for _, t := range totals {
		if kept[t.Resource] < limit {
			kept[t.Resource]++
			out = append(out, t)
		}
	}
	return out
}

// Why a node's numbers are missing from a ClusterSummary.
const (
	// NodeNeedsUpdate: the node is reachable but too old to answer statsTotals.
	NodeNeedsUpdate = "update"
	// NodeUnreachable: the node did not answer, or answered something else.
	NodeUnreachable = "unreachable"
)

// NodeNote names a node whose traffic is not part of a ClusterSummary.
type NodeNote struct {
	Name   string
	Reason string
}

// ClusterSummary is the master's own summary with the traffic the enabled
// nodes counted added to it. Missing lists the nodes that could not be asked,
// so a screen can say its numbers are short.
type ClusterSummary struct {
	StatsSummary
	Missing []NodeNote
}

// GetClusterSummary returns the summary of the whole deployment. Only what the
// master owns is taken from a node: the clients it also has under that name,
// and the inbounds it replicates from that node. A node's own outbounds and
// whatever else exists only there stay private, as in the clients' online list.
// A node that cannot answer is left out and reported in Missing; the master's
// own numbers are always there.
func (s *StatsService) GetClusterSummary(since, bucket int64) ClusterSummary {
	local, err := s.GetSummary(since, bucket)
	if err != nil {
		logger.Warning("stats summary: ", err)
		local = &StatsSummary{Since: since, Bucket: bucket, Totals: []StatsTotal{}, Series: []StatsPoint{}}
	}
	out := ClusterSummary{StatsSummary: *local}

	db := database.GetDB()
	var nodes []model.Node
	if err := db.Where("enable = ?", true).Order("id").Find(&nodes).Error; err != nil || len(nodes) == 0 {
		return out
	}
	var names []string
	if err := db.Model(model.Client{}).Pluck("name", &names).Error; err != nil {
		logger.Warning("stats summary: ", err)
		return out
	}
	clients := make(map[string]bool, len(names))
	for _, n := range names {
		clients[n] = true
	}
	var replicas []struct {
		Tag    string
		NodeId uint
	}
	if err := db.Model(model.Inbound{}).Select("tag", "node_id").Where("node_id IS NOT NULL").Scan(&replicas).Error; err != nil {
		logger.Warning("stats summary: ", err)
		return out
	}
	owned := map[uint]map[string]bool{}
	for _, r := range replicas {
		if owned[r.NodeId] == nil {
			owned[r.NodeId] = map[string]bool{}
		}
		owned[r.NodeId][r.Tag] = true
	}

	type answer struct {
		sum  *StatsSummary
		note string
	}
	answers := make([]answer, len(nodes))
	var wg sync.WaitGroup
	sem := make(chan struct{}, nodeProbeParallel)
	for i := range nodes {
		node := nodes[i]
		if nodeKnownOffline(node.Id) {
			answers[i].note = NodeUnreachable
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(slot *answer) {
			defer wg.Done()
			defer func() { <-sem }()
			slot.sum, slot.note = askNodeSummary(&node, since, bucket)
		}(&answers[i])
	}
	wg.Wait()

	type key struct{ resource, tag string }
	merged := map[key]*StatsTotal{}
	for i := range out.Totals {
		t := out.Totals[i]
		merged[key{t.Resource, t.Tag}] = &t
	}
	type pointKey struct {
		tag string
		at  int64
	}
	points := map[pointKey]int64{}
	for _, p := range out.Series {
		points[pointKey{p.Tag, p.At}] += p.Traffic
	}
	for i, a := range answers {
		if a.note != "" {
			out.Missing = append(out.Missing, NodeNote{Name: nodes[i].Name, Reason: a.note})
			continue
		}
		mine := owned[nodes[i].Id]
		for _, t := range a.sum.Totals {
			switch t.Resource {
			case "user":
				if !clients[t.Tag] {
					continue
				}
			case "inbound":
				if !mine[t.Tag] {
					continue
				}
			default:
				continue
			}
			m := merged[key{t.Resource, t.Tag}]
			if m == nil {
				m = &StatsTotal{Resource: t.Resource, Tag: t.Tag}
				merged[key{t.Resource, t.Tag}] = m
			}
			m.Up += t.Up
			m.Down += t.Down
		}
		for _, p := range a.sum.Series {
			if mine[p.Tag] {
				points[pointKey{p.Tag, p.At}] += p.Traffic
			}
		}
	}
	out.Totals = out.Totals[:0]
	for _, t := range merged {
		out.Totals = append(out.Totals, *t)
	}
	out.Totals = busiest(out.Totals, len(merged))
	out.Series = out.Series[:0]
	for k, v := range points {
		out.Series = append(out.Series, StatsPoint{Tag: k.tag, At: k.at, Traffic: v})
	}
	sortSeries(out.Series)
	return out
}

// nodeKnownOffline tells whether the probe found the node down a moment ago,
// in which case asking it would only make the caller wait for the timeout.
func nodeKnownOffline(id uint) bool {
	nodeStatusMu.RLock()
	st, ok := nodeStatuses[id]
	nodeStatusMu.RUnlock()
	return ok && st.State == "offline" && time.Since(time.Unix(st.CheckedAt, 0)) < nodeSummarySkipOffline
}

// askNodeSummary fetches one node's statsTotals. The second result is empty on
// success, else the reason the node's numbers are missing.
func askNodeSummary(node *model.Node, since, bucket int64) (*StatsSummary, string) {
	client := nodePushClient(node)
	client.Timeout = nodeSummaryTimeout
	defer closeNodeIdle(client)
	q := url.Values{"since": {strconv.FormatInt(since, 10)}, "bucket": {strconv.FormatInt(bucket, 10)}}
	raw, err := (&NodeService{}).nodeGet(node, client, statsTotalsAction, q)
	if err != nil {
		if strings.Contains(err.Error(), "unknown action") {
			return nil, NodeNeedsUpdate
		}
		logger.Warning("stats summary: node ", node.Name, ": ", err)
		return nil, NodeUnreachable
	}
	var sum StatsSummary
	if err := json.Unmarshal(raw, &sum); err != nil {
		logger.Warning("stats summary: node ", node.Name, ": unexpected answer")
		return nil, NodeUnreachable
	}
	return &sum, ""
}
