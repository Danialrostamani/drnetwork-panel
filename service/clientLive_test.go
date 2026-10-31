package service

import (
	"encoding/json"
	"github.com/Danialrostamani/drnetwork-panel/core"
	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"path/filepath"
	"testing"
)

func TestLiveClientIncludesPendingTrafficWithoutWriting(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "live.db")); err != nil {
		t.Fatal(err)
	}
	c := model.Client{Name: "alice", Enable: true, Up: 100, Down: 200, Inbounds: json.RawMessage(`[]`), Links: json.RawMessage(`[]`), Config: json.RawMessage(`{}`)}
	if err := database.GetDB().Create(&c).Error; err != nil {
		t.Fatal(err)
	}
	statsMu.Lock()
	saved := pendingStats
	pendingStats = []model.Stats{{Resource: "user", Tag: "alice", Direction: true, Traffic: 3}, {Resource: "user", Tag: "alice", Traffic: 5}, {Resource: "user", Tag: "bob", Traffic: 999}, {Resource: "inbound", Tag: "alice", Traffic: 888}}
	statsMu.Unlock()
	t.Cleanup(func() { statsMu.Lock(); pendingStats = saved; statsMu.Unlock() })
	for range 3 {
		got, err := LiveClient("alice")
		if err != nil || got.Up != 103 || got.Down != 205 {
			t.Fatalf("snapshot %+v %v", got, err)
		}
	}
	got, err := readLiveClient("alice", core.NewSessionTracker())
	if err != nil || got.Up != 103 || got.Down != 205 {
		t.Fatal("core snapshot changed totals")
	}
	var stored model.Client
	database.GetDB().First(&stored, c.Id)
	if stored.Up != 100 || stored.Down != 200 {
		t.Fatal("live polling wrote statistics")
	}
	if _, err := LiveClient("missing"); err == nil {
		t.Fatal("missing client accepted")
	}
}
