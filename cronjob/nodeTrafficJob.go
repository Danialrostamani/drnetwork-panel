package cronjob

import (
	"github.com/Danialrostamani/drnetwork-panel/service"
	"sync"
)

type NodeTrafficJob struct {
	service.NodeSyncService
	running sync.Mutex
}

func NewNodeTrafficJob() *NodeTrafficJob { return &NodeTrafficJob{} }
func (j *NodeTrafficJob) Run() {
	if !j.running.TryLock() {
		return
	}
	defer j.running.Unlock()
	j.NodeSyncService.CollectTraffic()
}
