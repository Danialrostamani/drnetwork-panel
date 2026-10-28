package service

import (
	"github.com/Danialrostamani/drnetwork-panel/core"
	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
)

// LiveClient includes undrained local traffic and failed-commit traffic. Never
// drains counters, writes the database or polls remote nodes. Remote totals
// remain at their latest collected sample. quotaMu covers the read and drain.
func LiveClient(name string) (*model.Client, error) {
	quotaMu.Lock()
	defer quotaMu.Unlock()
	return readLiveClient(name, liveSessionTracker())
}

func readLiveClient(name string, st *core.SessionTracker) (*model.Client, error) {
	var c model.Client
	if err := database.GetDB().Where("name = ?", name).First(&c).Error; err != nil {
		return nil, err
	}
	statsMu.Lock()
	for _, stat := range pendingStats {
		if stat.Resource == "user" && stat.Tag == name {
			if stat.Direction {
				c.Up = core.SatAdd(c.Up, stat.Traffic)
			} else {
				c.Down = core.SatAdd(c.Down, stat.Traffic)
			}
		}
	}
	statsMu.Unlock()
	if st != nil {
		up, down := st.UserTraffic(name)
		c.Up, c.Down = core.SatAdd(c.Up, up), core.SatAdd(c.Down, down)
	}
	return &c, nil
}
