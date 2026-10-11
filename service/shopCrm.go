package service

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
)

// FindUsers looks shop customers up by Telegram id, @username or a part of
// the name or username.
func (s *ShopService) FindUsers(query string, limit int) ([]model.ShopUser, error) {
	q := strings.TrimSpace(query)
	var out []model.ShopUser
	if q == "" {
		return out, nil
	}
	db := database.GetDB()
	if id, err := strconv.ParseInt(q, 10, 64); err == nil {
		err := db.Where("tg_id = ?", id).Limit(limit).Find(&out).Error
		return out, err
	}
	if strings.HasPrefix(q, "@") {
		err := db.Where("LOWER(username) = ?", strings.ToLower(strings.TrimPrefix(q, "@"))).Limit(limit).Find(&out).Error
		return out, err
	}
	like := "%" + strings.ToLower(q) + "%"
	err := db.Where("LOWER(name) LIKE ? OR LOWER(username) LIKE ?", like, like).Order("seen_at desc, joined_at desc").Limit(limit).Find(&out).Error
	return out, err
}

// UserSummary is what a customer's profile counts.
type UserSummary struct {
	Orders  int64 // approved purchases and renewals
	Spent   int64
	Tickets int64
	Open    int64 // tickets not closed
}

func (s *ShopService) Summary(tgID int64) UserSummary {
	db := database.GetDB()
	var out UserSummary
	db.Model(&model.ShopOrder{}).Where("tg_id = ? AND status = ? AND kind <> ?", tgID, model.OrderApproved, model.OrderTopup).Count(&out.Orders)
	db.Raw("SELECT COALESCE(SUM(paid),0) FROM shop_orders WHERE tg_id = ? AND status = ? AND kind <> ?", tgID, model.OrderApproved, model.OrderTopup).Scan(&out.Spent)
	db.Model(&model.ShopTicket{}).Where("tg_id = ?", tgID).Count(&out.Tickets)
	db.Model(&model.ShopTicket{}).Where("tg_id = ? AND status <> ?", tgID, model.TicketClosed).Count(&out.Open)
	return out
}

// ---- broadcast audiences ----

// Broadcast audiences: everybody (""), customers who bought ("buyers"),
// never bought ("never"), bought nothing for 30 days ("idle30"), have an
// active service ("active") or only ended ones ("expired"); "node:<id>" the
// customers with a service on a node, "group:<name>" in a client group.
const (
	AudienceBuyers  = "buyers"
	AudienceNever   = "never"
	AudienceIdle30  = "idle30"
	AudienceActive  = "active"
	AudienceExpired = "expired"
)

func clientActive(c model.Client, now int64) bool {
	return c.Enable && (c.Expiry <= 0 || c.Expiry > now) && (c.Volume <= 0 || c.Up+c.Down < c.Volume)
}

// AudienceTargets is who a broadcast to the audience reaches; blocked users
// and those who left the bot are never in it.
func (s *ShopService) AudienceTargets(aud string, now time.Time) ([]int64, error) {
	all, err := s.BroadcastTargets()
	if err != nil || aud == "" {
		return all, err
	}
	db := database.GetDB()
	keep := map[int64]bool{}
	drop := false // keep holds who to leave out rather than who to take
	var clients []model.Client
	needClients := aud == AudienceActive || aud == AudienceExpired || strings.HasPrefix(aud, "node:") || strings.HasPrefix(aud, "group:")
	if needClients {
		if err := db.Model(&model.Client{}).Where("tg_id <> 0").Find(&clients).Error; err != nil {
			return nil, err
		}
	}
	switch {
	case aud == AudienceBuyers || aud == AudienceNever || aud == AudienceIdle30:
		q := db.Model(&model.ShopOrder{}).Distinct("tg_id").Where("status = ? AND kind <> ?", model.OrderApproved, model.OrderTopup)
		if aud == AudienceIdle30 {
			q = q.Where("decided_at >= ?", now.Add(-30*24*time.Hour).Unix())
		}
		var ids []int64
		if err := q.Pluck("tg_id", &ids).Error; err != nil {
			return nil, err
		}
		for _, id := range ids {
			keep[id] = true
		}
		drop = aud != AudienceBuyers
	case aud == AudienceActive || aud == AudienceExpired:
		active := map[int64]bool{}
		for _, c := range clients {
			if clientActive(c, now.Unix()) {
				active[c.TgId] = true
			}
		}
		if aud == AudienceActive {
			keep = active
		} else {
			for _, c := range clients {
				if !active[c.TgId] {
					keep[c.TgId] = true
				}
			}
		}
	case strings.HasPrefix(aud, "node:"):
		id, _ := strconv.ParseUint(strings.TrimPrefix(aud, "node:"), 10, 32)
		var inbounds []uint
		if err := db.Model(&model.Inbound{}).Where("node_id = ?", id).Pluck("id", &inbounds).Error; err != nil {
			return nil, err
		}
		on := map[uint]bool{}
		for _, i := range inbounds {
			on[i] = true
		}
		for _, c := range clients {
			var ids []uint
			_ = json.Unmarshal(c.Inbounds, &ids)
			for _, i := range ids {
				if on[i] {
					keep[c.TgId] = true
				}
			}
		}
	case strings.HasPrefix(aud, "group:"):
		g := strings.TrimPrefix(aud, "group:")
		for _, c := range clients {
			if strings.EqualFold(strings.TrimSpace(c.Group), g) {
				keep[c.TgId] = true
			}
		}
	default:
		return nil, errors.New("unknown audience")
	}
	var out []int64
	for _, id := range all {
		if keep[id] != drop {
			out = append(out, id)
		}
	}
	return out, nil
}

// ClientGroups is the groups of clients bound to Telegram, sorted.
func (s *ShopService) ClientGroups() []string {
	var groups []string
	database.GetDB().Model(&model.Client{}).Where("tg_id <> 0 AND TRIM(\"group\") <> ''").Distinct("\"group\"").Order("\"group\"").Pluck("\"group\"", &groups)
	return groups
}

// NodeNames is the managed nodes, id to name.
func (s *ShopService) NodeNames() []model.Node {
	var nodes []model.Node
	database.GetDB().Model(&model.Node{}).Select("id", "name").Order("name").Find(&nodes)
	return nodes
}
