package sub

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
)

func TestJSONAndClashSourceExcludeNodeReplicas(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "subscription.db")); err != nil {
		t.Fatal(err)
	}
	local := model.Inbound{Type: "vless", Tag: "local", Options: json.RawMessage(`{"listen_port":443}`), OutJson: json.RawMessage(`{"type":"vless","tag":"local","server":"master.example","server_port":443}`), Addrs: json.RawMessage(`[]`)}
	if err := database.GetDB().Create(&local).Error; err != nil {
		t.Fatal(err)
	}
	node := model.Node{Name: "node-a", Enable: true, BaseUrl: "https://node.example", WebPath: "/app/", Token: "token"}
	if err := database.GetDB().Create(&node).Error; err != nil {
		t.Fatal(err)
	}
	replica := model.Inbound{Type: "vless", Tag: "remote", NodeId: &node.Id, Options: json.RawMessage(`{"listen_port":443}`), OutJson: json.RawMessage(`{"type":"vless","tag":"remote","server":"node.example","server_port":443}`), Addrs: json.RawMessage(`[]`)}
	if err := database.GetDB().Create(&replica).Error; err != nil {
		t.Fatal(err)
	}
	ids, _ := json.Marshal([]uint{local.Id, replica.Id})
	client := model.Client{Name: "alice", Enable: true, Config: json.RawMessage(`{"vless":{"uuid":"11111111-1111-1111-1111-111111111111"}}`), Inbounds: ids, Links: json.RawMessage(`[{"type":"external","remark":"[node-a] remote","uri":"vless://11111111-1111-1111-1111-111111111111@node.example:443?security=none#[node-a]%20remote"}]`)}
	if err := database.GetDB().Create(&client).Error; err != nil {
		t.Fatal(err)
	}

	gotClient, inbounds, err := (&JsonService{}).getData("alice")
	if err != nil {
		t.Fatal(err)
	}
	if gotClient.Id != client.Id || len(inbounds) != 1 || inbounds[0].Id != local.Id {
		t.Fatalf("subscription source duplicated replica: client=%d inbounds=%#v", gotClient.Id, inbounds)
	}
	ext, tags := (&LinkService{}).GetExternalOutbounds(&gotClient.Links)
	if len(ext) != 1 || len(tags) != 1 {
		t.Fatalf("node external link not available to JSON/Clash: outbounds=%#v tags=%#v", ext, tags)
	}
}
