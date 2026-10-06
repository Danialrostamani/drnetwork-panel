package service

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/Danialrostamani/drnetwork-panel/database/model"

	"gorm.io/gorm"
)

// Volume limits of the Telegram bot's administrators, the part that lives in
// the database (the bot's side is in service/tgbot/quota.go).
//
// A limit is not a balance that is charged when volume is handed out. It is
// the traffic the administrator's clients actually consume. A client "counts"
// for an administrator when a bot_quota_clients row says so; its Base is the
// lifetime traffic it had when it started to count. The lifetime traffic is
// total_up + total_down + up + down: the panel folds up and down into the
// totals at every reset, so a reset never gives usage back. What a counted
// client used is its lifetime traffic less its Base.
//
// A client that is deleted takes its usage with it, so its usage is moved to
// the administrator's Banked count in the same transaction as the deletion
// (bankDeletedClients), and the number only ever goes up from then on.
//
// Everything is derived when it is read, in one statement, so there is no
// job to keep in step with the traffic job and no balance that can drift.

// idChunk keeps the statements that list client IDs or names well under
// SQLite's limit on bound variables.
const idChunk = 400

func chunks[T any](list []T, size int) [][]T {
	var out [][]T
	for len(list) > size {
		out = append(out, list[:size])
		list = list[size:]
	}
	if len(list) > 0 {
		out = append(out, list)
	}
	return out
}

// BotQuotaUsage is the traffic counted against tg's volume limit: what the
// clients that count for them used since they started to count, plus what the
// ones deleted since had used. live is how many of those clients still exist.
// It is one statement, so the two numbers always agree.
func BotQuotaUsage(db *gorm.DB, tg int64) (used int64, live int, err error) {
	var row struct {
		Used int64
		Live int
	}
	err = db.Raw(`SELECT
		COALESCE((SELECT banked FROM bot_quotas WHERE tg_id = ?), 0) +
		COALESCE((SELECT SUM(MAX(c.total_up + c.total_down + c.up + c.down - m.base, 0))
			FROM bot_quota_clients m JOIN clients c ON c.id = m.client_id
			WHERE m.tg_id = ?), 0) AS used,
		(SELECT COUNT(*) FROM bot_quota_clients m JOIN clients c ON c.id = m.client_id
			WHERE m.tg_id = ?) AS live`, tg, tg, tg).Scan(&row).Error
	return row.Used, row.Live, err
}

// BotQuotaAttach makes the clients with these names count for tg from their
// first byte. It is for clients tg has just created. A client that already
// counts for somebody keeps counting for them.
func BotQuotaAttach(db *gorm.DB, tg int64, names []string) error {
	clean := make([]string, 0, len(names))
	for _, n := range names {
		if n = strings.TrimSpace(n); n != "" {
			clean = append(clean, n)
		}
	}
	for _, chunk := range chunks(clean, idChunk) {
		err := db.Exec(`INSERT OR IGNORE INTO bot_quota_clients (client_id, tg_id, base)
			SELECT id, ?, 0 FROM clients WHERE name IN ?`, tg, chunk).Error
		if err != nil {
			return err
		}
	}
	return nil
}

// botQuotaAdopt makes clients that existed before count for tg from now on:
// whatever they used so far is not counted.
func botQuotaAdopt(db *gorm.DB, tg int64, ids []uint) error {
	for _, chunk := range chunks(ids, idChunk) {
		err := db.Exec(`INSERT OR IGNORE INTO bot_quota_clients (client_id, tg_id, base)
			SELECT id, ?, MAX(total_up + total_down + up + down, 0) FROM clients WHERE id IN ?`, tg, chunk).Error
		if err != nil {
			return err
		}
	}
	return nil
}

// BotQuotaAdoptGroup makes every client of a group that counts for nobody yet
// count for tg, the administrator limited to that group. Group names compare
// like the bot's own: without regard to case or surrounding spaces. The
// clients the master pushed to a node (the cluster group) are never adopted.
func BotQuotaAdoptGroup(db *gorm.DB, tg int64, group string) error {
	group = strings.TrimSpace(group)
	if group == "" || strings.EqualFold(group, ClusterGroup) {
		return nil
	}
	var rows []struct {
		Id  uint
		Grp string
	}
	err := db.Raw(`SELECT id, "group" AS grp FROM clients
		WHERE "group" <> '' AND id NOT IN (SELECT client_id FROM bot_quota_clients)`).Scan(&rows).Error
	if err != nil {
		return err
	}
	var ids []uint
	for _, r := range rows {
		if strings.EqualFold(strings.TrimSpace(r.Grp), group) {
			ids = append(ids, r.Id)
		}
	}
	return botQuotaAdopt(db, tg, ids)
}

// createdNearly is how far, in seconds, the creation time of a client may be
// from the time of the history entry that created it.
const createdNearly = 300

// namesIn reads the client names out of what a "new" or "addbulk" save sent:
// one client, or a list of them.
func namesIn(raw json.RawMessage) []string {
	var many []struct {
		Name string `json:"name"`
	}
	var one struct {
		Name string `json:"name"`
	}
	var names []string
	switch {
	case json.Unmarshal(raw, &many) == nil:
		for _, c := range many {
			names = append(names, c.Name)
		}
	case json.Unmarshal(raw, &one) == nil:
		names = append(names, one.Name)
	}
	return names
}

// telegramActorID is the Telegram ID in the actor the bot writes into the
// change history ("telegram:42").
func telegramActorID(actor string) (int64, bool) {
	rest, ok := strings.CutPrefix(actor, TelegramActor+":")
	if !ok {
		return 0, false
	}
	id, err := strconv.ParseInt(rest, 10, 64)
	return id, err == nil && id > 0
}

// botQuotaOnClientSave runs in the transaction that creates clients: the
// clients a Telegram administrator with a volume limit creates count against
// that limit from their first byte. Doing it here, not after the save, means a
// client cannot exist without counting.
func botQuotaOnClientSave(tx *gorm.DB, actor, act string, data json.RawMessage) error {
	if act != "new" && act != "addbulk" {
		return nil
	}
	tg, ok := telegramActorID(actor)
	if !ok {
		return nil
	}
	var limits int64
	if err := tx.Model(&model.BotQuota{}).Where("tg_id = ?", tg).Count(&limits).Error; err != nil || limits == 0 {
		return err
	}
	return BotQuotaAttach(tx, tg, namesIn(data))
}

// BotQuotaAdoptCreated finds, in the change history, the clients the bot
// created for tg before their limit existed, and makes them count from now on.
// actor is how the history names tg ("telegram:<id>"). A client is matched by
// name, and only when it was created at the time the history says, so a name
// that was reused later by somebody else is not taken.
func BotQuotaAdoptCreated(db *gorm.DB, tg int64, actor string) error {
	var rows []model.Changes
	err := db.Where(&model.Changes{Actor: actor, Key: "clients"}).
		Where("action IN ?", []string{"new", "addbulk"}).Find(&rows).Error
	if err != nil {
		return err
	}
	type made struct {
		name string
		at   int64
	}
	var all []made
	var names []string
	seen := map[string]bool{}
	for _, r := range rows {
		for _, name := range namesIn(r.Obj) {
			if name = strings.TrimSpace(name); name == "" {
				continue
			}
			all = append(all, made{name, r.DateTime})
			if !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
	}
	byName := map[string][]model.Client{}
	for _, chunk := range chunks(names, idChunk) {
		var found []model.Client
		if err := db.Select("id", "name", "created_at").Where("name IN ?", chunk).Find(&found).Error; err != nil {
			return err
		}
		for _, c := range found {
			byName[c.Name] = append(byName[c.Name], c)
		}
	}
	var ids []uint
	for _, m := range all {
		for _, c := range byName[m.name] {
			// A client from before creation times were kept matches by name alone.
			if c.CreatedAt == 0 || (c.CreatedAt-m.at <= createdNearly && m.at-c.CreatedAt <= createdNearly) {
				ids = append(ids, c.Id)
			}
		}
	}
	return botQuotaAdopt(db, tg, ids)
}

// BotQuotaRestart starts tg's count from nothing: every client that counts
// for them is counted from its traffic now, and what deleted clients had used
// is forgotten. The clients themselves keep counting.
func BotQuotaRestart(db *gorm.DB, tg int64) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`DELETE FROM bot_quota_clients
			WHERE tg_id = ? AND client_id NOT IN (SELECT id FROM clients)`, tg).Error; err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE bot_quota_clients SET base = COALESCE((
				SELECT MAX(c.total_up + c.total_down + c.up + c.down, 0) FROM clients c
				WHERE c.id = bot_quota_clients.client_id), 0)
			WHERE tg_id = ?`, tg).Error; err != nil {
			return err
		}
		return tx.Exec(`UPDATE bot_quotas SET banked = 0 WHERE tg_id = ?`, tg).Error
	})
}

// BotQuotaForget takes every client away from tg's count, for when their
// limit is removed: a limit set later starts from nothing.
func BotQuotaForget(db *gorm.DB, tg int64) error {
	return db.Where("tg_id = ?", tg).Delete(&model.BotQuotaClient{}).Error
}

// bankDeletedClients is called inside the transaction that deletes clients,
// before the delete: what the clients that count for an administrator had used
// since they started to count is added to that administrator's Banked count,
// and they stop counting. Without it, deleting a client would hand its usage
// back.
func bankDeletedClients(tx *gorm.DB, ids []uint) error {
	for _, chunk := range chunks(ids, idChunk) {
		err := tx.Exec(`UPDATE bot_quotas SET banked = banked + COALESCE((
				SELECT SUM(MAX(c.total_up + c.total_down + c.up + c.down - m.base, 0))
				FROM bot_quota_clients m JOIN clients c ON c.id = m.client_id
				WHERE m.tg_id = bot_quotas.tg_id AND m.client_id IN ?), 0)
			WHERE tg_id IN (SELECT tg_id FROM bot_quota_clients WHERE client_id IN ?)`, chunk, chunk).Error
		if err != nil {
			return err
		}
		if err := tx.Exec(`DELETE FROM bot_quota_clients WHERE client_id IN ?`, chunk).Error; err != nil {
			return err
		}
	}
	return nil
}
