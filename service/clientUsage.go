package service

import (
	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"

	"gorm.io/gorm"
)

// addClientUsage adds what a client used on one server (node 0 is the master)
// to its row of the current cycle. A row left from before the client's last
// reset starts over at this traffic: the reset moved up/down into the totals,
// so its epoch no longer matches.
func addClientUsage(tx *gorm.DB, name string, nodeID uint, up, down int64) error {
	if up <= 0 && down <= 0 {
		return nil
	}
	return tx.Exec(`INSERT INTO client_usages (client_id, node_id, epoch, up, down)
		SELECT id, ?, total_up + total_down, ?, ? FROM clients WHERE name = ?
		ON CONFLICT (client_id, node_id) DO UPDATE SET
			up = CASE WHEN client_usages.epoch = excluded.epoch THEN client_usages.up + excluded.up ELSE excluded.up END,
			down = CASE WHEN client_usages.epoch = excluded.epoch THEN client_usages.down + excluded.down ELSE excluded.down END,
			epoch = excluded.epoch`,
		nodeID, max(up, 0), max(down, 0), name).Error
}

// ServerUsage is what a client used on one server in its current cycle.
type ServerUsage struct {
	NodeId uint
	Up     int64
	Down   int64
}

// ClientServerUsage is what a client used on each server in its current cycle,
// by node ID (0 for the master). Rows from an earlier cycle are left out.
func ClientServerUsage(c model.Client) map[uint]ServerUsage {
	var rows []model.ClientUsage
	if err := database.GetDB().Where("client_id = ? AND epoch = ?", c.Id, c.TotalUp+c.TotalDown).Find(&rows).Error; err != nil {
		return nil
	}
	out := make(map[uint]ServerUsage, len(rows))
	for _, r := range rows {
		out[r.NodeId] = ServerUsage{NodeId: r.NodeId, Up: r.Up, Down: r.Down}
	}
	return out
}

// DelStaleClientUsage drops the rows of deleted clients.
func DelStaleClientUsage() error {
	return database.GetDB().Where("client_id NOT IN (?)", database.GetDB().Model(model.Client{}).Select("id")).Delete(&model.ClientUsage{}).Error
}
