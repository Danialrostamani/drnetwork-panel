package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
)

func TestAdoptInboundCreatesReadOnlyRemoteReplica(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "adopt.db")); err != nil {
		t.Fatal(err)
	}
	const token = "node-token"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Token") != token {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/app/apiv2/inbounds" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.URL.Query().Get("id") == "" {
			_, _ = w.Write([]byte(`{"success":true,"obj":{"inbounds":[{"id":7,"type":"vless","tag":"remote-vless"}]}}`))
			return
		}
		_, _ = w.Write([]byte(`{"success":true,"obj":{"inbounds":[{"id":7,"type":"vless","tag":"remote-vless","tls_id":4,"tls":{"enabled":true},"listen":"::","listen_port":443,"users":[{"name":"remote"}],"addrs":["node.example"],"out_json":{"server":"node.example","server_port":443}}]}}`))
	}))
	defer srv.Close()

	node := model.Node{Name: "node-a", Enable: true, BaseUrl: srv.URL, WebPath: "/app/", Token: token}
	if err := database.GetDB().Create(&node).Error; err != nil {
		t.Fatal(err)
	}
	svc := NodeSyncService{}
	before, err := svc.FetchNodeInbounds(node.Id)
	if err != nil || len(before) != 1 || before[0].Adopted {
		t.Fatalf("before adopt = %#v, %v", before, err)
	}
	if err := svc.AdoptInbounds(node.Id, []string{"remote-vless"}, "admin"); err != nil {
		t.Fatal(err)
	}

	var replica model.Inbound
	if err := database.GetDB().Where("tag = ?", "remote-vless").First(&replica).Error; err != nil {
		t.Fatal(err)
	}
	if replica.NodeId == nil || *replica.NodeId != node.Id || replica.TlsId != 0 {
		t.Fatalf("bad replica ownership/TLS: %#v", replica)
	}
	var options map[string]interface{}
	if err := json.Unmarshal(replica.Options, &options); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"id", "tls_id", "tls", "users", "node_id", "out_json", "addrs"} {
		if _, ok := options[forbidden]; ok {
			t.Fatalf("panel-only key %q leaked into replica options", forbidden)
		}
	}
	configs, err := (&InboundService{}).GetAllConfig(database.GetDB())
	if err != nil || len(configs) != 0 {
		t.Fatalf("replica entered local sing-box config: %s, %v", configs, err)
	}
	after, err := svc.FetchNodeInbounds(node.Id)
	if err != nil || !after[0].Adopted {
		t.Fatalf("after adopt = %#v, %v", after, err)
	}

	edit, _ := json.Marshal(map[string]interface{}{"id": replica.Id, "type": replica.Type, "tag": replica.Tag, "node_id": node.Id})
	tx := database.GetDB().Begin()
	if err := (&InboundService{}).Save(tx, "edit", edit, "", "master.example"); err == nil {
		tx.Rollback()
		t.Fatal("replica edit was accepted")
	} else {
		tx.Rollback()
	}

	nodePayload, _ := json.Marshal(node.Id)
	tx = database.GetDB().Begin()
	if err := (&NodeService{}).Save(tx, "del", nodePayload); err == nil {
		tx.Rollback()
		t.Fatal("node deletion with adopted replicas was accepted")
	} else {
		tx.Rollback()
	}

	deletePayload, _ := json.Marshal(replica.Tag)
	tx = database.GetDB().Begin()
	if err := (&InboundService{}).Save(tx, "del", deletePayload, "", "master.example"); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatal(err)
	}
}
