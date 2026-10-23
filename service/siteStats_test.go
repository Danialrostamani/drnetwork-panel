package service

import (
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
)

// A chart bucket is a whole number of stored buckets and starts on one, so a
// steady flow is drawn flat.
func TestChartWindowLaysBucketsOverStoredOnes(t *testing.T) {
	for _, tc := range []struct{ start, end, bucket int64 }{
		{1000, 1000 + 3600, 60},
		{1000017, 1000017 + 86400*3 + 1234, 60},
		{5, 5 + 86400*30, 60},
		{7, 7 + 600, 300},
	} {
		start, end, span, n := chartWindow(tc.start, tc.end, tc.bucket)
		if start%tc.bucket != 0 || start > tc.start || span%tc.bucket != 0 || n > maxChartBuckets || end < tc.end || end != start+int64(n)*span {
			t.Errorf("%+v: %d..%d span %d × %d", tc, start, end, span, n)
		}
	}
	// One stored row a minute for a day: every chart bucket gets the same.
	var rows []model.Stats
	start, end, span, n := chartWindow(1000017, 1000017+86400, 60)
	for at := start; at < end; at += 60 {
		rows = append(rows, model.Stats{DateTime: at, Resource: "user", Tag: "a", Direction: false, Traffic: 10})
	}
	got := (&StatsService{}).downsampleStats(rows, start, span, n).(map[string]any)["stats"].(map[int64][]int64)
	want := 10 * span / 60
	for i := int64(0); i < int64(n); i++ {
		if got[i][1] != want {
			t.Fatalf("bucket %d = %d, want %d (all equal)", i, got[i][1], want)
		}
	}
}

// Site traffic is written per hour and added up per site over a window.
func TestSiteUsageAddsUpPerSite(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "sites.db")); err != nil {
		t.Fatal(err)
	}
	db := database.GetDB()
	now := time.Now().Unix()
	hour := now - now%3600
	rows := []model.SiteStat{
		{User: "ali", DateTime: hour, Site: "google.com", Up: 10, Down: 100},
		{User: "ali", DateTime: hour - 3600, Site: "google.com", Up: 1, Down: 2},
		{User: "ali", DateTime: hour, Site: "instagram.com", Up: 5, Down: 500},
		{User: "ali", DateTime: hour - 10*86400, Site: "old.com", Up: 1, Down: 1},
		{User: "bob", DateTime: hour, Site: "google.com", Up: 9, Down: 9},
	}
	if err := saveSiteRows(db, rows); err != nil {
		t.Fatal(err)
	}
	// Writing the same hour again adds to it.
	if err := saveSiteRows(db, []model.SiteStat{{User: "ali", DateTime: hour, Site: "google.com", Up: 1, Down: 1}}); err != nil {
		t.Fatal(err)
	}
	r, err := (&StatsService{}).GetSiteUsage("ali", now-86400, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Sites) != 2 || r.Sites[0].Site != "instagram.com" || r.Sites[1].Site != "google.com" || r.Sites[1].Up != 12 || r.Sites[1].Down != 103 {
		t.Fatalf("sites = %+v", r.Sites)
	}
	if err := delOldSiteStats(db, now-86400*5); err != nil {
		t.Fatal(err)
	}
	var n int64
	db.Model(model.SiteStat{}).Where("site = ?", "old.com").Count(&n)
	if n != 0 {
		t.Fatal("old site row kept")
	}
}

// The master adds up what each node counted for the client.
func TestSiteUsageAsksTheNodes(t *testing.T) {
	w := setUpUserChartWorld(t, nil, nil)
	addClient(t, "ali", "local", "nl-a")
	w.nl.srv.Config.Handler = siteNodeHandler(t, `{"user":"ali","sites":[{"site":"google.com","up":1,"down":2},{"site":"x.com","up":3,"down":4}]}`)
	now := time.Now().Unix()
	if err := saveSiteRows(database.GetDB(), []model.SiteStat{{User: "ali", DateTime: now - now%3600, Site: "google.com", Up: 10, Down: 20}}); err != nil {
		t.Fatal(err)
	}
	r, err := (&StatsService{}).GetSiteUsage("ali", now-3600, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Sites) != 2 || r.Sites[0].Site != "google.com" || r.Sites[0].Up != 11 || r.Sites[0].Down != 22 || r.Sites[1].Site != "x.com" || len(r.Missing) != 0 {
		t.Fatalf("report = %+v", r)
	}
}

// siteNodeHandler is a node's api/siteStats answering body for its own part.
func siteNodeHandler(t *testing.T, body string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Token") != fakeNodeToken || r.URL.Path != "/app/apiv2/siteStats" || r.URL.Query().Get("local") != "1" || r.URL.Query().Get("user") != "ali" {
			t.Errorf("node asked %s %v", r.URL.Path, r.URL.Query())
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"success":true,"obj":` + body + `}`))
	})
}
