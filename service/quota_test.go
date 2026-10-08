package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"github.com/Danialrostamani/drnetwork-panel/core"
	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
)

func TestClientQuotas(t *testing.T) {
	clients := []model.Client{
		{Name: "alice", Volume: 1000, Up: 300, Down: 200},
		{Name: "bob", Volume: 0, Up: 100, Down: 300},
		{Name: "carol", Volume: 100, Up: 100, Down: 50},
		{Name: "dave", Volume: 0},
	}
	got := clientQuotas(clients, nil, nil)
	want := map[string]int64{"alice": 500, "carol": -50}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("own volumes: got %v, want %v", got, want)
	}

	// Traffic not committed yet is used all the same; on a node the master's
	// allowance applies too, whichever is less.
	pending := map[string]int64{"alice": 50}
	fromMaster := map[string]int64{"alice": 700, "bob": 900}
	got = clientQuotas(clients, pending, fromMaster)
	want = map[string]int64{"alice": 150, "bob": 500, "carol": -50}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("with the master's allowance: got %v, want %v", got, want)
	}
}

func TestRefreshQuotasHoldsClientsToTheirVolume(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "quota.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		quotaMu.Lock()
		nodeQuotas = nil
		quotaMu.Unlock()
	})
	for _, c := range []model.Client{
		{Name: "alice", Enable: true, Volume: 1000, Up: 300, Down: 200},
		{Name: "carol", Enable: true, Volume: 100, Up: 100, Down: 50},
		{Name: "erin", Enable: true, Up: 10, Down: 10},
	} {
		c.Inbounds = json.RawMessage(`[]`)
		c.Links = json.RawMessage(`[]`)
		if err := database.GetDB().Create(&c).Error; err != nil {
			t.Fatal(err)
		}
	}
	st := core.NewSessionTracker()
	quotaMu.Lock()
	refreshQuotasLocked(st)
	quotaMu.Unlock()
	if !st.Exhausted("carol") {
		t.Fatal("carol is past her volume and must be cut off")
	}
	if st.Exhausted("alice") || st.Exhausted("erin") {
		t.Fatal("alice and erin have volume left")
	}

	// The master allows erin 15 more bytes on this node than she used here.
	if err := ApplyNodeQuotas(map[string]int64{"erin": 20}); err != nil {
		t.Fatal(err)
	}
	quotaMu.Lock()
	refreshQuotasLocked(st)
	quotaMu.Unlock()
	if !st.Exhausted("erin") {
		t.Fatal("erin used all the master allows her here")
	}
	if err := ApplyNodeQuotas(map[string]int64{"erin": 1 << 30}); err != nil {
		t.Fatal(err)
	}
	quotaMu.Lock()
	refreshQuotasLocked(st)
	quotaMu.Unlock()
	if st.Exhausted("erin") {
		t.Fatal("erin was renewed on the master")
	}
	if err := ApplyNodeQuotas(map[string]int64{"": 1}); err == nil {
		t.Fatal("a quota for an empty name was accepted")
	}
}

func TestNodeTrafficPushesQuotas(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "push.db")); err != nil {
		t.Fatal(err)
	}
	const token = "node-token"
	var mu sync.Mutex
	var pushed map[string]int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Token") != token {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/app/apiv2/clients":
			body, _ := json.Marshal(map[string]interface{}{"success": true, "obj": map[string]interface{}{"clients": []map[string]interface{}{
				{"id": 10, "name": "alice", "group": clusterGroup, "enable": true, "config": map[string]interface{}{}, "inbounds": []uint{7}, "up": 100, "down": 200},
				{"id": 11, "name": "bob", "group": clusterGroup, "enable": true, "config": map[string]interface{}{}, "inbounds": []uint{7}, "up": 1, "down": 2},
				{"id": 12, "name": "node-local", "group": "local", "up": 999, "down": 999},
			}}})
			_, _ = w.Write(body)
		case "/app/apiv2/quota":
			var got map[string]int64
			if err := json.Unmarshal([]byte(r.PostFormValue("data")), &got); err != nil {
				t.Errorf("quota payload: %v", err)
			}
			mu.Lock()
			pushed = got
			mu.Unlock()
			_, _ = w.Write([]byte(`{"success":true}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	node := model.Node{Name: "node-a", Enable: true, BaseUrl: srv.URL, WebPath: "/app/", Token: token}
	if err := database.GetDB().Create(&node).Error; err != nil {
		t.Fatal(err)
	}
	for _, c := range []model.Client{
		{Name: "alice", Enable: true, Volume: 10000, Up: 5, Down: 6},
		{Name: "bob", Enable: true},
	} {
		c.Inbounds = json.RawMessage(`[]`)
		c.Links = json.RawMessage(`[]`)
		if err := database.GetDB().Create(&c).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := (&NodeSyncService{}).collectNodeTraffic(&node); err != nil {
		t.Fatal(err)
	}
	// alice now has 10000 - (105+206) left, on top of the 300 the node has
	// counted for her; bob has no volume, so no cap.
	mu.Lock()
	defer mu.Unlock()
	want := map[string]int64{"alice": 300 + 10000 - 311}
	if !reflect.DeepEqual(pushed, want) {
		t.Fatalf("pushed %v, want %v", pushed, want)
	}
}
