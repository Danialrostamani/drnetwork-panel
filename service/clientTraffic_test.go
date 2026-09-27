package service

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
)

func TestGetTrafficSnapshotReturnsOnlyLiveCounters(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "traffic-snapshot.db")); err != nil {
		t.Fatal(err)
	}
	secret := json.RawMessage(`{"vless":{"uuid":"secret"}}`)
	if err := database.GetDB().Create(&model.Client{
		Name: "alice", Up: 123, Down: 456, OnlineAt: 789, Config: secret,
		Inbounds: json.RawMessage(`[]`), Links: json.RawMessage(`[]`),
	}).Error; err != nil {
		t.Fatal(err)
	}
	got, err := (&ClientService{}).GetTrafficSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if got["alice"] != (ClientTraffic{Up: 123, Down: 456, OnlineAt: 789}) {
		t.Fatalf("snapshot = %#v", got)
	}
}
