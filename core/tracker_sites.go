package core

import (
	"net/netip"
	"strings"
	"sync/atomic"

	"golang.org/x/net/publicsuffix"
)

// siteKey is one user's traffic to one site.
type siteKey struct {
	user string
	site string
}

// siteCounter counts a user's traffic to a site. refs is how many live
// sessions count into it: a drain only forgets a counter no session holds, so
// no byte is counted into a counter that is gone.
type siteCounter struct {
	up, down *atomic.Int64
	refs     int
}

// SiteStat is what a user moved to and from one site since the last drain.
type SiteStat struct {
	User string
	Site string
	Up   int64
	Down int64
}

// siteOf is the site a connection is for: the registrable domain of its name
// (www.google.com and rr3.googlevideo.com are google.com and googlevideo.com),
// or its IP address when it had no name.
func siteOf(domain, destination string) string {
	domain = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(domain)), ".")
	if domain != "" {
		if _, err := netip.ParseAddr(domain); err == nil {
			return domain
		}
		if site, err := publicsuffix.EffectiveTLDPlusOne(domain); err == nil {
			return site
		}
		return domain
	}
	if ap, err := netip.ParseAddrPort(destination); err == nil {
		return ap.Addr().Unmap().String()
	}
	if a, err := netip.ParseAddr(destination); err == nil {
		return a.Unmap().String()
	}
	return ""
}

// siteCounterLocked is the counter of a user's site, held once more by the
// session about to count into it. The caller holds t.access.
func (t *SessionTracker) siteCounterLocked(key siteKey) *siteCounter {
	if t.sites == nil {
		t.sites = make(map[siteKey]*siteCounter)
	}
	c, ok := t.sites[key]
	if !ok {
		c = &siteCounter{up: &atomic.Int64{}, down: &atomic.Int64{}}
		t.sites[key] = c
	}
	c.refs++
	return c
}

// releaseSiteLocked lets go of a session's site counter. The caller holds
// t.access.
func (t *SessionTracker) releaseSiteLocked(session *Session) {
	if session.site == nil {
		return
	}
	session.site.refs--
	session.site = nil
}

// GetSiteStats drains the per-site counters, like GetStats does the others,
// and forgets the counters no live session holds any more.
func (t *SessionTracker) GetSiteStats() []SiteStat {
	t.access.Lock()
	defer t.access.Unlock()
	var out []SiteStat
	for key, c := range t.sites {
		up, down := c.up.Swap(0), c.down.Swap(0)
		if up > 0 || down > 0 {
			out = append(out, SiteStat{User: key.user, Site: key.site, Up: up, Down: down})
		}
		if c.refs <= 0 {
			delete(t.sites, key)
		}
	}
	return out
}
