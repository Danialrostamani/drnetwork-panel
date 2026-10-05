package tgbot

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/service"
)

const statsNodeToken = "node-token"

// addStatsNode registers a node whose API answers statsTotals the way mode
// says: "ok" with the given summary, "old" like a node from before statsTotals,
// "down" with a gateway error. It returns the node and a counter of the calls.
func addStatsNode(t *testing.T, name, mode string, answer service.StatsSummary) (model.Node, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Token") != statsNodeToken || r.URL.Path != "/app/apiv2/statsTotals" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		calls.Add(1)
		switch mode {
		case "old":
			_, _ = w.Write([]byte(`{"success":false,"msg":"failed: unknown action: statsTotals"}`))
		case "down":
			w.WriteHeader(http.StatusBadGateway)
		default:
			body, _ := json.Marshal(map[string]any{"success": true, "obj": answer})
			_, _ = w.Write(body)
		}
	}))
	t.Cleanup(srv.Close)
	node := model.Node{Name: name, Enable: true, BaseUrl: srv.URL, WebPath: "/app/", Token: statsNodeToken}
	if err := database.GetDB().Create(&node).Error; err != nil {
		t.Fatal(err)
	}
	return node, &calls
}

func addStatRow(t *testing.T, resource, tag string, at, up, down int64) {
	t.Helper()
	rows := []model.Stats{
		{DateTime: at, Resource: resource, Tag: tag, Direction: true, Traffic: up},
		{DateTime: at, Resource: resource, Tag: tag, Direction: false, Traffic: down},
	}
	if err := database.GetDB().Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
}

// A client served by a node moves its traffic through the node, which counts
// it: the Stats screen has to add that to what the master counted, and say so
// when a node's numbers are not part of it.
func TestStatsScreenAddsTheNodesTraffic(t *testing.T) {
	e := newAccessEnv(t)
	hour := time.Now().Unix() / 3600
	slot := hour*3600 - 3600 // the previous hour: inside every range, whatever the minute

	// What the master counted: o1 used 1 GiB up and 2 GiB down; a master inbound, an outbound.
	addStatRow(t, "user", "o1", slot+10, gib, 2*gib)
	addStatRow(t, "inbound", "local", slot+10, 1<<20, 0)
	addStatRow(t, "outbound", "direct", slot+10, 1024, 0)

	statsOf := func(code string) string {
		t.Helper()
		n := len(e.got())
		e.press(fullAdmin, "t:"+code)
		return lastText(e.since(n), "editMessageText")
	}

	// Without nodes the screen is the master's own.
	text := statsOf("d1")
	for _, want := range []string{"Traffic — 24 h", "o1 — 3.00 GiB  (↑ 1.00 GiB ↓ 2.00 GiB)", "local — 1.00 MiB", "direct — 1.00 KiB", "📈"} {
		if !strings.Contains(text, want) {
			t.Errorf("master-only screen lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "⚠️") {
		t.Errorf("a warning without any node:\n%s", text)
	}

	// A node that serves o1 through a replica inbound, and reports a few things
	// the master does not own.
	nl, nlCalls := addStatsNode(t, "nl", "ok", service.StatsSummary{
		Totals: []service.StatsTotal{
			{Resource: "user", Tag: "o1", Up: 3 * gib, Down: 0},
			{Resource: "user", Tag: "ghost", Up: 100 * gib, Down: 100 * gib},
			{Resource: "inbound", Tag: "nl-a", Up: 5 * gib, Down: 0},
			{Resource: "inbound", Tag: "nl-private", Up: 90 * gib, Down: 0},
			{Resource: "outbound", Tag: "nl-out", Up: 90 * gib, Down: 0},
		},
		Series: []service.StatsPoint{
			{Tag: "nl-a", At: slot - 2*3600, Traffic: 5 * gib},
			{Tag: "nl-private", At: slot - 2*3600, Traffic: 90 * gib},
		},
	})
	replica := model.Inbound{Type: "vless", Tag: "nl-a", NodeId: &nl.Id, Options: json.RawMessage(`{"listen_port":443}`)}
	if err := database.GetDB().Create(&replica).Error; err != nil {
		t.Fatal(err)
	}

	text = statsOf("d1")
	if nlCalls.Load() != 1 {
		t.Errorf("the node was asked %d times", nlCalls.Load())
	}
	for _, want := range []string{
		"o1 — 6.00 GiB  (↑ 4.00 GiB ↓ 2.00 GiB)",
		"nl-a — 5.00 GiB  (↑ 5.00 GiB ↓ 0 B)",
		"local — 1.00 MiB",
		"direct — 1.00 KiB",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the screen lacks %q:\n%s", want, text)
		}
	}
	for _, private := range []string{"ghost", "nl-private", "nl-out"} {
		if strings.Contains(text, private) {
			t.Errorf("%s belongs to the node alone but is on the master's screen:\n%s", private, text)
		}
	}
	if strings.Contains(text, "⚠️") {
		t.Errorf("a warning although every node answered:\n%s", text)
	}
	// The busiest inbound is listed first.
	if strings.Index(text, "nl-a —") > strings.Index(text, "local —") {
		t.Errorf("inbounds are not busiest first:\n%s", text)
	}
	// The sparkline places each sample in its hour: 24 hourly buckets, the
	// current one last; the node's traffic two hours before the previous hour.
	if time.Now().Unix()/3600 == hour {
		vals := make([]int64, 24)
		vals[22] = 1 << 20
		vals[20] = 5 * gib
		if want := "📈 " + spark(vals); !strings.Contains(text, want) {
			t.Errorf("the timeline is not %q:\n%s", want, text)
		}
	}

	// The other ranges ask the nodes as well.
	if text := statsOf("d7"); !strings.Contains(text, "7 days") || !strings.Contains(text, "o1 — 6.00 GiB") {
		t.Errorf("the 7 days screen:\n%s", text)
	}

	// Nodes that cannot be counted are named, and the rest is still shown.
	addStatsNode(t, "old", "old", service.StatsSummary{})
	addStatsNode(t, "far", "down", service.StatsSummary{})
	text = statsOf("d1")
	if !strings.Contains(text, "⚠️ Not included: old (needs an update), far (unreachable)") {
		t.Errorf("the screen does not name the nodes that are missing:\n%s", text)
	}
	if !strings.Contains(text, "o1 — 6.00 GiB") {
		t.Errorf("a node that fails hid the others:\n%s", text)
	}
	e.b.cfg.Lang = "fa"
	text = statsOf("d1")
	e.b.cfg.Lang = "en"
	if !strings.Contains(text, "⚠️ ترافیک این نودها در آمار نیست: old (نیاز به به‌روزرسانی)، far (در دسترس نیست)") {
		t.Errorf("the Persian warning:\n%s", text)
	}

	// A node name is text, not markup.
	database.GetDB().Model(&model.Node{}).Where("name = ?", "far").Update("name", "<b>far</b>")
	if text := statsOf("d1"); strings.Contains(text, "<b>far</b>") || !strings.Contains(text, "&lt;b&gt;far&lt;/b&gt; (unreachable)") {
		t.Errorf("a node name was put into the message unescaped:\n%s", text)
	}
}

// Every way into the screen shows the same numbers: the command, the menu
// button and the range chips.
func TestStatsScreenIsTheSameFromEveryEntryPoint(t *testing.T) {
	e := newAccessEnv(t)
	slot := (time.Now().Unix()/3600)*3600 - 3600
	addStatRow(t, "user", "o1", slot+10, gib, 0)
	for _, from := range []func(){
		func() { e.say(fullAdmin, "/stats") },
		func() { e.say(fullAdmin, "/traffic") },
		func() { e.press(fullAdmin, "m:traffic") },
		func() { e.press(fullAdmin, "t:d1") },
	} {
		n := len(e.got())
		from()
		text := lastText(e.since(n), "sendMessage", "editMessageText")
		if !strings.Contains(text, "o1 — 1.00 GiB") {
			t.Errorf("a screen without the numbers:\n%s", text)
		}
	}
}
