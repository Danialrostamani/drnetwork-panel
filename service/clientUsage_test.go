package service

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
)

// Per-server usage adds up within a cycle and starts over after a reset.
func TestClientServerUsageFollowsTheCycle(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "usage.db")); err != nil {
		t.Fatal(err)
	}
	db := database.GetDB()
	c := model.Client{Name: "u1", Enable: true, Inbounds: json.RawMessage(`[]`), Links: json.RawMessage(`[]`)}
	if err := db.Create(&c).Error; err != nil {
		t.Fatal(err)
	}
	for _, step := range []struct {
		node     uint
		up, down int64
	}{{0, 10, 100}, {0, 5, 50}, {3, 1, 2}} {
		if err := addClientUsage(db, "u1", step.node, step.up, step.down); err != nil {
			t.Fatal(err)
		}
	}
	if err := addClientUsage(db, "nobody", 0, 1, 1); err != nil {
		t.Fatal(err)
	}
	got := ClientServerUsage(c)
	if got[0].Up != 15 || got[0].Down != 150 || got[3].Up != 1 || got[3].Down != 2 || len(got) != 2 {
		t.Fatalf("usage = %+v", got)
	}
	// A reset folds up/down into the totals: the old rows no longer count.
	c.TotalUp, c.TotalDown = 15, 152
	if err := db.Model(&c).Updates(map[string]any{"total_up": 15, "total_down": 152, "up": 0, "down": 0}).Error; err != nil {
		t.Fatal(err)
	}
	if got := ClientServerUsage(c); len(got) != 0 {
		t.Fatalf("after reset = %+v", got)
	}
	if err := addClientUsage(db, "u1", 0, 7, 8); err != nil {
		t.Fatal(err)
	}
	if got := ClientServerUsage(c); got[0].Up != 7 || got[0].Down != 8 || len(got) != 1 {
		t.Fatalf("new cycle = %+v", got)
	}
	if err := db.Delete(&c).Error; err != nil {
		t.Fatal(err)
	}
	if err := DelStaleClientUsage(); err != nil {
		t.Fatal(err)
	}
	var n int64
	db.Model(model.ClientUsage{}).Count(&n)
	if n != 0 {
		t.Fatalf("rows left after delete = %d", n)
	}
}

// The newest traffic is the current cycle's: it is taken newest first until it
// adds up to what the client used, the last sample in part.
func TestAttributeRecentTakesTheNewestTraffic(t *testing.T) {
	samples := []usageSample{
		{at: 100, node: 2, up: 10, down: 90},
		{at: 300, node: 0, up: 5, down: 45},
		{at: 200, node: 1, up: 20, down: 180},
	}
	got, covered := attributeRecent(samples, 300)
	if !covered || got[0].Up+got[0].Down != 50 || got[1].Up+got[1].Down != 200 || got[2].Up+got[2].Down != 50 {
		t.Fatalf("got %+v covered=%v", got, covered)
	}
	if got[2].Up != 5 || got[2].Down != 45 {
		t.Fatalf("partial sample split = %+v", got[2])
	}
	if _, covered := attributeRecent(samples, 400<<20); covered {
		t.Fatal("history shorter than the usage counted as covering it")
	}
}

// A card reads what the client used on the master and on each node from their
// own traffic history, so traffic from before the per-server count is placed.
func TestClientLocationUsageReadsMasterAndNodeHistory(t *testing.T) {
	const m = int64(1 << 20)
	w := setUpUserChartWorld(t,
		map[string][]int64{"5": {100 * m, 1000 * m}},
		map[string][]int64{"0": {10 * m, 100 * m}})
	nodeStatusMu.Lock()
	nodeStatuses[w.nlNode.Id] = NodeStatus{State: "online"}
	nodeStatuses[w.deNode.Id] = NodeStatus{State: "online"}
	nodeStatusMu.Unlock()
	t.Cleanup(func() {
		nodeStatusMu.Lock()
		delete(nodeStatuses, w.nlNode.Id)
		delete(nodeStatuses, w.deNode.Id)
		nodeStatusMu.Unlock()
	})
	addClient(t, "ali", "local", "nl-a", "nl-b", "de-a")
	now := time.Now().Unix()
	addUserTraffic(t, "ali", now-60, 5*m, 50*m)
	db := database.GetDB()
	if err := db.Model(model.Client{}).Where("name = ?", "ali").Updates(map[string]any{"up": 115 * m, "down": 1100 * m, "created_at": now - 30*86400}).Error; err != nil {
		t.Fatal(err)
	}
	var c model.Client
	if err := db.Where("name = ?", "ali").First(&c).Error; err != nil {
		t.Fatal(err)
	}
	got := (&StatsService{}).ClientLocationUsage(c)
	if u := got[0]; u.Up != 5*m || u.Down != 50*m {
		t.Errorf("master = %+v", u)
	}
	if u := got[w.nlNode.Id]; u.Up != 100*m || u.Down != 1000*m {
		t.Errorf("nl = %+v", u)
	}
	if u := got[w.deNode.Id]; u.Up+u.Down != 60*m {
		t.Errorf("de = %+v (want the newest 60 MiB of it)", u)
	}
}
