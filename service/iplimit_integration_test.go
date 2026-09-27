package service

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
)

func TestClientLimitIPRoundTripAndNodeProjection(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "limit-ip.db")); err != nil {
		t.Fatal(err)
	}
	db := database.GetDB()

	nodeID := uint(9)
	replica := model.Inbound{NodeId: &nodeID, Tag: "remote-in"}
	if err := db.Create(&replica).Error; err != nil {
		t.Fatal(err)
	}
	inbounds, _ := json.Marshal([]uint{replica.Id})
	client := model.Client{
		Name: "alice", Enable: true, LimitIp: 3,
		Config: json.RawMessage(`{}`), Inbounds: inbounds, Links: json.RawMessage(`[]`),
	}
	if err := db.Create(&client).Error; err != nil {
		t.Fatal(err)
	}

	listed, err := (&ClientService{}).GetAll()
	if err != nil || len(*listed) != 1 || (*listed)[0].LimitIp != 3 {
		t.Fatalf("client list lost limitIp: clients=%+v err=%v", listed, err)
	}
	payload, err := json.Marshal(*listed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (&ClientService{}).Save(db, "editbulk", payload, "example.com"); err != nil {
		t.Fatal(err)
	}
	var saved model.Client
	if err := db.First(&saved, client.Id).Error; err != nil {
		t.Fatal(err)
	}
	if saved.LimitIp != 3 {
		t.Fatalf("bulk round trip changed limitIp to %d", saved.LimitIp)
	}

	expected, err := (&NodeSyncService{}).expectedClients(nodeID, map[string]uint{"remote-in": 44})
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := expected["alice"]["limitIp"].(int); !ok || got != 3 {
		t.Fatalf("node projection limitIp = %#v, want 3", expected["alice"]["limitIp"])
	}
	currentLimit := 2
	current := nodeClientState{Enable: true, Expiry: saved.Expiry, Config: saved.Config, Inbounds: json.RawMessage(`[44]`), LimitIp: &currentLimit}
	if !clientDiffers(expected["alice"], current) {
		t.Fatal("node limitIp drift was not detected")
	}
	currentLimit = 3
	if clientDiffers(expected["alice"], current) {
		t.Fatal("equal node limitIp was reported as drift")
	}
}
