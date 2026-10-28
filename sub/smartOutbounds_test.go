package sub

import (
	"context"
	"encoding/json"
	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"path/filepath"
	"testing"

	"github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/include"
	"github.com/sagernet/sing-box/option"
	SJSON "github.com/sagernet/sing/common/json"
)

func TestSmartOutboundsActualNodeGroups(t *testing.T) {
	tags := []string{"local", "a-ws", "a-tcp", "b", "c", "backup"}
	outs := []map[string]interface{}{}
	for _, tag := range tags {
		outs = append(outs, map[string]interface{}{"type": "trojan", "tag": tag, "server": "127.0.0.1", "server_port": 443, "password": "test"})
	}
	(&JsonService{}).addDefaultOutbounds(&outs, &tags)
	nodes := map[string]string{"a-ws": "a", "a-tcp": "a", "b": "b", "c": "c", "backup": "b"}
	if err := addSmartOutbounds(&outs, &tags, nodes, "a", map[string]bool{"a": true, "b": true}); err != nil {
		t.Fatal(err)
	}
	byTag := map[string]map[string]interface{}{}
	for _, o := range outs {
		byTag[o["tag"].(string)] = o
	}
	auto, _ := json.Marshal(byTag["auto"]["outbounds"])
	if string(auto) != `["a-ws","a-tcp"]` {
		t.Fatalf("auto bypasses assigned node %s", auto)
	}
	if byTag["proxy"]["default"] != "auto" {
		t.Fatal("assigned node not selected by default")
	}
	if byTag["backup-2"] == nil {
		t.Fatal("backup tag collision")
	}
	back, _ := json.Marshal(byTag["backup-2"]["outbounds"])
	if string(back) != `["b","backup"]` {
		t.Fatalf("backup uses unavailable or wrong node %s", back)
	}
	// Parse the actual generated groups with sing-box's registered option types,
	// then instantiate the engine (without starting network listeners).
	raw, _ := json.Marshal(map[string]interface{}{"outbounds": outs, "route": map[string]interface{}{"final": "proxy"}})
	ctx := include.Context(context.Background())
	opts, err := SJSON.UnmarshalExtendedContext[option.Options](ctx, raw)
	if err != nil {
		t.Fatalf("sing-box rejects generated configuration: %v", err)
	}
	instance, err := box.New(box.Options{Options: opts, Context: ctx})
	if err != nil {
		t.Fatalf("sing-box rejects generated references: %v", err)
	}
	_ = instance.Close()
}
func TestSmartOutboundsMissingAssignedProtocolFails(t *testing.T) {
	tags := []string{"other"}
	outs := []map[string]interface{}{}
	if addSmartOutbounds(&outs, &tags, map[string]string{"other": "b"}, "a", nil) == nil {
		t.Fatal("missing assigned protocol silently fell back")
	}
}

// Bind generated, collision-renamed tags to authoritative node remarks, not
// a guess from a display tag. Local/manual names cannot become a managed node.
func TestSmartOutboundMetadataKeepsRenamedNodeTags(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "smart-json.db")); err != nil {
		t.Fatal(err)
	}
	db := database.GetDB()
	if err := db.Create(&model.Node{Name: "de", Enable: true}).Error; err != nil {
		t.Fatal(err)
	}
	c := model.Client{Id: 1, Name: "smart-json", Config: json.RawMessage(`{}`), Links: json.RawMessage(`[
      {"type":"external","remark":"[de] main","uri":"trojan://pw@one.example:443#proxy"},
      {"type":"external","remark":"[de] second","uri":"trojan://pw@two.example:443#proxy"},
      {"type":"external","remark":"manual","uri":"trojan://pw@other.example:443#manual"}
    ]`)}
	_, tags, nodes, err := (&JsonService{}).namedOutboundsDetailed(&c, nil, false, defaultOutboundTags...)
	if err != nil {
		t.Fatal(err)
	}
	if len(*tags) != 3 || (*tags)[0] != "proxy-2" || (*tags)[1] != "proxy-1" || nodes[(*tags)[0]] != "de" || nodes[(*tags)[1]] != "de" || nodes["manual"] != "" {
		t.Fatalf("tags %v nodes %v", tags, nodes)
	}
}
