package database

import (
	"encoding/json"
	"net"
	"net/url"
	"strings"

	"github.com/Danialrostamani/drnetwork-panel/database/model"

	"gorm.io/gorm"
)

// panelHostSentinel stands in for this machine's addresses in a backup, so a
// restore on another server does not keep the old IP.
const panelHostSentinel = "__DRNETWORK_HOST__"

// panelIPs and panelHost are the machine's own addresses and the one a restore
// should write; tests point them elsewhere.
var panelIPs = localInterfaceIPs
var panelHost = primaryPanelHost

func localInterfaceIPs() []string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, a := range addrs {
		ipnet, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		ip := ipnet.IP
		if ip == nil || ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			continue
		}
		s := ip.String()
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func primaryPanelHost() string {
	var first4, first6, pub4 string
	for _, s := range panelIPs() {
		ip := net.ParseIP(s)
		if ip == nil {
			continue
		}
		if v4 := ip.To4(); v4 != nil {
			if first4 == "" {
				first4 = v4.String()
			}
			if v4.IsGlobalUnicast() && !v4.IsPrivate() && pub4 == "" {
				pub4 = v4.String()
			}
			continue
		}
		if first6 == "" {
			first6 = ip.String()
		}
	}
	if pub4 != "" {
		return pub4
	}
	if first4 != "" {
		return first4
	}
	return first6
}

// scrubPanelHost rewrites this machine's IPs in a backup copy so the file can
// move to another server.
func scrubPanelHost(tx *gorm.DB) error {
	return rewritePanelHost(tx, panelIPs(), panelHostSentinel)
}

// fillPanelHost puts the current machine's address where a backup left the
// placeholder, after a restore.
func fillPanelHost(tx *gorm.DB) error {
	host := panelHost()
	if host == "" {
		host = "0.0.0.0"
	}
	return rewritePanelHost(tx, []string{panelHostSentinel}, host)
}

func rewritePanelHost(tx *gorm.DB, olds []string, neu string) error {
	if neu == "" || len(olds) == 0 {
		return nil
	}
	var live []string
	for _, old := range olds {
		if old == "" || old == neu {
			continue
		}
		live = append(live, old)
	}
	if len(live) == 0 {
		return nil
	}

	var settings []model.Setting
	if err := tx.Find(&settings).Error; err != nil {
		return err
	}
	for i := range settings {
		v := rewriteAll(settings[i].Value, live, neu)
		if v == settings[i].Value {
			continue
		}
		if err := tx.Model(&model.Setting{}).Where("id = ?", settings[i].Id).Update("value", v).Error; err != nil {
			return err
		}
	}

	var inbounds []model.Inbound
	if err := tx.Find(&inbounds).Error; err != nil {
		return err
	}
	for i := range inbounds {
		addrs := rewriteJSON(inbounds[i].Addrs, live, neu)
		out := rewriteJSON(inbounds[i].OutJson, live, neu)
		opt := rewriteJSON(inbounds[i].Options, live, neu)
		if bytesEqual(addrs, inbounds[i].Addrs) && bytesEqual(out, inbounds[i].OutJson) && bytesEqual(opt, inbounds[i].Options) {
			continue
		}
		if err := tx.Model(&model.Inbound{}).Where("id = ?", inbounds[i].Id).Updates(map[string]interface{}{
			"addrs":    addrs,
			"out_json": out,
			"options":  opt,
		}).Error; err != nil {
			return err
		}
	}

	var clients []model.Client
	if err := tx.Find(&clients).Error; err != nil {
		return err
	}
	for i := range clients {
		links := rewriteJSON(clients[i].Links, live, neu)
		if bytesEqual(links, clients[i].Links) {
			continue
		}
		if err := tx.Model(&model.Client{}).Where("id = ?", clients[i].Id).Update("links", links).Error; err != nil {
			return err
		}
	}
	return nil
}

func rewriteAll(s string, olds []string, neu string) string {
	for _, old := range olds {
		s = replaceHost(s, old, neu)
	}
	return s
}

func replaceHost(s, old, neu string) string {
	if s == "" || old == "" || !strings.Contains(s, old) {
		return s
	}
	if s == old {
		return neu
	}
	if h, p, err := net.SplitHostPort(s); err == nil && h == old {
		return net.JoinHostPort(neu, p)
	}
	if u, err := url.Parse(s); err == nil && u.Scheme != "" && u.Hostname() == old {
		if p := u.Port(); p != "" {
			u.Host = net.JoinHostPort(neu, p)
		} else if neu == "" {
			u.Host = ""
		} else {
			u.Host = neu
		}
		return u.String()
	}
	if strings.Contains(s, ",") {
		parts := strings.Split(s, ",")
		changed := false
		for i, p := range parts {
			trim := strings.TrimSpace(p)
			next := replaceHost(trim, old, neu)
			if next == trim {
				continue
			}
			if pad := len(p) - len(strings.TrimLeft(p, " \t")); pad > 0 {
				parts[i] = p[:pad] + next
			} else {
				parts[i] = next
			}
			changed = true
		}
		if changed {
			return strings.Join(parts, ",")
		}
	}
	return s
}

func rewriteJSON(raw []byte, olds []string, neu string) []byte {
	if len(raw) == 0 {
		return raw
	}
	var v interface{}
	if json.Unmarshal(raw, &v) != nil {
		return []byte(rewriteAll(string(raw), olds, neu))
	}
	next := rewriteValue(v, olds, neu)
	b, err := json.Marshal(next)
	if err != nil {
		return raw
	}
	return b
}

func rewriteValue(v interface{}, olds []string, neu string) interface{} {
	switch t := v.(type) {
	case string:
		return rewriteAll(t, olds, neu)
	case map[string]interface{}:
		for k, val := range t {
			t[k] = rewriteValue(val, olds, neu)
		}
		return t
	case []interface{}:
		for i, val := range t {
			t[i] = rewriteValue(val, olds, neu)
		}
		return t
	default:
		return v
	}
}

func bytesEqual(a, b []byte) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
