package sub

import (
	"encoding/json"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/service"
	"github.com/Danialrostamani/drnetwork-panel/util"
)

// The link name template (subNameTemplate) names every link the panel makes
// for a client -- its own inbounds and the replicas of node inbounds -- the
// same way in the base64 list, the sing-box JSON and the Clash config.
// Links added by hand and outside subscriptions keep their own names.

// linkInfo is what a template knows of one link besides the client.
type linkInfo struct {
	inbound, route, node, protocol, network, server string
	port                                            int
}

// as is the same link for another protocol: a mixed inbound gives a socks
// and an http one.
func (l *linkInfo) as(protocol string) *linkInfo {
	c := *l
	c.protocol = protocol
	return &c
}

func (l *linkInfo) vars(client map[string]string) map[string]string {
	v := make(map[string]string, len(client)+8)
	for k, val := range client {
		v[k] = val
	}
	v["INBOUND"], v["ROUTE"], v["NODE"] = l.inbound, l.route, l.node
	v["PROTOCOL"], v["NETWORK"], v["SERVER"] = l.protocol, l.network, util.NormalizeHost(l.server)
	if l.port > 0 {
		v["PORT"] = strconv.Itoa(l.port)
	}
	return v
}

// namer hands out the names of one subscription.
type namer struct {
	tpl   string
	vars  map[string]string
	taken map[string]bool
	// nodes are the names of the nodes, read when first needed: only links
	// of a real node are the panel's to rename.
	nodes map[string]bool
}

func newNamer(c *model.Client) *namer {
	ss := &service.SettingService{}
	tpl, _ := ss.GetSubNameTemplate()
	return newNamerWith(c, tpl)
}

func newNamerWith(c *model.Client, tpl string) *namer {
	n := &namer{tpl: tpl, taken: map[string]bool{}}
	if tpl != "" && c != nil {
		loc, _ := (&service.SettingService{}).GetTimeLocation()
		n.vars = util.ClientNameVars(c, time.Now(), loc)
	}
	return n
}

func (n *namer) active() bool { return n != nil && n.tpl != "" }

// reserve marks names the subscription uses for something else.
func (n *namer) reserve(names ...string) {
	for _, name := range names {
		n.taken[name] = true
	}
}

func (n *namer) isNode(name string) bool {
	if n.nodes == nil {
		n.nodes = map[string]bool{}
		var names []string
		if db := database.GetDB(); db != nil {
			db.Model(model.Node{}).Pluck("name", &names)
		}
		for _, v := range names {
			n.nodes[v] = true
		}
	}
	return n.nodes[name]
}

// name is the name of a link: the template's when it gives one, otherwise
// def. unique numbers a name that is already taken.
func (n *namer) name(info *linkInfo, def string, unique bool) string {
	out := def
	if n.active() && info != nil {
		if r := util.RenderName(n.tpl, info.vars(n.vars)); r != "" {
			out = r
		}
	}
	if unique {
		out = uniqueOutboundTag(out, n.taken)
	}
	n.taken[out] = true
	return out
}

// renameOutbounds gives every outbound its final tag and returns the tags in
// order. Outbounds without info keep their tag, numbered if a generated one
// took it first: sing-box and Clash both refuse a tag used twice.
func (n *namer) renameOutbounds(outbounds []map[string]interface{}, infos []*linkInfo) []string {
	tags := make([]string, len(outbounds))
	for i, out := range outbounds {
		def, _ := out["tag"].(string)
		var info *linkInfo
		if i < len(infos) {
			info = infos[i]
		}
		tag := n.name(info, def, true)
		out["tag"] = tag
		tags[i] = tag
	}
	return tags
}

var nodeRemarkRe = regexp.MustCompile(`^\[(.+?)\] (.+)$`)

// schemeProtocol maps a link scheme to the inbound type it comes from.
func schemeProtocol(scheme string) string {
	switch strings.ToLower(scheme) {
	case "ss":
		return "shadowsocks"
	case "hy2", "hysteria2":
		return "hysteria2"
	case "hy", "hysteria":
		return "hysteria"
	case "socks", "socks5", "socks4", "socks4a":
		return "socks"
	case "http", "https":
		return "http"
	case "naive+https", "naive+quic", "naive", "http2":
		return "naive"
	}
	return strings.ToLower(scheme)
}

func quicProtocol(p string) bool { return p == "hysteria" || p == "hysteria2" || p == "tuic" }

// routeOf takes the address remark back out of a generated name, which is
// util.JoinRemark(client remark, inbound tag + address remark).
func routeOf(name, clientRemark, tag string) string {
	if r, ok := strings.CutPrefix(name, util.JoinRemark(clientRemark, tag)); ok {
		return strings.TrimSpace(r)
	}
	return ""
}

// parseLink reads a generated link: what the template needs and its name.
func parseLink(uri string) (info *linkInfo, name string, ok bool) {
	scheme, rest, found := strings.Cut(uri, "://")
	if !found {
		return nil, "", false
	}
	if strings.EqualFold(scheme, "vmess") {
		raw, err := util.B64StrToByte(rest)
		if err != nil {
			return nil, "", false
		}
		var obj map[string]interface{}
		if json.Unmarshal(raw, &obj) != nil {
			return nil, "", false
		}
		info = &linkInfo{protocol: "vmess"}
		info.server, _ = obj["add"].(string)
		switch p := obj["port"].(type) {
		case string:
			info.port, _ = strconv.Atoi(p)
		case float64:
			info.port = int(p)
		}
		info.network, _ = obj["net"].(string)
		if t, _ := obj["type"].(string); t == "http" && info.network == "tcp" {
			info.network = "http"
		}
		name, _ = obj["ps"].(string)
		return info, name, true
	}
	u, err := url.Parse(uri)
	if err != nil {
		return nil, "", false
	}
	info = &linkInfo{protocol: schemeProtocol(scheme), server: u.Hostname()}
	info.port, _ = strconv.Atoi(u.Port())
	if strings.EqualFold(scheme, "http2") {
		// The naive "http2" link hides user, host and port in base64.
		info.server, info.port = "", 0
		end := strings.IndexAny(rest, "?#")
		if end < 0 {
			end = len(rest)
		}
		if raw, err := util.B64StrToByte(rest[:end]); err == nil {
			if inner, err := url.Parse("naive://" + string(raw)); err == nil {
				info.server = inner.Hostname()
				info.port, _ = strconv.Atoi(inner.Port())
			}
		}
	}
	info.network = u.Query().Get("type")
	if info.network == "" {
		info.network = "tcp"
		if quicProtocol(info.protocol) || strings.EqualFold(scheme, "naive+quic") {
			info.network = "quic"
		}
	}
	return info, u.Fragment, true
}

// setLinkName replaces the name of a link.
func setLinkName(uri, name string) string {
	scheme, rest, found := strings.Cut(uri, "://")
	if !found {
		return uri
	}
	if strings.EqualFold(scheme, "vmess") {
		raw, err := util.B64StrToByte(rest)
		if err != nil {
			return uri
		}
		var obj map[string]interface{}
		if json.Unmarshal(raw, &obj) != nil {
			return uri
		}
		obj["ps"] = name
		out, err := json.MarshalIndent(obj, "", "  ")
		if err != nil {
			return uri
		}
		return "vmess://" + util.ByteToB64Str(out)
	}
	base, _, _ := strings.Cut(uri, "#")
	frag := (&url.URL{Fragment: name}).String()
	return base + frag
}

// infoForLink is the template's view of a stored link of a client: its own
// inbounds' links ("local") and the links of node replicas ("external" with
// a "[node] tag" remark). Other links get nil and keep their names.
func (n *namer) infoForLink(link Link, clientRemark string) (*linkInfo, string) {
	var node, tag string
	switch link.Type {
	case "local":
		tag = link.Remark
	case "external":
		m := nodeRemarkRe.FindStringSubmatch(link.Remark)
		if m == nil || !n.isNode(m[1]) {
			return nil, ""
		}
		node, tag = m[1], m[2]
	default:
		return nil, ""
	}
	info, name, ok := parseLink(link.Uri)
	if !ok {
		return nil, ""
	}
	info.node, info.inbound = node, tag
	info.route = routeOf(name, clientRemark, tag)
	return info, name
}

// PreviewNames renders a template for the links of a client, the way its
// base64 subscription would name them.
func PreviewNames(c *model.Client, tpl string) ([]string, error) {
	if err := util.CheckNameTemplate(tpl); err != nil {
		return nil, err
	}
	n := newNamerWith(c, tpl)
	var links []Link
	if len(c.Links) > 0 {
		if err := json.Unmarshal(c.Links, &links); err != nil {
			return nil, err
		}
	}
	out := []string{}
	for _, l := range links {
		info, def := n.infoForLink(l, c.Remark)
		if info == nil {
			continue
		}
		out = append(out, n.name(info, def, true))
	}
	return out, nil
}
