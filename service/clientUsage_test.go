package service

import (
	"encoding/json"
	"path/filepath"
	"testing"

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
