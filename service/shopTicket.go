package service

import (
	"errors"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"gorm.io/gorm"
)

// Support tickets: a customer writes to the support from the bot, the
// support answers there.

// TicketMaxWaiting is how many messages a customer can stack on a ticket
// nobody answered yet: the cap keeps a flood out of the support's chat.
const TicketMaxWaiting = 30

var (
	ErrTicketNotFound = errors.New("ticket not found")
	ErrTicketClosed   = errors.New("the ticket is closed")
	ErrTicketFlood    = errors.New("too many messages are waiting for an answer")
	ErrTicketEmpty    = errors.New("empty message")
)

func ticketSubject(text, kind string) string {
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return "[" + kind + "]"
	}
	if utf8.RuneCountInString(text) > 60 {
		text = string([]rune(text)[:60]) + "…"
	}
	return text
}

// TicketWrite adds a customer's message to their open ticket, or opens one.
// It tells whether the ticket is new.
func (s *ShopService) TicketWrite(tgID int64, kind, text, fileID string) (*model.ShopTicket, bool, error) {
	text = strings.TrimSpace(text)
	if text == "" && fileID == "" {
		return nil, false, ErrTicketEmpty
	}
	if kind == "" {
		kind = "text"
	}
	var t model.ShopTicket
	created := false
	err := database.GetDB().Transaction(func(tx *gorm.DB) error {
		now := time.Now().Unix()
		err := tx.Where("tg_id = ? AND status <> ?", tgID, model.TicketClosed).Order("id desc").First(&t).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			t = model.ShopTicket{TgId: tgID, Status: model.TicketOpen, Subject: ticketSubject(text, kind), CreatedAt: now, UpdatedAt: now}
			if err := tx.Create(&t).Error; err != nil {
				return err
			}
			created = true
		case err != nil:
			return err
		default:
			var waiting int64
			tx.Model(&model.ShopTicketMsg{}).Where("ticket_id = ? AND answer = ? AND id > (SELECT COALESCE(MAX(id), 0) FROM shop_ticket_msgs WHERE ticket_id = ? AND answer = ?)", t.Id, false, t.Id, true).Count(&waiting)
			if waiting >= TicketMaxWaiting {
				return ErrTicketFlood
			}
			t.Status, t.UpdatedAt = model.TicketOpen, now
			if err := tx.Model(&t).Updates(map[string]any{"status": t.Status, "updated_at": now}).Error; err != nil {
				return err
			}
		}
		return tx.Create(&model.ShopTicketMsg{TicketId: t.Id, FromTg: tgID, Kind: kind, Text: text, FileId: fileID, CreatedAt: now}).Error
	})
	if err != nil {
		return nil, false, err
	}
	return &t, created, nil
}

// TicketAnswer adds the support's answer to a ticket that is not closed.
func (s *ShopService) TicketAnswer(id uint, adminTg int64, text string) (*model.ShopTicket, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, ErrTicketEmpty
	}
	var t model.ShopTicket
	err := database.GetDB().Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&t, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTicketNotFound
			}
			return err
		}
		if t.Status == model.TicketClosed {
			return ErrTicketClosed
		}
		now := time.Now().Unix()
		t.Status, t.UpdatedAt = model.TicketAnswered, now
		if err := tx.Model(&t).Updates(map[string]any{"status": t.Status, "updated_at": now}).Error; err != nil {
			return err
		}
		return tx.Create(&model.ShopTicketMsg{TicketId: t.Id, FromTg: adminTg, Answer: true, Kind: "text", Text: text, CreatedAt: now}).Error
	})
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// TicketClose closes a ticket; tgID, when not 0, must be its customer.
func (s *ShopService) TicketClose(id uint, tgID int64) (*model.ShopTicket, error) {
	t, err := s.Ticket(id)
	if err != nil {
		return nil, err
	}
	if tgID != 0 && t.TgId != tgID {
		return nil, ErrTicketNotFound
	}
	t.Status, t.UpdatedAt = model.TicketClosed, time.Now().Unix()
	if err := database.GetDB().Model(t).Updates(map[string]any{"status": t.Status, "updated_at": t.UpdatedAt}).Error; err != nil {
		return nil, err
	}
	return t, nil
}

func (s *ShopService) Ticket(id uint) (*model.ShopTicket, error) {
	var t model.ShopTicket
	if err := database.GetDB().First(&t, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTicketNotFound
		}
		return nil, err
	}
	return &t, nil
}

// Tickets lists the newest tickets: of one customer when tgID is not 0, and
// only the ones not closed when open is set.
func (s *ShopService) Tickets(tgID int64, open bool, limit int) ([]model.ShopTicket, error) {
	q := database.GetDB().Model(&model.ShopTicket{})
	if tgID != 0 {
		q = q.Where("tg_id = ?", tgID)
	}
	if open {
		q = q.Where("status <> ?", model.TicketClosed)
	}
	var out []model.ShopTicket
	err := q.Order("updated_at desc, id desc").Limit(limit).Find(&out).Error
	return out, err
}

// OpenTickets counts the tickets waiting for the support.
func (s *ShopService) OpenTickets() int64 {
	var n int64
	database.GetDB().Model(&model.ShopTicket{}).Where("status = ?", model.TicketOpen).Count(&n)
	return n
}

// TicketMsgs is the last messages of a ticket, oldest first.
func (s *ShopService) TicketMsgs(id uint, limit int) ([]model.ShopTicketMsg, error) {
	var out []model.ShopTicketMsg
	if err := database.GetDB().Where("ticket_id = ?", id).Order("id desc").Limit(limit).Find(&out).Error; err != nil {
		return nil, err
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

// ---- activity ----

var seenAt sync.Map // tg id -> unix time of the last write

// seenEvery is how often a customer's last activity is written down.
const seenEvery = 5 * 60

// Seen notes that a shop customer used the bot.
func (s *ShopService) Seen(tgID int64) {
	now := time.Now().Unix()
	if last, ok := seenAt.Load(tgID); ok && now-last.(int64) < seenEvery {
		return
	}
	seenAt.Store(tgID, now)
	database.GetDB().Model(&model.ShopUser{}).Where("tg_id = ?", tgID).Update("seen_at", now)
}

// TakeMenu marks the bottom menu of version ver as a shop user's, and tells
// whether they did not have it yet.
func (s *ShopService) TakeMenu(tgID int64, ver int) bool {
	res := database.GetDB().Model(&model.ShopUser{}).Where("tg_id = ? AND menu_ver < ?", tgID, ver).Update("menu_ver", ver)
	return res.Error == nil && res.RowsAffected > 0
}

// ActiveSince counts the customers who used the bot since then.
func (s *ShopService) ActiveSince(since int64) int64 {
	var n int64
	database.GetDB().Model(&model.ShopUser{}).Where("seen_at >= ? OR joined_at >= ?", since, since).Count(&n)
	return n
}
