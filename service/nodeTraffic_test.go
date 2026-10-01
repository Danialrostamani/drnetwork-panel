package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
)

func TestNodeTrafficUsesPersistentDeltaBaselinesAndHandlesReset(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "traffic.db")); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	up, down := int64(100), int64(200)
	const token = "node-token"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Token") != token || r.URL.Path != "/app/apiv2/clients" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		mu.Lock()
		body, _ := json.Marshal(map[string]interface{}{"success": true, "obj": map[string]interface{}{"clients": []map[string]interface{}{
			{"id": 10, "name": "alice", "group": clusterGroup, "enable": true, "config": map[string]interface{}{}, "inbounds": []uint{7}, "up": up, "down": down},
			{"id": 11, "name": "node-local", "group": "local", "up": 999, "down": 999},
		}}})
		mu.Unlock()
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	node := model.Node{Name: "node-a", Enable: true, BaseUrl: srv.URL, WebPath: "/app/", Token: token}
	alice := model.Client{Name: "alice", Enable: true, Inbounds: json.RawMessage(`[]`), Links: json.RawMessage(`[]`), Up: 5, Down: 6}
	if err := database.GetDB().Create(&node).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().Create(&alice).Error; err != nil {
		t.Fatal(err)
	}
	svc := NodeSyncService{}
	if err := svc.collectNodeTraffic(&node); err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().First(&alice, alice.Id).Error; err != nil {
		t.Fatal(err)
	}
	if alice.Up != 105 || alice.Down != 206 {
		t.Fatalf("first collection = %d/%d", alice.Up, alice.Down)
	}
	if alice.OnlineAt == 0 || time.Now().Unix()-alice.OnlineAt > 5 {
		t.Fatalf("node traffic did not refresh onlineAt: %d", alice.OnlineAt)
	}

	mu.Lock()
	up, down = 150, 260
	mu.Unlock()
	if err := database.GetDB().First(&node, node.Id).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.collectNodeTraffic(&node); err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().First(&alice, alice.Id).Error; err != nil {
		t.Fatal(err)
	}
	if alice.Up != 155 || alice.Down != 266 {
		t.Fatalf("delta collection = %d/%d", alice.Up, alice.Down)
	}

	mu.Lock()
	up, down = 10, 20 // node-side reset: rebase and count only the new values
	mu.Unlock()
	if err := database.GetDB().First(&node, node.Id).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.collectNodeTraffic(&node); err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().First(&alice, alice.Id).Error; err != nil {
		t.Fatal(err)
	}
	if alice.Up != 165 || alice.Down != 286 {
		t.Fatalf("reset collection = %d/%d", alice.Up, alice.Down)
	}
	if err := database.GetDB().First(&node, node.Id).Error; err != nil {
		t.Fatal(err)
	}
	var baseline map[string]trafficBaseline
	if err := json.Unmarshal(node.Baselines, &baseline); err != nil || baseline["alice"].Up != 10 || baseline["alice"].Down != 20 {
		t.Fatalf("baseline = %#v, %v", baseline, err)
	}
}

func TestClusterOnlinesMergesKnownFreshNodeUsersOnly(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "online.db")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"local", "alice"} {
		if err := database.GetDB().Create(&model.Client{Name: name, Inbounds: json.RawMessage(`[]`), Links: json.RawMessage(`[]`)}).Error; err != nil {
			t.Fatal(err)
		}
	}
	statsMu.Lock()
	previousOnline := onlineResources
	onlineResources = &onlines{User: []string{"local"}}
	statsMu.Unlock()
	nodeStatusMu.Lock()
	previousStatuses := nodeStatuses
	nodeStatuses = map[uint]NodeStatus{
		1: {State: "online", onlineUsers: []string{"alice", "local", "stranger", "alice"}, onlineCheckedAt: time.Now().Unix()},
		2: {State: "online", onlineUsers: []string{"stale"}, onlineCheckedAt: time.Now().Add(-nodeOnlineTTL - time.Second).Unix()},
	}
	nodeStatusMu.Unlock()
	t.Cleanup(func() {
		statsMu.Lock()
		onlineResources = previousOnline
		statsMu.Unlock()
		nodeStatusMu.Lock()
		nodeStatuses = previousStatuses
		nodeStatusMu.Unlock()
	})
	got, err := (&StatsService{}).GetClusterOnlines()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.User, []string{"local", "alice"}) {
		t.Fatalf("cluster online users = %v", got.User)
	}
	localOnly, err := (&StatsService{}).GetOnlines()
	if err != nil || !reflect.DeepEqual(localOnly.User, []string{"local"}) {
		t.Fatalf("node-facing local online list changed: %v, %v", localOnly.User, err)
	}
}

func TestNodeReportedOnlineUserRefreshesOnlineAtWithoutTraffic(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "onlineat.db")); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := json.Marshal(map[string]interface{}{"success": true, "obj": map[string]interface{}{"clients": []map[string]interface{}{
			{"id": 10, "name": "alice", "group": clusterGroup, "up": 0, "down": 0},
		}}})
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	node := model.Node{Name: "n", Enable: true, BaseUrl: srv.URL, WebPath: "/app/", Token: "t"}
	alice := model.Client{Name: "alice", Enable: true, Inbounds: json.RawMessage(`[]`), Links: json.RawMessage(`[]`)}
	bob := model.Client{Name: "bob", Enable: true, Inbounds: json.RawMessage(`[]`), Links: json.RawMessage(`[]`)}
	for _, v := range []interface{}{&node, &alice, &bob} {
		if err := database.GetDB().Create(v).Error; err != nil {
			t.Fatal(err)
		}
	}
	nodeStatusMu.Lock()
	previous := nodeStatuses
	nodeStatuses = map[uint]NodeStatus{node.Id: {State: "online", onlineUsers: []string{"alice", "stranger"}, onlineCheckedAt: time.Now().Unix()}}
	nodeStatusMu.Unlock()
	defer func() {
		nodeStatusMu.Lock()
		nodeStatuses = previous
		nodeStatusMu.Unlock()
	}()
	if err := (&NodeSyncService{}).collectNodeTraffic(&node); err != nil {
		t.Fatal(err)
	}
	database.GetDB().First(&alice, alice.Id)
	database.GetDB().First(&bob, bob.Id)
	if alice.OnlineAt == 0 {
		t.Fatal("alice reported online by node but onlineAt not updated")
	}
	if bob.OnlineAt != 0 {
		t.Fatalf("bob was not online but onlineAt = %d", bob.OnlineAt)
	}
}

func TestClusterOnlinesMergesKnownInboundsFromNodes(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "inb.db")); err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().Create(&model.Inbound{Tag: "hy2-nl", Type: "hysteria2", Options: json.RawMessage(`{}`)}).Error; err != nil {
		t.Fatal(err)
	}
	previousOnline := onlineResources
	onlineResources = &onlines{}
	nodeStatusMu.Lock()
	previous := nodeStatuses
	nodeStatuses = map[uint]NodeStatus{1: {State: "online", onlineInbounds: []string{"hy2-nl", "node-private"}, onlineCheckedAt: time.Now().Unix()}}
	nodeStatusMu.Unlock()
	defer func() {
		onlineResources = previousOnline
		nodeStatusMu.Lock()
		nodeStatuses = previous
		nodeStatusMu.Unlock()
	}()
	got, err := (&StatsService{}).GetClusterOnlines()
	if err != nil || !reflect.DeepEqual(got.Inbound, []string{"hy2-nl"}) {
		t.Fatalf("cluster online inbounds = %v, %v", got.Inbound, err)
	}
}
