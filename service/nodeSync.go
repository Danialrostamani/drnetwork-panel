package service

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/util/common"
	"gorm.io/gorm"
)

// NodeSyncService owns remote inbound discovery and adoption. Client
// reconciliation and traffic collection build on this same service.
type NodeSyncService struct{ NodeService }

const nodePushTimeout = 15 * time.Second

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
	reconcileMu   sync.Mutex
	reconcileBusy = map[uint]bool{}
	reconcileLast = map[uint]time.Time{}
	dirtyGen      uint64
)

type nodeClientState struct {
	Id       uint            `json:"id"`
	Name     string          `json:"name"`
	Enable   bool            `json:"enable"`
	Config   json.RawMessage `json:"config"`
	Inbounds json.RawMessage `json:"inbounds"`
	Expiry   int64           `json:"expiry"`
	Group    string          `json:"group"`
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
	return s.runReconcile(nodeID, gen)
}

func (s *NodeSyncService) ReconcileNow(nodeID uint) error {
	gen, ok := s.claimReconcile(nodeID, true)
	if !ok {
		return common.NewError("a sync for this node is already running; try again shortly")
	}
	defer s.releaseReconcile(nodeID)
	return s.runReconcile(nodeID, gen)
}

func (s *NodeSyncService) runReconcile(nodeID uint, startGen uint64) error {
	node, err := s.getNodeByID(nodeID)
	if err != nil {
		return err
	}
	client := nodePushClient(node)
	defer closeNodeIdle(client)
	tagToID, err := s.nodeInboundTagMap(node, client)
	if err != nil {
		return err
	}
	expected, err := s.expectedClients(nodeID, tagToID)
	if err != nil {
		return err
	}
	actual, err := s.actualClusterClients(node, client)
	if err != nil {
		return err
	}
	for name, want := range expected {
		current, exists := actual[name]
		if !exists {
			if err := s.pushClient(node, client, "new", want); err != nil {
				return common.NewErrorf("push new client %s to %s: %v", name, node.Name, err)
			}
		} else if clientDiffers(want, current) {
			want["id"] = current.Id
			if err := s.pushClient(node, client, "edit", want); err != nil {
				return common.NewErrorf("push edit client %s to %s: %v", name, node.Name, err)
			}
		}
	}
	for name, current := range actual {
		if _, exists := expected[name]; !exists {
			if err := s.pushClient(node, client, "del", current.Id); err != nil {
				return common.NewErrorf("delete stale client %s from %s: %v", name, node.Name, err)
			}
		}
	}
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
	var replicas []model.Inbound
	if err := database.GetDB().Where("node_id = ?", nodeID).Find(&replicas).Error; err != nil {
		return nil, err
	}
	replicaTag := make(map[uint]string, len(replicas))
	for _, replica := range replicas {
		replicaTag[replica.Id] = replica.Tag
	}
	var clients []model.Client
	if err := database.GetDB().Find(&clients).Error; err != nil {
		return nil, err
	}
	expected := map[string]map[string]interface{}{}
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
		encodedIDs, _ := json.Marshal(remoteIDs)
		expected[client.Name] = map[string]interface{}{
			"name": client.Name, "enable": client.Enable, "config": client.Config,
			"inbounds": json.RawMessage(encodedIDs), "links": json.RawMessage("[]"),
			"volume": 0, "expiry": client.Expiry, "group": clusterGroup, "desc": client.Desc,
		}
	}
	return expected, nil
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
	return !jsonEqual(want["inbounds"], current.Inbounds)
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
