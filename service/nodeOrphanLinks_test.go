package service

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"

	"gorm.io/gorm"
)

// TestNodeDeleteTakesItsLinks deletes a node and checks its links left every
// client, and that links an older deletion left behind are found and removed
// while a live node's links and the client's own links stay.
func TestNodeDeleteTakesItsLinks(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "orphans.db")); err != nil {
		t.Fatal(err)
	}
	db := database.GetDB()
	live := model.Node{Name: "DE", BaseUrl: "https://de.example:2095/app/", Token: "t1"}
	gone := model.Node{Name: "TR", BaseUrl: "https://tr.example:2095/app/", Token: "t2"}
	if err := db.Create(&live).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&gone).Error; err != nil {
		t.Fatal(err)
	}
	links := func(extra ...map[string]string) json.RawMessage {
		l := []map[string]string{
			{"remark": "in1", "type": "local", "uri": "vless://local"},
			{"remark": "[DE] in1", "type": "external", "uri": "vless://de"},
			{"remark": "[TR] in1", "type": "external", "uri": "vless://tr"},
			{"remark": "[OLD] in1", "type": "external", "uri": "vless://old"},
			{"remark": "mine", "type": "external", "uri": "vless://mine"},
		}
		raw, _ := json.Marshal(append(l, extra...))
		return raw
	}
	for _, name := range []string{"a", "b"} {
		c := model.Client{Enable: true, Name: name, Config: json.RawMessage(`{}`), Inbounds: json.RawMessage(`[]`), Links: links()}
		if err := db.Create(&c).Error; err != nil {
			t.Fatal(err)
		}
	}
	orph, err := OrphanNodeLinks()
	if err != nil || len(orph) != 1 || orph[0].Name != "OLD" || orph[0].Clients != 2 || orph[0].Links != 2 {
		t.Fatalf("orphans before delete: %+v %v", orph, err)
	}
	id, _ := json.Marshal(gone.Id)
	if err := db.Transaction(func(tx *gorm.DB) error { return new(NodeService).Save(tx, "del", id) }); err != nil {
		t.Fatal(err)
	}
	remarks := func() []string {
		var c model.Client
		db.Where("name = ?", "a").First(&c)
		var l []map[string]string
		_ = json.Unmarshal(c.Links, &l)
		out := []string{}
		for _, x := range l {
			out = append(out, x["remark"])
		}
		return out
	}
	if got := remarks(); len(got) != 4 || got[2] != "[OLD] in1" {
		t.Fatalf("after delete: %v", got)
	}
	n, err := RemoveOrphanNodeLinks([]string{"OLD", "DE"}, "admin")
	if err != nil || n != 2 {
		t.Fatalf("remove: %d %v", n, err)
	}
	if got := remarks(); len(got) != 3 || got[0] != "in1" || got[1] != "[DE] in1" || got[2] != "mine" {
		t.Fatalf("after remove: %v", got)
	}
	if orph, _ := OrphanNodeLinks(); len(orph) != 0 {
		t.Fatalf("orphans left: %+v", orph)
	}
}

// TestClientSaveMakesNodeLinks saves clients with a node's inbound and checks
// the node's links are there at once, before any sync, only for the clients
// the node's access serves, and follow a group changed in a bulk edit.
func TestClientSaveMakesNodeLinks(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "save-links.db")); err != nil {
		t.Fatal(err)
	}
	db := database.GetDB()
	node := model.Node{Name: "DE", Enable: true, BaseUrl: "https://de.example:2095", Token: "t", Access: model.NodeAccess{Groups: []string{"vip"}}}
	if err := db.Create(&node).Error; err != nil {
		t.Fatal(err)
	}
	replica := model.Inbound{Type: "vless", Tag: "rv", NodeId: &node.Id, Options: json.RawMessage(`{"listen_port":443}`), OutJson: json.RawMessage(`{"type":"vless","tag":"rv","server":"de.example","server_port":443}`), Addrs: json.RawMessage(`[]`)}
	if err := db.Create(&replica).Error; err != nil {
		t.Fatal(err)
	}
	inbounds, _ := json.Marshal([]uint{replica.Id})
	cs := new(ClientService)
	save := func(act string, v any) {
		t.Helper()
		raw, _ := json.Marshal(v)
		if err := db.Transaction(func(tx *gorm.DB) error { _, err := cs.Save(tx, act, raw, "panel.example"); return err }); err != nil {
			t.Fatal(err)
		}
	}
	cfg := json.RawMessage(`{"vless":{"name":"x","uuid":"11111111-1111-1111-1111-111111111111"}}`)
	save("new", model.Client{Enable: true, Name: "vip1", Group: "vip", Config: cfg, Inbounds: inbounds, Links: json.RawMessage(`[{"remark":"mine","type":"external","uri":"vless://mine"}]`)})
	save("new", model.Client{Enable: true, Name: "std1", Group: "std", Config: cfg, Inbounds: inbounds, Links: json.RawMessage(`[]`)})
	links := func(name string) ([]map[string]string, model.Client) {
		var c model.Client
		db.Where("name = ?", name).First(&c)
		var l []map[string]string
		_ = json.Unmarshal(c.Links, &l)
		return l, c
	}
	if l, _ := links("vip1"); len(l) != 2 || l[0]["remark"] != "mine" || l[1]["remark"] != "[DE] rv" || l[1]["type"] != "external" {
		t.Fatalf("vip1 links: %v", l)
	}
	l, std := links("std1")
	if len(l) != 0 {
		t.Fatalf("std1 got the node's links: %v", l)
	}
	std.Group = "vip"
	save("editbulk", []model.Client{std})
	if l, _ := links("std1"); len(l) != 1 || l[0]["remark"] != "[DE] rv" {
		t.Fatalf("std1 after regroup: %v", l)
	}
}
