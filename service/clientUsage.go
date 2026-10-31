package service

import (
	"encoding/json"
	"net/url"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"

	"gorm.io/gorm"
)

// addClientUsage adds what a client used on one server (node 0 is the master)
// to its row of the current cycle. A row left from before the client's last
// reset starts over at this traffic: the reset moved up/down into the totals,
// so its epoch no longer matches.
func addClientUsage(tx *gorm.DB, name string, nodeID uint, up, down int64) error {
	if up <= 0 && down <= 0 {
		return nil
	}
	return tx.Exec(`INSERT INTO client_usages (client_id, node_id, epoch, up, down)
		SELECT id, ?, total_up + total_down, ?, ? FROM clients WHERE name = ?
		ON CONFLICT (client_id, node_id) DO UPDATE SET
			up = CASE WHEN client_usages.epoch = excluded.epoch THEN client_usages.up + excluded.up ELSE excluded.up END,
			down = CASE WHEN client_usages.epoch = excluded.epoch THEN client_usages.down + excluded.down ELSE excluded.down END,
			epoch = excluded.epoch`,
		nodeID, max(up, 0), max(down, 0), name).Error
}

// ServerUsage is what a client used on one server in its current cycle.
type ServerUsage struct {
	NodeId uint
	Up     int64
	Down   int64
}

// ClientServerUsage is what a client used on each server in its current cycle,
// by node ID (0 for the master). Rows from an earlier cycle are left out.
func ClientServerUsage(c model.Client) map[uint]ServerUsage {
	var rows []model.ClientUsage
	if err := database.GetDB().Where("client_id = ? AND epoch = ?", c.Id, c.TotalUp+c.TotalDown).Find(&rows).Error; err != nil {
		return nil
	}
	out := make(map[uint]ServerUsage, len(rows))
	for _, r := range rows {
		out[r.NodeId] = ServerUsage{NodeId: r.NodeId, Up: r.Up, Down: r.Down}
	}
	return out
}

// DelStaleClientUsage drops the rows of deleted clients.
func DelStaleClientUsage() error {
	return database.GetDB().Where("client_id NOT IN (?)", database.GetDB().Model(model.Client{}).Select("id")).Delete(&model.ClientUsage{}).Error
}

// usageSample is what a client moved on one server in one stats bucket.
type usageSample struct {
	at       int64
	node     uint
	up, down int64
}

// attributeRecent splits a client's current-cycle usage over its servers by
// the traffic history: the cycle's bytes are the newest ones, so the samples
// are taken newest first until they add up to used (the last one in part).
// covered tells whether the history held all of used.
func attributeRecent(samples []usageSample, used int64) (out map[uint]ServerUsage, covered bool) {
	sort.SliceStable(samples, func(i, j int) bool { return samples[i].at > samples[j].at })
	out = map[uint]ServerUsage{}
	left := used
	for _, s := range samples {
		if left <= 0 {
			break
		}
		total := s.up + s.down
		if total <= 0 {
			continue
		}
		up, down := s.up, s.down
		if total > left {
			up = int64(float64(s.up) * float64(left) / float64(total))
			down = left - up
		}
		u := out[s.node]
		u.NodeId = s.node
		u.Up += up
		u.Down += down
		out[s.node] = u
		left -= up + down
	}
	return out, left <= usageSlack
}

// usageSlack is how far the history may fall short of the counters and still
// count as all of it: the last seconds of traffic may not be in a bucket yet.
const usageSlack = 1 << 20

// locationHistoryDays is how far back the history is read.
const locationHistoryDays = 90

// locationTimeout keeps a card from waiting long on a slow node.
const locationTimeout = 4 * time.Second

type locationCacheEntry struct {
	at    time.Time
	used  int64
	usage map[uint]ServerUsage
}

var (
	locationCacheMu sync.Mutex
	locationCache   = map[uint]locationCacheEntry{}
)

// ClientLocationUsage is what a client used on each server (node ID, 0 for
// the master) in its current cycle. It reads the traffic history of the
// master and of each node the client is on, so the traffic from before the
// per-server count began is placed too; where that history is too short (or
// traffic history is off), the per-server count fills in.
func (s *StatsService) ClientLocationUsage(c model.Client) map[uint]ServerUsage {
	counted := ClientServerUsage(c)
	used := max(c.Up, 0) + max(c.Down, 0)
	if used == 0 {
		return counted
	}
	var sum int64
	for _, u := range counted {
		sum += u.Up + u.Down
	}
	if used-sum <= usageSlack {
		return counted
	}
	locationCacheMu.Lock()
	if e, ok := locationCache[c.Id]; ok && e.used == used && time.Since(e.at) < time.Minute {
		locationCacheMu.Unlock()
		return e.usage
	}
	locationCacheMu.Unlock()

	now := time.Now().Unix()
	since := now - locationHistoryDays*86400
	if c.CreatedAt > since {
		since = c.CreatedAt - 3600
	}
	samples := s.locationSamples(c, since, now)
	hist, covered := attributeRecent(samples, used)
	out := hist
	if !covered {
		out = map[uint]ServerUsage{}
		for id, u := range hist {
			out[id] = u
		}
		for id, u := range counted {
			if h := out[id]; u.Up+u.Down > h.Up+h.Down {
				out[id] = u
			}
		}
	}
	locationCacheMu.Lock()
	locationCache[c.Id] = locationCacheEntry{at: time.Now(), used: used, usage: out}
	locationCacheMu.Unlock()
	return out
}

// locationSamples is the client's traffic history in [since, now]: the
// master's own stats and those of every online node it is on.
func (s *StatsService) locationSamples(c model.Client, since, now int64) []usageSample {
	db := database.GetDB()
	var local []model.Stats
	_ = db.Where("resource = ? AND tag = ? AND date_time >= ?", "user", c.Name, since).Find(&local).Error
	samples := statsSamples(local, 0)

	var ids []uint
	_ = json.Unmarshal(c.Inbounds, &ids)
	if len(ids) == 0 {
		return samples
	}
	var nodeIDs []uint
	if err := db.Model(model.Inbound{}).Distinct().Where("id IN ? AND node_id IS NOT NULL", ids).Pluck("node_id", &nodeIDs).Error; err != nil || len(nodeIDs) == 0 {
		return samples
	}
	var nodes []model.Node
	if err := db.Where("id IN ? AND enable = ?", nodeIDs, true).Find(&nodes).Error; err != nil {
		return samples
	}
	statuses := (&NodeService{}).GetStatuses()
	q := url.Values{
		"resource": {"user"}, "tag": {c.Name},
		"limit": {strconv.FormatInt((now-since+3599)/3600, 10)},
		"start": {strconv.FormatInt(since, 10)}, "end": {strconv.FormatInt(now, 10)},
	}
	answers := make([][]usageSample, len(nodes))
	var wg sync.WaitGroup
	for i := range nodes {
		if st, ok := statuses[nodes[i].Id]; !ok || st.State != "online" {
			continue
		}
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			node := nodes[i]
			client := nodePushClient(&node)
			client.Timeout = locationTimeout
			defer closeNodeIdle(client)
			raw, err := (&NodeService{}).nodeGet(&node, client, "stats", q)
			if err != nil {
				return
			}
			rows, err := nodeChartRows(raw, c.Name, since, now)
			if err == nil {
				answers[i] = statsSamples(rows, node.Id)
			}
		}(i)
	}
	wg.Wait()
	for _, a := range answers {
		samples = append(samples, a...)
	}
	return samples
}

// statsSamples folds stats rows (one per direction) into samples of a server.
func statsSamples(rows []model.Stats, node uint) []usageSample {
	byAt := map[int64]*usageSample{}
	var out []usageSample
	for _, r := range rows {
		if r.Traffic <= 0 {
			continue
		}
		s, ok := byAt[r.DateTime]
		if !ok {
			s = &usageSample{at: r.DateTime, node: node}
			byAt[r.DateTime] = s
		}
		if r.Direction {
			s.up += r.Traffic
		} else {
			s.down += r.Traffic
		}
	}
	for _, s := range byAt {
		out = append(out, *s)
	}
	return out
}
