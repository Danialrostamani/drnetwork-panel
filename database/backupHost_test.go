package database

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Danialrostamani/drnetwork-panel/database/model"
)

func TestReplaceHostRewritesListenURIAndLists(t *testing.T) {
	old, neu := "203.0.113.10", "198.51.100.20"
	cases := map[string]string{
		old:                                      neu,
		old + ":2095":                            neu + ":2095",
		"https://" + old + "/app/":               "https://" + neu + "/app/",
		"vless://id@" + old + ":443?type=ws#in1": "vless://id@" + neu + ":443?type=ws#in1",
		old + ", sub.example.com":                neu + ", sub.example.com",
		"example.com":                            "example.com",
		"203.0.113.100":                          "203.0.113.100",
	}
	for in, want := range cases {
		if got := replaceHost(in, old, neu); got != want {
			t.Errorf("replaceHost(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBackupDropsThisServersAddress(t *testing.T) {
	old, neu := "203.0.113.10", "198.51.100.20"
	prevIPs, prevHost := panelIPs, panelHost
	panelIPs = func() []string { return []string{old} }
	panelHost = func() string { return neu }
	t.Cleanup(func() { panelIPs, panelHost = prevIPs, prevHost })

	if err := InitDB(filepath.Join(t.TempDir(), "host.db")); err != nil {
		t.Fatal(err)
	}
	addrs, _ := json.Marshal([]map[string]interface{}{{"server": old, "server_port": 443, "remark": "-direct"}})
	out, _ := json.Marshal(map[string]interface{}{"type": "vless", "tag": "in-1", "server": old, "server_port": 443})
	opt, _ := json.Marshal(map[string]interface{}{"listen": old, "listen_port": 443})
	links, _ := json.Marshal([]map[string]string{{"type": "local", "remark": "in-1", "uri": "vless://id@" + old + ":443?type=ws#in-1"}})
	if err := db.Create(&model.Setting{Key: "subURI", Value: "https://" + old + ":2096/sub/"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Setting{Key: "webListen", Value: old + ":2095"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Inbound{Type: "vless", Tag: "in-ip", Addrs: addrs, OutJson: out, Options: opt}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Client{Name: "alice", Enable: true, Config: json.RawMessage(`{}`), Inbounds: json.RawMessage(`[]`), Links: links}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Node{Name: "other", Enable: true, BaseUrl: "https://" + old + ":2096", WebPath: "/app/", Token: "t"}).Error; err != nil {
		t.Fatal(err)
	}

	contents, err := GetDb("")
	if err != nil {
		t.Fatal(err)
	}
	restored := openBackup(t, contents)
	assertNoHost := func(label, got string) {
		t.Helper()
		if strings.Contains(got, old) {
			t.Errorf("%s still has this server's IP: %s", label, got)
		}
		if !strings.Contains(got, panelHostSentinel) {
			t.Errorf("%s: want sentinel, got %s", label, got)
		}
	}
	var sub0 model.Setting
	if err := restored.Where("key = ?", "subURI").First(&sub0).Error; err != nil {
		t.Fatal(err)
	}
	assertNoHost("subURI", sub0.Value)
	if err := fillPanelHost(restored); err != nil {
		t.Fatal(err)
	}

	var sub model.Setting
	if err := restored.Where("key = ?", "subURI").First(&sub).Error; err != nil {
		t.Fatal(err)
	}
	if sub.Value != "https://"+neu+":2096/sub/" {
		t.Errorf("subURI %q", sub.Value)
	}
	var listen model.Setting
	if err := restored.Where("key = ?", "webListen").First(&listen).Error; err != nil {
		t.Fatal(err)
	}
	if listen.Value != neu+":2095" {
		t.Errorf("webListen %q", listen.Value)
	}
	var in model.Inbound
	if err := restored.Where("tag = ?", "in-ip").First(&in).Error; err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(in.Addrs)+string(in.OutJson)+string(in.Options), old) || !strings.Contains(string(in.Addrs), neu) {
		t.Errorf("inbound addrs=%s out=%s opt=%s", in.Addrs, in.OutJson, in.Options)
	}
	var c model.Client
	if err := restored.Where("name = ?", "alice").First(&c).Error; err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(c.Links), old) || !strings.Contains(string(c.Links), neu) {
		t.Errorf("client links %s", c.Links)
	}
	var n model.Node
	if err := restored.Where("name = ?", "other").First(&n).Error; err != nil {
		t.Fatal(err)
	}
	if n.BaseUrl != "https://"+old+":2096" {
		t.Errorf("a node's address was rewritten: %s", n.BaseUrl)
	}
}
