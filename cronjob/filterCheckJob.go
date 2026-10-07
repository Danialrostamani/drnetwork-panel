package cronjob

import "github.com/Danialrostamani/drnetwork-panel/service"

// FilterCheckJob checks the nodes from Iran when the set interval is due.
type FilterCheckJob struct{}

func NewFilterCheckJob() *FilterCheckJob { return &FilterCheckJob{} }
func (j *FilterCheckJob) Run()           { service.RunFilterCheck() }
