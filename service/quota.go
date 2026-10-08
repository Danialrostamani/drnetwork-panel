package service

import (
	"sync"

	"github.com/Danialrostamani/drnetwork-panel/core"
	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/logger"
	"github.com/Danialrostamani/drnetwork-panel/util/common"
)

// The core holds every client with a volume to what is left of it, byte by
// byte (core.SessionTracker.SetQuotas). The volume used to be enforced only
// after the stats job had saved the traffic and the deplete check had seen it,
// seconds later, and a fast download went that far past the volume.

var (
	// quotaMu keeps one drain of the core's counters and the commit of that
	// traffic together, so the quotas are always set against a database that
	// has the traffic exactly up to the last drain.
	quotaMu sync.Mutex
	// nodeQuotas is, on a node, what the master last allowed each of its
	// clients here, as a total of the client's up+down on this node. Guarded by
	// quotaMu.
	nodeQuotas map[string]int64
)

// liveSessionTracker returns the running core's session tracker, or nil.
func liveSessionTracker() *core.SessionTracker {
	if corePtr == nil || !corePtr.IsRunning() {
		return nil
	}
	box := corePtr.GetInstance()
	if box == nil {
		return nil
	}
	return box.SessionTracker()
}

// pendingUserTraffic is the traffic per user that was drained from the core
// but could not be committed yet.
func pendingUserTraffic() map[string]int64 {
	statsMu.Lock()
	defer statsMu.Unlock()
	if len(pendingStats) == 0 {
		return nil
	}
	pending := map[string]int64{}
	for _, stat := range pendingStats {
		if stat.Resource == "user" {
			pending[stat.Tag] += stat.Traffic
		}
	}
	return pending
}

// clientQuotas works out how many bytes each capped client has left: its own
// volume, and on a node also what the master allows it here, whichever is
// less. A client past its volume gets zero or less, which keeps it cut off.
func clientQuotas(clients []model.Client, pending map[string]int64, fromMaster map[string]int64) map[string]int64 {
	remaining := make(map[string]int64)
	for _, c := range clients {
		used := c.Up + c.Down + pending[c.Name]
		left, capped := int64(0), false
		if c.Volume > 0 {
			left, capped = c.Volume-used, true
		}
		if total, ok := fromMaster[c.Name]; ok {
			if l := total - used; !capped || l < left {
				left, capped = l, true
			}
		}
		if !capped {
			continue
		}
		if prev, dup := remaining[c.Name]; dup && prev < left {
			left = prev
		}
		remaining[c.Name] = left
	}
	return remaining
}

// refreshQuotasLocked hands the core the current quotas. Called with quotaMu
// held. On a read error the core keeps the quotas it has.
func refreshQuotasLocked(st *core.SessionTracker) {
	db := database.GetDB()
	if st == nil || db == nil {
		return
	}
	var clients []model.Client
	query := db.Model(model.Client{}).Select("name", "volume", "up", "down")
	if len(nodeQuotas) == 0 {
		query = query.Where("volume > 0")
	}
	if err := query.Find(&clients).Error; err != nil {
		logger.Debug("quota: read clients: ", err)
		return
	}
	st.SetQuotas(clientQuotas(clients, pendingUserTraffic(), nodeQuotas))
}

// RefreshQuotas sets the quotas at once, on a core that was just started. It
// does not wait for a stats cycle that is still saving: that one hands over
// the quotas itself when done, and the next one reaches the new core.
func RefreshQuotas() {
	if !quotaMu.TryLock() {
		return
	}
	defer quotaMu.Unlock()
	refreshQuotasLocked(liveSessionTracker())
}

// ApplyNodeQuotas takes from the master what each of its clients may use on
// this node, as a total of the client's up+down here, and holds the clients to
// it at once. Each call replaces the previous one: a client left out has no
// cap from the master.
func ApplyNodeQuotas(totals map[string]int64) error {
	for name := range totals {
		if name == "" {
			return common.NewError("quota for an empty client name")
		}
	}
	quotaMu.Lock()
	defer quotaMu.Unlock()
	nodeQuotas = totals
	refreshQuotasLocked(liveSessionTracker())
	return nil
}

// masterQuotaTotals is, for the master, what each capped client of a node may
// use there: the client's total on the node as just collected, plus what it
// has left overall. Read after the node's traffic is in the database, so the
// remainder already accounts for it.
func masterQuotaTotals(current map[string]nodeClientState) (map[string]int64, error) {
	if len(current) == 0 {
		return nil, nil
	}
	var clients []model.Client
	if err := database.GetDB().Model(model.Client{}).Select("name", "volume", "up", "down").Where("volume > 0").Find(&clients).Error; err != nil {
		return nil, err
	}
	left := clientQuotas(clients, nil, nil)
	totals := make(map[string]int64)
	for name, remote := range current {
		if l, ok := left[name]; ok {
			totals[name] = core.SatAdd(remote.Up+remote.Down, l)
		}
	}
	return totals, nil
}
