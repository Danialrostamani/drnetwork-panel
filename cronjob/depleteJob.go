package cronjob

import (
	"sync"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/logger"
	"github.com/Danialrostamani/drnetwork-panel/service"
)

type DepleteJob struct {
	service.ClientService
	service.InboundService
	service.NodeSyncService
	service.StatsService
}

func NewDepleteJob() *DepleteJob {
	return new(DepleteJob)
}

// The minute job and the quick check may both find work; one at a time.
var depleteMu sync.Mutex

// kickDelay is how long after the first disconnect the depleted clients are
// disconnected once more, for a connection that slipped in while the nodes
// were being synced.
var kickDelay = 15 * time.Second

func (s *DepleteJob) Run() {
	depleteMu.Lock()
	defer depleteMu.Unlock()
	inboundIds, users, err := s.ClientService.DepleteClients()
	if err != nil {
		logger.Warning("Disable depleted users failed: ", err)
		return
	}
	if len(inboundIds) > 0 {
		err := s.InboundService.UpdateInboundsUsers(database.GetDB(), inboundIds)
		if err != nil {
			logger.Error("unable to update inbound users: ", err)
		}
	}
	if len(users) == 0 {
		return
	}
	// The nodes serve these clients too: sync them now, then drop the open
	// connections of the clients everywhere, so nobody keeps using a running
	// download or an idle tunnel past the volume or the expiry.
	s.NodeSyncService.MarkAllDirty()
	go func() {
		s.NodeSyncService.ReconcileDirtyOnlineWait()
		s.kick(users)
		time.Sleep(kickDelay)
		s.kick(users)
	}()
}

func (s *DepleteJob) kick(users []string) {
	for _, u := range users {
		if err := s.StatsService.CloseClusterSessions(u); err != nil {
			logger.Debug("disconnect depleted ", u, ": ", err)
		}
	}
}

// QuickDepleteJob runs every few seconds and does the full work only when a
// client has just used up its volume or time.
type QuickDepleteJob struct {
	DepleteJob
}

func NewQuickDepleteJob() *QuickDepleteJob {
	return new(QuickDepleteJob)
}

func (s *QuickDepleteJob) Run() {
	if s.ClientService.HasDepleted() {
		s.DepleteJob.Run()
	}
}
