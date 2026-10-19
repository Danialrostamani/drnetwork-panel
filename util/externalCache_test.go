package util

import (
	"sync"
	"testing"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/logger"

	"github.com/op/go-logging"
)

func cacheEnv(t *testing.T) (now *time.Time, body *string, calls *int, mu *sync.Mutex) {
	t.Helper()
	logger.InitLogger(logging.ERROR)
	oldFetch, oldNow := externalFetch, externalNow
	externalMu.Lock()
	oldCache, oldSize := externalCache, externalSize
	externalCache, externalSize = map[string]*externalEntry{}, 0
	externalMu.Unlock()
	clock := time.Unix(1_000_000, 0)
	served := "one"
	n := 0
	var lock sync.Mutex
	externalFetch = func(string) string {
		lock.Lock()
		defer lock.Unlock()
		n++
		return served
	}
	externalNow = func() time.Time {
		lock.Lock()
		defer lock.Unlock()
		return clock
	}
	t.Cleanup(func() {
		externalFetch, externalNow = oldFetch, oldNow
		externalMu.Lock()
		externalCache, externalSize = oldCache, oldSize
		externalMu.Unlock()
	})
	return &clock, &served, &n, &lock
}

func waitRefresh(t *testing.T, url string) {
	t.Helper()
	for i := 0; i < 200; i++ {
		externalMu.Lock()
		busy := externalCache[url] != nil && externalCache[url].refreshes
		externalMu.Unlock()
		if !busy {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("the refresh never ended")
}

// A fresh copy is served without a fetch, a stale one at once while a refresh
// runs, and a failed refresh keeps the copy that worked -- for a day.
func TestExternalSubscriptionsAreCached(t *testing.T) {
	now, body, calls, mu := cacheEnv(t)
	const url = "https://example.org/sub"
	set := func(f func()) { mu.Lock(); f(); mu.Unlock() }
	get := func() (string, int) {
		got := GetExternalLinkCached(url)
		mu.Lock()
		defer mu.Unlock()
		return got, *calls
	}

	if got, n := get(); got != "one" || n != 1 {
		t.Fatalf("first: %q after %d fetches", got, n)
	}
	set(func() { *body = "two"; *now = now.Add(time.Minute) })
	if got, n := get(); got != "one" || n != 1 {
		t.Fatalf("fresh: %q after %d fetches", got, n)
	}
	set(func() { *now = now.Add(externalFresh) })
	if got, _ := get(); got != "one" {
		t.Fatalf("stale copy not served at once: %q", got)
	}
	waitRefresh(t, url)
	if got, n := get(); got != "two" || n != 2 {
		t.Fatalf("after the refresh: %q after %d fetches", got, n)
	}

	set(func() { *body = ""; *now = now.Add(externalFresh) })
	get()
	waitRefresh(t, url)
	if got, _ := get(); got != "two" {
		t.Fatalf("a failed refresh lost the copy: %q", got)
	}
	waitRefresh(t, url)
	set(func() { *now = now.Add(externalKeep) })
	if got, _ := get(); got != "" {
		t.Fatalf("a day-old copy still served: %q", got)
	}
}

// The cache keeps to its size, dropping the copies used longest ago.
func TestExternalCacheDropsTheOldest(t *testing.T) {
	now, body, _, mu := cacheEnv(t)
	big := string(make([]byte, externalCacheBytes/2+1))
	mu.Lock()
	*body = big
	mu.Unlock()
	GetExternalLinkCached("https://a.example/")
	mu.Lock()
	*now = now.Add(time.Second)
	mu.Unlock()
	GetExternalLinkCached("https://b.example/")
	externalMu.Lock()
	defer externalMu.Unlock()
	if externalCache["https://a.example/"] != nil || externalCache["https://b.example/"] == nil || externalSize != len(big) {
		t.Fatalf("%d copies, %d bytes", len(externalCache), externalSize)
	}
}
