package service

import (
	"errors"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Auto-renewal: a customer can have a client renewed from the wallet, with
// the plan it was last bought or renewed with, when it is about to run out.

// ErrNoRenewPlan is returned for a client no plan still on sale was bought
// for.
var ErrNoRenewPlan = errors.New("no plan to renew this client with")

// RenewPlanOf is the plan the client was last bought or renewed with, while
// that plan is still on sale.
func (s *ShopService) RenewPlanOf(clientID uint) (*model.ShopPlan, error) {
	var o model.ShopOrder
	err := database.GetDB().Where("client_id = ? AND status = ? AND kind IN ? AND plan_id > 0",
		clientID, model.OrderApproved, []string{model.OrderBuy, model.OrderRenew}).
		Order("id DESC").Limit(1).Find(&o).Error
	if err != nil {
		return nil, err
	}
	if o.Id == 0 {
		return nil, ErrNoRenewPlan
	}
	p, err := s.Plan(o.PlanId)
	if err != nil || !p.Enable {
		return nil, ErrNoRenewPlan
	}
	return p, nil
}

// AutoRenewOf is the client's auto-renewal row, if it has one.
func (s *ShopService) AutoRenewOf(clientID uint) (*model.ShopAutoRenew, bool) {
	var r model.ShopAutoRenew
	if database.GetDB().Where("client_id = ?", clientID).Limit(1).Find(&r).Error != nil || r.ClientId == 0 {
		return nil, false
	}
	return &r, true
}

// SetAutoRenew turns a client's auto-renewal on or off. Turning it on needs a
// plan to renew with, which it returns.
func (s *ShopService) SetAutoRenew(clientID uint, tgID int64, enable bool) (*model.ShopPlan, error) {
	db := database.GetDB()
	if !enable {
		return nil, db.Model(&model.ShopAutoRenew{}).Where("client_id = ?", clientID).Update("enable", false).Error
	}
	p, err := s.RenewPlanOf(clientID)
	if err != nil {
		return nil, err
	}
	r := model.ShopAutoRenew{ClientId: clientID, TgId: tgID, PlanId: p.Id, Enable: true}
	err = db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "client_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"tg_id", "plan_id", "enable", "fails"}),
	}).Create(&r).Error
	if err != nil {
		return nil, err
	}
	return p, nil
}

// AutoRenews is every client with auto-renewal on.
func (s *ShopService) AutoRenews() ([]model.ShopAutoRenew, error) {
	var out []model.ShopAutoRenew
	err := database.GetDB().Where("enable = ?", true).Order("client_id").Find(&out).Error
	return out, err
}

// DropAutoRenew forgets a client's auto-renewal.
func (s *ShopService) DropAutoRenew(clientID uint) {
	database.GetDB().Where("client_id = ?", clientID).Delete(&model.ShopAutoRenew{})
}

// AutoRenewed records a renewal with planID at the given time, or with
// planID 0 a failure the customer was told about.
func (s *ShopService) AutoRenewed(clientID, planID uint, at int64) {
	q := database.GetDB().Model(&model.ShopAutoRenew{}).Where("client_id = ?", clientID)
	if planID == 0 {
		q.Updates(map[string]any{"last_fail_at": at, "fails": gorm.Expr("fails + 1")})
		return
	}
	q.Updates(map[string]any{"last_at": at, "plan_id": planID, "last_fail_at": 0, "fails": 0})
}

// AutoRenewDue reports whether a client is about to run out: a day or less
// left, or 95% of its volume used.
func AutoRenewDue(c *model.Client, now int64) bool {
	if !c.DelayStart && c.Expiry > 0 && c.Expiry <= now+86400 {
		return true
	}
	return c.Volume > 0 && (c.Up+c.Down)*100 >= c.Volume*95
}
