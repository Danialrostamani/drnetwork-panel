package cronjob

import "github.com/Danialrostamani/drnetwork-panel/service"

// RemoteBackupJob sends the database to the backup storage when it is due.
type RemoteBackupJob struct{}

func NewRemoteBackupJob() *RemoteBackupJob { return &RemoteBackupJob{} }
func (j *RemoteBackupJob) Run()            { service.RunRemoteBackup() }
