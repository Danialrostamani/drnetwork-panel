package core

import (
	"math"
	"sync/atomic"
)

// noQuota is the limit of a user without a volume cap.
const noQuota = math.MaxInt64

// userQuota holds a user to their remaining volume in the data path itself.
// The panel learns about traffic only when the stats job drains the counters
// and acts on it seconds later, and at line rate a user went that much past the
// volume. Checked on every read and write, the volume runs out at the byte.
type userQuota struct {
	// used is every byte the user moved through this core, both ways. It is
	// never drained, so a limit can be set against it.
	used atomic.Int64
	// limit is the value of used at which the volume runs out.
	limit atomic.Int64
	// tripped is set while a cut-off for the user is pending or done, so one
	// is not started per read.
	tripped atomic.Bool
	// atDrain is used as it was at the last GetStats: the point up to which the
	// database has the user's traffic. Guarded by SessionTracker.access.
	atDrain int64
}

func newUserQuota() *userQuota {
	q := &userQuota{}
	q.limit.Store(noQuota)
	return q
}

func (q *userQuota) exhausted() bool {
	return q.used.Load() >= q.limit.Load()
}

// SatAdd adds without wrapping around.
func SatAdd(a, b int64) int64 {
	if b > 0 && a > math.MaxInt64-b {
		return math.MaxInt64
	}
	if b < 0 && a < math.MinInt64-b {
		return math.MinInt64
	}
	return a + b
}

// loadOrCreateQuota is called with t.access held.
func (t *SessionTracker) loadOrCreateQuota(user string) *userQuota {
	q, loaded := t.quotas[user]
	if !loaded {
		q = newUserQuota()
		t.quotas[user] = q
	}
	return q
}

// chargeFunc counts the user's traffic against the quota, and cuts the user
// off as soon as it runs out. The cut runs on its own goroutine: this is
// called inside reads and writes, where closing connections could deadlock.
func (t *SessionTracker) chargeFunc(user string, q *userQuota) func(int64) {
	return func(n int64) {
		if q.used.Add(n) >= q.limit.Load() && q.tripped.CompareAndSwap(false, true) {
			go t.exhaust(user, q)
		}
	}
}

// exhaust cuts off a user who ran out of volume: every routed connection, and
// the protocol sessions that carry them.
func (t *SessionTracker) exhaust(user string, q *userQuota) {
	// The limit may have been raised (a renewal) since the trip.
	if !q.exhausted() {
		q.tripped.Store(false)
		return
	}
	t.CloseByUser(user)
	if t.onExhausted != nil {
		t.onExhausted(user)
	}
}

// SetQuotas sets how many more bytes each user may move, counted from the last
// GetStats -- the traffic the database already has. Users missing from the map
// have no cap. A user already past the new cap is cut off at once; one whose
// cap went up (a renewal or a reset) is let through again.
func (t *SessionTracker) SetQuotas(remaining map[string]int64) {
	type trip struct {
		user string
		q    *userQuota
	}
	var trips []trip
	t.access.Lock()
	for user := range remaining {
		if user != "" {
			t.loadOrCreateQuota(user)
		}
	}
	for user, q := range t.quotas {
		limit := int64(noQuota)
		if r, ok := remaining[user]; ok {
			limit = SatAdd(q.atDrain, r)
		}
		q.limit.Store(limit)
		if q.used.Load() < limit {
			q.tripped.Store(false)
		} else if q.tripped.CompareAndSwap(false, true) {
			trips = append(trips, trip{user, q})
		}
	}
	t.access.Unlock()
	for _, it := range trips {
		go t.exhaust(it.user, it.q)
	}
}

// Exhausted reports whether the user has run out of the volume set by
// SetQuotas.
func (t *SessionTracker) Exhausted(user string) bool {
	t.access.Lock()
	q := t.quotas[user]
	t.access.Unlock()
	return q != nil && q.exhausted()
}
