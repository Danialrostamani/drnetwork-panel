package sub

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
)

// A Hysteria2 client that reaches the server through a realm must not also get
// one outbound per inbound address: the realm is how it finds the server, and
// sing-box refuses a realm outbound that names a server. (S-UI #1317.)
func TestGetOutboundsRealmIgnoresAddrs(t *testing.T) {
	inbound := &model.Inbound{
		Type:    "hysteria2",
		Tag:     "hy2",
		OutJson: json.RawMessage(`{"type": "hysteria2", "tag": "hy2", "realm": {"server_url": "https://realm.example.com", "realm_id": "home"}}`),
		Addrs:   json.RawMessage(`[{"server": "a.example.com", "server_port": 443}, {"server": "b.example.com", "server_port": 443}]`),
	}
	client := &model.Client{Name: "user", Config: json.RawMessage(`{"hysteria2": {"name": "user", "password": "secret"}}`)}

	outbounds, _, _, err := (&JsonService{}).getOutbounds(client, []*model.Inbound{inbound})
	if err != nil {
		t.Fatal(err)
	}
	if len(*outbounds) != 1 {
		t.Fatalf("got %d outbounds, want one realm outbound", len(*outbounds))
	}
	out := (*outbounds)[0]
	if _, ok := out["server"]; ok {
		t.Errorf("server conflicts with realm, got %v", out["server"])
	}
	if out["password"] != "secret" {
		t.Errorf("password = %v, want the client's", out["password"])
	}
}

// Clash has no hysteria realm, so a realm outbound is left out of the Clash
// profile rather than written without an address.
func TestClashSkipsRealmOutbounds(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "test.db")); err != nil {
		t.Fatal(err)
	}
	outbounds := []map[string]interface{}{
		{"type": "hysteria2", "tag": "via-realm", "password": "p", "realm": map[string]interface{}{"server_url": "https://realm.example.com"}},
		{"type": "hysteria2", "tag": "direct-hy2", "password": "p", "server": "a.example.com", "server_port": 443},
	}
	out, err := (&ClashService{}).ConvertToClashMeta(&outbounds, basicClashConfig)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "via-realm") || !strings.Contains(out, "direct-hy2") {
		t.Errorf("clash profile should hold direct-hy2 and not via-realm:\n%s", out)
	}
}
