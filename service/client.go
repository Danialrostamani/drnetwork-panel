package service

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/logger"
	"github.com/Danialrostamani/drnetwork-panel/util"
	"github.com/Danialrostamani/drnetwork-panel/util/common"

	"gorm.io/gorm"
)

type ClientService struct{}

func (s *ClientService) Get(id string) (*[]model.Client, error) {
	if id == "" {
		return s.GetAll()
	}
	return s.getById(id)
}

func (s *ClientService) getById(id string) (*[]model.Client, error) {
	db := database.GetDB()
	var client []model.Client
	err := db.Model(model.Client{}).Where("id in ?", strings.Split(id, ",")).Scan(&client).Error
	if err != nil {
		return nil, err
	}

	return &client, nil
}

func (s *ClientService) GetAllWithConfig() (*[]model.Client, error) {
	db := database.GetDB()
	var clients []model.Client
	if err := db.Model(model.Client{}).Find(&clients).Error; err != nil {
		return nil, err
	}
	return &clients, nil
}

type ClientTraffic struct {
	Up       int64 `json:"up"`
	Down     int64 `json:"down"`
	OnlineAt int64 `json:"onlineAt"`
}

// GetTrafficSnapshot is the lightweight live payload used by the clients page.
// It deliberately excludes credentials, links and assignments so polling it
// frequently does not turn a traffic refresh into a full client-list reload.
func (s *ClientService) GetTrafficSnapshot() (map[string]ClientTraffic, error) {
	var rows []struct {
		Name     string
		Up       int64
		Down     int64
		OnlineAt int64
	}
	if err := database.GetDB().Model(model.Client{}).
		Select("`name`, `up`, `down`, `online_at`").Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]ClientTraffic, len(rows))
	for _, row := range rows {
		out[row.Name] = ClientTraffic{Up: row.Up, Down: row.Down, OnlineAt: row.OnlineAt}
	}
	return out, nil
}

// GetAll is the client list of the panel. The panel's bulk edit posts these
// rows back, and a save writes every column of the row it is given, so the
// settings a bulk edit does not touch have to be in the list: without delay
// start, auto reset, reset days and the next reset, a bulk edit switched them
// off for every client it covered.
func (s *ClientService) GetAll() (*[]model.Client, error) {
	db := database.GetDB()
	var clients []model.Client
	err := db.Model(model.Client{}).
		Select("`id`, `enable`, `name`, `desc`, `group`, `remark`, `inbounds`, `up`, `down`, `volume`, `expiry`, `created_at`, `online_at`, `limit_ip`, `delay_start`, `auto_reset`, `reset_days`, `next_reset`").
		Scan(&clients).Error
	if err != nil {
		return nil, err
	}
	return &clients, nil
}

// validateClientName rejects empty names and globally duplicate names,
// then stores the trimmed name back on the client.
func (s *ClientService) validateClientName(tx *gorm.DB, client *model.Client) error {
	name := strings.TrimSpace(client.Name)
	if name == "" {
		return common.NewError("client name must not be empty")
	}
	query := tx.Model(model.Client{}).Where("name = ?", name)
	if client.Id != 0 {
		query = query.Where("id != ?", client.Id)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return common.NewErrorf("client name %q is already in use", name)
	}
	client.Name = name
	return nil
}

func (s *ClientService) Save(tx *gorm.DB, act string, data json.RawMessage, hostname string) ([]uint, error) {
	var err error
	var inboundIds []uint

	switch act {
	case "new", "edit":
		var client model.Client
		err = json.Unmarshal(data, &client)
		if err != nil {
			return nil, err
		}
		if client.Group == clusterGroup {
			client.Name = strings.TrimSpace(client.Name)
			if client.Name == "" {
				return nil, common.NewError("client name must not be empty")
			}
		} else if err = s.validateClientName(tx, &client); err != nil {
			return nil, err
		}
		if err = setConfigIdentity(&client); err != nil {
			return nil, err
		}
		err = s.updateLinksWithFixedInbounds(tx, []*model.Client{&client}, hostname)
		if err != nil {
			return nil, err
		}
		if act == "edit" {
			// Find changed inbounds
			inboundIds, err = s.findInboundsChanges(tx, &client, false)
			if err != nil {
				return nil, err
			}
			// Preserve managed timestamps (immutable createdAt, stats-managed onlineAt)
			s.preserveServerOwnedFields(tx, &client)
		} else {
			client.CreatedAt = time.Now().Unix()
			err = json.Unmarshal(client.Inbounds, &inboundIds)
			if err != nil {
				return nil, err
			}
		}
		err = tx.Save(&client).Error
		if err != nil {
			return nil, err
		}
	case "addbulk":
		var clients []*model.Client
		err = json.Unmarshal(data, &clients)
		if err != nil {
			return nil, err
		}
		now := time.Now().Unix()
		// Every client is validated before any of them is written, so the
		// batch has to be checked against itself as well as against the table.
		seen := make(map[string]bool, len(clients))
		for _, client := range clients {
			if err = s.validateClientName(tx, client); err != nil {
				return nil, err
			}
			if seen[client.Name] {
				return nil, common.NewErrorf("duplicate client name %q in request", client.Name)
			}
			seen[client.Name] = true
			if err = setConfigIdentity(client); err != nil {
				return nil, err
			}
			client.CreatedAt = now
			var ids []uint
			if err = json.Unmarshal(client.Inbounds, &ids); err != nil {
				return nil, err
			}
			inboundIds = common.UnionUintArray(inboundIds, ids)
		}
		err = s.updateLinksWithFixedInbounds(tx, clients, hostname)
		if err != nil {
			return nil, err
		}
		err = tx.Save(clients).Error
		if err != nil {
			return nil, err
		}
	case "editbulk":
		var clients []*model.Client
		err = json.Unmarshal(data, &clients)
		if err != nil {
			return nil, err
		}
		seen := make(map[string]bool, len(clients))
		for _, client := range clients {
			if err = s.validateClientName(tx, client); err != nil {
				return nil, err
			}
			if seen[client.Name] {
				return nil, common.NewErrorf("duplicate client name %q in request", client.Name)
			}
			seen[client.Name] = true
			changedInboundIds, err := s.findInboundsChanges(tx, client, true)
			if err != nil {
				return nil, err
			}
			if err = setConfigIdentity(client); err != nil {
				return nil, err
			}
			s.preserveServerOwnedFields(tx, client)
			if len(changedInboundIds) > 0 {
				inboundIds = common.UnionUintArray(inboundIds, changedInboundIds)
			}
		}
		if len(inboundIds) > 0 {
			err = s.updateLinksWithFixedInbounds(tx, clients, hostname)
			if err != nil {
				return nil, err
			}
		}
		err = tx.Save(clients).Error
		if err != nil {
			return nil, err
		}
	case "attachall":
		inboundIds, err = s.attachAllInbounds(tx, hostname)
		if err != nil {
			return nil, err
		}
	case "delbulk":
		var ids []uint
		err = json.Unmarshal(data, &ids)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			var client model.Client
			err = tx.Where("id = ?", id).First(&client).Error
			if err != nil {
				return nil, err
			}
			var clientInbounds []uint
			err = json.Unmarshal(client.Inbounds, &clientInbounds)
			if err != nil {
				return nil, err
			}
			inboundIds = common.UnionUintArray(inboundIds, clientInbounds)
		}
		// What these clients used stays on their administrator's volume limit.
		err = bankDeletedClients(tx, ids)
		if err != nil {
			return nil, err
		}
		err = tx.Where("id in ?", ids).Delete(model.Client{}).Error
		if err != nil {
			return nil, err
		}
	case "del":
		var id uint
		err = json.Unmarshal(data, &id)
		if err != nil {
			return nil, err
		}
		var client model.Client
		err = tx.Where("id = ?", id).First(&client).Error
		if err != nil {
			return nil, err
		}
		err = json.Unmarshal(client.Inbounds, &inboundIds)
		if err != nil {
			return nil, err
		}
		err = bankDeletedClients(tx, []uint{id})
		if err != nil {
			return nil, err
		}
		err = tx.Where("id = ?", id).Delete(model.Client{}).Error
		if err != nil {
			return nil, err
		}
	default:
		return nil, common.NewErrorf("unknown action: %s", act)
	}

	return inboundIds, nil
}

// attachPlan is what "add every inbound to every client" would do.
type attachPlan struct {
	// inbounds are all the inbounds that take clients, ascending.
	inbounds []uint
	// changed are the clients that lack some of them, with the complete inbound
	// list already set.
	changed []*model.Client
	// touched are the inbounds that gain at least one client, ascending.
	touched []uint
}

// planAttachAll works out which clients lack which inbounds. It writes nothing.
// The inbounds are those that take clients (see inboundTakesClients; the ones
// hosted on nodes included). The clients the master pushed to this node (the
// cluster group) are left out: the master owns them and would put them back on
// its next sync.
func (s *ClientService) planAttachAll(tx *gorm.DB) (*attachPlan, error) {
	plan := &attachPlan{}
	var inbounds []model.Inbound
	if err := tx.Model(model.Inbound{}).Select("id", "type", "tag", "options").Order("id").Find(&inbounds).Error; err != nil {
		return nil, err
	}
	for i := range inbounds {
		if inbounds[i].Tag != "" && inboundTakesClients(&inbounds[i]) {
			plan.inbounds = append(plan.inbounds, inbounds[i].Id)
		}
	}
	if len(plan.inbounds) == 0 {
		return plan, nil
	}

	var clients []model.Client
	if err := tx.Model(model.Client{}).Order("id").Find(&clients).Error; err != nil {
		return nil, err
	}
	touched := map[uint]bool{}
	for i := range clients {
		client := &clients[i]
		if client.Group == clusterGroup {
			continue
		}
		var have []uint
		if len(client.Inbounds) > 0 {
			if err := json.Unmarshal(client.Inbounds, &have); err != nil {
				return nil, common.NewErrorf("client %q has an unreadable inbound list: %v", client.Name, err)
			}
		}
		owned := make(map[uint]bool, len(have))
		for _, id := range have {
			owned[id] = true
		}
		added := false
		for _, id := range plan.inbounds {
			if !owned[id] {
				have = append(have, id)
				touched[id] = true
				added = true
			}
		}
		if !added {
			continue
		}
		sort.Slice(have, func(a, b int) bool { return have[a] < have[b] })
		encoded, err := json.MarshalIndent(have, "", "  ")
		if err != nil {
			return nil, err
		}
		client.Inbounds = encoded
		plan.changed = append(plan.changed, client)
	}
	for _, id := range plan.inbounds {
		if touched[id] {
			plan.touched = append(plan.touched, id)
		}
	}
	return plan, nil
}

// AttachAllPreview counts what attachAllInbounds would change: the clients that
// lack some inbound, and the inbounds that take clients.
func (s *ClientService) AttachAllPreview() (clients int, inbounds int, err error) {
	plan, err := s.planAttachAll(database.GetDB())
	if err != nil {
		return 0, 0, err
	}
	return len(plan.changed), len(plan.inbounds), nil
}

// attachAllInbounds adds every inbound that takes clients to every client, so
// nobody has to open the clients one by one after adding a node or an inbound.
// A client that already has an inbound keeps it.
//
// Only the inbounds and links columns are written, so the traffic counters the
// stats job keeps updating and the client's other settings are untouched. It
// returns the inbounds that gained at least one client, for the running core to
// pick the new users up.
func (s *ClientService) attachAllInbounds(tx *gorm.DB, hostname string) ([]uint, error) {
	plan, err := s.planAttachAll(tx)
	if err != nil {
		return nil, err
	}
	if len(plan.changed) == 0 {
		return nil, nil
	}
	if err := s.updateLinksWithFixedInbounds(tx, plan.changed, hostname); err != nil {
		return nil, err
	}
	for _, client := range plan.changed {
		if err := tx.Model(client).Select("Inbounds", "Links").Updates(client).Error; err != nil {
			return nil, err
		}
	}
	return plan.touched, nil
}

// preserveServerOwnedFields restores the columns the panel maintains itself.
// The traffic counters matter as much as the timestamps: the stats job writes
// up/down every ten seconds, so a stale form would roll them back.
func (s *ClientService) preserveServerOwnedFields(tx *gorm.DB, client *model.Client) {
	var existing model.Client
	if err := tx.Model(model.Client{}).
		Select("created_at", "online_at", "tg_id", "up", "down", "total_up", "total_down").
		Where("id = ?", client.Id).First(&existing).Error; err != nil {
		return
	}
	client.CreatedAt = existing.CreatedAt
	client.OnlineAt = existing.OnlineAt
	client.TgId = existing.TgId

	if client.Up == 0 && client.Down == 0 {
		client.TotalUp = existing.TotalUp + existing.Up
		client.TotalDown = existing.TotalDown + existing.Down
		return
	}

	client.Up = existing.Up
	client.Down = existing.Down
	client.TotalUp = existing.TotalUp
	client.TotalDown = existing.TotalDown
}

// clientNameJSON encodes a client name for the changes log. Built by string
// concatenation, a name with a quote or backslash produced unreadable JSON --
// cmd/migration/1_1.go already repairs the previous generation of this bug.
func clientNameJSON(name string) json.RawMessage {
	encoded, err := json.Marshal(name)
	if err != nil {
		// json.Marshal of a string cannot fail, but never emit broken JSON.
		return json.RawMessage(`""`)
	}
	return json.RawMessage(encoded)
}

func (s *ClientService) updateLinksWithFixedInbounds(tx *gorm.DB, clients []*model.Client, hostname string) error {
	clientInboundIds := make([][]uint, len(clients))
	var allIds []uint
	for i, client := range clients {
		var ids []uint
		if err := json.Unmarshal(client.Inbounds, &ids); err != nil {
			return err
		}
		clientInboundIds[i] = ids
		allIds = common.UnionUintArray(allIds, ids)
	}

	// Zero inbounds means removing local links only
	var inbounds []model.Inbound
	if len(allIds) > 0 {
		err := tx.Model(model.Inbound{}).Preload("Tls").Where("id in ? and type in ? and node_id IS NULL", allIds, util.InboundTypeWithLink).Find(&inbounds).Error
		if err != nil {
			return err
		}
	}
	inboundById := make(map[uint]*model.Inbound, len(inbounds))
	for i := range inbounds {
		inboundById[inbounds[i].Id] = &inbounds[i]
	}

	// Node links are system-owned. Preserve them only while the edited client
	// still references the corresponding replica, even if that node is offline.
	var replicas []model.Inbound
	if len(allIds) > 0 {
		if err := tx.Model(model.Inbound{}).Select("id", "tag", "node_id").Where("id in ? AND node_id IS NOT NULL", allIds).Find(&replicas).Error; err != nil {
			return err
		}
	}
	var nodes []model.Node
	if err := tx.Model(model.Node{}).Select("id", "name").Find(&nodes).Error; err != nil {
		return err
	}
	nodeNameByID := make(map[uint]string, len(nodes))
	nodeNames := make([]string, 0, len(nodes))
	for _, node := range nodes {
		nodeNameByID[node.Id] = node.Name
		nodeNames = append(nodeNames, node.Name)
	}
	replicaRemarkByID := make(map[uint]string, len(replicas))
	for _, replica := range replicas {
		if replica.NodeId != nil {
			replicaRemarkByID[replica.Id] = nodeLinkPrefix(nodeNameByID[*replica.NodeId]) + replica.Tag
		}
	}

	for index, client := range clients {
		var clientLinks []map[string]string
		if err := json.Unmarshal(client.Links, &clientLinks); err != nil {
			return err
		}

		newClientLinks := []map[string]string{}
		for _, id := range clientInboundIds[index] {
			inbound, ok := inboundById[id]
			if !ok {
				continue
			}
			newLinks := util.LinkGenerator(client.Config, inbound, hostname, client.Remark)
			for _, newLink := range newLinks {
				newClientLinks = append(newClientLinks, map[string]string{
					"remark": inbound.Tag,
					"type":   "local",
					"uri":    newLink,
				})
			}
		}

		allowedNodeLinks := make(map[string]bool)
		for _, inboundID := range clientInboundIds[index] {
			if remark := replicaRemarkByID[inboundID]; remark != "" {
				allowedNodeLinks[remark] = true
			}
		}
		for _, clientLink := range clientLinks {
			if clientLink["type"] == "local" {
				continue
			}
			if isNodeOwnedRemark(clientLink["remark"], nodeNames) && !allowedNodeLinks[clientLink["remark"]] {
				continue
			}
			newClientLinks = append(newClientLinks, clientLink)
		}

		links, err := json.MarshalIndent(newClientLinks, "", "  ")
		if err != nil {
			return err
		}
		clients[index].Links = links
	}
	return nil
}

func (s *ClientService) UpdateClientsOnInboundAdd(tx *gorm.DB, initIds string, inboundId uint, hostname string) error {
	clientIds := strings.Split(initIds, ",")
	var clients []model.Client
	err := tx.Model(model.Client{}).Where("id in ?", clientIds).Find(&clients).Error
	if err != nil {
		return err
	}
	var inbound model.Inbound
	err = tx.Model(model.Inbound{}).Preload("Tls").Where("id = ?", inboundId).Find(&inbound).Error
	if err != nil {
		return err
	}
	for _, client := range clients {
		// Add inbounds
		var clientInbounds []uint
		json.Unmarshal(client.Inbounds, &clientInbounds)
		clientInbounds = append(clientInbounds, inboundId)
		client.Inbounds, err = json.MarshalIndent(clientInbounds, "", "  ")
		if err != nil {
			return err
		}
		// Add links
		var clientLinks, newClientLinks []map[string]string
		json.Unmarshal(client.Links, &clientLinks)
		newLinks := util.LinkGenerator(client.Config, &inbound, hostname, client.Remark)
		for _, newLink := range newLinks {
			newClientLinks = append(newClientLinks, map[string]string{
				"remark": inbound.Tag,
				"type":   "local",
				"uri":    newLink,
			})
		}
		for _, clientLink := range clientLinks {
			if clientLink["remark"] != inbound.Tag {
				newClientLinks = append(newClientLinks, clientLink)
			}
		}

		client.Links, err = json.MarshalIndent(newClientLinks, "", "  ")
		if err != nil {
			return err
		}
		err = tx.Save(&client).Error
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *ClientService) UpdateClientsOnInboundDelete(tx *gorm.DB, id uint, tag string) error {
	var clientIds []uint
	err := tx.Raw("SELECT clients.id FROM clients, json_each(clients.inbounds) AS je WHERE je.value = ?", id).Scan(&clientIds).Error
	if err != nil {
		return err
	}
	if len(clientIds) == 0 {
		return nil
	}
	var clients []model.Client
	err = tx.Model(model.Client{}).Where("id IN ?", clientIds).Find(&clients).Error
	if err != nil {
		return err
	}
	var nodeNames []string
	if err = tx.Model(model.Node{}).Pluck("name", &nodeNames).Error; err != nil {
		return err
	}
	for _, client := range clients {
		// Delete inbounds
		var clientInbounds, newClientInbounds []uint
		json.Unmarshal(client.Inbounds, &clientInbounds)
		for _, clientInbound := range clientInbounds {
			if clientInbound != id {
				newClientInbounds = append(newClientInbounds, clientInbound)
			}
		}
		client.Inbounds, err = json.MarshalIndent(newClientInbounds, "", "  ")
		if err != nil {
			return err
		}
		// Delete links
		var clientLinks, newClientLinks []map[string]string
		json.Unmarshal(client.Links, &clientLinks)
		for _, clientLink := range clientLinks {
			if clientLink["remark"] != tag && !isNodeLinkFor(clientLink["remark"], tag, nodeNames) {
				newClientLinks = append(newClientLinks, clientLink)
			}
		}
		client.Links, err = json.MarshalIndent(newClientLinks, "", "  ")
		if err != nil {
			return err
		}
		err = tx.Save(&client).Error
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *ClientService) UpdateLinksByInboundChange(tx *gorm.DB, inbounds *[]model.Inbound, hostname string, oldTag string) error {
	var err error
	for _, inbound := range *inbounds {
		var clientIds []uint
		err = tx.Raw("SELECT clients.id FROM clients, json_each(clients.inbounds) AS je WHERE je.value = ?", inbound.Id).Scan(&clientIds).Error
		if err != nil {
			return err
		}
		if len(clientIds) == 0 {
			continue
		}
		var clients []model.Client
		err = tx.Model(model.Client{}).Where("id IN ?", clientIds).Find(&clients).Error
		if err != nil {
			return err
		}
		for _, client := range clients {
			var clientLinks, newClientLinks []map[string]string
			json.Unmarshal(client.Links, &clientLinks)
			newLinks := util.LinkGenerator(client.Config, &inbound, hostname, client.Remark)
			for _, newLink := range newLinks {
				newClientLinks = append(newClientLinks, map[string]string{
					"remark": inbound.Tag,
					"type":   "local",
					"uri":    newLink,
				})
			}
			for _, clientLink := range clientLinks {
				if clientLink["type"] != "local" || (clientLink["remark"] != inbound.Tag && clientLink["remark"] != oldTag) {
					newClientLinks = append(newClientLinks, clientLink)
				}
			}

			client.Links, err = json.MarshalIndent(newClientLinks, "", "  ")
			if err != nil {
				return err
			}
			err = tx.Save(&client).Error
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *ClientService) DepleteClients() ([]uint, error) {
	var err error
	var clients []model.Client
	var changes []model.Changes
	var users []string
	var inboundIds []uint

	dt := time.Now().Unix()
	db := database.GetDB()

	tx := db.Begin()
	defer func() {
		if err == nil {
			tx.Commit()
			if err1 := db.Exec("PRAGMA wal_checkpoint(FULL)").Error; err1 != nil {
				logger.Error("Error checkpointing WAL: ", err1.Error())
			}
		} else {
			tx.Rollback()
		}
	}()

	// Reset clients
	inboundIds, err = s.ResetClients(tx, dt)
	if err != nil {
		return nil, err
	}

	// Deplete clients
	err = tx.Model(model.Client{}).Where("enable = true AND ((volume >0 AND up+down > volume) OR (expiry > 0 AND expiry < ?))", dt).Scan(&clients).Error
	if err != nil {
		return nil, err
	}

	for _, client := range clients {
		logger.Debug("Client ", client.Name, " is going to be disabled")
		users = append(users, client.Name)
		var userInbounds []uint
		json.Unmarshal(client.Inbounds, &userInbounds)
		// Find changed inbounds
		inboundIds = common.UnionUintArray(inboundIds, userInbounds)
		changes = append(changes, model.Changes{
			DateTime: dt,
			Actor:    "DepleteJob",
			Key:      "clients",
			Action:   "disable",
			Obj:      clientNameJSON(client.Name),
		})
	}

	// Save changes
	if len(changes) > 0 {
		err = tx.Model(model.Client{}).Where("enable = true AND ((volume >0 AND up+down > volume) OR (expiry > 0 AND expiry < ?))", dt).Update("enable", false).Error
		if err != nil {
			return nil, err
		}
		err = tx.Model(model.Changes{}).Create(&changes).Error
		if err != nil {
			return nil, err
		}
		LastUpdate = dt
	}

	return inboundIds, nil
}

func (s *ClientService) ResetClients(tx *gorm.DB, dt int64) ([]uint, error) {
	var err error
	var resetClients, allClients []*model.Client
	var changes []model.Changes
	var inboundIds []uint
	// Set delay start without periodic reset
	err = tx.Model(model.Client{}).
		Where("enable = true AND delay_start = true AND auto_reset = false AND (Up + Down) > 0").Find(&resetClients).Error
	if err != nil {
		return nil, err
	}
	for _, client := range resetClients {
		client.Expiry = dt + (int64(client.ResetDays) * 86400)
		client.DelayStart = false
		changes = append(changes, model.Changes{
			DateTime: dt,
			Actor:    "ResetJob",
			Key:      "clients",
			Action:   "reset",
			Obj:      clientNameJSON(client.Name),
		})
	}
	allClients = append(allClients, resetClients...)

	// Set delay start with periodic reset
	err = tx.Model(model.Client{}).
		Where("enable = true AND delay_start = true AND auto_reset = true AND (Up + Down) > 0").Find(&resetClients).Error
	if err != nil {
		return nil, err
	}
	for _, client := range resetClients {
		client.NextReset = dt + (int64(client.ResetDays) * 86400)
		client.DelayStart = false
		changes = append(changes, model.Changes{
			DateTime: dt,
			Actor:    "ResetJob",
			Key:      "clients",
			Action:   "reset",
			Obj:      clientNameJSON(client.Name),
		})
	}
	allClients = append(allClients, resetClients...)

	// reset_days > 0 is a backstop: at zero, NextReset becomes dt + 0 == dt, so
	// the row matches every minute and the volume quota is never reached.
	err = tx.Model(model.Client{}).
		Where("delay_start = false AND auto_reset = true AND reset_days > 0 AND next_reset < ?", dt).Find(&resetClients).Error
	if err != nil {
		return nil, err
	}
	for _, client := range resetClients {
		client.NextReset = dt + (int64(client.ResetDays) * 86400)
		client.TotalUp += client.Up
		client.TotalDown += client.Down
		client.Up = 0
		client.Down = 0
		if !client.Enable {
			client.Enable = true
			var clientInboundIds []uint
			json.Unmarshal(client.Inbounds, &clientInboundIds)
			inboundIds = common.UnionUintArray(inboundIds, clientInboundIds)
		}
	}
	allClients = append(allClients, resetClients...)

	// Save clients
	if len(allClients) > 0 {
		err = tx.Save(allClients).Error
		if err != nil {
			return nil, err
		}
	}

	// Save changes
	if len(changes) > 0 {
		err = tx.Model(model.Changes{}).Create(&changes).Error
		if err != nil {
			return nil, err
		}
		LastUpdate = dt
	}
	return inboundIds, nil
}

// ResetAllClientsTraffic zeroes up/down for every client (accumulating into the
// total counters) and re-enables all of them, in a single bulk update. Used by
// the global periodic traffic reset; the caller restarts the core afterwards so
// re-enabled clients take effect.
func (s *ClientService) ResetAllClientsTraffic() error {
	db := database.GetDB()
	dt := time.Now().Unix()

	result := db.Model(model.Client{}).
		Where("(up + down) > 0 OR enable = false").
		UpdateColumns(map[string]interface{}{
			"total_up":   gorm.Expr("total_up + up"),
			"total_down": gorm.Expr("total_down + down"),
			"up":         0,
			"down":       0,
			"enable":     true,
		})
	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected > 0 {
		if err := db.Create(&model.Changes{
			DateTime: dt,
			Actor:    "ResetTrafficJob",
			Key:      "clients",
			Action:   "reset",
			Obj:      json.RawMessage("\"all\""),
		}).Error; err != nil {
			return err
		}
		LastUpdate = dt
	}

	return nil
}

func setConfigIdentity(client *model.Client) error {
	if client.Name == "" || len(client.Config) < 2 {
		return nil
	}
	var configs map[string]map[string]interface{}
	if err := json.Unmarshal(client.Config, &configs); err != nil {
		return err
	}
	for _, cfg := range configs {
		if _, ok := cfg["name"]; ok {
			cfg["name"] = client.Name
		} else if _, ok := cfg["username"]; ok {
			cfg["username"] = client.Name
		}
	}
	newConfig, err := json.Marshal(configs)
	if err != nil {
		return err
	}
	client.Config = newConfig
	return nil
}

func (s *ClientService) findInboundsChanges(tx *gorm.DB, client *model.Client, fillOmitted bool) ([]uint, error) {
	var err error
	var oldClient model.Client
	var oldInboundIds, newInboundIds []uint
	err = tx.Model(model.Client{}).Where("id = ?", client.Id).First(&oldClient).Error
	if err != nil {
		return nil, err
	}
	if fillOmitted {
		client.Links = oldClient.Links
		client.Config = oldClient.Config
	}
	err = json.Unmarshal(oldClient.Inbounds, &oldInboundIds)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(client.Inbounds, &newInboundIds)
	if err != nil {
		return nil, err
	}

	// Check client.Config changes
	if !bytes.Equal(oldClient.Config, client.Config) ||
		oldClient.Name != client.Name ||
		oldClient.Enable != client.Enable {
		return common.UnionUintArray(oldInboundIds, newInboundIds), nil
	}

	// Check client.Inbounds changes
	diffInbounds := common.DiffUintArray(oldInboundIds, newInboundIds)

	return diffInbounds, nil
}
