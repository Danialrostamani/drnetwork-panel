package tgbot

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/service"

	"gorm.io/gorm"
)

// What the panel's customers tab does to one customer beyond blocking,
// moving and crediting them: edit the owner's note and the configs bound to
// them, give them another test account, or forget them.

// orderReset marks a test account the owner reset, so the customer may take
// another one; it is no longer counted as the trial they had.
const orderReset = "reset"

const maxCustomerNote = 500

var errNoCustomer = errors.New("bad telegram id")

// PanelEditCustomer sets the owner's note on a customer and, with
// setClients, binds exactly these clients to them: the ones bound now and
// not listed are unbound, the listed ones are bound even if another customer
// had them.
func PanelEditCustomer(tgID int64, note string, setClients bool, clients []uint) error {
	if tgID <= 0 {
		return errNoCustomer
	}
	note = strings.TrimSpace(note)
	if r := []rune(note); len(r) > maxCustomerNote {
		note = string(r[:maxCustomerNote])
	}
	db := database.GetDB()
	return db.Transaction(func(tx *gorm.DB) error {
		if setClients {
			ids := uniqueIDs(clients)
			if len(ids) > 0 {
				var n int64
				if err := tx.Model(&model.Client{}).Where("id IN ?", ids).Count(&n).Error; err != nil {
					return err
				}
				if int(n) != len(ids) {
					return errors.New("a chosen client no longer exists")
				}
			}
			q := tx.Model(&model.Client{}).Where("tg_id = ?", tgID)
			if len(ids) > 0 {
				q = q.Where("id NOT IN ?", ids)
			}
			if err := q.Update("tg_id", 0).Error; err != nil {
				return err
			}
			if len(ids) > 0 {
				if err := tx.Model(&model.Client{}).Where("id IN ?", ids).Update("tg_id", tgID).Error; err != nil {
					return err
				}
			}
		}
		var c model.BotCustomer
		if err := tx.Where("tg_id = ?", tgID).Limit(1).Find(&c).Error; err != nil {
			return err
		}
		c.TgId, c.Note = tgID, note
		if c.FirstSeen == 0 {
			c.FirstSeen = time.Now().Unix()
		}
		return tx.Save(&c).Error
	})
}

func uniqueIDs(ids []uint) []uint {
	seen := map[uint]bool{}
	var out []uint
	for _, id := range ids {
		if id != 0 && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// deleteClientsBy removes these clients the way the web panel does, so the
// inbounds, the nodes and the sellers' volume follow.
func deleteClientsBy(cs *service.ConfigService, actor, host string, ids []uint) error {
	if len(ids) == 0 {
		return nil
	}
	if cs == nil {
		return errors.New("the panel is not ready to save clients")
	}
	raw, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	_, err = cs.Save("clients", "delbulk", raw, "", actor, host)
	return err
}

// PanelTrialReset is what resetting a customer's test account did.
type PanelTrialReset struct {
	Orders  int      `json:"orders"`
	Deleted []string `json:"deleted"`
	// Bound is how many clients the customer still has; the bot gives a
	// test account only to customers without one.
	Bound    int  `json:"bound"`
	Notified bool `json:"notified"`
}

// PanelResetTrial lets a customer take another test account: their test
// orders stop counting and, with deleteClient, the test clients they got are
// deleted. With notify the customer is told through the bot.
func PanelResetTrial(ctx context.Context, cs *service.ConfigService, actor, host string, tgID int64, deleteClient, notify bool) (PanelTrialReset, error) {
	out := PanelTrialReset{Deleted: []string{}}
	if tgID <= 0 {
		return out, errNoCustomer
	}
	db := database.GetDB()
	var orders []model.BotOrder
	if err := db.Where("tg_id = ? AND topic = ? AND status = ?", tgID, "test", orderApproved).Find(&orders).Error; err != nil {
		return out, err
	}
	if len(orders) == 0 {
		return out, errors.New("this customer has not had a test account")
	}
	if deleteClient {
		var ids []uint
		for _, o := range orders {
			if o.ClientId != 0 {
				ids = append(ids, o.ClientId)
			}
		}
		var list []model.Client
		if ids = uniqueIDs(ids); len(ids) > 0 {
			if err := db.Select("id", "name").Where("id IN ?", ids).Order("id").Find(&list).Error; err != nil {
				return out, err
			}
		}
		ids = ids[:0]
		for _, c := range list {
			ids = append(ids, c.Id)
			out.Deleted = append(out.Deleted, c.Name)
		}
		if err := deleteClientsBy(cs, actor, host, ids); err != nil {
			return PanelTrialReset{Deleted: []string{}}, err
		}
	}
	res := db.Model(&model.BotOrder{}).Where("tg_id = ? AND topic = ? AND status = ?", tgID, "test", orderApproved).Update("status", orderReset)
	if res.Error != nil {
		return out, res.Error
	}
	out.Orders = int(res.RowsAffected)
	out.Bound = len(boundClients(tgID))
	if notify && !customerBlocked(tgID) {
		if b, err := panelBot(); err == nil && b.cfg.Enable && b.cfg.Token != "" {
			ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			msg := b.tr("🔑 اکانت تست شما ریست شد؛ می‌توانید دوباره اکانت تست بگیرید.",
				"🔑 Your test account was reset; you can get a test account again.")
			if out.Bound > 0 {
				msg = b.tr("🔑 اکانت تست شما ریست شد.", "🔑 Your test account was reset.")
			}
			out.Notified = b.call(ctx, "sendMessage", map[string]any{"chat_id": tgID, "text": msg, "parse_mode": "HTML"}, nil) == nil
			cancel()
		}
	}
	return out, nil
}

// PanelCustomerDeleted is what deleting a customer did.
type PanelCustomerDeleted struct {
	Deleted int   `json:"deleted"`
	Unbound int   `json:"unbound"`
	Balance int64 `json:"balance"`
}

// PanelDeleteCustomer forgets a customer: their profile, introducer, wallet
// and orders that were not approved go; the approved ones stay for the sellers'
// accounts but no longer name them. Their clients are deleted with
// deleteClients, else only unbound so they keep working. If they write to
// the bot again they start as a new customer.
func PanelDeleteCustomer(cs *service.ConfigService, actor, host string, tgID int64, deleteClients bool) (PanelCustomerDeleted, error) {
	var out PanelCustomerDeleted
	if tgID <= 0 {
		return out, errNoCustomer
	}
	db := database.GetDB()
	var bound []uint
	if err := db.Model(&model.Client{}).Where("tg_id = ?", tgID).Order("id").Pluck("id", &bound).Error; err != nil {
		return out, err
	}
	if deleteClients {
		if err := deleteClientsBy(cs, actor, host, bound); err != nil {
			return out, err
		}
		out.Deleted = len(bound)
	}
	out.Balance = walletBalance(tgID)
	err := db.Transaction(func(tx *gorm.DB) error {
		if !deleteClients && len(bound) > 0 {
			res := tx.Model(&model.Client{}).Where("tg_id = ?", tgID).Update("tg_id", 0)
			if res.Error != nil {
				return res.Error
			}
			out.Unbound = int(res.RowsAffected)
		}
		if err := tx.Where("tg_id = ? AND status <> ?", tgID, orderApproved).Delete(&model.BotOrder{}).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.BotOrder{}).Where("tg_id = ?", tgID).Update("tg_id", 0).Error; err != nil {
			return err
		}
		for _, m := range []any{&model.BotRef{}, &model.BotWallet{}, &model.BotWalletTx{}, &model.BotCustomer{}} {
			if err := tx.Where("tg_id = ?", tgID).Delete(m).Error; err != nil {
				return err
			}
		}
		return nil
	})
	return out, err
}
