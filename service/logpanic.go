package service

import (
	"runtime/debug"

	"github.com/Danialrostamani/drnetwork-panel/logger"
)

// LogPanic, deferred first thing in a goroutine, logs a panic instead of
// letting it end the process. gin recovers its handlers and the cron jobs run
// under cron.Recover, but a goroutine started from either is on its own: a
// panic while reading one node's malformed answer used to take down the whole
// panel, and the core with it.
func LogPanic(what string) {
	if r := recover(); r != nil {
		logger.Error(what, ": panic: ", r, "\n", string(debug.Stack()))
	}
}
