package service

import (
	"encoding/json"
	"net/url"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/core"
	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/util/common"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type onlines struct {
	Inbound  []string `json:"inbound,omitempty"`
	User     []string `json:"user,omitempty"`
	Outbound []string `json:"outbound,omitempty"`
}

var (
	// Guards both values below: SaveStats runs on the ten-second cron while
	// GetOnlines is read from a gin handler.
	statsMu         sync.Mutex
	onlineResources = &onlines{}

	// Traffic drained from the core that has not reached the database yet.
	// GetStats is destructive (it Swap(0)s every counter), so without this a
	// single SQLITE_BUSY loses the whole ten-second window for every user.
	pendingStats []model.Stats
)

type StatsService struct {
}

// SaveStats drains the core's counters into the database, then hands the core
// the quotas that follow from it. Both under quotaMu: the quotas count from
// the drain, so the database must have exactly the traffic up to it.
func (s *StatsService) SaveStats(enableTraffic bool, bucketSeconds int64) error {
	quotaMu.Lock()
	defer quotaMu.Unlock()
	st := liveSessionTracker()
	if st == nil {
		return nil
	}
	err := s.saveStats(st, enableTraffic, bucketSeconds)
	// Also after a failed commit: the traffic held back for the next cycle is
	// counted as used.
	refreshQuotasLocked(st)
	return err
}

func (s *StatsService) saveStats(st *core.SessionTracker, enableTraffic bool, bucketSeconds int64) error {
	drained := st.GetStats()

	statsMu.Lock()
	// Anything a previous cycle could not commit goes in ahead of this one.
	batch := append(pendingStats, (*drained)...)
	pendingStats = nil
	online := &onlines{}
	statsMu.Unlock()

	if len(batch) == 0 {
		statsMu.Lock()
		onlineResources = online
		statsMu.Unlock()
		return nil
	}

	var err error
	db := database.GetDB()
	tx := db.Begin()
	defer func() {
		if err == nil {
			if cErr := tx.Commit().Error; cErr != nil {
				err = cErr
			}
		} else {
			tx.Rollback()
		}
		statsMu.Lock()
		if err != nil {
			// Hold the drained traffic for the next cycle rather than dropping
			// it on the floor.
			pendingStats = batch
		} else {
			onlineResources = online
		}
		statsMu.Unlock()
	}()

	now := time.Now().Unix()

	// Aggregate per-resource so each active inbound/outbound/user is reported
	// online once (a tag may now appear in both directions), and each user's
	// up+down collapse into a single UPDATE.
	type traffic struct{ up, down int64 }
	userTraffic := map[string]*traffic{}
	seenInbound := map[string]bool{}
	seenOutbound := map[string]bool{}
	for _, stat := range batch {
		switch stat.Resource {
		case "inbound":
			if !seenInbound[stat.Tag] {
				seenInbound[stat.Tag] = true
				online.Inbound = append(online.Inbound, stat.Tag)
			}
		case "outbound":
			if !seenOutbound[stat.Tag] {
				seenOutbound[stat.Tag] = true
				online.Outbound = append(online.Outbound, stat.Tag)
			}
		case "user":
			t, ok := userTraffic[stat.Tag]
			if !ok {
				t = &traffic{}
				userTraffic[stat.Tag] = t
				online.User = append(online.User, stat.Tag)
			}
			if stat.Direction {
				t.up += stat.Traffic
			} else {
				t.down += stat.Traffic
			}
		}
	}

	for name, t := range userTraffic {
		update := map[string]interface{}{"online_at": now}
		if t.up > 0 {
			update["up"] = gorm.Expr("up + ?", t.up)
		}
		if t.down > 0 {
			update["down"] = gorm.Expr("down + ?", t.down)
		}
		err = tx.Model(model.Client{}).Where("name = ?", name).Updates(update).Error
		if err != nil {
			return err
		}
	}

	if !enableTraffic {
		return nil
	}

	// Round each sample down to its bucket and upsert, so all 10s cycles within
	// the same bucket accumulate into one row per (resource, tag, direction).
	if bucketSeconds < 1 {
		bucketSeconds = 1
	}
	bucket := now - (now % bucketSeconds)
	for i := range batch {
		batch[i].DateTime = bucket
	}
	err = tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "resource"}, {Name: "tag"}, {Name: "date_time"}, {Name: "direction"}},
		DoUpdates: clause.Assignments(map[string]interface{}{"traffic": gorm.Expr("stats.traffic + excluded.traffic")}),
	}).Create(&batch).Error
	return err
}

// nodeInboundStats answers the traffic chart of an inbound that lives on a
// managed node. The master only keeps a read-only replica of it and its core
// does not run it, so the master's own stats table has no samples for that tag
// (the chart was always empty). The node that does run the inbound holds them,
// so the request is forwarded there and its answer is returned as is -- it has
// the same shape as a local one. handled is false for every other inbound.
func (s *StatsService) nodeInboundStats(tag string, limit int, start, end int64) (obj any, handled bool, err error) {
	db := database.GetDB()
	var inbound model.Inbound
	if err := db.Select("id", "tag", "node_id").Where("tag = ? AND node_id IS NOT NULL", tag).Limit(1).Find(&inbound).Error; err != nil || inbound.Id == 0 || inbound.NodeId == nil {
		return nil, false, nil
	}
	var node model.Node
	if err := db.First(&node, *inbound.NodeId).Error; err != nil {
		return nil, true, common.NewError("node not found")
	}
	if !node.Enable {
		return nil, true, common.NewError("node is disabled")
	}
	q := url.Values{"resource": {"inbound"}, "tag": {tag}, "limit": {strconv.Itoa(limit)}}
	if start > 0 && end > start {
		q.Set("start", strconv.FormatInt(start, 10))
		q.Set("end", strconv.FormatInt(end, 10))
	}
	client := nodePushClient(&node)
	defer closeNodeIdle(client)
	raw, err := (&NodeService{}).nodeGet(&node, client, "stats", q)
	if err != nil {
		return nil, true, err
	}
	return raw, true, nil
}

// nodeStatsTimeout bounds how long one unreachable node can hold up a chart.
const nodeStatsTimeout = 8 * time.Second

// nodeUserStats gathers a client's traffic samples from the managed nodes that
// serve it. What a client moves through an inbound hosted on a node is counted
// by that node's core and never by the master's, so the master's own stats
// table only covers the inbounds its own core runs: the usage chart of anyone
// served by a node came out empty (or far too short). The nodes hold a mirror
// of the client under the same name, so every node that hosts one of its
// inbounds is asked for the same window and its buckets are returned as rows,
// ready to be folded in with the master's own.
//
// A node that cannot answer is skipped, so a chart still draws what is known.
// The error is only returned when nodes were asked and not one of them
// answered, so the caller can tell a chart that is empty from one that could
// not be fetched.
func (s *StatsService) nodeUserStats(name string, startTime, endTime int64) ([]model.Stats, error) {
	if endTime <= startTime {
		return nil, nil
	}
	db := database.GetDB()
	var clients []model.Client
	if err := db.Select("id", "inbounds").Where("name = ?", name).Find(&clients).Error; err != nil {
		return nil, err
	}
	var inboundIds []uint
	for _, client := range clients {
		var own []uint
		if json.Unmarshal(client.Inbounds, &own) == nil {
			inboundIds = common.UnionUintArray(inboundIds, own)
		}
	}
	if len(inboundIds) == 0 {
		return nil, nil
	}
	var nodeIds []uint
	if err := db.Model(model.Inbound{}).Distinct().Where("id IN ? AND node_id IS NOT NULL", inboundIds).Pluck("node_id", &nodeIds).Error; err != nil {
		return nil, err
	}
	if len(nodeIds) == 0 {
		return nil, nil
	}
	var nodes []model.Node
	if err := db.Where("id IN ?", nodeIds).Order("id").Find(&nodes).Error; err != nil {
		return nil, err
	}

	// The window is fixed here and sent as it is, so every node buckets the
	// same span; limit is only for a node too old to read start and end.
	q := url.Values{
		"resource": {"user"}, "tag": {name},
		"limit": {strconv.FormatInt((endTime-startTime+3599)/3600, 10)},
		"start": {strconv.FormatInt(startTime, 10)}, "end": {strconv.FormatInt(endTime, 10)},
	}
	type answer struct {
		rows []model.Stats
		err  error
	}
	answers := make([]answer, len(nodes))
	var wg sync.WaitGroup
	for i := range nodes {
		node := nodes[i]
		if !node.Enable {
			answers[i].err = common.NewErrorf("node %s is disabled", node.Name)
			continue
		}
		wg.Add(1)
		go func(slot *answer) {
			defer wg.Done()
			client := nodePushClient(&node)
			client.Timeout = nodeStatsTimeout
			defer closeNodeIdle(client)
			raw, err := (&NodeService{}).nodeGet(&node, client, "stats", q)
			if err == nil {
				slot.rows, err = nodeChartRows(raw, name, startTime, endTime)
			}
			if err != nil {
				slot.err = common.NewErrorf("node %s: %v", node.Name, err)
			}
		}(&answers[i])
	}
	wg.Wait()

	var rows []model.Stats
	var firstErr error
	answered := 0
	for _, a := range answers {
		if a.err != nil {
			if firstErr == nil {
				firstErr = a.err
			}
			continue
		}
		answered++
		rows = append(rows, a.rows...)
	}
	if answered == 0 {
		return nil, firstErr
	}
	return rows, nil
}

// nodeChartRows turns the answer of a node's api/stats back into samples: one
// per direction and bucket, dated at the middle of the bucket it came from, so
// they can be bucketed again with the master's own span.
func nodeChartRows(raw json.RawMessage, user string, startTime, endTime int64) ([]model.Stats, error) {
	var chart struct {
		Stats      map[string][]int64 `json:"stats"`
		StartTime  int64              `json:"startTime"`
		BucketSpan int64              `json:"bucketSpan"`
	}
	if err := json.Unmarshal(raw, &chart); err != nil {
		return nil, common.NewError("unexpected stats answer from node")
	}
	origin := chart.StartTime
	if origin == 0 {
		origin = startTime
	}
	span := chart.BucketSpan
	if span < 1 {
		span = 1
	}
	rows := make([]model.Stats, 0, 2*len(chart.Stats))
	for key, pair := range chart.Stats {
		idx, err := strconv.ParseInt(key, 10, 64)
		if err != nil || idx < 0 || idx > 1<<20 || len(pair) < 2 {
			continue
		}
		at := origin + idx*span + span/2
		if at <= startTime {
			at = startTime + 1
		}
		if at > endTime {
			at = endTime
		}
		if pair[0] > 0 {
			rows = append(rows, model.Stats{DateTime: at, Resource: "user", Tag: user, Direction: true, Traffic: pair[0]})
		}
		if pair[1] > 0 {
			rows = append(rows, model.Stats{DateTime: at, Resource: "user", Tag: user, Direction: false, Traffic: pair[1]})
		}
	}
	return rows, nil
}

func (s *StatsService) GetStats(resource string, tag string, limit int, start int64, end int64) (any, error) {
	var err error
	var result []model.Stats

	if resource == "inbound" && tag != "" {
		if obj, handled, nodeErr := s.nodeInboundStats(tag, limit, start, end); handled {
			return obj, nodeErr
		}
	}

	// Custom range when both start and end are provided, otherwise the last
	// `limit` hours up to now.
	var startTime, endTime int64
	if start > 0 && end > start {
		startTime, endTime = start, end
	} else {
		endTime = time.Now().Unix()
		startTime = endTime - (int64(limit) * 3600)
	}

	db := database.GetDB()
	resources := []string{resource}
	if resource == "endpoint" {
		resources = []string{"inbound", "outbound"}
	}
	err = db.Model(model.Stats{}).Where("resource in ? AND tag = ? AND date_time > ? AND date_time <= ?", resources, tag, startTime, endTime).Order("date_time ASC").Scan(&result).Error
	if err != nil {
		return nil, err
	}
	if resource == "user" && tag != "" {
		// Whatever the client moved through inbounds hosted on nodes was
		// counted there, not here.
		nodeRows, nodeErr := s.nodeUserStats(tag, startTime, endTime)
		if nodeErr != nil && len(result) == 0 {
			return nil, nodeErr
		}
		result = append(result, nodeRows...)
	}

	bucketSeconds, _ := (&SettingService{}).GetStatsBucketSeconds()
	if bucketSeconds < 1 {
		bucketSeconds = 1
	}
	numBuckets := 360
	if maxBuckets := (endTime - startTime) / bucketSeconds; maxBuckets < int64(numBuckets) {
		numBuckets = int(maxBuckets)
	}
	if numBuckets < 1 {
		numBuckets = 1
	}

	return s.downsampleStats(result, startTime, endTime, numBuckets), nil
}

func (s *StatsService) downsampleStats(stats []model.Stats, startTime, endTime int64, numBuckets int) any {
	result := make(map[int64][]int64)
	bucketSpan := (endTime - startTime) / int64(numBuckets)
	if bucketSpan == 0 {
		bucketSpan = 1
	}

	for _, r := range stats {
		bucket := (r.DateTime - startTime) / bucketSpan
		if bucket < 0 {
			bucket = 0
		}
		if bucket >= int64(numBuckets) {
			bucket = int64(numBuckets) - 1
		}
		if _, ok := result[bucket]; !ok {
			result[bucket] = []int64{0, 0}
		}
		if r.Direction {
			result[bucket][0] += r.Traffic
		} else {
			result[bucket][1] += r.Traffic
		}
	}

	return map[string]any{"stats": result, "startTime": startTime, "bucketSpan": bucketSpan, "numBuckets": numBuckets}
}

func (s *StatsService) GetOnlines() (onlines, error) {
	statsMu.Lock()
	defer statsMu.Unlock()
	// Copied: the caller must not hold slices the next cron tick replaces.
	return onlines{
		Inbound:  append([]string(nil), onlineResources.Inbound...),
		User:     append([]string(nil), onlineResources.User...),
		Outbound: append([]string(nil), onlineResources.Outbound...),
	}, nil
}

const nodeOnlineTTL = 20 * time.Second

// GetClusterOnlines merges recent node snapshots with the local online list.
// Only clients owned by this master are exposed; node-local users stay private.
func (s *StatsService) GetClusterOnlines() (onlines, error) {
	result, err := s.GetOnlines()
	if err != nil {
		return onlines{}, err
	}
	nodeStatusMu.RLock()
	now := time.Now().Unix()
	var remote onlines
	for _, status := range nodeStatuses {
		if status.State == "online" && status.onlineCheckedAt > 0 && now-status.onlineCheckedAt <= int64(nodeOnlineTTL.Seconds()) {
			remote.User = append(remote.User, status.onlineUsers...)
			remote.Inbound = append(remote.Inbound, status.onlineInbounds...)
			remote.Outbound = append(remote.Outbound, status.onlineOutbounds...)
		}
	}
	nodeStatusMu.RUnlock()
	if len(remote.User) == 0 && len(remote.Inbound) == 0 && len(remote.Outbound) == 0 {
		return result, nil
	}
	db := database.GetDB()
	var err2 error
	mergeKnown := func(local []string, remoteTags []string, model interface{}, column string) []string {
		if len(remoteTags) == 0 || err2 != nil {
			return local
		}
		sort.Strings(remoteTags)
		var names []string
		if err2 = db.Model(model).Pluck(column, &names).Error; err2 != nil {
			return local
		}
		known := make(map[string]bool, len(names))
		for _, name := range names {
			known[name] = true
		}
		merged := append([]string(nil), local...)
		seen := make(map[string]bool, len(merged))
		for _, name := range merged {
			seen[name] = true
		}
		for _, name := range remoteTags {
			if known[name] && !seen[name] {
				merged = append(merged, name)
				seen[name] = true
			}
		}
		return merged
	}
	// Only objects owned by this master are exposed; node-local users and
	// inbounds stay private.
	result.User = mergeKnown(result.User, remote.User, model.Client{}, "name")
	result.Inbound = mergeKnown(result.Inbound, remote.Inbound, model.Inbound{}, "tag")
	result.Outbound = mergeKnown(result.Outbound, remote.Outbound, model.Outbound{}, "tag")
	if err2 != nil {
		return onlines{}, err2
	}
	return result, nil
}

// GetSessions lists the live routed connections, narrowed to one user, inbound
// or outbound. Sorted newest first so the panel shows fresh connections on top.
func (s *StatsService) GetSessions(resource string, tag string) ([]core.SessionInfo, error) {
	if corePtr == nil || !corePtr.IsRunning() {
		return []core.SessionInfo{}, nil
	}
	box := corePtr.GetInstance()
	if box == nil {
		return []core.SessionInfo{}, nil
	}
	sessions := box.SessionTracker().Sessions()
	if tag != "" {
		var match func(core.SessionInfo) bool
		switch resource {
		case "user":
			match = func(session core.SessionInfo) bool { return session.User == tag }
		case "inbound":
			match = func(session core.SessionInfo) bool { return session.Inbound == tag }
		case "outbound":
			match = func(session core.SessionInfo) bool { return session.Outbound == tag }
		case "endpoint":
			// An endpoint can serve either side, so it is matched on both.
			match = func(session core.SessionInfo) bool {
				return session.Inbound == tag || session.Outbound == tag
			}
		default:
			return nil, common.NewError("unknown resource: ", resource)
		}
		filtered := sessions[:0]
		for _, session := range sessions {
			if match(session) {
				filtered = append(filtered, session)
			}
		}
		sessions = filtered
	}
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].CreatedAt > sessions[j].CreatedAt
	})
	return sessions, nil
}

// CloseUserSessions disconnects a user: every routed connection of theirs is
// closed, and so is every protocol-level session that has one of its own, which
// is what a multiplex or QUIC client would otherwise keep using. The user stays
// enabled, so nothing stops them from connecting again.
func (s *StatsService) CloseUserSessions(user string) error {
	if user == "" {
		return common.NewError("empty user name")
	}
	if corePtr == nil || !corePtr.IsRunning() {
		return common.NewError("core is not running")
	}
	box := corePtr.GetInstance()
	if box == nil {
		return common.NewError("core is not running")
	}
	box.SessionTracker().CloseByUser(user)
	corePtr.KickUserSessions(user)
	return nil
}

// delOldStatsChunk caps how many rows one DELETE removes, so the write lock is
// released between chunks.
const delOldStatsChunk = 5000

// DelOldStats drops stats older than the retention window, in bounded chunks.
// One unbounded DELETE held the write lock past the busy timeout, which made
// the daily cleanup itself a cause of lost traffic accounting.
func (s *StatsService) DelOldStats(days int) error {
	oldTime := time.Now().AddDate(0, 0, -(days)).Unix()
	db := database.GetDB()
	for {
		res := db.Where("id IN (?)",
			db.Model(model.Stats{}).Select("id").Where("date_time < ?", oldTime).Limit(delOldStatsChunk),
		).Delete(model.Stats{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected < delOldStatsChunk {
			return nil
		}
	}
}
