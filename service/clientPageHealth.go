package service

import (
	"context"
	"net"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
)

// What the subscription page tells a user about the servers: whether each of
// their locations works, and whether their own traffic comes out of one.

// ServerHealth is one location of a client and how it is doing: "ok",
// "busy" (the server is loaded) or "down".
type ServerHealth struct {
	Name  string `json:"name"`
	State string `json:"state"`
}

// Past these a server counts as busy.
const (
	busyCPU = 85.0
	busyMem = 0.92
)

// MasterCoreUp tells whether the master's core is running.
func MasterCoreUp() bool { return corePtr != nil && corePtr.IsRunning() }

// ClientServers is the state of each location of a client.
func (s *StatsService) ClientServers(locs []ClientLocation) []ServerHealth {
	if len(locs) == 0 {
		return nil
	}
	statuses := (&NodeService{}).GetStatuses()
	out := make([]ServerHealth, 0, len(locs))
	for _, l := range locs {
		state := "down"
		if l.Node == 0 {
			if MasterCoreUp() {
				state = "ok"
				if (&ServerService{}).GetCpuPercent() >= busyCPU {
					state = "busy"
				}
			}
		} else if st, ok := statuses[l.Node]; ok && st.State == "online" && !st.Maintenance {
			state = "ok"
			if st.Cpu >= busyCPU || (st.Mem.Total > 0 && float64(st.Mem.Current)/float64(st.Mem.Total) >= busyMem) {
				state = "busy"
			}
		}
		out = append(out, ServerHealth{Name: l.Name, State: state})
	}
	return out
}

// ClientDevices is how many addresses a client with an address limit is
// connected from now, and the limit; limit 0 when it has none.
func ClientDevices(c model.Client) (used, limit int) {
	if c.LimitIp <= 0 {
		return 0, 0
	}
	return GetIPCounts()[c.Name], c.LimitIp
}

const serverIPsTTL = 5 * time.Minute

var (
	serverIPsMu sync.Mutex
	serverIPsAt time.Time
	serverIPs   map[netip.Addr]uint
)

// ServerOf is the server (node ID, 0 for the master) an address belongs to:
// traffic that comes out of one of them comes through the VPN. An IPv6
// address matches its /64, where a server's outgoing addresses live.
func ServerOf(raw string) (uint, bool) {
	addr, err := netip.ParseAddr(strings.TrimSpace(raw))
	if err != nil {
		return 0, false
	}
	addr = addr.Unmap()
	ips := serverIPList()
	if id, ok := ips[addr]; ok {
		return id, true
	}
	if addr.Is6() {
		p, _ := addr.Prefix(64)
		for ip, id := range ips {
			if ip.Is6() && p.Contains(ip) {
				return id, true
			}
		}
	}
	return 0, false
}

func serverIPList() map[netip.Addr]uint {
	serverIPsMu.Lock()
	defer serverIPsMu.Unlock()
	if serverIPs != nil && time.Since(serverIPsAt) < serverIPsTTL {
		return serverIPs
	}
	out := map[netip.Addr]uint{}
	add := func(raw string, id uint) {
		raw = strings.TrimSpace(raw)
		if i := strings.IndexByte(raw, '/'); i >= 0 {
			raw = raw[:i]
		}
		a, err := netip.ParseAddr(raw)
		if err != nil {
			return
		}
		a = a.Unmap()
		if a.IsLoopback() || a.IsPrivate() || a.IsLinkLocalUnicast() || a.IsUnspecified() {
			return
		}
		if _, seen := out[a]; !seen {
			out[a] = id
		}
	}
	if addrs, err := net.InterfaceAddrs(); err == nil {
		for _, a := range addrs {
			add(a.String(), 0)
		}
	}
	var nodes []model.Node
	if db := database.GetDB(); db != nil {
		_ = db.Select("id", "base_url").Find(&nodes).Error
	}
	statuses := (&NodeService{}).GetStatuses()
	for _, n := range nodes {
		st := statuses[n.Id]
		for _, ip := range append(append([]string{}, st.IPv4...), st.IPv6...) {
			add(ip, n.Id)
		}
		if u, err := url.Parse(n.BaseUrl); err == nil && u.Hostname() != "" {
			host := u.Hostname()
			if _, err := netip.ParseAddr(host); err == nil {
				add(host, n.Id)
				continue
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			found, _ := net.DefaultResolver.LookupIPAddr(ctx, host)
			cancel()
			for _, ip := range found {
				add(ip.IP.String(), n.Id)
			}
		}
	}
	serverIPs, serverIPsAt = out, time.Now()
	return out
}
