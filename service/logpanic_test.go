package service

import (
	"sync"
	"testing"
)

// A panic in a goroutine that defers LogPanic ends that goroutine only; the
// deferred calls before it still run.
func TestLogPanicKeepsTheProcessAlive(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer LogPanic("test")
		var m map[string]int
		m["boom"] = 1 // assignment to a nil map panics
	}()
	wg.Wait()
}
