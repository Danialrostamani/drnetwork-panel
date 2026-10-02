package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
)

// The traffic chart of an inbound hosted on a node has to come from that node:
// the master only keeps a replica of the inbound and never sees its traffic.
func TestInboundTrafficChartOfANodeInboundIsServedByTheNode(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "nodestats.db")); err != nil {
		t.Fatal(err)
	}
	const token = "node-token"
	var mu sync.Mutex
	var queries []map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Token") != token || r.URL.Path != "/app/apiv2/stats" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		q := map[string]string{}
		for k := range r.URL.Query() {
			q[k] = r.URL.Query().Get(k)
		}
		mu.Lock()
		queries = append(queries, q)
		mu.Unlock()
		body, _ := json.Marshal(map[string]interface{}{"success": true, "obj": map[string]interface{}{
			"stats": map[string][]int64{"3": {111, 222}}, "startTime": 1000, "bucketSpan": 10, "numBuckets": 6,
		}})
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	db := database.GetDB()
	node := model.Node{Name: "nl", Enable: true, BaseUrl: srv.URL, WebPath: "/app/", Token: token}
	if err := db.Create(&node).Error; err != nil {
		t.Fatal(err)
	}
	replica := model.Inbound{Type: "hysteria2", Tag: "nl-hy2", NodeId: &node.Id, Options: json.RawMessage(`{"listen_port":443}`)}
	local := model.Inbound{Type: "vless", Tag: "local-vless", Options: json.RawMessage(`{"listen_port":8443}`)}
	if err := db.Create(&replica).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&local).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	if err := db.Create(&[]model.Stats{
		{DateTime: now - 60, Resource: "inbound", Tag: "local-vless", Direction: true, Traffic: 500},
		{DateTime: now - 60, Resource: "inbound", Tag: "local-vless", Direction: false, Traffic: 700},
	}).Error; err != nil {
		t.Fatal(err)
	}

	svc := &StatsService{}
	got, err := svc.GetStats("inbound", "nl-hy2", 24, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(got)
	var chart struct {
		Stats      map[string][]int64 `json:"stats"`
		NumBuckets int                `json:"numBuckets"`
	}
	if err := json.Unmarshal(raw, &chart); err != nil {
		t.Fatal(err)
	}
	if chart.NumBuckets != 6 || len(chart.Stats["3"]) != 2 || chart.Stats["3"][0] != 111 || chart.Stats["3"][1] != 222 {
		t.Fatalf("node chart = %s", raw)
	}
	mu.Lock()
	first := queries[len(queries)-1]
	mu.Unlock()
	if first["resource"] != "inbound" || first["tag"] != "nl-hy2" || first["limit"] != "24" || first["start"] != "" || first["end"] != "" {
		t.Fatalf("forwarded query = %v", first)
	}

	// A custom range is forwarded too.
	if _, err := svc.GetStats("inbound", "nl-hy2", 0, now-3600, now); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	custom := queries[len(queries)-1]
	mu.Unlock()
	if custom["start"] == "" || custom["end"] == "" {
		t.Fatalf("custom range was not forwarded: %v", custom)
	}

	// An inbound that runs on the master keeps using the master's own samples
	// and never calls the node.
	mu.Lock()
	before := len(queries)
	mu.Unlock()
	got, err = svc.GetStats("inbound", "local-vless", 24, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(got)
	var local2 struct {
		Stats map[string][]int64 `json:"stats"`
	}
	_ = json.Unmarshal(raw, &local2)
	var up, down int64
	for _, pair := range local2.Stats {
		up += pair[0]
		down += pair[1]
	}
	if up != 500 || down != 700 {
		t.Fatalf("local chart = %s", raw)
	}
	mu.Lock()
	after := len(queries)
	mu.Unlock()
	if after != before {
		t.Fatal("a local inbound chart called the node")
	}

	// A disabled node reports why instead of drawing an empty chart.
	if err := db.Model(&model.Node{}).Where("id = ?", node.Id).Update("enable", false).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetStats("inbound", "nl-hy2", 24, 0, 0); err == nil {
		t.Fatal("expected an error for a disabled node")
	}
}
