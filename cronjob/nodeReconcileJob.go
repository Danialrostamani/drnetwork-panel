package cronjob

import (
	"github.com/alireza0/s-ui/service"
	"sync"
)

type NodeReconcileJob struct {
	service.NodeSyncService
	running sync.Mutex
}

func NewNodeReconcileJob() *NodeReconcileJob { return &NodeReconcileJob{} }
func (j *NodeReconcileJob) Run() {
	if !j.running.TryLock() {
		return
	}
	defer j.running.Unlock()
	j.NodeSyncService.ReconcileAllOnline()
}
