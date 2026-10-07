package service

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
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
	"github.com/Danialrostamani/drnetwork-panel/util/common"
	"gorm.io/gorm"
)

// What the panel does to a node through its API, one node or many at once.

// NodeActionResult is how one node took an action given to several.
type NodeActionResult struct {
	Id    uint   `json:"id"`
	Name  string `json:"name"`
	Ok    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

const (
	nodeActionParallel = 4
	// A node's database can be big, and it is built before it is sent.
	nodeBackupTimeout = 3 * time.Minute
	nodeBackupMaxSize = 1 << 30
	// Testing an outbound opens a connection through it.
	nodeCheckTimeout = 30 * time.Second
	nodeLogsMax      = 1000
)

var nodeActionNames = map[string]bool{
	"probe": true, "restartSb": true, "restartApp": true, "maintenanceOn": true, "maintenanceOff": true,
	"enable": true, "disable": true, "sync": true, "fullSync": true,
}

// nodeRemoteActions change something on the node, so the node is probed again
// soon after to show it.
var nodeRemoteActions = map[string]bool{"restartSb": true, "restartApp": true, "maintenanceOn": true, "maintenanceOff": true}

func uniqueNodeIDs(ids []uint) []uint {
	seen := map[uint]bool{}
	out := make([]uint, 0, len(ids))
	for _, id := range ids {
		if id != 0 && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// NodeAction runs one action on each of the nodes and tells how each took it.
func (s *NodeSyncService) NodeAction(ids []uint, action, actor string) ([]NodeActionResult, error) {
	if !nodeActionNames[action] {
		return nil, common.NewErrorf("unknown node action: %s", action)
	}
	ids = uniqueNodeIDs(ids)
	if len(ids) == 0 {
		return nil, common.NewError("no node selected")
	}
	var nodes []model.Node
	if err := database.GetDB().Where("id IN ?", ids).Order("id").Find(&nodes).Error; err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, common.NewError("node not found")
	}
	switch action {
	case "enable", "disable":
		return s.setNodesEnabled(nodes, action == "enable", actor)
	case "probe":
		enabled := make([]uint, 0, len(nodes))
		for _, n := range nodes {
			if n.Enable {
				enabled = append(enabled, n.Id)
			}
		}
		statuses, err := s.ProbeNow(enabled)
		if err != nil {
			return nil, err
		}
		results := make([]NodeActionResult, 0, len(nodes))
		for _, n := range nodes {
			r := NodeActionResult{Id: n.Id, Name: n.Name}
			st, ok := statuses[n.Id]
			switch {
			case !n.Enable:
				r.Error = "node is disabled"
			case !ok:
				r.Error = "not probed"
			case st.State == "online":
				r.Ok = true
			case st.Error != "":
				r.Error = st.Error
			default:
				r.Error = st.State
			}
			results = append(results, r)
		}
		return results, nil
	}
	results := make([]NodeActionResult, len(nodes))
	var wg sync.WaitGroup
	sem := make(chan struct{}, nodeActionParallel)
	for i := range nodes {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			n := &nodes[i]
			r := NodeActionResult{Id: n.Id, Name: n.Name}
			if !n.Enable {
				r.Error = "node is disabled"
			} else if err := s.nodeActionOne(n, action); err != nil {
				r.Error = err.Error()
			} else {
				r.Ok = true
			}
			results[i] = r
		}(i)
	}
	wg.Wait()
	if nodeRemoteActions[action] {
		// A failed request may still have restarted something, so every node
		// the action went to is probed, the disabled ones excepted.
		var asked []uint
		for i := range nodes {
			if nodes[i].Enable {
				asked = append(asked, nodes[i].Id)
			}
		}
		if len(asked) > 0 {
			s.probeSoon(asked, 3*time.Second)
		}
	}
	return results, nil
}

// actionProbes counts the probes that actions left running.
var actionProbes sync.WaitGroup

// WaitNodeActionProbes returns once the probes started after node actions
// have finished, so nothing writes a node's status after this point.
func WaitNodeActionProbes() {
	actionProbes.Wait()
}

// probeSoon probes the nodes again after a while, to show what an action did.
func (s *NodeSyncService) probeSoon(ids []uint, after time.Duration) {
	actionProbes.Add(1)
	go func() {
		defer actionProbes.Done()
		defer func() {
			if r := recover(); r != nil {
				logger.Error("nodes: probe after action: ", r)
			}
		}()
		time.Sleep(after)
		if _, err := s.ProbeNow(ids); err != nil {
			logger.Warning("nodes: probe after action: ", err)
		}
	}()
}

func (s *NodeSyncService) nodeActionOne(n *model.Node, action string) error {
	switch action {
	case "sync":
		return s.ReconcileNow(n.Id)
	case "fullSync":
		return s.ReconcileFull(n.Id)
	}
	client := nodePushClient(n)
	defer closeNodeIdle(client)
	var err error
	switch action {
	case "restartSb":
		_, err = s.nodePost(n, client, "restartSb", url.Values{})
	case "restartApp":
		_, err = s.nodePost(n, client, "restartApp", url.Values{})
	case "maintenanceOn":
		_, err = s.nodePost(n, client, "maintenance", url.Values{"enable": {"true"}})
	case "maintenanceOff":
		_, err = s.nodePost(n, client, "maintenance", url.Values{"enable": {"false"}})
	default:
		err = common.NewErrorf("unknown node action: %s", action)
	}
	return err
}

// setNodesEnabled turns nodes on or off. A node turned back on has missed the
// syncs made while it was off, so it is synced again.
func (s *NodeSyncService) setNodesEnabled(nodes []model.Node, enable bool, actor string) ([]NodeActionResult, error) {
	results := make([]NodeActionResult, 0, len(nodes))
	var changed []uint
	now := time.Now().Unix()
	err := database.GetDB().Transaction(func(tx *gorm.DB) error {
		for _, n := range nodes {
			results = append(results, NodeActionResult{Id: n.Id, Name: n.Name, Ok: true})
			if n.Enable == enable {
				continue
			}
			updates := map[string]interface{}{"enable": enable}
			if enable {
				updates["dirty"] = true
			}
			if err := tx.Model(model.Node{}).Where("id = ?", n.Id).Updates(updates).Error; err != nil {
				return err
			}
			audit, _ := json.Marshal(map[string]interface{}{"id": n.Id, "name": n.Name, "enable": enable})
			if err := tx.Create(&model.Changes{DateTime: now, Actor: actor, Key: "nodes", Action: "edit", Obj: audit}).Error; err != nil {
				return err
			}
			changed = append(changed, n.Id)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(changed) > 0 {
		if enable {
			bumpDirtyGen()
		}
		for _, id := range changed {
			invalidateNodeClient(id)
		}
		invalidateNodeLinkRules()
		LastUpdate = time.Now().Unix()
		if enable {
			s.probeSoon(changed, 0)
		}
	}
	return results, nil
}

// ---- one node at a time ----

// NodeLogs is the tail of a node's log.
func (s *NodeSyncService) NodeLogs(id uint, count int, level string) (json.RawMessage, error) {
	node, err := s.getNodeByID(id)
	if err != nil {
		return nil, err
	}
	if count < 1 {
		count = 100
	}
	if count > nodeLogsMax {
		count = nodeLogsMax
	}
	switch level {
	case "", "debug", "info", "warning", "warn", "error", "err":
	default:
		return nil, common.NewError("unknown log level")
	}
	client := nodePushClient(node)
	defer closeNodeIdle(client)
	q := url.Values{"c": {strconv.Itoa(count)}}
	if level != "" {
		q.Set("l", level)
	}
	return s.nodeGet(node, client, "logs", q)
}

// NodeChanges is the change history a node keeps.
func (s *NodeSyncService) NodeChanges(id uint, actor, key string, count int) (json.RawMessage, error) {
	node, err := s.getNodeByID(id)
	if err != nil {
		return nil, err
	}
	if count < 1 || count > nodeLogsMax {
		count = 100
	}
	client := nodePushClient(node)
	defer closeNodeIdle(client)
	q := url.Values{"c": {strconv.Itoa(count)}}
	if actor != "" {
		q.Set("a", actor)
	}
	if key != "" {
		q.Set("k", key)
	}
	return s.nodeGet(node, client, "changes", q)
}

// NodeOutbound is an outbound or an endpoint of a node, which its core can
// test.
type NodeOutbound struct {
	Tag  string `json:"tag"`
	Type string `json:"type"`
	// "outbound" or "endpoint".
	Kind string `json:"kind"`
}

// NodeOutbounds lists the outbounds and the endpoints of a node.
func (s *NodeSyncService) NodeOutbounds(id uint) ([]NodeOutbound, error) {
	node, err := s.getNodeByID(id)
	if err != nil {
		return nil, err
	}
	client := nodePushClient(node)
	defer closeNodeIdle(client)
	out := []NodeOutbound{}
	for _, kind := range []string{"outbounds", "endpoints"} {
		raw, err := s.nodeGet(node, client, kind, nil)
		if err != nil {
			if kind == "endpoints" {
				// Older nodes may not list endpoints; the outbounds still help.
				logger.Warning("nodes: endpoints of ", node.Name, ": ", err)
				continue
			}
			return nil, err
		}
		var payload map[string][]struct {
			Tag  string `json:"tag"`
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			return nil, common.NewErrorf("unexpected %s payload from node", kind)
		}
		for _, o := range payload[kind] {
			if o.Tag != "" {
				out = append(out, NodeOutbound{Tag: o.Tag, Type: o.Type, Kind: strings.TrimSuffix(kind, "s")})
			}
		}
	}
	return out, nil
}

// NodeCheckOutbound has a node's core test one of its outbounds.
func (s *NodeSyncService) NodeCheckOutbound(id uint, tag string) (json.RawMessage, error) {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return nil, common.NewError("missing outbound tag")
	}
	node, err := s.getNodeByID(id)
	if err != nil {
		return nil, err
	}
	client := buildNodeHTTPClient(node)
	client.Timeout = nodeCheckTimeout
	defer closeNodeIdle(client)
	return s.nodeGet(node, client, "checkOutbound", url.Values{"tag": {tag}})
}

// NodeOnlines are who and what the last probe found connected to a node.
type NodeOnlines struct {
	User      []string `json:"user"`
	Inbound   []string `json:"inbound"`
	Outbound  []string `json:"outbound"`
	CheckedAt int64    `json:"checkedAt"`
}

func (s *NodeService) GetNodeOnlines(id uint) NodeOnlines {
	st, _ := s.GetStatus(id)
	out := NodeOnlines{User: []string{}, Inbound: []string{}, Outbound: []string{}}
	if st.State != "online" {
		return out
	}
	out.User = append(out.User, st.onlineUsers...)
	out.Inbound = append(out.Inbound, st.onlineInbounds...)
	out.Outbound = append(out.Outbound, st.onlineOutbounds...)
	sort.Strings(out.User)
	sort.Strings(out.Inbound)
	sort.Strings(out.Outbound)
	out.CheckedAt = st.onlineCheckedAt
	return out
}

// ---- backups ----

var sqliteMagic = []byte("SQLite format 3\x00")

// NodeBackup downloads the database of one node.
func (s *NodeSyncService) NodeBackup(id uint, exclude string) (*model.Node, []byte, error) {
	node, err := s.getNodeByID(id)
	if err != nil {
		return nil, nil, err
	}
	data, err := fetchNodeDb(node, exclude)
	return node, data, err
}

// fetchNodeDb asks a node for its database. A node that cannot make it
// answers with an error message in JSON instead.
func fetchNodeDb(node *model.Node, exclude string) ([]byte, error) {
	client := buildNodeHTTPClient(node)
	client.Timeout = nodeBackupTimeout
	defer closeNodeIdle(client)
	u := nodeAPIURL(node, "getdb")
	if exclude != "" {
		u += "?" + url.Values{"exclude": {exclude}}.Encode()
	}
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Token", node.Token)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nodeHTTPStatusError(resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, nodeBackupMaxSize+1))
	if err != nil {
		return nil, err
	}
	if len(body) > nodeBackupMaxSize {
		return nil, common.NewError("the node's database is larger than 1 GB")
	}
	if bytes.HasPrefix(body, sqliteMagic) {
		return body, nil
	}
	var msg struct {
		Success bool   `json:"success"`
		Msg     string `json:"msg"`
	}
	if json.Unmarshal(body, &msg) == nil && msg.Msg != "" {
		return nil, common.NewErrorf("node refused: %s", msg.Msg)
	}
	return nil, common.NewError("the node did not send a database")
}

// safeFileName keeps a node's name usable as a file name.
func safeFileName(name string) string {
	name = strings.Map(func(r rune) rune {
		switch {
		case r < 32, strings.ContainsRune(`/\:*?"<>|`, r):
			return '_'
		}
		return r
	}, strings.TrimSpace(name))
	if name == "" || name == "." || name == ".." {
		return "node"
	}
	return name
}

// NodeBackupName is the file name a node's database downloads as.
func NodeBackupName(node *model.Node, at time.Time) string {
	return "s-ui_" + safeFileName(node.Name) + "_" + at.Format("20060102-150405") + ".db"
}

// BackupAll makes the master's database first, so an error there is reported
// before anything is sent, and returns what writes the zip: the master's
// database, then the database of each enabled node. A node that cannot be
// reached gets a text file that says why, in place of its database.
func (s *NodeSyncService) BackupAll(exclude string) (func(w io.Writer) error, error) {
	master, err := database.GetDb(exclude)
	if err != nil {
		return nil, err
	}
	var nodes []model.Node
	if err := database.GetDB().Where("enable = ?", true).Order("id").Find(&nodes).Error; err != nil {
		return nil, err
	}
	return func(w io.Writer) error {
		now := time.Now()
		zw := zip.NewWriter(w)
		add := func(name string, data []byte) error {
			f, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: now})
			if err != nil {
				return err
			}
			_, err = f.Write(data)
			return err
		}
		if err := add("master.db", master); err != nil {
			return err
		}
		for i := range nodes {
			n := &nodes[i]
			base := fmt.Sprintf("nodes/%d-%s", n.Id, safeFileName(n.Name))
			data, err := fetchNodeDb(n, exclude)
			if err != nil {
				logger.Warning("nodes: backup of ", n.Name, ": ", err)
				if err := add(base+".error.txt", []byte(err.Error()+"\n")); err != nil {
					return err
				}
				continue
			}
			if err := add(base+".db", data); err != nil {
				return err
			}
		}
		return zw.Close()
	}, nil
}
