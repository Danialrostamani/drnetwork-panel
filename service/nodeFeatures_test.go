package service

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
)

// saveNodes runs NodeService.Save in a transaction the way ConfigService does.
func saveNodes(action string, v interface{}) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	tx := database.GetDB().Begin()
	if err := (&NodeService{}).Save(tx, action, raw); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit().Error
}

func nodeByName(t *testing.T, name string) model.Node {
	t.Helper()
	var n model.Node
	if err := database.GetDB().Where("name = ?", name).First(&n).Error; err != nil {
		t.Fatalf("node %s: %v", name, err)
	}
	return n
}

func TestNodeSaveKeepsServerFieldsAndValidates(t *testing.T) {
	resetNodeState(t)
	db := database.GetDB()
	if err := saveNodes("new", map[string]interface{}{"name": " off ", "baseUrl": "https://off.example/", "token": "tok", "enable": false,
		"tags": []string{" de ", "DE", "fast", ""}, "country": "de", "cap": map[string]interface{}{"limit": 1 << 40}}); err != nil {
		t.Fatal(err)
	}
	off := nodeByName(t, "off")
	if off.Enable || off.BaseUrl != "https://off.example" || strings.Join(off.Tags, ",") != "de,fast" || off.Country != "DE" ||
		off.Cap.Day != 1 || off.Cap.Mode != "total" || off.Access.Groups == nil {
		t.Fatalf("created node: %+v", off)
	}
	// What the master keeps about the node survives an edit, and so does the
	// token when the edit leaves it empty.
	db.Model(&off).Updates(map[string]interface{}{"net_base": []byte(`{"src":"nic"}`), "cap_state": []byte(`{"cycle":1,"level":80}`), "sync_report": []byte(`{"at":5}`), "last_seen": 77})
	if err := saveNodes("edit", map[string]interface{}{"id": off.Id, "name": "off", "baseUrl": "https://off.example", "token": "", "desc": "edited", "enable": false}); err != nil {
		t.Fatal(err)
	}
	edited := nodeByName(t, "off")
	if edited.Token != "tok" || string(edited.NetBase) != `{"src":"nic"}` || string(edited.CapState) == "" || string(edited.SyncReport) != `{"at":5}` || edited.LastSeen != 77 ||
		edited.Desc != "edited" || strings.Join(edited.Tags, ",") != "de,fast" || edited.Cap.Limit != 1<<40 {
		t.Fatalf("edit lost something: %+v", edited)
	}
	if edited.Dirty {
		t.Fatal("an edit that changes nothing a node serves marked it dirty")
	}
	// Serving other clients needs a sync.
	if err := saveNodes("edit", map[string]interface{}{"id": off.Id, "access": map[string]interface{}{"groups": []string{" vip", "VIP", "@cluster"}, "clients": []uint{3, 3, 0}}}); err != nil {
		t.Fatal(err)
	}
	edited = nodeByName(t, "off")
	if !edited.Dirty || strings.Join(edited.Access.Groups, ",") != "vip" || len(edited.Access.Clients) != 1 || edited.Access.Clients[0] != 3 {
		t.Fatalf("access edit: dirty %v access %+v", edited.Dirty, edited.Access)
	}
	// Turning the node back on needs a sync too.
	db.Model(&edited).Update("dirty", false)
	if err := saveNodes("edit", map[string]interface{}{"id": off.Id, "enable": true}); err != nil {
		t.Fatal(err)
	}
	if edited = nodeByName(t, "off"); !edited.Enable || !edited.Dirty {
		t.Fatalf("re-enabled node: enable %v dirty %v", edited.Enable, edited.Dirty)
	}

	// A rename moves the prefix of the node's links.
	client := model.Client{Name: "c", Enable: true, Config: json.RawMessage(`{}`), Inbounds: json.RawMessage(`[]`),
		Links: json.RawMessage(`[{"remark":"[off] vless","type":"external","uri":"vless://x"},{"remark":"[offline] other","type":"external","uri":"vless://y"}]`)}
	db.Create(&client)
	if err := saveNodes("edit", map[string]interface{}{"id": off.Id, "name": "on"}); err != nil {
		t.Fatal(err)
	}
	db.First(&client, client.Id)
	if !strings.Contains(string(client.Links), `"[on] vless"`) || !strings.Contains(string(client.Links), `"[offline] other"`) {
		t.Fatalf("links after rename: %s", client.Links)
	}

	bad := []map[string]interface{}{
		{"name": "", "baseUrl": "https://x", "token": "t"},
		{"name": "a[b]", "baseUrl": "https://x", "token": "t"},
		{"name": "x", "baseUrl": "ftp://x", "token": "t"},
		{"name": "x", "baseUrl": "https://x", "token": ""},
		{"name": "on", "baseUrl": "https://x", "token": "t"},
		{"name": "x", "baseUrl": "https://x", "token": "t", "tags": []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11"}},
		{"name": "x", "baseUrl": "https://x", "token": "t", "tags": []string{strings.Repeat("ب", 25)}},
		{"name": "x", "baseUrl": "https://x", "token": "t", "country": "DEU"},
		{"name": "x", "baseUrl": "https://x", "token": "t", "country": "1A"},
		{"name": "x", "baseUrl": "https://x", "token": "t", "alerts": map[string]interface{}{"cpu": 101}},
		{"name": "x", "baseUrl": "https://x", "token": "t", "alerts": map[string]interface{}{"ping": -1}},
		{"name": "x", "baseUrl": "https://x", "token": "t", "alerts": map[string]interface{}{"certDays": 366}},
		{"name": "x", "baseUrl": "https://x", "token": "t", "cap": map[string]interface{}{"limit": -1}},
		{"name": "x", "baseUrl": "https://x", "token": "t", "cap": map[string]interface{}{"day": 32}},
		{"name": "x", "baseUrl": "https://x", "token": "t", "cap": map[string]interface{}{"mode": "both"}},
	}
	for _, b := range bad {
		if err := saveNodes("new", b); err == nil {
			t.Errorf("accepted %v", b)
		}
	}
	if err := saveNodes("new", map[string]interface{}{"name": "x", "baseUrl": "https://x", "token": "t", "tags": []string{strings.Repeat("ب", 24)},
		"alerts": map[string]interface{}{"cpu": 0, "ping": 60000, "certDays": 365, "version": false}}); err != nil {
		t.Fatalf("limits refused: %v", err)
	}
	x := nodeByName(t, "x")
	if x.Alerts.CpuLimit() != 0 || x.Alerts.MemLimit() != 90 || x.Alerts.PingLimit() != 60000 || x.Alerts.VersionCheck() {
		t.Fatalf("alerts: cpu %d mem %d ping %d version %v", x.Alerts.CpuLimit(), x.Alerts.MemLimit(), x.Alerts.PingLimit(), x.Alerts.VersionCheck())
	}
	// An edit that sends the alerts again does not share them with the
	// stored row (JSON decodes pointers in place).
	if err := saveNodes("edit", map[string]interface{}{"id": x.Id, "alerts": map[string]interface{}{"cpu": 50}}); err != nil {
		t.Fatal(err)
	}
	if x = nodeByName(t, "x"); x.Alerts.CpuLimit() != 50 || x.Alerts.PingLimit() != 60000 {
		t.Fatalf("alerts after a partial edit: cpu %d ping %d", x.Alerts.CpuLimit(), x.Alerts.PingLimit())
	}

	// Deleting a node drops its history.
	db.Create(&model.NodeMetric{NodeId: x.Id, DateTime: 60, Probes: 1})
	db.Create(&model.NodeTraffic{NodeId: x.Id, DateTime: 3600, Up: 1})
	db.Create(&model.NodeOutage{NodeId: x.Id, Start: 1})
	if err := saveNodes("del", x.Id); err != nil {
		t.Fatal(err)
	}
	for _, m := range []interface{}{&model.NodeMetric{}, &model.NodeTraffic{}, &model.NodeOutage{}} {
		var count int64
		db.Model(m).Where("node_id = ?", x.Id).Count(&count)
		if count != 0 {
			t.Fatalf("%T rows left after delete: %d", m, count)
		}
	}
}

func TestNodeMultiAddIsAllOrNothing(t *testing.T) {
	resetNodeState(t)
	db := database.GetDB()
	err := saveNodes("multi", []map[string]interface{}{
		{"name": "m1", "baseUrl": "https://m1", "token": "t"},
		{"name": "m2", "baseUrl": "m2.example", "token": "t"},
	})
	if err == nil || !strings.Contains(err.Error(), "node 2 (m2)") {
		t.Fatalf("multi error = %v", err)
	}
	var count int64
	db.Model(model.Node{}).Count(&count)
	if count != 0 {
		t.Fatalf("a failed multi-add left %d nodes", count)
	}
	if err := saveNodes("multi", []map[string]interface{}{{"name": "m1", "baseUrl": "https://m1", "token": "t"}, {"name": "m1", "baseUrl": "https://m2", "token": "t"}}); err == nil {
		t.Fatal("two nodes with one name were accepted")
	}
	if err := saveNodes("multi", []map[string]interface{}{{"name": "m1", "baseUrl": "https://m1", "token": "t", "webPath": "app"}, {"name": "m2", "baseUrl": "https://m2", "token": "u", "enable": false}}); err != nil {
		t.Fatal(err)
	}
	if m1, m2 := nodeByName(t, "m1"), nodeByName(t, "m2"); m1.WebPath != "/app/" || !m1.Enable || m2.Enable || m2.Token != "u" {
		t.Fatalf("multi-added: %+v %+v", m1, m2)
	}
	if err := saveNodes("multi", []map[string]interface{}{}); err == nil {
		t.Fatal("an empty multi-add was accepted")
	}
	many := make([]map[string]interface{}, nodeMultiMax+1)
	for i := range many {
		many[i] = map[string]interface{}{"name": fmt.Sprintf("n%d", i), "baseUrl": "https://n", "token": "t"}
	}
	if err := saveNodes("multi", many); err == nil {
		t.Fatal("more than the limit was accepted")
	}
}

func TestRedactNodeTokenArrays(t *testing.T) {
	one := string(redactNodeToken(json.RawMessage(`{"name":"a","token":"secret"}`)))
	many := string(redactNodeToken(json.RawMessage(`[{"name":"a","token":"s1"},{"name":"b","token":""},{"name":"c","token":"s3"}]`)))
	if strings.Contains(one, "secret") || !strings.Contains(one, `"***"`) {
		t.Fatalf("one: %s", one)
	}
	if strings.Contains(many, "s1") || strings.Contains(many, "s3") || strings.Count(many, `"***"`) != 2 {
		t.Fatalf("many: %s", many)
	}
	if got := string(redactNodeToken(json.RawMessage(`5`))); got != "5" {
		t.Fatalf("an id: %s", got)
	}
}

// syncFakeNode is a node with one inbound that keeps the clients pushed to it.
type syncFakeNode struct {
	mu      sync.Mutex
	clients map[uint]map[string]interface{}
	nextID  uint
	actions []string
}

func newSyncFakeNode(t *testing.T, token string) (*httptest.Server, *syncFakeNode) {
	t.Helper()
	f := &syncFakeNode{clients: map[uint]map[string]interface{}{}, nextID: 100}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Token") != token {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/app/apiv2/inbounds":
			if r.URL.Query().Get("id") != "" {
				_, _ = w.Write([]byte(`{"success":true,"obj":{"inbounds":[{"id":77,"type":"vless","tag":"remote-vless","listen_port":443,"addrs":[],"out_json":{"type":"vless","tag":"remote-vless","server":"node.example","server_port":443,"tls":{"enabled":false}}}]}}`))
				return
			}
			_, _ = w.Write([]byte(`{"success":true,"obj":{"inbounds":[{"id":77,"type":"vless","tag":"remote-vless"}]}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/app/apiv2/clients":
			list := make([]map[string]interface{}, 0, len(f.clients))
			for _, c := range f.clients {
				list = append(list, c)
			}
			obj, _ := json.Marshal(map[string]interface{}{"clients": list})
			body, _ := json.Marshal(map[string]interface{}{"success": true, "obj": json.RawMessage(obj)})
			_, _ = w.Write(body)
		case r.Method == http.MethodPost && r.URL.Path == "/app/apiv2/save":
			_ = r.ParseForm()
			action := r.FormValue("action")
			f.actions = append(f.actions, action)
			switch action {
			case "new", "edit":
				var c map[string]interface{}
				_ = json.Unmarshal([]byte(r.FormValue("data")), &c)
				if action == "new" {
					c["id"] = float64(f.nextID)
					f.nextID++
				}
				f.clients[uint(c["id"].(float64))] = c
			case "del":
				id, _ := strconv.ParseUint(r.FormValue("data"), 10, 64)
				delete(f.clients, uint(id))
			}
			_, _ = w.Write([]byte(`{"success":true,"obj":{}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, f
}

func (f *syncFakeNode) names() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var names []string
	for _, c := range f.clients {
		names = append(names, c["name"].(string))
	}
	sortStrings(names)
	return strings.Join(names, ",")
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

func TestNodeAccessLimitsSyncPreviewAndReport(t *testing.T) {
	resetNodeState(t)
	db := database.GetDB()
	const token = "node-token"
	srv, fake := newSyncFakeNode(t, token)
	node := model.Node{Name: "node-a", Enable: true, BaseUrl: srv.URL, WebPath: "/app/", Token: token}
	db.Create(&node)
	replica := model.Inbound{Type: "vless", Tag: "remote-vless", NodeId: &node.Id, Options: json.RawMessage(`{"listen_port":443}`), OutJson: json.RawMessage(`{"type":"vless","tag":"remote-vless","server":"node.example","server_port":443}`)}
	db.Create(&replica)
	inbounds, _ := json.Marshal([]uint{replica.Id})
	mk := func(name, group string) model.Client {
		c := model.Client{Name: name, Group: group, Enable: true, Inbounds: inbounds, Links: json.RawMessage(`[]`),
			Config: json.RawMessage(`{"vless":{"uuid":"11111111-1111-1111-1111-11111111111` + strconv.Itoa(len(name)) + `","name":"` + name + `"}}`)}
		if err := db.Create(&c).Error; err != nil {
			t.Fatal(err)
		}
		return c
	}
	alice, bob, carol := mk("alice", "VIP"), mk("bob", "basic"), mk("carol", "")
	mk("dave", "vip")
	db.Create(&model.Client{Name: "erin", Enable: true, Inbounds: json.RawMessage(`[]`), Links: json.RawMessage(`[]`), Config: json.RawMessage(`{}`)})
	if err := saveNodes("edit", map[string]interface{}{"id": node.Id, "access": map[string]interface{}{"groups": []string{"vip"}, "clients": []uint{carol.Id}}}); err != nil {
		t.Fatal(err)
	}

	svc := NodeSyncService{}
	preview, err := svc.PreviewSync(node.Id)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(preview.Add, ",") != "alice,carol,dave" || len(preview.Edit)+len(preview.Del) != 0 || !preview.Restricted || preview.Skipped != 1 || preview.Replicas != 1 || len(preview.Missing) != 0 {
		t.Fatalf("preview: %+v", preview)
	}
	if fake.names() != "" {
		t.Fatal("the preview changed the node")
	}
	if r, err := svc.GetSyncReport(node.Id); err != nil || r != nil {
		t.Fatalf("report before the first sync: %+v, %v", r, err)
	}
	if err := svc.ReconcileNow(node.Id); err != nil {
		t.Fatal(err)
	}
	if fake.names() != "alice,carol,dave" {
		t.Fatalf("node clients: %s", fake.names())
	}
	report, err := svc.GetSyncReport(node.Id)
	if err != nil || report == nil || report.Trigger != "manual" || report.AddedCount != 3 || strings.Join(report.Added, ",") != "alice,carol,dave" || report.Skipped != 1 || report.Error != "" || report.At == 0 {
		t.Fatalf("report: %+v, %v", report, err)
	}
	// The node gives links only to the clients it serves.
	db.First(&alice, alice.Id)
	db.First(&bob, bob.Id)
	if !strings.Contains(string(alice.Links), "[node-a] remote-vless") || strings.Contains(string(bob.Links), "[node-a]") {
		t.Fatalf("links: alice %s bob %s", alice.Links, bob.Links)
	}
	// Nothing changed: an ordinary sync leaves the clients alone, a full one
	// sends them all again.
	if err := svc.ReconcileNow(node.Id); err != nil {
		t.Fatal(err)
	}
	if report, _ = svc.GetSyncReport(node.Id); report.Unchanged != 3 || report.EditedCount != 0 || report.AddedCount != 0 {
		t.Fatalf("second sync: %+v", report)
	}
	if err := svc.ReconcileFull(node.Id); err != nil {
		t.Fatal(err)
	}
	if report, _ = svc.GetSyncReport(node.Id); report.Trigger != "full" || report.EditedCount != 3 || report.Unchanged != 0 {
		t.Fatalf("full sync: %+v", report)
	}
	// Everyone again: bob is added, nobody removed.
	if err := saveNodes("edit", map[string]interface{}{"id": node.Id, "access": map[string]interface{}{"groups": []string{}, "clients": []uint{}}}); err != nil {
		t.Fatal(err)
	}
	if preview, _ = svc.PreviewSync(node.Id); strings.Join(preview.Add, ",") != "bob" || preview.Restricted || preview.Skipped != 0 || preview.Same != 3 {
		t.Fatalf("preview without limits: %+v", preview)
	}
	// Narrowed to carol: the others are deleted from the node.
	if err := saveNodes("edit", map[string]interface{}{"id": node.Id, "access": map[string]interface{}{"clients": []uint{carol.Id}}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.ReconcileNow(node.Id); err != nil {
		t.Fatal(err)
	}
	if fake.names() != "carol" {
		t.Fatalf("node clients after narrowing: %s", fake.names())
	}
	if report, _ = svc.GetSyncReport(node.Id); report.DeletedCount != 2 || strings.Join(report.Deleted, ",") != "alice,dave" || report.Skipped != 3 {
		t.Fatalf("narrowed report: %+v", report)
	}
	db.First(&alice, alice.Id)
	if strings.Contains(string(alice.Links), "[node-a]") {
		t.Fatalf("alice keeps the node's links: %s", alice.Links)
	}
	// A failed sync is reported too.
	srv.Close()
	if err := svc.ReconcileNow(node.Id); err == nil {
		t.Fatal("a sync with the node gone succeeded")
	}
	if report, _ = svc.GetSyncReport(node.Id); report.Error == "" {
		t.Fatalf("failed sync report: %+v", report)
	}
}

func TestPlanSyncAndReportLimits(t *testing.T) {
	expected := map[string]map[string]interface{}{
		"new":  {"enable": true, "inbounds": json.RawMessage(`[1]`)},
		"same": {"enable": true, "inbounds": json.RawMessage(`[1]`)},
		"off":  {"enable": false, "inbounds": json.RawMessage(`[1]`)},
	}
	actual := map[string]nodeClientState{
		"same":  {Id: 1, Enable: true, Inbounds: json.RawMessage(`[1]`)},
		"off":   {Id: 2, Enable: true, Inbounds: json.RawMessage(`[1]`)},
		"stale": {Id: 3, Enable: true, Inbounds: json.RawMessage(`[1]`)},
	}
	plan := planSync(expected, actual, false)
	if strings.Join(plan.Add, ",") != "new" || strings.Join(plan.Edit, ",") != "off" || strings.Join(plan.Del, ",") != "stale" || plan.Same != 1 {
		t.Fatalf("plan: %+v", plan)
	}
	full := planSync(expected, actual, true)
	if strings.Join(full.Edit, ",") != "off,same" || full.Same != 0 {
		t.Fatalf("full plan: %+v", full)
	}
	var r SyncReport
	for i := 0; i < syncReportNames+5; i++ {
		r.note(&r.Added, &r.AddedCount, fmt.Sprint(i))
	}
	if len(r.Added) != syncReportNames || r.AddedCount != syncReportNames+5 {
		t.Fatalf("report keeps %d names, counts %d", len(r.Added), r.AddedCount)
	}
}

func TestProbeReadsTheNewStatusFields(t *testing.T) {
	resetNodeState(t)
	const token = "tok"
	nic := true
	var mu sync.Mutex
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Token") != token {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		mu.Lock()
		withNic := nic
		mu.Unlock()
		switch r.URL.Path {
		case "/apiv2/status":
			if !strings.Contains(r.URL.Query().Get("r"), "nic") {
				t.Errorf("status asked for %q", r.URL.Query().Get("r"))
			}
			nicJSON := ""
			if withNic {
				nicJSON = `"nic":{"sent":500,"recv":700,"ifs":{"eth0":[500,700]}},`
			}
			fmt.Fprintf(w, `{"success":true,"obj":{"cpu":12.5,"mem":{"current":10,"total":20},"dsk":{"current":30,"total":100},"swp":{"current":1,"total":4},`+
				`"net":{"sent":900,"recv":1100},%s"sys":{"appVersion":"1.6.3","appFull":"1.6.3-drnetwork.22","hostName":"de-1","cpuType":"EPYC","cpuCount":4,"ipv4":["1.2.3.4"],"ipv6":["2001:db8::1"],"bootTime":1700000000},`+
				`"sbd":{"running":true,"version":"1.12.0","maintenance":false,"stats":{"Uptime":3600}}}}`, nicJSON)
		case "/apiv2/onlines":
			_, _ = w.Write([]byte(`{"success":true,"obj":{"user":["alice","bob"],"inbound":["in"],"outbound":["direct"]}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	node := model.Node{Name: "tls-node", Enable: true, BaseUrl: srv.URL, WebPath: "/", Token: token, Insecure: true}
	database.GetDB().Create(&node)
	svc := NodeService{}
	statuses, err := svc.ProbeNow([]uint{node.Id})
	if err != nil {
		t.Fatal(err)
	}
	st := statuses[node.Id]
	if st.State != "online" || st.Disk.Total != 100 || st.Swap.Total != 4 || st.AppFull != "1.6.3-drnetwork.22" || st.HostName != "de-1" || st.CpuCount != 4 ||
		st.CpuType != "EPYC" || len(st.IPv4) != 1 || len(st.IPv6) != 1 || st.BootTime != 1700000000 || st.CoreUptime != 3600 || st.Online != 2 || st.CoreVersion != "1.12.0" {
		t.Fatalf("status: %+v", st)
	}
	if st.netSrc != "nic" || st.netSent != 500 || st.netIfs["eth0"] != [2]uint64{500, 700} {
		t.Fatalf("counters: %s %d %v", st.netSrc, st.netSent, st.netIfs)
	}
	leaf := srv.Certificate()
	if st.CertExpiry != leaf.NotAfter.Unix() {
		t.Fatalf("cert expiry %d, want %d", st.CertExpiry, leaf.NotAfter.Unix())
	}
	on := svc.GetNodeOnlines(node.Id)
	if strings.Join(on.User, ",") != "alice,bob" || len(on.Inbound) != 1 || on.CheckedAt == 0 {
		t.Fatalf("onlines: %+v", on)
	}
	// An older node without "nic" falls back to the sum of every interface.
	mu.Lock()
	nic = false
	mu.Unlock()
	statuses, _ = svc.ProbeNow([]uint{node.Id})
	if st := statuses[node.Id]; st.netSrc != "net" || st.netSent != 900 || st.netIfs != nil {
		t.Fatalf("fallback counters: %s %d %v", st.netSrc, st.netSent, st.netIfs)
	}
	// The certificate check does not weaken the TLS check of a pinned or
	// verified node: without Insecure the self-signed certificate fails.
	strict := model.Node{Name: "strict", Enable: true, BaseUrl: srv.URL, WebPath: "/", Token: token}
	database.GetDB().Create(&strict)
	statuses, _ = svc.ProbeNow([]uint{strict.Id})
	if st := statuses[strict.Id]; st.State != "offline" {
		t.Fatalf("a self-signed node without insecure is %s", st.State)
	}
}

func TestNodeActionsEnableDisableAndAudit(t *testing.T) {
	resetNodeState(t)
	db := database.GetDB()
	a := model.Node{Name: "a", Enable: true, BaseUrl: "http://127.0.0.1:1", Token: "t"}
	b := model.Node{Name: "b", Enable: true, BaseUrl: "http://127.0.0.1:1", Token: "t"}
	db.Create(&a)
	db.Create(&b)
	svc := NodeSyncService{}
	if _, err := svc.NodeAction([]uint{a.Id}, "explode", "admin"); err == nil {
		t.Fatal("an unknown action ran")
	}
	if _, err := svc.NodeAction([]uint{0, 0}, "probe", "admin"); err == nil {
		t.Fatal("no node, no action")
	}
	if _, err := svc.NodeAction([]uint{999}, "probe", "admin"); err == nil {
		t.Fatal("a missing node was acted on")
	}
	res, err := svc.NodeAction([]uint{b.Id, a.Id, a.Id}, "disable", "admin")
	if err != nil || len(res) != 2 || !res[0].Ok || !res[1].Ok || res[0].Name != "a" {
		t.Fatalf("disable: %+v, %v", res, err)
	}
	var changes []model.Changes
	db.Where("key = ?", "nodes").Find(&changes)
	if len(changes) != 2 || changes[0].Actor != "admin" {
		t.Fatalf("audit: %+v", changes)
	}
	if n := nodeByName(t, "a"); n.Enable {
		t.Fatal("still enabled")
	}
	// Already disabled: nothing more to record.
	if _, err := svc.NodeAction([]uint{a.Id}, "disable", "admin"); err != nil {
		t.Fatal(err)
	}
	db.Where("key = ?", "nodes").Find(&changes)
	if len(changes) != 2 {
		t.Fatalf("a no-op was audited: %d", len(changes))
	}
	// A disabled node takes no remote action.
	res, _ = svc.NodeAction([]uint{a.Id}, "restartSb", "admin")
	if len(res) != 1 || res[0].Ok || res[0].Error != "node is disabled" {
		t.Fatalf("restart of a disabled node: %+v", res)
	}
	res, _ = svc.NodeAction([]uint{a.Id}, "probe", "admin")
	if len(res) != 1 || res[0].Ok || res[0].Error != "node is disabled" {
		t.Fatalf("probe of a disabled node: %+v", res)
	}
	if _, err := svc.NodeAction([]uint{a.Id}, "enable", "admin"); err != nil {
		t.Fatal(err)
	}
	if n := nodeByName(t, "a"); !n.Enable || !n.Dirty {
		t.Fatalf("enabled node: enable %v dirty %v", n.Enable, n.Dirty)
	}
	// Unreachable: the probe tells why.
	res, _ = svc.NodeAction([]uint{a.Id}, "probe", "admin")
	if len(res) != 1 || res[0].Ok || res[0].Error == "" {
		t.Fatalf("probe of an unreachable node: %+v", res)
	}
}

func TestNodeBackupFetchAndBackupAll(t *testing.T) {
	resetNodeState(t)
	db := database.GetDB()
	sqlite := append([]byte("SQLite format 3\x00"), bytes.Repeat([]byte{1}, 64)...)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Header.Get("Token") {
		case "good":
			if r.URL.Query().Get("exclude") != "" && r.URL.Query().Get("exclude") != "stats" {
				t.Errorf("exclude = %q", r.URL.Query().Get("exclude"))
			}
			_, _ = w.Write(sqlite)
		case "refuse":
			_, _ = w.Write([]byte(`{"success":false,"msg":"nope"}`))
		case "html":
			_, _ = w.Write([]byte(`<html>login</html>`))
		default:
			w.WriteHeader(http.StatusForbidden)
		}
	}))
	defer srv.Close()
	mk := func(name, token string, enable bool) model.Node {
		n := model.Node{Name: name, Enable: true, BaseUrl: srv.URL, WebPath: "/", Token: token}
		db.Create(&n)
		if !enable {
			db.Model(&n).Update("enable", false)
		}
		return n
	}
	good := mk("good/one", "good", true)
	refuse := mk("refuse", "refuse", true)
	html := mk("html", "html", true)
	forbidden := mk("forbidden", "x", true)
	off := mk("off", "good", false)
	svc := NodeSyncService{}
	if _, data, err := svc.NodeBackup(good.Id, "stats"); err != nil || !bytes.Equal(data, sqlite) {
		t.Fatalf("good backup: %v", err)
	}
	if _, _, err := svc.NodeBackup(refuse.Id, ""); err == nil || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("refused backup: %v", err)
	}
	if _, _, err := svc.NodeBackup(html.Id, ""); err == nil || !strings.Contains(err.Error(), "did not send a database") {
		t.Fatalf("html backup: %v", err)
	}
	if _, _, err := svc.NodeBackup(forbidden.Id, ""); err == nil {
		t.Fatal("a refused request was a backup")
	}
	if _, _, err := svc.NodeBackup(off.Id, ""); err == nil {
		t.Fatal("a disabled node was backed up")
	}
	if name := NodeBackupName(&good, time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)); name != "s-ui_good_one_20250102-030405.db" {
		t.Fatalf("backup name %q", name)
	}
	write, err := svc.BackupAll("stats")
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := write(&buf); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for _, f := range zr.File {
		rc, _ := f.Open()
		body, _ := io.ReadAll(rc)
		rc.Close()
		files[f.Name] = string(body)
	}
	want := []string{"master.db", fmt.Sprintf("nodes/%d-good_one.db", good.Id), fmt.Sprintf("nodes/%d-refuse.error.txt", refuse.Id),
		fmt.Sprintf("nodes/%d-html.error.txt", html.Id), fmt.Sprintf("nodes/%d-forbidden.error.txt", forbidden.Id)}
	if len(files) != len(want) {
		t.Fatalf("zip files: %v", keysOf(files))
	}
	for _, name := range want {
		if _, ok := files[name]; !ok {
			t.Fatalf("zip lacks %s: %v", name, keysOf(files))
		}
	}
	if !strings.HasPrefix(files["master.db"], "SQLite format 3\x00") || files[want[1]] != string(sqlite) || !strings.Contains(files[want[2]], "nope") {
		t.Fatal("zip contents")
	}
}

func keysOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sortStrings(out)
	return out
}

func TestSafeFileName(t *testing.T) {
	for in, want := range map[string]string{"de-1": "de-1", "a/b\\c:d": "a_b_c_d", "  ": "node", "..": "node", "نود": "نود", "x\ny": "x_y"} {
		if got := safeFileName(in); got != want {
			t.Errorf("safeFileName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAddTokenValue(t *testing.T) {
	resetNodeState(t)
	svc := UserService{}
	for _, bad := range []string{"short", strings.Repeat("a", 129), "abcdefghijklmnop!", "abcdefgh ijklmnop"} {
		if _, err := svc.AddTokenValue(bad, ""); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	token := "Abcdefghijklmnop1234567890ABCDEF"
	added, err := svc.AddTokenValue("  "+token+" ", " master ")
	if err != nil || !added {
		t.Fatalf("add: %v %v", added, err)
	}
	added, err = svc.AddTokenValue(token, "again")
	if err != nil || added {
		t.Fatalf("second add: %v %v", added, err)
	}
	var rows []model.Tokens
	database.GetDB().Find(&rows)
	if len(rows) != 1 || rows[0].Token != token || rows[0].Desc != "master" || rows[0].UserId == 0 {
		t.Fatalf("tokens: %+v", rows)
	}
}

func TestIsVirtualNic(t *testing.T) {
	for name, want := range map[string]bool{
		"lo": true, "lo0": true, "LO": true, "docker0": true, "veth1a2b": true, "br-12ab": true, "virbr0": true, "vnet3": true, "tun0": true, "tap1": true,
		"wg0": true, "tailscale0": true, "zt1234": true, "utun2": true, "cni0": true, "flannel.1": true, "cali123": true, "vxlan.calico": true, "kube-ipvs0": true,
		"ifb0": true, "dummy0": true, "singbox_tun": true, "warp": true, "CloudflareWARP": true,
		"eth0": false, "ens3": false, "enp0s3": false, "venet0": false, "ppp0": false, "bond0": false, "wlan0": false, "local": false, "lof": false,
	} {
		if got := isVirtualNic(name); got != want {
			t.Errorf("isVirtualNic(%q) = %v, want %v", name, got, want)
		}
	}
}
