package sub

import (
	"net"
	"sync"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"

	"github.com/gin-gonic/gin"
)

// A subscription link names its client by a random token. The old links name
// it by the client's name, which can be guessed, so those are let through only
// while subNameLinks is on, and an address that keeps asking for names that do
// not exist is cut off from them for a while. Tokens are never cut off: they
// cannot be guessed, and an address shared behind a CDN or NAT must keep
// working for everybody else on it.
const (
	subFailLimit  = 30
	subFailWindow = 10 * time.Minute
	subFailBlock  = 15 * time.Minute
	// The table forgets the oldest addresses past this many.
	subFailMaxIPs = 10000
)

type subFailEntry struct {
	count        int
	windowStart  time.Time
	blockedUntil time.Time
}

type subFailTable struct {
	mu      sync.Mutex
	entries map[string]*subFailEntry
	now     func() time.Time
}

var subFails = &subFailTable{entries: map[string]*subFailEntry{}, now: time.Now}

func (t *subFailTable) blocked(ip string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	e := t.entries[ip]
	return e != nil && t.now().Before(e.blockedUntil)
}

func (t *subFailTable) fail(ip string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()
	e := t.entries[ip]
	if e == nil {
		if len(t.entries) >= subFailMaxIPs {
			t.prune(now)
		}
		e = &subFailEntry{windowStart: now}
		t.entries[ip] = e
	}
	if now.Sub(e.windowStart) > subFailWindow {
		e.count, e.windowStart = 0, now
	}
	e.count++
	if e.count >= subFailLimit {
		e.blockedUntil = now.Add(subFailBlock)
		e.count, e.windowStart = 0, now
	}
}

// prune drops the addresses that are neither blocked nor in a live window,
// and when that is not enough, everything: a flood of addresses must not grow
// the table without end.
func (t *subFailTable) prune(now time.Time) {
	for ip, e := range t.entries {
		if now.After(e.blockedUntil) && now.Sub(e.windowStart) > subFailWindow {
			delete(t.entries, ip)
		}
	}
	if len(t.entries) >= subFailMaxIPs {
		t.entries = map[string]*subFailEntry{}
	}
}

// subRequestIP is the address a subscription request counts against: the
// connection's, or the forwarded one when the connection comes from a proxy
// on this machine or the local network. A forwarded address from anywhere
// else could be anything the sender liked.
func subRequestIP(c *gin.Context) string {
	remote := c.RemoteIP()
	if ip := net.ParseIP(remote); ip != nil && (ip.IsLoopback() || ip.IsPrivate()) {
		if fwd := c.ClientIP(); fwd != "" {
			return fwd
		}
	}
	return remote
}

// resolveSub finds the name of the client a subscription link is for: by its
// token, or by its name while the old links are allowed.
func (s *SubHandler) resolveSub(c *gin.Context, subID string) (string, bool) {
	if subID == "" {
		return "", false
	}
	db := database.GetDB()
	var client model.Client
	if len(subID) == model.SubTokenLength {
		if err := db.Select("name").Where("sub_token = ?", subID).First(&client).Error; err == nil {
			return client.Name, true
		}
	}
	ip := subRequestIP(c)
	if !s.SettingService.GetSubNameLinks() {
		subFails.fail(ip)
		return "", false
	}
	if subFails.blocked(ip) {
		return "", false
	}
	if err := db.Select("name").Where("name = ?", subID).First(&client).Error; err == nil {
		return client.Name, true
	}
	subFails.fail(ip)
	return "", false
}
