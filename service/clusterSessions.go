package service

import (
	"encoding/json"
	"net/url"
	"strconv"
	"sync"

	"github.com/Danialrostamani/drnetwork-panel/core"
	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/logger"
	"github.com/Danialrostamani/drnetwork-panel/util/common"
)

// ClusterSession is a live connection on the master or, with Node set, on a
// managed node.
type ClusterSession struct {
	core.SessionInfo
	Node string `json:"node,omitempty"`
}

// onlineNodes returns the enabled nodes the last probe found online.
func onlineNodes() []*model.Node {
	var nodes []*model.Node
	if err := database.GetDB().Where("enable = ?", true).Order("sort_order, id").Find(&nodes).Error; err != nil {
		logger.Warning("sessions: load nodes: ", err)
		return nil
	}
	nodeStatusMu.RLock()
	defer nodeStatusMu.RUnlock()
	out := nodes[:0]
	for _, n := range nodes {
		if nodeStatuses[n.Id].State == "online" {
			out = append(out, n)
		}
	}
	return out
}

// GetClusterSessions is GetSessions with, for a client, its connections on
// the managed nodes as well: a client served by the nodes has none on the
// master, so the list there used to come up empty.
func (s *StatsService) GetClusterSessions(resource, tag string) ([]ClusterSession, error) {
	local, err := s.GetSessions(resource, tag)
	if err != nil {
		return nil, err
	}
	out := make([]ClusterSession, 0, len(local))
	for _, l := range local {
		out = append(out, ClusterSession{SessionInfo: l})
	}
	if (resource != "" && resource != "user") || tag == "" {
		return out, nil
	}
	nodes := onlineNodes()
	remote := make([][]ClusterSession, len(nodes))
	var wg sync.WaitGroup
	var ns NodeService
	for i, n := range nodes {
		wg.Add(1)
		go func(i int, n *model.Node) {
			defer wg.Done()
			raw, err := ns.nodeGet(n, nodeHTTPClient(n), "sessions", url.Values{"resource": {"user"}, "tag": {tag}})
			if err != nil {
				logger.Debug("sessions: node ", n.Name, ": ", err)
				return
			}
			var list []core.SessionInfo
			if json.Unmarshal(raw, &list) != nil {
				return
			}
			for _, si := range list {
				// The ids are the node's; prefixed so two nodes never clash.
				si.ID = strconv.FormatUint(uint64(n.Id), 10) + ":" + si.ID
				remote[i] = append(remote[i], ClusterSession{SessionInfo: si, Node: n.Name})
			}
		}(i, n)
	}
	wg.Wait()
	for _, r := range remote {
		out = append(out, r...)
	}
	return out, nil
}

// CloseClusterSessions cuts a client's connections on the master and on every
// online node.
func (s *StatsService) CloseClusterSessions(user string) error {
	if user == "" {
		return common.NewError("empty user name")
	}
	localErr := s.CloseUserSessions(user)
	nodes := onlineNodes()
	if len(nodes) == 0 {
		return localErr
	}
	var (
		wg     sync.WaitGroup
		mu     sync.Mutex
		failed []string
		syncer NodeSyncService
	)
	for _, n := range nodes {
		wg.Add(1)
		go func(n *model.Node) {
			defer wg.Done()
			if _, err := syncer.nodePost(n, nodeHTTPClient(n), "closeSessions", url.Values{"u": {user}}); err != nil {
				mu.Lock()
				failed = append(failed, n.Name+": "+err.Error())
				mu.Unlock()
			}
		}(n)
	}
	wg.Wait()
	if len(failed) > 0 {
		return common.NewError("some nodes did not disconnect: ", failed)
	}
	return nil
}
