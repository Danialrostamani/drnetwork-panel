package service

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
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
