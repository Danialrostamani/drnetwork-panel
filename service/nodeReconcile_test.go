package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
)

func TestReconcileCreatesUpdatesAndDeletesOnlyClusterClients(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "reconcile.db")); err != nil {
		t.Fatal(err)
	}
	type remoteClient struct {
		Id       uint            `json:"id"`
		Name     string          `json:"name"`
		Enable   bool            `json:"enable"`
		Config   json.RawMessage `json:"config"`
		Inbounds json.RawMessage `json:"inbounds"`
		Expiry   int64           `json:"expiry"`
		Group    string          `json:"group"`
	}
	var mu sync.Mutex
	remote := map[uint]remoteClient{9: {Id: 9, Name: "node-local", Enable: true, Config: json.RawMessage(`{}`), Inbounds: json.RawMessage(`[77]`), Group: "local"}}
	var nextID uint = 100
	var actions []string
	const token = "node-token"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Token") != token {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/app/apiv2/inbounds":
			_, _ = w.Write([]byte(`{"success":true,"obj":{"inbounds":[{"id":77,"type":"vless","tag":"remote-vless"}]}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/app/apiv2/clients":
			mu.Lock()
			clients := make([]remoteClient, 0, len(remote))
			for _, client := range remote {
				clients = append(clients, client)
			}
			mu.Unlock()
			obj, _ := json.Marshal(map[string]interface{}{"clients": clients})
			body, _ := json.Marshal(map[string]interface{}{"success": true, "obj": json.RawMessage(obj)})
			_, _ = w.Write(body)
		case r.Method == http.MethodPost && r.URL.Path == "/app/apiv2/save":
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			action := r.FormValue("action")
			actions = append(actions, action)
			mu.Lock()
			switch action {
			case "new":
				var client remoteClient
				_ = json.Unmarshal([]byte(r.FormValue("data")), &client)
				client.Id = nextID
				nextID++
				remote[client.Id] = client
			case "edit":
				var client remoteClient
				_ = json.Unmarshal([]byte(r.FormValue("data")), &client)
				remote[client.Id] = client
			case "del":
				id, _ := strconv.ParseUint(r.FormValue("data"), 10, 64)
				delete(remote, uint(id))
			}
			mu.Unlock()
			_, _ = w.Write([]byte(`{"success":true,"obj":{}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	node := model.Node{Name: "node-a", Enable: true, BaseUrl: srv.URL, WebPath: "/app/", Token: token}
	if err := database.GetDB().Create(&node).Error; err != nil {
		t.Fatal(err)
	}
	replica := model.Inbound{Type: "vless", Tag: "remote-vless", NodeId: &node.Id, Options: json.RawMessage(`{"listen_port":443}`)}
	if err := database.GetDB().Create(&replica).Error; err != nil {
		t.Fatal(err)
	}
	inbounds, _ := json.Marshal([]uint{replica.Id})
	alice := model.Client{Name: "alice", Enable: true, Config: json.RawMessage(`{"vless":{"uuid":"u-1","name":"alice"}}`), Inbounds: inbounds, Links: json.RawMessage(`[]`), Expiry: 12345}
	bob := model.Client{Name: "bob", Enable: true, Config: json.RawMessage(`{}`), Inbounds: json.RawMessage(`[]`), Links: json.RawMessage(`[]`)}
	if err := database.GetDB().Create(&alice).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().Create(&bob).Error; err != nil {
		t.Fatal(err)
	}

	svc := NodeSyncService{}
	svc.MarkAllDirty()
	if err := svc.ReconcileNow(node.Id); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	if len(remote) != 2 || remote[9].Name != "node-local" {
		t.Fatalf("local node client was touched: %#v", remote)
	}
	var cluster remoteClient
	for _, client := range remote {
		if client.Group == clusterGroup {
			cluster = client
		}
	}
	mu.Unlock()
	if cluster.Name != "alice" || string(cluster.Inbounds) != "[77]" || cluster.Expiry != alice.Expiry {
		t.Fatalf("bad pushed client: %#v", cluster)
	}

	if err := database.GetDB().Model(&alice).Updates(map[string]interface{}{"enable": false, "config": json.RawMessage(`{"vless":{"uuid":"u-2","name":"alice"}}`)}).Error; err != nil {
		t.Fatal(err)
	}
	svc.MarkAllDirty()
	if err := svc.ReconcileNow(node.Id); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	updated := remote[cluster.Id]
	mu.Unlock()
	if updated.Enable || string(updated.Config) != `{"vless":{"uuid":"u-2","name":"alice"}}` {
		t.Fatalf("cluster client not updated: %#v", updated)
	}

	if err := database.GetDB().Model(&alice).Update("inbounds", json.RawMessage(`[]`)).Error; err != nil {
		t.Fatal(err)
	}
	svc.MarkAllDirty()
	if err := svc.ReconcileNow(node.Id); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	_, clusterStillExists := remote[cluster.Id]
	_, localStillExists := remote[9]
	mu.Unlock()
	if clusterStillExists || !localStillExists {
		t.Fatalf("delete scope wrong: %#v", remote)
	}
	if got := actions; len(got) != 3 || got[0] != "new" || got[1] != "edit" || got[2] != "del" {
		t.Fatalf("actions = %v", got)
	}
	if err := database.GetDB().First(&node, node.Id).Error; err != nil {
		t.Fatal(err)
	}
	if node.Dirty || node.LastSync == 0 {
		t.Fatalf("node sync state not finalized: dirty=%v lastSync=%d", node.Dirty, node.LastSync)
	}
}
