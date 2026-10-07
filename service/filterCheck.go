package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/logger"
)

// Filter detection: every few minutes each online node's address is tried
// over TCP from check-host.net's servers inside Iran. A node the master
// reaches but none of those servers do, twice in a row, is taken as filtered;
// one success from Iran clears it.

var (
	// checkHostBase is the check-host.net API; tests point it elsewhere.
	checkHostBase   = "https://check-host.net"
	checkHostClient = &http.Client{Timeout: 20 * time.Second}
	// How long a check's results are waited for, and how often polled.
	checkHostWait = 25 * time.Second
	checkHostPoll = 3 * time.Second
	// Used when the list of check-host servers cannot be read.
	checkHostIranFallback = []string{"ir1.node.check-host.net", "ir3.node.check-host.net", "ir5.node.check-host.net", "ir6.node.check-host.net"}
)

// Rounds in a row with no answer from Iran before a node counts as filtered.
const filterMinRounds = 2

type filterNodeState struct {
	misses   int
	filtered bool
	// The last round: servers that reached the node and that tried.
	ok, tried int
	checked   int64
	target    string
}

// FilterStatus is what the panel shows about one node's reachability from Iran.
type FilterStatus struct {
	Filtered bool   `json:"filtered"`
	Ok       int    `json:"ok"`
	Tried    int    `json:"tried"`
	Checked  int64  `json:"checked"`
	Target   string `json:"target"`
}

var (
	filterMu     sync.Mutex
	filterStates = map[uint]*filterNodeState{}
	filterLast   time.Time
	filterBusy   sync.Mutex
	// filterHideOn mirrors the filterHide setting for settle.
	filterHideOn bool
)

// nodeFilterHidden tells whether a filtered node's links leave the
// subscriptions now.
func nodeFilterHidden(id uint) bool {
	filterMu.Lock()
	defer filterMu.Unlock()
	st := filterStates[id]
	return st != nil && st.filtered && filterHideOn
}

// nodeFilterState tells whether the filter check finds the node filtered.
func nodeFilterState(id uint) bool {
	filterMu.Lock()
	defer filterMu.Unlock()
	st := filterStates[id]
	return st != nil && st.filtered
}

// FilterStatuses returns the last check of each node, by node id.
func FilterStatuses() map[uint]FilterStatus {
	filterMu.Lock()
	defer filterMu.Unlock()
	out := make(map[uint]FilterStatus, len(filterStates))
	for id, st := range filterStates {
		out[id] = FilterStatus{Filtered: st.filtered, Ok: st.ok, Tried: st.tried, Checked: st.checked, Target: st.target}
	}
	return out
}

// RunFilterCheck runs a round when the interval set in the settings is due.
// The cron calls it every minute.
func RunFilterCheck() {
	var s SettingService
	minutes, err := s.getInt("filterCheck")
	if err != nil || minutes <= 0 {
		filterMu.Lock()
		if len(filterStates) > 0 {
			filterStates = map[uint]*filterNodeState{}
		}
		filterMu.Unlock()
		return
	}
	hide, _ := s.getBool("filterHide")
	filterMu.Lock()
	filterHideOn = hide
	due := time.Since(filterLast) >= time.Duration(minutes)*time.Minute
	filterMu.Unlock()
	if !due || !filterBusy.TryLock() {
		return
	}
	defer filterBusy.Unlock()
	filterMu.Lock()
	filterLast = time.Now()
	filterMu.Unlock()
	CheckFilters(context.Background())
}

// filterTarget is the address clients reach a node on: the host of its panel
// URL and the port of its first inbound.
func filterTarget(n *model.Node, inbounds []model.Inbound) string {
	u, err := url.Parse(n.BaseUrl)
	if err != nil || u.Hostname() == "" {
		return ""
	}
	port := u.Port()
	for _, in := range inbounds {
		var opt struct {
			ListenPort int `json:"listen_port"`
		}
		if json.Unmarshal(in.Options, &opt) == nil && opt.ListenPort > 0 {
			port = strconv.Itoa(opt.ListenPort)
			break
		}
	}
	if port == "" {
		if u.Scheme == "http" {
			port = "80"
		} else {
			port = "443"
		}
	}
	return net.JoinHostPort(u.Hostname(), port)
}

// CheckFilters checks every enabled node that is online now and records the
// verdicts. Nodes the master cannot reach are left as they were: down is not
// filtered.
func CheckFilters(ctx context.Context) {
	var nodes []model.Node
	db := database.GetDB()
	if err := db.Where("enable = ?", true).Order("id").Find(&nodes).Error; err != nil {
		logger.Warning("filter check: load nodes: ", err)
		return
	}
	nodeStatusMu.RLock()
	online := map[uint]bool{}
	for _, n := range nodes {
		online[n.Id] = nodeStatuses[n.Id].State == "online"
	}
	nodeStatusMu.RUnlock()

	servers := iranCheckServers(ctx)
	seen := map[uint]bool{}
	for i := range nodes {
		n := &nodes[i]
		seen[n.Id] = true
		if !online[n.Id] {
			continue
		}
		var inbounds []model.Inbound
		db.Where("node_id = ?", n.Id).Order("id").Find(&inbounds)
		target := filterTarget(n, inbounds)
		if target == "" {
			continue
		}
		ok, tried, err := checkTCPFrom(ctx, target, servers)
		if err != nil {
			logger.Warning("filter check: ", n.Name, ": ", err)
			continue
		}
		recordFilterResult(n, target, ok, tried, time.Now().Unix())
	}
	filterMu.Lock()
	for id := range filterStates {
		if !seen[id] {
			delete(filterStates, id)
		}
	}
	filterMu.Unlock()
}

// recordFilterResult applies one round to a node and announces a change.
func recordFilterResult(n *model.Node, target string, ok, tried int, now int64) {
	if tried == 0 {
		return // no server in Iran answered at all: says nothing about the node
	}
	filterMu.Lock()
	st := filterStates[n.Id]
	if st == nil {
		st = &filterNodeState{}
		filterStates[n.Id] = st
	}
	st.ok, st.tried, st.checked, st.target = ok, tried, now, target
	was := st.filtered
	if ok > 0 {
		st.misses, st.filtered = 0, false
	} else {
		st.misses++
		if st.misses >= filterMinRounds {
			st.filtered = true
		}
	}
	changed := was != st.filtered
	filtered := st.filtered
	filterMu.Unlock()
	if !changed {
		return
	}
	kind := "unfiltered"
	if filtered {
		kind = "filtered"
	}
	pushNodeEvent(NodeEvent{NodeId: n.Id, Name: n.Name, Kind: kind, At: now})
}

// iranCheckServers lists check-host.net's servers in Iran.
func iranCheckServers(ctx context.Context) []string {
	var body struct {
		Nodes map[string]struct {
			Location []string `json:"location"`
		} `json:"nodes"`
	}
	if err := checkHostGet(ctx, "/nodes/hosts", &body); err != nil || len(body.Nodes) == 0 {
		return checkHostIranFallback
	}
	var out []string
	for host, info := range body.Nodes {
		if len(info.Location) > 0 && strings.EqualFold(info.Location[0], "ir") {
			out = append(out, host)
		}
	}
	if len(out) == 0 {
		return checkHostIranFallback
	}
	sort.Strings(out)
	return out
}

func checkHostGet(ctx context.Context, path string, into interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, checkHostBase+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := checkHostClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("check-host.net: %s", resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(into)
}

// checkTCPFrom asks the servers to open a TCP connection to target and counts
// those that did and those that answered at all.
func checkTCPFrom(ctx context.Context, target string, servers []string) (ok, tried int, err error) {
	q := url.Values{"host": {target}}
	for _, s := range servers {
		q.Add("node", s)
	}
	var start struct {
		Ok        int    `json:"ok"`
		RequestId string `json:"request_id"`
		Error     string `json:"error"`
	}
	if err := checkHostGet(ctx, "/check-tcp?"+q.Encode(), &start); err != nil {
		return 0, 0, err
	}
	if start.Ok != 1 || start.RequestId == "" {
		return 0, 0, fmt.Errorf("check-host.net refused the check: %s", start.Error)
	}
	deadline := time.Now().Add(checkHostWait)
	for {
		select {
		case <-ctx.Done():
			return 0, 0, ctx.Err()
		case <-time.After(checkHostPoll):
		}
		var res map[string]json.RawMessage
		if err := checkHostGet(ctx, "/check-result/"+url.PathEscape(start.RequestId), &res); err != nil {
			return 0, 0, err
		}
		ok, tried, pending := tallyTCP(res)
		if pending == 0 || time.Now().After(deadline) {
			return ok, tried, nil
		}
	}
}

// tallyTCP reads a check-result: null is still pending, a list holding an
// "error" failed, a list with a "time" connected.
func tallyTCP(res map[string]json.RawMessage) (ok, tried, pending int) {
	for _, raw := range res {
		if len(raw) == 0 || string(raw) == "null" {
			pending++
			continue
		}
		var rows []map[string]interface{}
		if json.Unmarshal(raw, &rows) != nil || len(rows) == 0 || rows[0] == nil {
			continue
		}
		tried++
		if _, failed := rows[0]["error"]; failed {
			continue
		}
		if _, connected := rows[0]["time"]; connected {
			ok++
		}
	}
	return ok, tried, pending
}
