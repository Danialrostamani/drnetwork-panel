package service

import (
	"testing"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
)

func TestNodeHistoryBucketsAndWeightedAverages(t *testing.T) {
	resetNodeState(t)
	db := database.GetDB()
	n := model.Node{Name: "n", Enable: true, BaseUrl: "http://a", Token: "t"}
	db.Create(&n)
	now := time.Now().Unix()
	m0 := now - now%300 - 600 // a five-minute bucket that is over
	rows := []model.NodeMetric{
		{NodeId: n.Id, DateTime: m0, Probes: 12, Up: 12, Latency: 10, Cpu: 10, Mem: 50, Disk: 20, Online: 2, Sent: 100, Recv: 200},
		{NodeId: n.Id, DateTime: m0 + 60, Probes: 12, Up: 4, Latency: 50, Cpu: 50, Mem: 70, Disk: 0, Online: 5, Sent: 10, Recv: 20},
		{NodeId: n.Id, DateTime: m0 + 120, Probes: 0, Up: 0},
		{NodeId: n.Id + 1, DateTime: m0, Probes: 12, Up: 12},
		{NodeId: n.Id, DateTime: now - 8*24*3600, Probes: 12, Up: 0},
	}
	for i := range rows {
		db.Create(&rows[i])
	}
	svc := NodeService{}
	h, err := svc.GetNodeHistory(n.Id, 24)
	if err != nil {
		t.Fatal(err)
	}
	if h.Bucket != 300 || len(h.Points) != 1 {
		t.Fatalf("history: %+v", h)
	}
	p := h.Points[0]
	// Averages weigh each minute by its online probes.
	if p.T != m0 || p.Uptime < 66.6 || p.Uptime > 66.7 || p.Latency != 20 || p.Cpu != 20 || p.Mem != 55 || p.Disk != 20 || p.Online != 5 || p.Sent != 110 || p.Recv != 220 {
		t.Fatalf("bucket: %+v", p)
	}
	h, _ = svc.GetNodeHistory(n.Id, 2)
	if h.Bucket != 60 || len(h.Points) != 3 || h.Points[2].Uptime != -1 {
		t.Fatalf("minute history: %+v", h)
	}
	h, _ = svc.GetNodeHistory(n.Id, 1000)
	if h.Bucket != 1800 || h.Since > now-7*24*3600+1800 {
		t.Fatalf("history is not capped at its retention: %+v", h)
	}
	if h, _ = svc.GetNodeHistory(n.Id, 0); h.Bucket != 60 {
		t.Fatalf("zero hours: %+v", h)
	}
}

func TestNodeOutagesReport(t *testing.T) {
	resetNodeState(t)
	db := database.GetDB()
	n := model.Node{Name: "n", Enable: true, BaseUrl: "http://a", Token: "t"}
	db.Create(&n)
	now := time.Now().Unix()
	day := int64(24 * 3600)
	for _, o := range []model.NodeOutage{
		{NodeId: n.Id, Start: now - 600, End: 0, State: "offline"},                     // ongoing, 10 minutes
		{NodeId: n.Id, Start: now - day - 300, End: now - day + 300, State: "offline"}, // 5 minutes in the last day
		{NodeId: n.Id, Start: now - 3*day, End: now - 3*day + 1000},
		{NodeId: n.Id, Start: now - 20*day, End: now - 20*day + 50},
		{NodeId: n.Id, Start: now - 40*day, End: now - 40*day + 50},
		{NodeId: n.Id + 1, Start: now - 100, End: 0},
	} {
		db.Create(&o)
	}
	db.Create(&model.NodeMetric{NodeId: n.Id, DateTime: now - 3600, Probes: 10, Up: 9})
	db.Create(&model.NodeMetric{NodeId: n.Id, DateTime: now - 3*day, Probes: 10, Up: 1})
	r, err := (&NodeService{}).GetNodeOutages(n.Id)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Outages) != 5 || r.Outages[0].Start != now-600 {
		t.Fatalf("outages: %+v", r.Outages)
	}
	near := func(got, want int64) bool { return got >= want-2 && got <= want+2 }
	if r.Count24 != 2 || !near(r.Down24, 900) || r.Count7d != 3 || !near(r.Down7d, 600+600+1000) || r.Count30d != 4 || !near(r.Down30d, 2250) {
		t.Fatalf("report: %+v", r)
	}
	if r.Uptime24 != 90 || r.Uptime7d != 50 {
		t.Fatalf("uptime %v / %v", r.Uptime24, r.Uptime7d)
	}
	empty, _ := (&NodeService{}).GetNodeOutages(999)
	if empty.Uptime24 != -1 || len(empty.Outages) != 0 || empty.Outages == nil {
		t.Fatalf("no history: %+v", empty)
	}
}

func TestNodeTrafficReportSlots(t *testing.T) {
	resetNodeState(t)
	db := database.GetDB()
	n := model.Node{Name: "n", Enable: false, BaseUrl: "http://a", Token: "t", Cap: model.NodeCap{Limit: 1 << 30, Day: capDay()}}
	db.Create(&n)
	db.Model(&n).Update("enable", false)
	now := time.Now().In(time.UTC)
	hour := localHour(now.Unix(), time.UTC)
	for _, r := range []model.NodeTraffic{
		{NodeId: n.Id, DateTime: hour, Up: 10, Down: 20},
		{NodeId: n.Id, DateTime: hour - 3600, Up: 1, Down: 2},
		{NodeId: n.Id, DateTime: hour - 30*3600, Up: 100, Down: 200},
		{NodeId: n.Id, DateTime: 0, Up: 1000, Down: 1000},
	} {
		db.Create(&r)
	}
	svc := NodeSyncService{}
	r, err := svc.GetTrafficReport(n.Id, "24h")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Points) != 24 || r.Points[23].T != hour || r.Points[23].Up != 10 || r.Points[22].Down != 2 || r.ClientsNote != "disabled" {
		t.Fatalf("24h: %d points, last %+v, note %q", len(r.Points), r.Points[len(r.Points)-1], r.ClientsNote)
	}
	var sum int64
	for _, p := range r.Points {
		sum += p.Up
	}
	if sum != 11 {
		t.Fatalf("24h counts %d up", sum)
	}
	if r.Summary.TotalUp != 1111 || r.Summary.CapLimit != 1<<30 {
		t.Fatalf("summary: %+v", r.Summary)
	}
	r, _ = svc.GetTrafficReport(n.Id, "7d")
	if len(r.Points) != 7 || r.Points[6].T != localDay(now.Unix(), time.UTC) {
		t.Fatalf("7d: %+v", r.Points)
	}
	sum = 0
	for _, p := range r.Points {
		sum += p.Up
	}
	if sum != 111 {
		t.Fatalf("7d counts %d up", sum)
	}
	r, _ = svc.GetTrafficReport(n.Id, "30d")
	if len(r.Points) != 30 {
		t.Fatalf("30d: %d points", len(r.Points))
	}
	r, _ = svc.GetTrafficReport(n.Id, "12m")
	if len(r.Points) != 12 || r.Points[11].T != localMonth(now.Unix(), time.UTC) || r.Points[0].T != time.Unix(localMonth(now.Unix(), time.UTC), 0).UTC().AddDate(0, -11, 0).Unix() {
		t.Fatalf("12m: %+v", r.Points)
	}
	if _, err := svc.GetTrafficReport(n.Id, "1y"); err == nil {
		t.Fatal("an unknown period was accepted")
	}
	if _, err := svc.GetTrafficReport(999, "24h"); err == nil {
		t.Fatal("a missing node has a report")
	}
}
