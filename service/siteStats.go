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

// siteFlushEvery is how often the per-site traffic goes to the database: it
// is collected every stats cycle but written once a minute, into hour rows.
const siteFlushEvery = time.Minute

type siteBufKey struct {
	user, site string
	hour       int64
}

var (
	siteBufMu   sync.Mutex
	siteBuf     = map[siteBufKey]*[2]int64{}
	siteFlushed time.Time
)

// collectSiteStats takes the per-site counters off the core and writes them
// out once a minute. Only with traffic history on: it is history too, and
// goes with it (same retention, same backup switch).
func collectSiteStats(st *core.SessionTracker, enableTraffic bool) {
	stats := st.GetSiteStats()
	if !enableTraffic {
		return
	}
	now := time.Now()
	hour := now.Unix() - now.Unix()%3600
	siteBufMu.Lock()
	defer siteBufMu.Unlock()
	for _, s := range stats {
		k := siteBufKey{user: s.User, site: s.Site, hour: hour}
		v := siteBuf[k]
		if v == nil {
			v = &[2]int64{}
			siteBuf[k] = v
		}
		v[0] += s.Up
		v[1] += s.Down
	}
	if len(siteBuf) == 0 || now.Sub(siteFlushed) < siteFlushEvery {
		return
	}
	rows := make([]model.SiteStat, 0, len(siteBuf))
	for k, v := range siteBuf {
		rows = append(rows, model.SiteStat{User: k.user, Site: k.site, DateTime: k.hour, Up: v[0], Down: v[1]})
	}
	if err := saveSiteRows(database.GetDB(), rows); err != nil {
		// Kept for the next try.
		return
	}
	siteBuf = map[siteBufKey]*[2]int64{}
	siteFlushed = now
}

func saveSiteRows(db *gorm.DB, rows []model.SiteStat) error {
	return db.Transaction(func(tx *gorm.DB) error {
		for start := 0; start < len(rows); start += 200 {
			end := min(start+200, len(rows))
			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "user"}, {Name: "date_time"}, {Name: "site"}},
				DoUpdates: clause.Assignments(map[string]interface{}{
					"up":   gorm.Expr("site_stats.up + excluded.up"),
					"down": gorm.Expr("site_stats.down + excluded.down"),
				}),
			}).Create(rows[start:end]).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// SiteUsage is what a client moved to and from one site in a window.
type SiteUsage struct {
	Site string `json:"site"`
	Up   int64  `json:"up"`
	Down int64  `json:"down"`
}

// SiteUsageReport is a client's traffic per site in [Start, End], from the
// master and every node the client is on. Missing names the nodes that could
// not be asked, so a short list says why.
type SiteUsageReport struct {
	User    string      `json:"user"`
	Start   int64       `json:"start"`
	End     int64       `json:"end"`
	Sites   []SiteUsage `json:"sites"`
	Missing []string    `json:"missing,omitempty"`
}

// LocalSiteUsage is what this server's own core counted for the client.
func LocalSiteUsage(user string, start, end int64) ([]SiteUsage, error) {
	var rows []SiteUsage
	err := database.GetDB().Model(model.SiteStat{}).
		Select("site, SUM(up) AS up, SUM(down) AS down").
		Where("user = ? AND date_time >= ? AND date_time <= ?", user, start-start%3600, end).
		Group("site").Scan(&rows).Error
	return rows, err
}

// siteTimeout keeps the report from waiting long on a slow node.
const siteTimeout = 8 * time.Second

// GetSiteUsage is the client's traffic per site in [start, end], the master's
// and its nodes' added up, biggest first.
func (s *StatsService) GetSiteUsage(user string, start, end int64) (*SiteUsageReport, error) {
	if user == "" {
		return nil, common.NewError("no client")
	}
	if end <= 0 {
		end = time.Now().Unix()
	}
	if start <= 0 || start >= end {
		start = end - 86400
	}
	report := &SiteUsageReport{User: user, Start: start, End: end}
	local, err := LocalSiteUsage(user, start, end)
	if err != nil {
		return nil, err
	}
	total := map[string]*SiteUsage{}
	add := func(list []SiteUsage) {
		for _, u := range list {
			t := total[u.Site]
			if t == nil {
				t = &SiteUsage{Site: u.Site}
				total[u.Site] = t
			}
			t.Up += u.Up
			t.Down += u.Down
		}
	}
	add(local)

	db := database.GetDB()
	var client model.Client
	var nodes []model.Node
	if db.Select("inbounds").Where("name = ?", user).First(&client).Error == nil {
		var ids []uint
		_ = json.Unmarshal(client.Inbounds, &ids)
		var nodeIDs []uint
		if len(ids) > 0 && db.Model(model.Inbound{}).Distinct().Where("id IN ? AND node_id IS NOT NULL", ids).Pluck("node_id", &nodeIDs).Error == nil && len(nodeIDs) > 0 {
			_ = db.Where("id IN ?", nodeIDs).Order("id").Find(&nodes).Error
		}
	}
	q := url.Values{"user": {user}, "start": {strconv.FormatInt(start, 10)}, "end": {strconv.FormatInt(end, 10)}, "local": {"1"}}
	answers := make([][]SiteUsage, len(nodes))
	failed := make([]bool, len(nodes))
	var wg sync.WaitGroup
	for i := range nodes {
		if !nodes[i].Enable {
			failed[i] = true
			continue
		}
		wg.Add(1)
		go func(i int) {
			defer LogPanic("nodes: site stats")
			defer wg.Done()
			node := nodes[i]
			client := nodePushClient(&node)
			client.Timeout = siteTimeout
			defer closeNodeIdle(client)
			raw, err := (&NodeService{}).nodeGet(&node, client, "siteStats", q)
			if err == nil {
				var r SiteUsageReport
				if err = json.Unmarshal(raw, &r); err == nil {
					answers[i] = r.Sites
				}
			}
			failed[i] = err != nil
		}(i)
	}
	wg.Wait()
	for i := range nodes {
		if failed[i] {
			report.Missing = append(report.Missing, nodes[i].Name)
			continue
		}
		add(answers[i])
	}
	report.Sites = make([]SiteUsage, 0, len(total))
	for _, t := range total {
		report.Sites = append(report.Sites, *t)
	}
	sort.Slice(report.Sites, func(i, j int) bool {
		a, b := report.Sites[i], report.Sites[j]
		if a.Up+a.Down != b.Up+b.Down {
			return a.Up+a.Down > b.Up+b.Down
		}
		return a.Site < b.Site
	})
	return report, nil
}

// LocalSiteReport answers a master asking this node for its own part.
func LocalSiteReport(user string, start, end int64) (*SiteUsageReport, error) {
	sites, err := LocalSiteUsage(user, start, end)
	if err != nil {
		return nil, err
	}
	return &SiteUsageReport{User: user, Start: start, End: end, Sites: sites}, nil
}

// delOldSiteStats drops site rows past the retention window.
func delOldSiteStats(db *gorm.DB, before int64) error {
	for {
		res := db.Where("id IN (?)", db.Model(model.SiteStat{}).Select("id").Where("date_time < ?", before).Limit(delOldStatsChunk)).Delete(model.SiteStat{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected < delOldStatsChunk {
			return nil
		}
	}
}
