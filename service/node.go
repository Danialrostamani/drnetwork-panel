package service

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/logger"
	"github.com/Danialrostamani/drnetwork-panel/util/common"
	"gorm.io/gorm"
)

type NodeMem struct {
	Current int64 `json:"current"`
	Total   int64 `json:"total"`
}

// NodeStatus is what the last probe found on a node, and what the master
// derives from the probes before it.
type NodeStatus struct {
	State   string  `json:"state"`
	Latency int64   `json:"latency"`
	Cpu     float64 `json:"cpu"`
	Mem     NodeMem `json:"mem"`
	Disk    NodeMem `json:"disk"`
	Swap    NodeMem `json:"swap"`
	// AppFull is the release, such as 32 (1.6.3-drnetwork.31 before DrNetwork
	// had version numbers of its own); older nodes only report AppVersion.
	AppVersion  string   `json:"appVersion"`
	AppFull     string   `json:"appFull,omitempty"`
	CoreVersion string   `json:"coreVersion"`
	HostName    string   `json:"hostName,omitempty"`
	CpuType     string   `json:"cpuType,omitempty"`
	CpuCount    int      `json:"cpuCount,omitempty"`
	IPv4        []string `json:"ipv4,omitempty"`
	IPv6        []string `json:"ipv6,omitempty"`
	// When the server booted, and for how many seconds the core has run.
	BootTime   int64 `json:"bootTime,omitempty"`
	CoreUptime int64 `json:"coreUptime,omitempty"`
	// Bytes per second the server's network interfaces sent and received
	// since the probe before.
	NetUp   int64 `json:"netUp"`
	NetDown int64 `json:"netDown"`
	// Users connected right now.
	Online int `json:"online"`
	// The core is stopped on purpose.
	Maintenance bool `json:"maintenance,omitempty"`
	// When the certificate of the node panel's HTTPS address runs out.
	CertExpiry int64  `json:"certExpiry,omitempty"`
	Error      string `json:"error,omitempty"`
	CheckedAt  int64  `json:"checkedAt"`
	LastOnline int64  `json:"lastOnline"`
	// Since when the node is not online; zero while it is.
	DownSince int64 `json:"downSince,omitempty"`
	// Why the node's links are out of the subscriptions: "down", "cap" or
	// "filtered".
	Hidden string `json:"hidden,omitempty"`
	// The filter check finds the node unreachable from Iran.
	Filtered bool          `json:"filtered,omitempty"`
	Warnings []NodeWarning `json:"warnings,omitempty"`
	// Percent of the probes of the last 24 hours and 7 days that found the
	// node online; -1 while there is no history.
	Uptime24 float64             `json:"uptime24"`
	Uptime7d float64             `json:"uptime7d"`
	Traffic  *NodeTrafficSummary `json:"traffic,omitempty"`

	onlineUsers     []string
	onlineInbounds  []string
	onlineOutbounds []string
	onlineCheckedAt int64
	// The interface counters the probe read, and what kind they are.
	netSent, netRecv uint64
	netIfs           map[string][2]uint64
	netSrc           string
}

var (
	nodeStatusMu sync.RWMutex
	nodeStatuses = map[uint]NodeStatus{}
	nodeClientMu sync.Mutex
	nodeClients  = map[uint]*http.Client{}
)

const (
	nodeProbeTimeout    = 8 * time.Second
	nodeProbeParallel   = 8
	nodeMaxResponseSize = 8 << 20
	nodeIdleConnTimeout = 90 * time.Second
	// What a probe asks a node for. A node too old to know a key leaves it out.
	nodeStatusRequest = "cpu,mem,dsk,swp,net,nic,sys,sbd"
	nodeMaxTags       = 10
	nodeTagMaxLen     = 24
	// How many nodes one multi-add may create.
	nodeMultiMax = 100
)

type NodeService struct{}

func normalizeWebPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		p = "/app/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	if !strings.HasSuffix(p, "/") {
		p += "/"
	}
	return p
}
func normalizeCertPin(pin string) string {
	return strings.NewReplacer(":", "", " ", "").Replace(strings.ToLower(strings.TrimSpace(pin)))
}
func nodeAPIURL(n *model.Node, action string) string {
	return strings.TrimRight(n.BaseUrl, "/") + normalizeWebPath(n.WebPath) + "apiv2/" + action
}
func pinVerifier(pin string) func([][]byte, [][]*x509.Certificate) error {
	return func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
		if len(rawCerts) == 0 {
			return common.NewError("no peer certificate received")
		}
		sum := sha256.Sum256(rawCerts[0])
		if hex.EncodeToString(sum[:]) != pin {
			return common.NewError("certificate pin mismatch")
		}
		return nil
	}
}
func buildNodeHTTPClient(n *model.Node) *http.Client {
	client := &http.Client{Timeout: nodeProbeTimeout}
	if strings.HasPrefix(strings.ToLower(n.BaseUrl), "https://") {
		cfg := &tls.Config{MinVersion: tls.VersionTLS12}
		if pin := normalizeCertPin(n.CertPin); pin != "" {
			cfg.InsecureSkipVerify = true
			cfg.VerifyPeerCertificate = pinVerifier(pin)
		} else if n.Insecure {
			cfg.InsecureSkipVerify = true
		}
		client.Transport = &http.Transport{TLSClientConfig: cfg, IdleConnTimeout: nodeIdleConnTimeout}
	}
	return client
}
func closeNodeIdle(client *http.Client) {
	if client != nil && client.Transport != nil && client.Transport != http.DefaultTransport {
		client.CloseIdleConnections()
	}
}
func nodeHTTPClient(n *model.Node) *http.Client {
	nodeClientMu.Lock()
	defer nodeClientMu.Unlock()
	if c, ok := nodeClients[n.Id]; ok {
		return c
	}
	c := buildNodeHTTPClient(n)
	nodeClients[n.Id] = c
	return c
}
func invalidateNodeClient(id uint) {
	nodeClientMu.Lock()
	defer nodeClientMu.Unlock()
	if c, ok := nodeClients[id]; ok {
		closeNodeIdle(c)
		delete(nodeClients, id)
	}
}
func nodeHTTPStatusError(status int) error {
	if status == http.StatusForbidden {
		return common.NewError("HTTP 403 from node panel; use its configured domain instead of an IP")
	}
	return common.NewErrorf("HTTP %d from node panel", status)
}
func (s *NodeService) nodeGet(n *model.Node, client *http.Client, action string, q url.Values) (json.RawMessage, error) {
	obj, _, err := s.nodeGetCert(n, client, action, q)
	return obj, err
}

// nodeGetCert is nodeGet that also tells when the certificate the node
// presented runs out; zero over plain HTTP.
func (s *NodeService) nodeGetCert(n *model.Node, client *http.Client, action string, q url.Values) (json.RawMessage, int64, error) {
	u := nodeAPIURL(n, action)
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Token", n.Token)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	var expiry int64
	if resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
		expiry = resp.TLS.PeerCertificates[0].NotAfter.Unix()
	}
	if resp.StatusCode != http.StatusOK {
		return nil, expiry, nodeHTTPStatusError(resp.StatusCode)
	}
	var msg struct {
		Success bool            `json:"success"`
		Msg     string          `json:"msg"`
		Obj     json.RawMessage `json:"obj"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, nodeMaxResponseSize)).Decode(&msg); err != nil {
		return nil, expiry, common.NewError("unexpected response from node API")
	}
	if !msg.Success {
		if msg.Msg == "" {
			msg.Msg = "check the API token"
		}
		return nil, expiry, common.NewErrorf("node refused: %s", msg.Msg)
	}
	return msg.Obj, expiry, nil
}
func isNodeNetworkError(err error) bool {
	var ne net.Error
	if errors.As(err, &ne) {
		return true
	}
	var ue *url.Error
	return errors.As(err, &ue)
}

// nodeCounters are interface counters as a node reports them. A key the node
// does not know stays nil.
type nodeCounters struct {
	Sent uint64 `json:"sent"`
	Recv uint64 `json:"recv"`
	// Per interface, sent and received ("nic" only).
	Ifs map[string][2]uint64 `json:"ifs"`
}

func (c *nodeCounters) present() bool {
	return c != nil && (c.Sent > 0 || c.Recv > 0 || len(c.Ifs) > 0)
}

type nodeStatusPayload struct {
	Cpu float64       `json:"cpu"`
	Mem NodeMem       `json:"mem"`
	Dsk NodeMem       `json:"dsk"`
	Swp NodeMem       `json:"swp"`
	Net *nodeCounters `json:"net"`
	Nic *nodeCounters `json:"nic"`
	Sys struct {
		AppVersion string   `json:"appVersion"`
		AppFull    string   `json:"appFull"`
		HostName   string   `json:"hostName"`
		CpuType    string   `json:"cpuType"`
		CpuCount   int      `json:"cpuCount"`
		IPv4       []string `json:"ipv4"`
		IPv6       []string `json:"ipv6"`
		BootTime   int64    `json:"bootTime"`
	} `json:"sys"`
	Sbd struct {
		Running     bool   `json:"running"`
		Version     string `json:"version"`
		Maintenance bool   `json:"maintenance"`
		Stats       struct {
			Uptime int64 `json:"Uptime"`
		} `json:"stats"`
	} `json:"sbd"`
}

func (s *NodeService) probe(n *model.Node, client *http.Client) NodeStatus {
	started := time.Now()
	st := NodeStatus{CheckedAt: started.Unix(), Uptime24: -1, Uptime7d: -1}
	q := url.Values{}
	q.Set("r", nodeStatusRequest)
	obj, certExpiry, err := s.nodeGetCert(n, client, "status", q)
	if err != nil && isNodeNetworkError(err) {
		// A stale keep-alive connection (dropped by NAT/firewall) or a single
		// lost packet should not flip the node to offline: retry once on a
		// fresh connection.
		client.CloseIdleConnections()
		if http.DefaultTransport != nil && client.Transport == nil {
			if t, ok := http.DefaultTransport.(*http.Transport); ok {
				t.CloseIdleConnections()
			}
		}
		started = time.Now()
		obj, certExpiry, err = s.nodeGetCert(n, client, "status", q)
	}
	st.Latency = time.Since(started).Milliseconds()
	st.CertExpiry = certExpiry
	if err != nil {
		st.State = "offline"
		st.Error = err.Error()
		return st
	}
	var payload nodeStatusPayload
	if json.Unmarshal(obj, &payload) != nil {
		st.State = "offline"
		st.Error = "unexpected status payload"
		return st
	}
	st.Cpu = payload.Cpu
	st.Mem, st.Disk, st.Swap = payload.Mem, payload.Dsk, payload.Swp
	st.AppVersion = payload.Sys.AppVersion
	st.AppFull = payload.Sys.AppFull
	st.CoreVersion = payload.Sbd.Version
	st.HostName, st.CpuType, st.CpuCount = payload.Sys.HostName, payload.Sys.CpuType, payload.Sys.CpuCount
	st.IPv4, st.IPv6 = payload.Sys.IPv4, payload.Sys.IPv6
	st.BootTime = payload.Sys.BootTime
	st.Maintenance = payload.Sbd.Maintenance
	// "nic" leaves out loopback and virtual interfaces; nodes older than it
	// only have "net", the sum of every interface.
	switch {
	case payload.Nic.present():
		st.netSent, st.netRecv, st.netIfs, st.netSrc = payload.Nic.Sent, payload.Nic.Recv, payload.Nic.Ifs, "nic"
	case payload.Net.present():
		st.netSent, st.netRecv, st.netSrc = payload.Net.Sent, payload.Net.Recv, "net"
	}
	if !payload.Sbd.Running {
		st.State = "core-stopped"
		return st
	}
	st.CoreUptime = payload.Sbd.Stats.Uptime
	st.State = "online"
	if onlineObj, err := s.nodeGet(n, client, "onlines", nil); err == nil {
		var online struct {
			User     []string `json:"user"`
			Inbound  []string `json:"inbound"`
			Outbound []string `json:"outbound"`
		}
		if json.Unmarshal(onlineObj, &online) == nil {
			st.onlineUsers = online.User
			st.onlineInbounds = online.Inbound
			st.onlineOutbounds = online.Outbound
			st.onlineCheckedAt = time.Now().Unix()
		}
	}
	st.Online = len(st.onlineUsers)
	return st
}

// probeNodes probes the nodes side by side.
func (s *NodeService) probeNodes(nodes []*model.Node) map[uint]NodeStatus {
	fresh := make(map[uint]NodeStatus, len(nodes))
	var wg sync.WaitGroup
	var mu sync.Mutex
	sem := make(chan struct{}, nodeProbeParallel)
	for _, node := range nodes {
		wg.Add(1)
		sem <- struct{}{}
		go func(n *model.Node) {
			defer wg.Done()
			defer func() { <-sem }()
			st := s.probe(n, nodeHTTPClient(n))
			mu.Lock()
			fresh[n.Id] = st
			mu.Unlock()
		}(node)
	}
	wg.Wait()
	return fresh
}

// RefreshAll probes every enabled node. It is the periodic round.
func (s *NodeService) RefreshAll() {
	var nodes []*model.Node
	if err := database.GetDB().Model(model.Node{}).Where("enable = ?", true).Find(&nodes).Error; err != nil {
		logger.Warning("nodes: load failed: ", err)
		return
	}
	s.applyProbes(nodes, s.probeNodes(nodes), true)
}

// ProbeNow probes these enabled nodes right away and returns what it found.
func (s *NodeService) ProbeNow(ids []uint) (map[uint]NodeStatus, error) {
	out := map[uint]NodeStatus{}
	if len(ids) == 0 {
		return out, nil
	}
	var nodes []*model.Node
	if err := database.GetDB().Model(model.Node{}).Where("enable = ? AND id IN ?", true, ids).Find(&nodes).Error; err != nil {
		return nil, err
	}
	s.applyProbes(nodes, s.probeNodes(nodes), false)
	statuses := s.GetStatuses()
	for _, n := range nodes {
		if st, ok := statuses[n.Id]; ok {
			out[n.Id] = st
		}
	}
	return out, nil
}

// GetStatuses is the last status of every probed node.
func (s *NodeService) GetStatuses() map[uint]NodeStatus {
	nodeStatusMu.RLock()
	defer nodeStatusMu.RUnlock()
	out := make(map[uint]NodeStatus, len(nodeStatuses))
	for id, st := range nodeStatuses {
		out[id] = st
	}
	return out
}

// GetStatus is the last status of one node.
func (s *NodeService) GetStatus(id uint) (NodeStatus, bool) {
	nodeStatusMu.RLock()
	defer nodeStatusMu.RUnlock()
	st, ok := nodeStatuses[id]
	return st, ok
}

func (s *NodeService) TestNode(data json.RawMessage) (NodeStatus, error) {
	var n model.Node
	if err := json.Unmarshal(data, &n); err != nil {
		return NodeStatus{}, err
	}
	n.BaseUrl = strings.TrimSpace(strings.TrimRight(n.BaseUrl, "/"))
	if !strings.HasPrefix(n.BaseUrl, "http://") && !strings.HasPrefix(n.BaseUrl, "https://") {
		return NodeStatus{}, common.NewError("baseUrl must start with http:// or https://")
	}
	if n.Token == "" && n.Id > 0 {
		if err := database.GetDB().Model(model.Node{}).Select("token").Where("id = ?", n.Id).Scan(&n.Token).Error; err != nil {
			return NodeStatus{}, err
		}
	}
	c := buildNodeHTTPClient(&n)
	defer closeNodeIdle(c)
	return s.probe(&n, c), nil
}

// nodeUsage counts, for each node, the inbounds adopted from it and the
// clients it serves: the clients with one of those inbounds that its access
// list lets in.
func nodeUsage(nodes []model.Node) (inbounds, clients map[uint]int, err error) {
	inbounds, clients = map[uint]int{}, map[uint]int{}
	var replicas []model.Inbound
	if err = database.GetDB().Model(model.Inbound{}).Select("id", "node_id").Where("node_id > 0").Find(&replicas).Error; err != nil {
		return nil, nil, err
	}
	if len(replicas) == 0 {
		return inbounds, clients, nil
	}
	nodeOf := make(map[uint]uint, len(replicas))
	for _, r := range replicas {
		if r.NodeId == nil {
			continue
		}
		nodeOf[r.Id] = *r.NodeId
		inbounds[*r.NodeId]++
	}
	byID := make(map[uint]*model.Node, len(nodes))
	for i := range nodes {
		byID[nodes[i].Id] = &nodes[i]
	}
	var all []model.Client
	if err = database.GetDB().Model(model.Client{}).Select("id", "group", "inbounds").Find(&all).Error; err != nil {
		return nil, nil, err
	}
	for i := range all {
		var ids []uint
		if json.Unmarshal(all[i].Inbounds, &ids) != nil {
			continue
		}
		seen := map[uint]bool{}
		for _, id := range ids {
			nodeID, ok := nodeOf[id]
			if !ok || seen[nodeID] {
				continue
			}
			seen[nodeID] = true
			if n := byID[nodeID]; n != nil && n.Access.Allows(all[i].Id, all[i].Group) {
				clients[nodeID]++
			}
		}
	}
	return inbounds, clients, nil
}

func (s *NodeService) GetAll() ([]map[string]interface{}, error) {
	var nodes []model.Node
	if err := database.GetDB().Order("id").Find(&nodes).Error; err != nil {
		return nil, err
	}
	inbounds, clients, err := nodeUsage(nodes)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]interface{}, 0, len(nodes))
	for _, n := range nodes {
		row := map[string]interface{}{
			"id": n.Id, "enable": n.Enable, "name": n.Name, "baseUrl": n.BaseUrl, "webPath": n.WebPath,
			"insecure": n.Insecure, "certPin": n.CertPin, "desc": n.Desc, "lastSeen": n.LastSeen,
			"tokenSet": n.Token != "", "dirty": n.Dirty, "lastSync": n.LastSync,
			"tags": nonNilStrings(n.Tags), "country": n.Country, "sortOrder": n.SortOrder,
			"alerts": n.Alerts, "cap": n.Cap, "hideDown": n.HideDown,
			"access":       model.NodeAccess{Groups: nonNilStrings(n.Access.Groups), Clients: nonNilIDs(n.Access.Clients)},
			"inboundCount": inbounds[n.Id], "clientCount": clients[n.Id],
		}
		if len(n.SyncReport) > 0 && json.Valid(n.SyncReport) {
			row["syncReport"] = json.RawMessage(n.SyncReport)
		}
		out = append(out, row)
	}
	return out, nil
}

func nonNilStrings(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

func nonNilIDs(v []uint) []uint {
	if v == nil {
		return []uint{}
	}
	return v
}

// redactNodeToken hides the API token of one node, or of each node in a list,
// before a save is recorded in the change history.
func redactNodeToken(data json.RawMessage) json.RawMessage {
	redact := func(m map[string]interface{}) bool {
		if t, ok := m["token"].(string); ok && t != "" {
			m["token"] = "***"
			return true
		}
		return false
	}
	var one map[string]interface{}
	if json.Unmarshal(data, &one) == nil {
		if redact(one) {
			if b, e := json.Marshal(one); e == nil {
				return b
			}
		}
		return data
	}
	var many []map[string]interface{}
	if json.Unmarshal(data, &many) == nil {
		changed := false
		for _, m := range many {
			if m != nil && redact(m) {
				changed = true
			}
		}
		if changed {
			if b, e := json.Marshal(many); e == nil {
				return b
			}
		}
	}
	return data
}

// nodeManagedColumns are written by the master as it works with a node; a
// save from the panel leaves them be.
var nodeManagedColumns = []string{"baselines", "net_base", "cap_state", "sync_report", "last_seen", "dirty", "last_sync"}

func (s *NodeService) Save(tx *gorm.DB, action string, data json.RawMessage) error {
	switch action {
	case "new", "edit":
		return s.saveNode(tx, action, data)
	case "multi":
		var items []json.RawMessage
		if err := json.Unmarshal(data, &items); err != nil {
			return err
		}
		if len(items) == 0 {
			return common.NewError("no nodes to add")
		}
		if len(items) > nodeMultiMax {
			return common.NewErrorf("at most %d nodes at once", nodeMultiMax)
		}
		for i, raw := range items {
			if err := s.saveNode(tx, "new", raw); err != nil {
				var named struct {
					Name string `json:"name"`
				}
				_ = json.Unmarshal(raw, &named)
				return common.NewErrorf("node %d (%s): %v", i+1, strings.TrimSpace(named.Name), err)
			}
		}
		return nil
	case "del":
		var id uint
		if err := json.Unmarshal(data, &id); err != nil {
			return err
		}
		if id == 0 {
			return common.NewError("node id is required")
		}
		var replicas int64
		if err := tx.Model(model.Inbound{}).Where("node_id = ?", id).Count(&replicas).Error; err != nil {
			return err
		}
		if replicas > 0 {
			return common.NewErrorf("node still has %d adopted inbound(s)", replicas)
		}
		if err := tx.Delete(&model.Node{}, id).Error; err != nil {
			return err
		}
		if err := deleteNodeHistory(tx, id); err != nil {
			return err
		}
		invalidateNodeClient(id)
		nodeStatusMu.Lock()
		delete(nodeStatuses, id)
		nodeStatusMu.Unlock()
		invalidateNodeLinkRules()
		return nil
	default:
		return common.NewErrorf("unknown action: %s", action)
	}
}

// saveNode creates or edits one node. An edit starts from the stored node, so
// a field the request leaves out keeps its value, and an empty token keeps
// the stored one.
func (s *NodeService) saveNode(tx *gorm.DB, action string, data json.RawMessage) error {
	var n, old model.Node
	if action == "new" {
		n = model.Node{Enable: true}
		if err := json.Unmarshal(data, &n); err != nil {
			return err
		}
		n.Id, n.Dirty, n.LastSync, n.LastSeen = 0, false, 0, 0
		n.Baselines, n.NetBase, n.CapState, n.SyncReport = nil, nil, nil, nil
	} else {
		var ref struct {
			Id uint `json:"id"`
		}
		if err := json.Unmarshal(data, &ref); err != nil {
			return err
		}
		if ref.Id == 0 {
			return common.NewError("node id is required")
		}
		if err := tx.First(&old, ref.Id).Error; err != nil {
			return err
		}
		n = old
		// JSON decodes slices and pointers in place: give n its own, so old
		// keeps what the database had.
		n.Tags = append([]string(nil), old.Tags...)
		n.Alerts = old.Alerts.Clone()
		n.Access = model.NodeAccess{Groups: append([]string(nil), old.Access.Groups...), Clients: append([]uint(nil), old.Access.Clients...)}
		n.Token = ""
		if err := json.Unmarshal(data, &n); err != nil {
			return err
		}
		n.Id = old.Id
		if n.Token == "" {
			n.Token = old.Token
		}
		n.LastSeen, n.Dirty, n.LastSync = old.LastSeen, old.Dirty, old.LastSync
	}
	if err := normalizeNode(&n); err != nil {
		return err
	}
	if n.Token == "" {
		return common.NewError("node API token is required")
	}
	var taken int64
	if err := tx.Model(model.Node{}).Where("name = ? AND id <> ?", n.Name, n.Id).Count(&taken).Error; err != nil {
		return err
	}
	if taken > 0 {
		return common.NewErrorf("a node named %s already exists", n.Name)
	}
	enable := n.Enable
	if err := tx.Omit(nodeManagedColumns...).Save(&n).Error; err != nil {
		return err
	}
	if action == "new" && !enable {
		// The column defaults to true: a create leaves a false bool to the
		// default, and reads the default back into n.
		if err := tx.Model(model.Node{}).Where("id = ?", n.Id).Update("enable", false).Error; err != nil {
			return err
		}
		n.Enable = false
	}
	if action == "edit" {
		if old.Name != n.Name {
			if err := renameNodeLinkPrefix(tx, old.Name, n.Name); err != nil {
				return err
			}
		}
		// A node that serves other clients now, or that was left out of the
		// syncs while disabled, needs a sync.
		accessChanged := !reflect.DeepEqual(normalizedAccess(old.Access), n.Access)
		if accessChanged || (!old.Enable && n.Enable) {
			if err := tx.Model(model.Node{}).Where("id = ?", n.Id).Update("dirty", true).Error; err != nil {
				return err
			}
			bumpDirtyGen()
		}
		if accessChanged || old.Name != n.Name || old.HideDown != n.HideDown || old.Cap.Hide != n.Cap.Hide {
			invalidateNodeLinkRules()
		}
	}
	invalidateNodeClient(n.Id)
	return nil
}

// normalizeNode tidies what the operator typed and refuses what cannot work.
func normalizeNode(n *model.Node) error {
	n.Name = strings.TrimSpace(n.Name)
	n.BaseUrl = strings.TrimSpace(strings.TrimRight(strings.TrimSpace(n.BaseUrl), "/"))
	n.WebPath = normalizeWebPath(n.WebPath)
	n.CertPin = normalizeCertPin(n.CertPin)
	n.Desc = strings.TrimSpace(n.Desc)
	if n.Name == "" {
		return common.NewError("node name is required")
	}
	if strings.ContainsAny(n.Name, "[]") {
		return common.NewError("node name cannot contain [ or ]")
	}
	if !strings.HasPrefix(n.BaseUrl, "http://") && !strings.HasPrefix(n.BaseUrl, "https://") {
		return common.NewError("node baseUrl must start with http:// or https://")
	}
	tags, err := normalizeNodeTags(n.Tags)
	if err != nil {
		return err
	}
	n.Tags = tags
	n.Country = strings.ToUpper(strings.TrimSpace(n.Country))
	if n.Country != "" && !isCountryCode(n.Country) {
		return common.NewError("country must be a two-letter code, such as DE")
	}
	if err := checkNodeAlerts(n.Alerts); err != nil {
		return err
	}
	if n.Cap.Limit < 0 {
		return common.NewError("the traffic cap cannot be negative")
	}
	if n.Cap.Day == 0 {
		n.Cap.Day = 1
	}
	if n.Cap.Day < 1 || n.Cap.Day > 31 {
		return common.NewError("the cap's reset day must be between 1 and 31")
	}
	switch n.Cap.Mode {
	case "":
		n.Cap.Mode = "total"
	case "total", "up", "down":
	default:
		return common.NewError("the cap counts total, up or down")
	}
	n.Access = normalizedAccess(n.Access)
	return nil
}

func normalizeNodeTags(tags []string) ([]string, error) {
	out := make([]string, 0, len(tags))
	seen := map[string]bool{}
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if utf8.RuneCountInString(t) > nodeTagMaxLen {
			return nil, common.NewErrorf("tag %s is longer than %d characters", t, nodeTagMaxLen)
		}
		if key := strings.ToLower(t); !seen[key] {
			seen[key] = true
			out = append(out, t)
		}
	}
	if len(out) > nodeMaxTags {
		return nil, common.NewErrorf("a node can have at most %d tags", nodeMaxTags)
	}
	return out, nil
}

func isCountryCode(c string) bool {
	if len(c) != 2 {
		return false
	}
	for _, r := range c {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}

func checkNodeAlerts(a model.NodeAlerts) error {
	for _, c := range []struct {
		v    *int
		max  int
		name string
	}{{a.Cpu, 100, "CPU"}, {a.Mem, 100, "memory"}, {a.Disk, 100, "disk"}, {a.Ping, 60000, "ping"}, {a.CertDays, 365, "certificate"}} {
		if c.v != nil && (*c.v < 0 || *c.v > c.max) {
			return common.NewErrorf("the %s alert must be between 0 and %d", c.name, c.max)
		}
	}
	return nil
}

// normalizedAccess is the access list trimmed, without repeats and sorted, so
// two lists that mean the same compare equal.
func normalizedAccess(a model.NodeAccess) model.NodeAccess {
	out := model.NodeAccess{Groups: []string{}, Clients: []uint{}}
	seen := map[string]bool{}
	for _, g := range a.Groups {
		g = strings.TrimSpace(g)
		key := strings.ToLower(g)
		if g == "" || g == clusterGroup || seen[key] {
			continue
		}
		seen[key] = true
		out.Groups = append(out.Groups, g)
	}
	sort.Slice(out.Groups, func(i, j int) bool { return strings.ToLower(out.Groups[i]) < strings.ToLower(out.Groups[j]) })
	ids := map[uint]bool{}
	for _, id := range a.Clients {
		if id != 0 && !ids[id] {
			ids[id] = true
			out.Clients = append(out.Clients, id)
		}
	}
	sort.Slice(out.Clients, func(i, j int) bool { return out.Clients[i] < out.Clients[j] })
	return out
}

func renameNodeLinkPrefix(tx *gorm.DB, oldName, newName string) error {
	oldPrefix, newPrefix := nodeLinkPrefix(oldName), nodeLinkPrefix(newName)
	var clients []model.Client
	if err := tx.Find(&clients).Error; err != nil {
		return err
	}
	for i := range clients {
		var links []map[string]string
		if json.Unmarshal(clients[i].Links, &links) != nil {
			continue
		}
		changed := false
		for _, link := range links {
			if strings.HasPrefix(link["remark"], oldPrefix) {
				link["remark"] = newPrefix + strings.TrimPrefix(link["remark"], oldPrefix)
				changed = true
			}
		}
		if !changed {
			continue
		}
		encoded, err := json.MarshalIndent(links, "", "  ")
		if err != nil {
			return err
		}
		if err := tx.Model(model.Client{}).Where("id = ?", clients[i].Id).Update("links", encoded).Error; err != nil {
			return err
		}
	}
	return nil
}
