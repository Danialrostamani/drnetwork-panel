package service

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
)

func clientConfigs(t *testing.T, name string) map[string]map[string]any {
	t.Helper()
	var c model.Client
	if err := database.GetDB().Where("name = ?", name).First(&c).Error; err != nil {
		t.Fatalf("client %s: %v", name, err)
	}
	var configs map[string]map[string]any
	if err := json.Unmarshal(c.Config, &configs); err != nil {
		t.Fatalf("client %s config %s: %v", name, c.Config, err)
	}
	return configs
}

// The fixture's clients have credentials for VLESS and Trojan only. Attached
// to the Shadowsocks and ShadowTLS inbounds as well, they get credentials for
// those, and keep the ones they had.
func TestAttachGivesMissingCredentials(t *testing.T) {
	db, _ := attachFixture(t)
	if _, err := (&ClientService{}).Save(db, "attachall", json.RawMessage(`{}`), "example.com"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"alice", "bob", "dave"} {
		configs := clientConfigs(t, name)
		for _, key := range []string{"shadowsocks", "shadowtls"} {
			if configs[key]["name"] != name || configs[key]["password"] == "" || configs[key]["password"] == nil {
				t.Fatalf("%s's %s credentials = %v", name, key, configs[key])
			}
		}
		if configs["vless"]["uuid"] != "11111111-1111-1111-1111-111111111111" || configs["trojan"]["password"] != "secret" {
			t.Fatalf("%s's own credentials changed: %v", name, configs)
		}
	}
	// A client the master pushed keeps the master's config.
	if configs := clientConfigs(t, "pushed"); configs["shadowsocks"] != nil {
		t.Fatalf("the pushed client got credentials of its own: %v", configs)
	}
}

// A client saved with fewer credentials than its inbounds need gets the rest,
// and an inbound's user list leaves out a client still without any instead of
// failing.
func TestSaveFillsMissingCredentials(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "credentials.db")); err != nil {
		t.Fatal(err)
	}
	db := database.GetDB()
	trojan := model.Inbound{Tag: "in-trojan", Type: "trojan", Addrs: json.RawMessage("[]"), OutJson: json.RawMessage("{}"), Options: json.RawMessage(`{"listen_port":443}`)}
	ss := model.Inbound{Tag: "in-ss", Type: "shadowsocks", Addrs: json.RawMessage("[]"), OutJson: json.RawMessage("{}"), Options: json.RawMessage(`{"listen_port":8388,"method":"2022-blake3-aes-128-gcm"}`)}
	for _, in := range []*model.Inbound{&trojan, &ss} {
		if err := db.Create(in).Error; err != nil {
			t.Fatal(err)
		}
	}
	inbounds, _ := json.Marshal([]uint{trojan.Id, ss.Id})
	data, _ := json.Marshal(map[string]any{
		"name": "omid", "enable": true, "inbounds": json.RawMessage(inbounds), "links": []any{},
		"config": map[string]any{"vless": map[string]any{"name": "omid", "uuid": "22222222-2222-2222-2222-222222222222"}},
	})
	if _, err := (&ClientService{}).Save(db, "new", data, "example.com"); err != nil {
		t.Fatal(err)
	}
	configs := clientConfigs(t, "omid")
	if configs["trojan"]["name"] != "omid" || configs["trojan"]["password"] == nil {
		t.Fatalf("trojan credentials = %v", configs["trojan"])
	}
	// The 2022 method reads the 16-byte key set.
	if configs["shadowsocks16"]["password"] == nil || configs["shadowsocks"] != nil {
		t.Fatalf("shadowsocks credentials = %v / %v", configs["shadowsocks16"], configs["shadowsocks"])
	}
	if configs["vless"]["uuid"] != "22222222-2222-2222-2222-222222222222" {
		t.Fatalf("vless credentials changed: %v", configs["vless"])
	}

	// Written past the service, a client without Trojan credentials is left
	// out of the inbound's users.
	bare := model.Client{Name: "bare", Enable: true, Inbounds: inbounds, Links: json.RawMessage("[]"), Config: json.RawMessage(`{"vless":{"name":"bare","uuid":"33333333-3333-3333-3333-333333333333"}}`)}
	if err := db.Create(&bare).Error; err != nil {
		t.Fatal(err)
	}
	out, err := (&InboundService{}).addUsers(db, []byte(`{"type":"trojan","tag":"in-trojan","listen_port":443}`), trojan.Id, "trojan")
	if err != nil {
		t.Fatal("trojan users: ", err)
	}
	var withUsers struct {
		Users []struct{ Name string } `json:"users"`
	}
	if err := json.Unmarshal(out, &withUsers); err != nil {
		t.Fatal(err)
	}
	if len(withUsers.Users) != 1 || withUsers.Users[0].Name != "omid" {
		t.Fatalf("trojan users = %+v, want omid only", withUsers.Users)
	}
}
