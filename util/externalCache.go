package util

import (
	"sync"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/logger"
)

// The subscriptions used to fetch every external subscription a client has on
// each of the client's own updates: a slow source held the update up to the
// ten-second timeout, and a source that was down took its servers out of the
// subscription until it came back. GetExternalLinkCached keeps the last good
// copy of each source: a fresh one is served as it is, a stale one is served
// at once while a refresh runs behind it, and a failed refresh keeps the copy
// that worked.
const (
	externalFresh = 5 * time.Minute
	// A copy older than this is not served even when the source is down.
	externalKeep = 24 * time.Hour
	// Total bytes of the cached copies; past it the oldest go.
	externalCacheBytes = 64 << 20
)

type externalEntry struct {
	body      string
	fetched   time.Time
	used      time.Time
	refreshes bool
}

var (
	externalMu    sync.Mutex
	externalCache = map[string]*externalEntry{}
	externalSize  int
	// externalFetch is GetExternalLink; tests replace it.
	externalFetch = GetExternalLink
	externalNow   = time.Now
)

// GetExternalLinkCached is GetExternalLink with the cache described above. Only
// the first request for a source, or one whose copy is too old to serve,
// waits for the fetch.
func GetExternalLinkCached(url string) string {
	externalMu.Lock()
	now := externalNow()
	entry := externalCache[url]
	if entry != nil && now.Sub(entry.fetched) < externalKeep {
		entry.used = now
		body := entry.body
		if now.Sub(entry.fetched) >= externalFresh && !entry.refreshes {
			entry.refreshes = true
			go refreshExternal(url)
		}
		externalMu.Unlock()
		return body
	}
	externalMu.Unlock()
	return refreshExternal(url)
}

// refreshExternal fetches a source and stores what came back, keeping the
// copy it had when the fetch fails. It returns the copy now cached.
func refreshExternal(url string) string {
	body := externalFetch(url)
	externalMu.Lock()
	defer externalMu.Unlock()
	now := externalNow()
	entry := externalCache[url]
	if body == "" {
		if entry != nil {
			entry.refreshes = false
			if now.Sub(entry.fetched) < externalKeep {
				logger.Warning("sub: external subscription failed, serving the copy from ", entry.fetched.Format(time.RFC3339), ": ", url)
				return entry.body
			}
			externalSize -= len(entry.body)
			delete(externalCache, url)
		}
		return ""
	}
	if entry != nil {
		externalSize -= len(entry.body)
	}
	externalCache[url] = &externalEntry{body: body, fetched: now, used: now}
	externalSize += len(body)
	for externalSize > externalCacheBytes && len(externalCache) > 1 {
		evictOldestExternal(url)
	}
	return body
}

// evictOldestExternal drops the copy used longest ago, other than keep.
func evictOldestExternal(keep string) {
	var oldest string
	var at time.Time
	for url, e := range externalCache {
		if url == keep {
			continue
		}
		if oldest == "" || e.used.Before(at) {
			oldest, at = url, e.used
		}
	}
	if oldest == "" {
		return
	}
	externalSize -= len(externalCache[oldest].body)
	delete(externalCache, oldest)
}
