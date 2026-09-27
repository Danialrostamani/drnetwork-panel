package cronjob

import (
	"github.com/alireza0/s-ui/service"
	"sync"
)

type NodesJob struct {
	service.NodeService
	service.NodeSyncService
	running sync.Mutex
}

func NewNodesJob() *NodesJob { return &NodesJob{} }
func (j *NodesJob) Run() {
	if !j.running.TryLock() {
		return
	}
	defer j.running.Unlock()
	j.NodeService.RefreshAll()
	j.NodeSyncService.ReconcileDirtyOnline()
}
