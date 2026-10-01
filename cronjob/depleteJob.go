package cronjob

import (
	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/logger"
	"github.com/Danialrostamani/drnetwork-panel/service"
)

type DepleteJob struct {
	service.ClientService
	service.InboundService
}

func NewDepleteJob() *DepleteJob {
	return new(DepleteJob)
}

func (s *DepleteJob) Run() {
	inboundIds, err := s.ClientService.DepleteClients()
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
}
