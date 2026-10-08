package service

import (
	"bytes"
	"encoding/json"
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
	"github.com/Danialrostamani/drnetwork-panel/util"
	"github.com/Danialrostamani/drnetwork-panel/util/common"
	"gorm.io/gorm"
)

// NodeSyncService owns remote inbound discovery and adoption. Client
// reconciliation and traffic collection build on this same service.
type NodeSyncService struct{ NodeService }

const nodePushTimeout = 15 * time.Second

// ClusterGroup is the client group a master gives the clients it pushes to its
// nodes. It is reserved: a hand-made group must not use it.
const ClusterGroup = clusterGroup

type remoteInbound struct {
	Id      uint   `json:"id"`
	Type    string `json:"type"`
	Tag     string `json:"tag"`
	Adopted bool   `json:"adopted"`
}

func nodePushClient(n *model.Node) *http.Client {
	client := buildNodeHTTPClient(n)
	client.Timeout = nodePushTimeout
	return client
}

func (s *NodeSyncService) getNodeByID(id uint) (*model.Node, error) {
	var node model.Node
	if err := database.GetDB().First(&node, id).Error; err != nil {
		return nil, common.NewError("node not found")
	}
	if !node.Enable {
		return nil, common.NewError("node is disabled")
	}
	return &node, nil
}

// FetchNodeInbounds lists remote inbounds and marks replicas already adopted.
func (s *NodeSyncService) FetchNodeInbounds(nodeID uint) ([]remoteInbound, error) {
	node, err := s.getNodeByID(nodeID)
	if err != nil {
		return nil, err
	}
	client := nodePushClient(node)
	defer closeNodeIdle(client)
	obj, err := s.nodeGet(node, client, "inbounds", nil)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Inbounds []struct {
			Id   uint   `json:"id"`
			Type string `json:"type"`
			Tag  string `json:"tag"`
		} `json:"inbounds"`
	}
	if err := json.Unmarshal(obj, &payload); err != nil {
		return nil, common.NewError("unexpected inbounds payload from node")
	}
	var tags []string
	if err := database.GetDB().Model(model.Inbound{}).Where("node_id = ?", nodeID).Pluck("tag", &tags).Error; err != nil {
		return nil, err
	}
	adopted := make(map[string]bool, len(tags))
	for _, tag := range tags {
		adopted[tag] = true
	}
	out := make([]remoteInbound, 0, len(payload.Inbounds))
	for _, inbound := range payload.Inbounds {
		out = append(out, remoteInbound{Id: inbound.Id, Type: inbound.Type, Tag: inbound.Tag, Adopted: adopted[inbound.Tag]})
	}
	return out, nil
}

// AdoptInbounds imports full remote panel-shape inbounds as read-only replicas.
// Tags remain unchanged because they are the stable reconciliation key.
func (s *NodeSyncService) AdoptInbounds(nodeID uint, tags []string, actor string) error {
	if len(tags) == 0 {
		return nil
	}
	node, err := s.getNodeByID(nodeID)
	if err != nil {
		return err
	}
	client := nodePushClient(node)
	defer closeNodeIdle(client)

	wanted := make(map[string]bool, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag != "" {
			wanted[tag] = true
		}
	}
	if len(wanted) == 0 {
		return common.NewError("no inbound tags selected")
	}

	listObj, err := s.nodeGet(node, client, "inbounds", nil)
	if err != nil {
		return err
	}
	var list struct {
		Inbounds []struct {
			Id  uint   `json:"id"`
			Tag string `json:"tag"`
		} `json:"inbounds"`
	}
	if err := json.Unmarshal(listObj, &list); err != nil {
		return common.NewError("unexpected inbounds payload from node")
	}
	ids := make([]string, 0, len(wanted))
	for _, inbound := range list.Inbounds {
		if wanted[inbound.Tag] {
			ids = append(ids, strconv.FormatUint(uint64(inbound.Id), 10))
		}
	}
	if len(ids) == 0 {
		return common.NewError("no matching inbounds found on the node")
	}

	fullObj, err := s.nodeGet(node, client, "inbounds", url.Values{"id": {strings.Join(ids, ",")}})
	if err != nil {
		return err
	}
	var payload struct {
		Inbounds []json.RawMessage `json:"inbounds"`
	}
	if err := json.Unmarshal(fullObj, &payload); err != nil {
		return common.NewError("unexpected full inbound payload from node")
	}

	err = database.GetDB().Transaction(func(tx *gorm.DB) error {
		adopted := 0
		for _, raw := range payload.Inbounds {
			var meta struct {
				Tag string `json:"tag"`
			}
			if err := json.Unmarshal(raw, &meta); err != nil {
				return err
			}
			if !wanted[meta.Tag] {
				continue
			}
			var count int64
			if err := tx.Model(model.Inbound{}).Where("tag = ?", meta.Tag).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return common.NewErrorf("tag %q already exists here; rename it on the node first", meta.Tag)
			}
			replica, err := buildReplicaInbound(raw, nodeID)
			if err != nil {
				return err
			}
			if err := tx.Create(replica).Error; err != nil {
				return err
			}
			adopted++
		}
		if adopted == 0 {
			return common.NewError("no matching inbounds found on the node")
		}
		audit, _ := json.Marshal(map[string]interface{}{"nodeId": nodeID, "tags": tags})
		if err := tx.Create(&model.Changes{DateTime: time.Now().Unix(), Actor: actor, Key: "inbounds", Action: "adopt", Obj: audit}).Error; err != nil {
			return err
		}
		// Claim a new dirty generation before publishing the database flag so a
		// concurrent reconcile cannot clear an adoption it did not observe.
		bumpDirtyGen()
		return tx.Model(model.Node{}).Where("id = ?", nodeID).Update("dirty", true).Error
	})
	if err == nil {
		LastUpdate = time.Now().Unix()
	}
	return err
}

// buildReplicaInbound preserves the node-side link snapshot but strips fields
// that are local-only or unsafe to feed into this panel's sing-box process.
func buildReplicaInbound(raw json.RawMessage, nodeID uint) (*model.Inbound, error) {
	var full map[string]interface{}
	if err := json.Unmarshal(raw, &full); err != nil {
		return nil, err
	}
	typ, _ := full["type"].(string)
	tag, _ := full["tag"].(string)
	if typ == "" || tag == "" {
		return nil, common.NewError("remote inbound is missing type or tag")
	}
	inbound := &model.Inbound{Type: typ, Tag: tag, NodeId: &nodeID}
	if value, ok := full["addrs"]; ok && value != nil {
		inbound.Addrs, _ = json.MarshalIndent(value, "", "  ")
	}
	if value, ok := full["out_json"]; ok && value != nil {
		inbound.OutJson, _ = json.MarshalIndent(value, "", "  ")
	}
	for _, key := range []string{"id", "tls_id", "tls", "addrs", "out_json", "users", "node_id", "type", "tag"} {
		delete(full, key)
	}
	options, err := json.MarshalIndent(full, "", "  ")
	if err != nil {
		return nil, err
	}
	inbound.Options = options
	return inbound, nil
}

const (
	clusterGroup     = "@cluster"
	reconcileBackoff = 30 * time.Second
)

var (
	reconcileMu    sync.Mutex
	reconcileBusy  = map[uint]bool{}
	reconcileLast  = map[uint]time.Time{}
	dirtyGen       uint64
	refreshLinksMu sync.Mutex
)

func nodeLinkPrefix(nodeName string) string { return "[" + nodeName + "] " }

func isNodeOwnedRemark(remark string, nodeNames []string) bool {
	for _, name := range nodeNames {
		if strings.HasPrefix(remark, nodeLinkPrefix(name)) {
			return true
		}
	}
	return false
}

func isNodeLinkFor(remark, tag string, nodeNames []string) bool {
	for _, name := range nodeNames {
		if remark == nodeLinkPrefix(name)+tag {
			return true
		}
	}
	return false
}

type nodeClientState struct {
	Id       uint            `json:"id"`
	Name     string          `json:"name"`
	Enable   bool            `json:"enable"`
	Config   json.RawMessage `json:"config"`
	Inbounds json.RawMessage `json:"inbounds"`
	Expiry   int64           `json:"expiry"`
	Group    string          `json:"group"`
	Up       int64           `json:"up"`
	Down     int64           `json:"down"`
	// Pointer distinguishes an older node that does not report this field.
	LimitIp *int `json:"limitIp"`
}

func (s *NodeSyncService) nodePost(node *model.Node, client *http.Client, action string, form url.Values) (json.RawMessage, error) {
	req, err := http.NewRequest(http.MethodPost, nodeAPIURL(node, action), strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Token", node.Token)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nodeHTTPStatusError(resp.StatusCode)
	}
	var msg struct {
		Success bool            `json:"success"`
		Msg     string          `json:"msg"`
		Obj     json.RawMessage `json:"obj"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, nodeMaxResponseSize)).Decode(&msg); err != nil {
		return nil, common.NewError("unexpected response from node")
	}
	if !msg.Success {
		if msg.Msg == "" {
			msg.Msg = "node rejected the request"
		}
		return nil, common.NewError(msg.Msg)
	}
	return msg.Obj, nil
}

func (s *NodeSyncService) pushClient(node *model.Node, client *http.Client, action string, payload interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	form := url.Values{"object": {"clients"}, "action": {action}, "data": {string(data)}}
	_, err = s.nodePost(node, client, "save", form)
	return err
}

func (s *NodeSyncService) Reconcile(nodeID uint) error {
	gen, ok := s.claimReconcile(nodeID, false)
	if !ok {
		return nil
	}
	defer s.releaseReconcile(nodeID)
	return s.runReconcile(nodeID, gen, syncAuto)
}

func (s *NodeSyncService) ReconcileNow(nodeID uint) error {
	gen, ok := s.claimReconcile(nodeID, true)
	if !ok {
		return common.NewError("a sync for this node is already running; try again shortly")
	}
	defer s.releaseReconcile(nodeID)
	return s.runReconcile(nodeID, gen, syncManual)
}

// ReconcileFull syncs a node and sends every client it has again, changed or
// not: for a node whose copies went wrong in a way the comparison misses.
func (s *NodeSyncService) ReconcileFull(nodeID uint) error {
	gen, ok := s.claimReconcile(nodeID, true)
	if !ok {
		return common.NewError("a sync for this node is already running; try again shortly")
	}
	defer s.releaseReconcile(nodeID)
	return s.runReconcile(nodeID, gen, syncFull)
}

// What started a sync.
const (
	syncAuto   = "auto"
	syncManual = "manual"
	syncFull   = "full"
	// A report names this many clients of each kind at most.
	syncReportNames = 50
)

// SyncPlan is what a sync changes on a node: the clients it creates, edits
// and deletes there, and how many it leaves as they are.
type SyncPlan struct {
	Add  []string `json:"add"`
	Edit []string `json:"edit"`
	Del  []string `json:"del"`
	Same int      `json:"same"`
}

// planSync compares the clients a node should have with those it has. full
// edits every client the node has, changed or not.
func planSync(expected map[string]map[string]interface{}, actual map[string]nodeClientState, full bool) SyncPlan {
	plan := SyncPlan{Add: []string{}, Edit: []string{}, Del: []string{}}
	for name, want := range expected {
		current, exists := actual[name]
		switch {
		case !exists:
			plan.Add = append(plan.Add, name)
		case full || clientDiffers(want, current):
			plan.Edit = append(plan.Edit, name)
		default:
			plan.Same++
		}
	}
	for name := range actual {
		if _, exists := expected[name]; !exists {
			plan.Del = append(plan.Del, name)
		}
	}
	sort.Strings(plan.Add)
	sort.Strings(plan.Edit)
	sort.Strings(plan.Del)
	return plan
}

// SyncReport is what the last sync of a node did.
type SyncReport struct {
	At int64 `json:"at"`
	// Milliseconds the sync took.
	Duration int64 `json:"duration"`
	// auto, manual or full.
	Trigger string `json:"trigger"`
	// The clients created, edited and deleted on the node; the lists stop at
	// syncReportNames names, the counts do not.
	Added        []string `json:"added"`
	Edited       []string `json:"edited"`
	Deleted      []string `json:"deleted"`
	AddedCount   int      `json:"addedCount"`
	EditedCount  int      `json:"editedCount"`
	DeletedCount int      `json:"deletedCount"`
	Unchanged    int      `json:"unchanged"`
	// Clients with one of the node's inbounds that its access list leaves out.
	Skipped int    `json:"skipped"`
	Error   string `json:"error,omitempty"`
}

func (r *SyncReport) note(list *[]string, count *int, name string) {
	*count++
	if len(*list) < syncReportNames {
		*list = append(*list, name)
	}
}

func saveSyncReport(nodeID uint, r *SyncReport) {
	raw, err := json.Marshal(r)
	if err != nil {
		return
	}
	if err := database.GetDB().Model(model.Node{}).Where("id = ?", nodeID).Update("sync_report", raw).Error; err != nil {
		logger.Warning("nodes: save sync report: ", err)
	}
}

func (s *NodeSyncService) runReconcile(nodeID uint, startGen uint64, trigger string) (err error) {
	node, err := s.getNodeByID(nodeID)
	if err != nil {
		return err
	}
	started := time.Now()
	report := SyncReport{At: started.Unix(), Trigger: trigger, Added: []string{}, Edited: []string{}, Deleted: []string{}}
	defer func() {
		report.Duration = time.Since(started).Milliseconds()
		if err != nil {
			report.Error = err.Error()
		}
		saveSyncReport(nodeID, &report)
	}()
	client := nodePushClient(node)
	defer closeNodeIdle(client)
	tagToID, err := s.nodeInboundTagMap(node, client)
	if err != nil {
		return err
	}
	if s.refreshReplicas(node, client, tagToID) {
		s.refreshNodeLinks(node)
	}
	expected, skipped, err := s.expectedClientsFor(node, tagToID)
	if err != nil {
		return err
	}
	report.Skipped = skipped
	actual, err := s.actualClusterClients(node, client)
	if err != nil {
		return err
	}
	plan := planSync(expected, actual, trigger == syncFull)
	report.Unchanged = plan.Same
	for _, name := range plan.Add {
		if err := s.pushClient(node, client, "new", expected[name]); err != nil {
			return common.NewErrorf("push new client %s to %s: %v", name, node.Name, err)
		}
		report.note(&report.Added, &report.AddedCount, name)
	}
	for _, name := range plan.Edit {
		want := expected[name]
		want["id"] = actual[name].Id
		if err := s.pushClient(node, client, "edit", want); err != nil {
			return common.NewErrorf("push edit client %s to %s: %v", name, node.Name, err)
		}
		report.note(&report.Edited, &report.EditedCount, name)
	}
	for _, name := range plan.Del {
		if err := s.pushClient(node, client, "del", actual[name].Id); err != nil {
			return common.NewErrorf("delete stale client %s from %s: %v", name, node.Name, err)
		}
		report.note(&report.Deleted, &report.DeletedCount, name)
	}
	s.refreshNodeLinks(node)
	now := time.Now().Unix()
	db := database.GetDB()
	if !s.dirtyUnchangedSince(startGen) {
		return db.Model(model.Node{}).Where("id = ?", nodeID).Update("last_sync", now).Error
	}
	if err := db.Model(model.Node{}).Where("id = ?", nodeID).Updates(map[string]interface{}{"dirty": false, "last_sync": now}).Error; err != nil {
		return err
	}
	LastUpdate = now
	return nil
}

func (s *NodeSyncService) expectedClients(nodeID uint, tagToID map[string]uint) (map[string]map[string]interface{}, error) {
	node := &model.Node{Id: nodeID}
	var stored model.Node
	if err := database.GetDB().First(&stored, nodeID).Error; err == nil {
		node = &stored
	}
	expected, _, err := s.expectedClientsFor(node, tagToID)
	return expected, err
}

// expectedClientsFor is the clients the node should have, as the master pushes
// them, and how many clients with one of its inbounds its access list leaves
// out.
func (s *NodeSyncService) expectedClientsFor(node *model.Node, tagToID map[string]uint) (map[string]map[string]interface{}, int, error) {
	var replicas []model.Inbound
	if err := database.GetDB().Where("node_id = ?", node.Id).Find(&replicas).Error; err != nil {
		return nil, 0, err
	}
	replicaTag := make(map[uint]string, len(replicas))
	for _, replica := range replicas {
		replicaTag[replica.Id] = replica.Tag
	}
	var clients []model.Client
	if err := database.GetDB().Find(&clients).Error; err != nil {
		return nil, 0, err
	}
	expected := map[string]map[string]interface{}{}
	skipped := 0
	for i := range clients {
		client := &clients[i]
		var inboundIDs []uint
		if json.Unmarshal(client.Inbounds, &inboundIDs) != nil {
			continue
		}
		var remoteIDs []uint
		for _, inboundID := range inboundIDs {
			if tag, ok := replicaTag[inboundID]; ok {
				if remoteID, exists := tagToID[tag]; exists {
					remoteIDs = append(remoteIDs, remoteID)
				}
			}
		}
		if len(remoteIDs) == 0 {
			continue
		}
		if !node.Access.Allows(client.Id, client.Group) {
			skipped++
			continue
		}
		encodedIDs, _ := json.Marshal(remoteIDs)
		expected[client.Name] = map[string]interface{}{
			"name": client.Name, "enable": client.Enable, "config": client.Config,
			"inbounds": json.RawMessage(encodedIDs), "links": json.RawMessage("[]"),
			"volume": 0, "expiry": client.Expiry, "group": clusterGroup, "desc": client.Desc,
			"limitIp": client.LimitIp,
		}
	}
	return expected, skipped, nil
}

// SyncPreview is what a sync of the node would do now, and what stands in its
// way.
type SyncPreview struct {
	SyncPlan
	// Inbounds adopted from the node, and the tags of those it no longer has.
	Replicas int      `json:"replicas"`
	Missing  []string `json:"missing"`
	// The node serves only some clients; Skipped is how many it leaves out.
	Restricted bool `json:"restricted"`
	Skipped    int  `json:"skipped"`
}

// PreviewSync tells what a sync of the node would change, without changing
// anything.
func (s *NodeSyncService) PreviewSync(nodeID uint) (*SyncPreview, error) {
	node, err := s.getNodeByID(nodeID)
	if err != nil {
		return nil, err
	}
	client := nodePushClient(node)
	defer closeNodeIdle(client)
	tagToID, err := s.nodeInboundTagMap(node, client)
	if err != nil {
		return nil, err
	}
	expected, skipped, err := s.expectedClientsFor(node, tagToID)
	if err != nil {
		return nil, err
	}
	actual, err := s.actualClusterClients(node, client)
	if err != nil {
		return nil, err
	}
	var replicas []model.Inbound
	if err := database.GetDB().Select("id", "tag").Where("node_id = ?", nodeID).Find(&replicas).Error; err != nil {
		return nil, err
	}
	out := &SyncPreview{SyncPlan: planSync(expected, actual, false), Replicas: len(replicas), Missing: []string{}, Restricted: node.Access.Restricted(), Skipped: skipped}
	for _, r := range replicas {
		if _, ok := tagToID[r.Tag]; !ok {
			out.Missing = append(out.Missing, r.Tag)
		}
	}
	sort.Strings(out.Missing)
	return out, nil
}

// GetSyncReport is what the last sync of the node did; nil before the first.
func (s *NodeSyncService) GetSyncReport(nodeID uint) (*SyncReport, error) {
	var node model.Node
	if err := database.GetDB().Select("id", "sync_report").First(&node, nodeID).Error; err != nil {
		return nil, common.NewError("node not found")
	}
	if len(node.SyncReport) == 0 {
		return nil, nil
	}
	var r SyncReport
	if err := json.Unmarshal(node.SyncReport, &r); err != nil {
		return nil, nil
	}
	return &r, nil
}

func (s *NodeSyncService) actualClusterClients(node *model.Node, client *http.Client) (map[string]nodeClientState, error) {
	obj, err := s.nodeGet(node, client, "clients", url.Values{"full": {"1"}})
	if err != nil {
		return nil, err
	}
	var payload struct {
		Clients []nodeClientState `json:"clients"`
	}
	if err := json.Unmarshal(obj, &payload); err != nil {
		return nil, common.NewError("unexpected clients payload from node")
	}
	out := map[string]nodeClientState{}
	for _, client := range payload.Clients {
		if client.Group == clusterGroup {
			out[client.Name] = client
		}
	}
	return out, nil
}

func (s *NodeSyncService) nodeInboundTagMap(node *model.Node, client *http.Client) (map[string]uint, error) {
	obj, err := s.nodeGet(node, client, "inbounds", nil)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Inbounds []struct {
			Id  uint   `json:"id"`
			Tag string `json:"tag"`
		} `json:"inbounds"`
	}
	if err := json.Unmarshal(obj, &payload); err != nil {
		return nil, common.NewError("unexpected inbounds payload from node")
	}
	out := make(map[string]uint, len(payload.Inbounds))
	for _, inbound := range payload.Inbounds {
		out[inbound.Tag] = inbound.Id
	}
	return out, nil
}

func clientDiffers(want map[string]interface{}, current nodeClientState) bool {
	enable, _ := want["enable"].(bool)
	if enable != current.Enable {
		return true
	}
	expiry, _ := want["expiry"].(int64)
	if expiry != current.Expiry {
		return true
	}
	if config, ok := want["config"].(json.RawMessage); ok && len(config) > 0 && len(current.Config) > 0 && !jsonEqual(config, current.Config) {
		return true
	}
	if !jsonEqual(want["inbounds"], current.Inbounds) {
		return true
	}
	if current.LimitIp != nil {
		if limit, ok := want["limitIp"].(int); !ok || limit != *current.LimitIp {
			return true
		}
	}
	return false
}

func jsonEqual(a interface{}, b json.RawMessage) bool {
	encoded, err := json.Marshal(a)
	if raw, ok := a.(json.RawMessage); ok {
		encoded = raw
	}
	if err != nil {
		return false
	}
	var left, right interface{}
	if json.Unmarshal(encoded, &left) != nil || json.Unmarshal(b, &right) != nil {
		return false
	}
	leftJSON, _ := json.Marshal(left)
	rightJSON, _ := json.Marshal(right)
	return string(leftJSON) == string(rightJSON)
}

func genNodeReplicaLinks(replica *model.Inbound, client *model.Client) (links []string) {
	defer func() {
		if recovered := recover(); recovered != nil {
			logger.Warning("nodes: link generation panic for ", replica.Tag, ": ", recovered)
			links = nil
		}
	}()
	if len(replica.OutJson) == 0 {
		return nil
	}
	var outbound map[string]interface{}
	if err := json.Unmarshal(replica.OutJson, &outbound); err != nil || outbound == nil {
		return nil
	}
	server, _ := outbound["server"].(string)
	if server == "" {
		return nil
	}
	base := map[string]interface{}{"server": server, "server_port": outbound["server_port"]}
	if tlsConfig, ok := outbound["tls"].(map[string]interface{}); ok {
		if _, valid := tlsConfig["enabled"].(bool); valid {
			base["tls"] = tlsConfig
		}
	}
	var addressBook []map[string]interface{}
	if len(replica.Addrs) > 0 {
		_ = json.Unmarshal(replica.Addrs, &addressBook)
	}
	addresses := make([]map[string]interface{}, 0, len(addressBook))
	for _, address := range addressBook {
		if address == nil {
			continue
		}
		if _, exists := address["server"]; !exists {
			address["server"] = base["server"]
		}
		if _, exists := address["server_port"]; !exists {
			address["server_port"] = base["server_port"]
		}
		if _, exists := address["tls"]; !exists {
			if tlsConfig, ok := base["tls"]; ok {
				address["tls"] = tlsConfig
			}
		}
		addresses = append(addresses, address)
	}
	if len(addresses) == 0 {
		addresses = []map[string]interface{}{base}
	}
	synthetic := *replica
	synthetic.TlsId = 0
	synthetic.Tls = nil
	synthetic.Addrs, _ = json.Marshal(addresses)
	return util.LinkGenerator(client.Config, &synthetic, server, client.Remark)
}

func (s *NodeSyncService) refreshReplicas(node *model.Node, client *http.Client, tagToID map[string]uint) bool {
	var replicas []model.Inbound
	if err := database.GetDB().Where("node_id = ?", node.Id).Find(&replicas).Error; err != nil {
		logger.Warning("nodes: load replicas for refresh: ", err)
		return false
	}
	byTag := make(map[string]*model.Inbound, len(replicas))
	ids := make([]string, 0, len(replicas))
	for i := range replicas {
		replica := &replicas[i]
		remoteID, exists := tagToID[replica.Tag]
		if !exists {
			continue
		}
		byTag[replica.Tag] = replica
		ids = append(ids, strconv.FormatUint(uint64(remoteID), 10))
	}
	if len(ids) == 0 {
		return false
	}
	obj, err := s.nodeGet(node, client, "inbounds", url.Values{"id": {strings.Join(ids, ",")}})
	if err != nil {
		logger.Warning("nodes: refresh replicas from ", node.Name, ": ", err)
		return false
	}
	var payload struct {
		Inbounds []json.RawMessage `json:"inbounds"`
	}
	if json.Unmarshal(obj, &payload) != nil {
		logger.Warning("nodes: unexpected full inbound payload from ", node.Name)
		return false
	}
	touched := false
	for _, raw := range payload.Inbounds {
		fresh, err := buildReplicaInbound(raw, node.Id)
		if err != nil {
			logger.Warning("nodes: parse refreshed replica: ", err)
			continue
		}
		current, exists := byTag[fresh.Tag]
		if !exists {
			continue
		}
		if fresh.Type == current.Type && bytes.Equal(fresh.Options, current.Options) && bytes.Equal(fresh.OutJson, current.OutJson) && bytes.Equal(fresh.Addrs, current.Addrs) {
			continue
		}
		audit, _ := json.Marshal(map[string]interface{}{"tag": fresh.Tag, "node": node.Name})
		if err := database.GetDB().Transaction(func(tx *gorm.DB) error {
			if err := tx.Model(model.Inbound{}).Where("id = ?", current.Id).Updates(map[string]interface{}{"type": fresh.Type, "options": fresh.Options, "out_json": fresh.OutJson, "addrs": fresh.Addrs}).Error; err != nil {
				return err
			}
			return tx.Create(&model.Changes{DateTime: time.Now().Unix(), Actor: "NodeSync", Key: "inbounds", Action: "edit", Obj: audit}).Error
		}); err != nil {
			logger.Warning("nodes: update replica ", fresh.Tag, ": ", err)
			continue
		}
		touched = true
	}
	if touched {
		LastUpdate = time.Now().Unix()
	}
	return touched
}

func (s *NodeSyncService) refreshNodeLinks(node *model.Node) {
	refreshLinksMu.Lock()
	defer refreshLinksMu.Unlock()
	var replicas []model.Inbound
	if err := database.GetDB().Where("node_id = ?", node.Id).Find(&replicas).Error; err != nil {
		logger.Warning("nodes: load replicas for links: ", err)
		return
	}
	replicaByID := make(map[uint]*model.Inbound, len(replicas))
	for i := range replicas {
		replicaByID[replicas[i].Id] = &replicas[i]
	}
	var clients []model.Client
	if err := database.GetDB().Find(&clients).Error; err != nil {
		logger.Warning("nodes: load clients for links: ", err)
		return
	}
	prefix := nodeLinkPrefix(node.Name)
	touched := false
	for i := range clients {
		client := &clients[i]
		var inboundIDs []uint
		_ = json.Unmarshal(client.Inbounds, &inboundIDs)
		desired := []map[string]string{}
		if !node.Access.Allows(client.Id, client.Group) {
			// The node does not serve this client: none of its links.
			inboundIDs = nil
		}
		for _, inboundID := range inboundIDs {
			replica, exists := replicaByID[inboundID]
			if !exists {
				continue
			}
			for _, uri := range genNodeReplicaLinks(replica, client) {
				desired = append(desired, map[string]string{"remark": prefix + replica.Tag, "type": "external", "uri": uri})
			}
		}
		var existing []map[string]string
		_ = json.Unmarshal(client.Links, &existing)
		hadPrefix := false
		kept := make([]map[string]string, 0, len(existing))
		for _, link := range existing {
			if link["type"] != "local" && strings.HasPrefix(link["remark"], prefix) {
				hadPrefix = true
				continue
			}
			kept = append(kept, link)
		}
		if len(desired) == 0 && !hadPrefix {
			continue
		}
		merged := append(kept, desired...)
		newLinks, err := json.MarshalIndent(merged, "", "  ")
		if err != nil || jsonEqual(client.Links, newLinks) {
			continue
		}
		audit, _ := json.Marshal(map[string]interface{}{"name": client.Name, "node": node.Name})
		if err := database.GetDB().Transaction(func(tx *gorm.DB) error {
			if err := tx.Model(model.Client{}).Where("id = ?", client.Id).Update("links", newLinks).Error; err != nil {
				return err
			}
			return tx.Create(&model.Changes{DateTime: time.Now().Unix(), Actor: "NodeSync", Key: "clients", Action: "edit", Obj: audit}).Error
		}); err != nil {
			logger.Warning("nodes: refresh links for ", client.Name, ": ", err)
			continue
		}
		touched = true
	}
	if touched {
		LastUpdate = time.Now().Unix()
	}
}

type trafficBaseline struct {
	Up   int64 `json:"up"`
	Down int64 `json:"down"`
}

// CollectTraffic folds each online node's cumulative @cluster counters into
// the master's client totals. Persisted baselines make restarts idempotent.
func (s *NodeSyncService) CollectTraffic() {
	var nodes []model.Node
	if err := database.GetDB().Where("enable = ?", true).Find(&nodes).Error; err != nil {
		return
	}
	statuses := s.GetStatuses()
	for i := range nodes {
		node := &nodes[i]
		if status, ok := statuses[node.Id]; !ok || status.State != "online" {
			continue
		}
		if err := s.collectNodeTraffic(node); err != nil {
			logger.Warning("nodes: collect traffic from ", node.Name, ": ", err)
		}
	}
}

func (s *NodeSyncService) collectNodeTraffic(node *model.Node) error {
	client := nodePushClient(node)
	defer closeNodeIdle(client)
	current, err := s.actualClusterClients(node, client)
	if err != nil {
		return err
	}
	baseline := map[string]trafficBaseline{}
	if len(node.Baselines) > 0 {
		_ = json.Unmarshal(node.Baselines, &baseline)
	}
	var names []string
	db := database.GetDB()
	if err := db.Model(model.Client{}).Pluck("name", &names).Error; err != nil {
		return err
	}
	owned := make(map[string]bool, len(names))
	for _, name := range names {
		owned[name] = true
	}
	newBaseline := map[string]trafficBaseline{}
	type delta struct{ up, down int64 }
	deltas := map[string]delta{}
	for name, remote := range current {
		newBaseline[name] = trafficBaseline{Up: remote.Up, Down: remote.Down}
		if !owned[name] {
			continue
		}
		previous := baseline[name]
		up := remote.Up - previous.Up
		down := remote.Down - previous.Down
		if up < 0 {
			up = remote.Up
		}
		if down < 0 {
			down = remote.Down
		}
		if up > 0 || down > 0 {
			deltas[name] = delta{up: up, down: down}
		}
	}
	encoded, err := json.Marshal(newBaseline)
	if err != nil {
		return err
	}
	// A client that moved traffic through the node, or that the node currently
	// reports as connected, is online right now. Without this the master's
	// "Last online" column only ever reflected sessions on the master itself.
	now := time.Now().Unix()
	seenOnline := map[string]bool{}
	for name := range deltas {
		seenOnline[name] = true
	}
	for _, name := range nodeFreshOnlineUsers(node.Id, now) {
		if owned[name] {
			seenOnline[name] = true
		}
	}
	return db.Transaction(func(tx *gorm.DB) error {
		for name, value := range deltas {
			if err := tx.Model(model.Client{}).Where("name = ?", name).Updates(map[string]interface{}{
				"up":        gorm.Expr("up + ?", value.up),
				"down":      gorm.Expr("down + ?", value.down),
				"online_at": now,
			}).Error; err != nil {
				return err
			}
		}
		for name := range seenOnline {
			if _, hasDelta := deltas[name]; hasDelta {
				continue
			}
			if err := tx.Model(model.Client{}).Where("name = ?", name).Update("online_at", now).Error; err != nil {
				return err
			}
		}
		return tx.Model(model.Node{}).Where("id = ?", node.Id).Update("baselines", encoded).Error
	})
}

// nodeFreshOnlineUsers returns the users a node reported as connected within
// the online TTL.
func nodeFreshOnlineUsers(nodeID uint, now int64) []string {
	nodeStatusMu.RLock()
	defer nodeStatusMu.RUnlock()
	status, ok := nodeStatuses[nodeID]
	if !ok || status.State != "online" || status.onlineCheckedAt <= 0 || now-status.onlineCheckedAt > int64(nodeOnlineTTL.Seconds()) {
		return nil
	}
	return append([]string(nil), status.onlineUsers...)
}

func (s *NodeSyncService) MarkAllDirty() {
	bumpDirtyGen()
	if err := database.GetDB().Model(model.Node{}).Where("enable = ? AND dirty = ?", true, false).Update("dirty", true).Error; err != nil {
		logger.Warning("nodes: mark dirty failed: ", err)
	}
}

func (s *NodeSyncService) ReconcileDirtyOnline() {
	var nodes []model.Node
	if database.GetDB().Select("id").Where("enable = ? AND dirty = ?", true, true).Find(&nodes).Error != nil {
		return
	}
	statuses := s.GetStatuses()
	for _, node := range nodes {
		if status, ok := statuses[node.Id]; ok && status.State == "online" {
			id := node.Id
			go func() {
				if err := s.Reconcile(id); err != nil {
					logger.Warning("nodes: reconcile failed: ", err)
				}
			}()
		}
	}
}

// ReconcileDirtyOnlineWait syncs the online nodes marked dirty, all at once,
// and returns when they are done.
func (s *NodeSyncService) ReconcileDirtyOnlineWait() {
	var nodes []model.Node
	if database.GetDB().Select("id").Where("enable = ? AND dirty = ?", true, true).Find(&nodes).Error != nil {
		return
	}
	statuses := s.GetStatuses()
	var wg sync.WaitGroup
	for _, node := range nodes {
		if status, ok := statuses[node.Id]; ok && status.State == "online" {
			wg.Add(1)
			go func(id uint) {
				defer wg.Done()
				if err := s.Reconcile(id); err != nil {
					logger.Warning("nodes: reconcile failed: ", err)
				}
			}(node.Id)
		}
	}
	wg.Wait()
}

func (s *NodeSyncService) ReconcileAllOnline() {
	var nodes []model.Node
	if database.GetDB().Select("id").Where("enable = ?", true).Find(&nodes).Error != nil {
		return
	}
	statuses := s.GetStatuses()
	for _, node := range nodes {
		if status, ok := statuses[node.Id]; ok && status.State == "online" {
			if err := s.Reconcile(node.Id); err != nil {
				logger.Warning("nodes: safety reconcile failed: ", err)
			}
		}
	}
}

func (s *NodeSyncService) claimReconcile(nodeID uint, force bool) (uint64, bool) {
	reconcileMu.Lock()
	defer reconcileMu.Unlock()
	if reconcileBusy[nodeID] {
		return 0, false
	}
	if !force && time.Since(reconcileLast[nodeID]) < reconcileBackoff {
		return 0, false
	}
	reconcileBusy[nodeID] = true
	return dirtyGen, true
}
func bumpDirtyGen() { reconcileMu.Lock(); dirtyGen++; reconcileMu.Unlock() }
func (s *NodeSyncService) dirtyUnchangedSince(gen uint64) bool {
	reconcileMu.Lock()
	defer reconcileMu.Unlock()
	return dirtyGen == gen
}
func (s *NodeSyncService) releaseReconcile(nodeID uint) {
	reconcileMu.Lock()
	reconcileBusy[nodeID] = false
	reconcileLast[nodeID] = time.Now()
	reconcileMu.Unlock()
}
