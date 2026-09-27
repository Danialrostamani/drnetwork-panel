package service

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/util/common"
	"gorm.io/gorm"
)

type NodeMem struct {
	Current int64 `json:"current"`
	Total   int64 `json:"total"`
}
type NodeStatus struct {
	State           string  `json:"state"`
	Latency         int64   `json:"latency"`
	Cpu             float64 `json:"cpu"`
	Mem             NodeMem `json:"mem"`
	AppVersion      string  `json:"appVersion"`
	CoreVersion     string  `json:"coreVersion"`
	Error           string  `json:"error,omitempty"`
	CheckedAt       int64   `json:"checkedAt"`
	LastOnline      int64   `json:"lastOnline"`
	onlineUsers     []string
	onlineCheckedAt int64
}

var (
	nodeStatusMu sync.RWMutex
	nodeStatuses = map[uint]NodeStatus{}
	nodeClientMu sync.Mutex
	nodeClients  = map[uint]*http.Client{}
)

const (
	nodeProbeTimeout    = 4 * time.Second
	nodeProbeParallel   = 8
	nodeMaxResponseSize = 8 << 20
	nodeIdleConnTimeout = 90 * time.Second
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
	u := nodeAPIURL(n, action)
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Token", n.Token)
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
		return nil, common.NewError("unexpected response from node API")
	}
	if !msg.Success {
		if msg.Msg == "" {
			msg.Msg = "check the API token"
		}
		return nil, common.NewErrorf("node refused: %s", msg.Msg)
	}
	return msg.Obj, nil
}
func (s *NodeService) probe(n *model.Node, client *http.Client) NodeStatus {
	started := time.Now()
	st := NodeStatus{CheckedAt: started.Unix()}
	q := url.Values{}
	q.Set("r", "cpu,mem,sys,sbd")
	obj, err := s.nodeGet(n, client, "status", q)
	st.Latency = time.Since(started).Milliseconds()
	if err != nil {
		st.State = "offline"
		st.Error = err.Error()
		return st
	}
	var payload struct {
		Cpu float64 `json:"cpu"`
		Mem struct {
			Current int64 `json:"current"`
			Total   int64 `json:"total"`
		} `json:"mem"`
		Sys struct {
			AppVersion string `json:"appVersion"`
		} `json:"sys"`
		Sbd struct {
			Running bool   `json:"running"`
			Version string `json:"version"`
		} `json:"sbd"`
	}
	if json.Unmarshal(obj, &payload) != nil {
		st.State = "offline"
		st.Error = "unexpected status payload"
		return st
	}
	st.Cpu = payload.Cpu
	st.Mem = NodeMem{Current: payload.Mem.Current, Total: payload.Mem.Total}
	st.AppVersion = payload.Sys.AppVersion
	st.CoreVersion = payload.Sbd.Version
	if !payload.Sbd.Running {
		st.State = "core-stopped"
		return st
	}
	st.State = "online"
	if onlineObj, err := s.nodeGet(n, client, "onlines", nil); err == nil {
		var online struct {
			User []string `json:"user"`
		}
		if json.Unmarshal(onlineObj, &online) == nil {
			st.onlineUsers = online.User
			st.onlineCheckedAt = time.Now().Unix()
		}
	}
	return st
}
func (s *NodeService) RefreshAll() {
	var nodes []*model.Node
	db := database.GetDB()
	if err := db.Model(model.Node{}).Where("enable = ?", true).Find(&nodes).Error; err != nil {
		logger.Warning("nodes: load failed: ", err)
		return
	}
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
	nodeStatusMu.Lock()
	old := nodeStatuses
	for id, st := range fresh {
		if st.State == "online" {
			st.LastOnline = st.CheckedAt
		} else if prev, ok := old[id]; ok {
			st.LastOnline = prev.LastOnline
		}
		fresh[id] = st
	}
	nodeStatuses = fresh
	nodeStatusMu.Unlock()
	for id, st := range fresh {
		if prev, ok := old[id]; ok && prev.State == "online" && st.State != "online" && prev.LastOnline > 0 {
			if err := db.Model(model.Node{}).Where("id = ?", id).Update("last_seen", prev.LastOnline).Error; err != nil {
				logger.Warning("nodes: last_seen update failed: ", err)
			}
		}
	}
}
func (s *NodeService) GetStatuses() map[uint]NodeStatus {
	nodeStatusMu.RLock()
	defer nodeStatusMu.RUnlock()
	out := make(map[uint]NodeStatus, len(nodeStatuses))
	for id, st := range nodeStatuses {
		out[id] = st
	}
	return out
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
func (s *NodeService) GetAll() ([]map[string]interface{}, error) {
	var nodes []model.Node
	if err := database.GetDB().Order("id").Find(&nodes).Error; err != nil {
		return nil, err
	}
	out := make([]map[string]interface{}, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, map[string]interface{}{"id": n.Id, "enable": n.Enable, "name": n.Name, "baseUrl": n.BaseUrl, "webPath": n.WebPath, "insecure": n.Insecure, "certPin": n.CertPin, "desc": n.Desc, "lastSeen": n.LastSeen, "tokenSet": n.Token != "", "dirty": n.Dirty, "lastSync": n.LastSync})
	}
	return out, nil
}
func redactNodeToken(data json.RawMessage) json.RawMessage {
	var m map[string]interface{}
	if json.Unmarshal(data, &m) == nil {
		if t, ok := m["token"].(string); ok && t != "" {
			m["token"] = "***"
			if b, e := json.Marshal(m); e == nil {
				return b
			}
		}
	}
	return data
}
func (s *NodeService) Save(tx *gorm.DB, action string, data json.RawMessage) error {
	switch action {
	case "new", "edit":
		var n model.Node
		if err := json.Unmarshal(data, &n); err != nil {
			return err
		}
		n.Name = strings.TrimSpace(n.Name)
		n.BaseUrl = strings.TrimSpace(strings.TrimRight(n.BaseUrl, "/"))
		n.WebPath = normalizeWebPath(n.WebPath)
		n.CertPin = normalizeCertPin(n.CertPin)
		if n.Name == "" {
			return common.NewError("node name is required")
		}
		if strings.ContainsAny(n.Name, "[]") {
			return common.NewError("node name cannot contain [ or ]")
		}
		if !strings.HasPrefix(n.BaseUrl, "http://") && !strings.HasPrefix(n.BaseUrl, "https://") {
			return common.NewError("node baseUrl must start with http:// or https://")
		}
		if action == "new" {
			n.Id = 0
			n.Dirty = false
			n.LastSync = 0
			n.LastSeen = 0
		} else {
			if n.Id == 0 {
				return common.NewError("node id is required")
			}
			var old model.Node
			if err := tx.First(&old, n.Id).Error; err != nil {
				return err
			}
			if n.Token == "" {
				n.Token = old.Token
			}
			n.LastSeen = old.LastSeen
			n.Dirty = old.Dirty
			n.LastSync = old.LastSync
		}
		if n.Token == "" {
			return common.NewError("node API token is required")
		}
		if err := tx.Save(&n).Error; err != nil {
			return err
		}
		invalidateNodeClient(n.Id)
		return nil
	case "del":
		var id uint
		if err := json.Unmarshal(data, &id); err != nil {
			return err
		}
		if id == 0 {
			return common.NewError("node id is required")
		}
		if err := tx.Delete(&model.Node{}, id).Error; err != nil {
			return err
		}
		invalidateNodeClient(id)
		nodeStatusMu.Lock()
		delete(nodeStatuses, id)
		nodeStatusMu.Unlock()
		return nil
	default:
		return common.NewErrorf("unknown action: %s", action)
	}
}
