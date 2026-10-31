package tgbot

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/service"
)

// The rest of the panel's Telegram bot page: its dashboard, and managing the
// sellers (administrators) themselves — who they are, what they may do, their
// volume limit and the owner's terms with them.

// PanelSection is one part of the bot an administrator can be given.
type PanelSection struct {
	Key  string `json:"key"`
	Icon string `json:"icon"`
	Fa   string `json:"fa"`
	En   string `json:"en"`
}

// PanelSections lists the sections in the order the bot shows them.
func PanelSections() []PanelSection {
	out := make([]PanelSection, 0, len(sectionDefs))
	for _, d := range sectionDefs {
		out = append(out, PanelSection{Key: d.key, Icon: d.icon, Fa: d.fa, En: d.en})
	}
	return out
}

// PanelQuota is an administrator's volume limit, in bytes.
type PanelQuota struct {
	Total   int64 `json:"total"`
	Used    int64 `json:"used"`
	Left    int64 `json:"left"`
	Clients int   `json:"clients"`
}

// panelAdminExtras fills in what the owner decided about an administrator,
// their limit and what they owe since the last settlement.
func (b *bot) panelAdminExtras(pa *PanelAdmin) {
	if pa.Role == "owner" {
		return
	}
	m := adminMeta(pa.Id)
	pa.Suspended, pa.RequireOwner, pa.ReceiptTo = m.Suspended, m.RequireOwner, m.ReceiptTo
	pa.MinPerGB, pa.RatePerGB = m.MinPerGB, m.RatePerGB
	if q, ok, err := b.quotaOf(pa.Id); err == nil && ok {
		pa.Quota = &PanelQuota{Total: q.Total, Used: q.Used, Left: q.Left(), Clients: q.Clients}
	}
	_, s, owed := unsettled(pa.Id)
	pa.UnsettledOrders, pa.UnsettledGB, pa.Owed = s.Orders, s.GB, owed
}

// PanelOrder is an order waiting for approval, as the dashboard lists it.
type PanelOrder struct {
	Id        uint   `json:"id"`
	TgId      int64  `json:"tgId"`
	Customer  string `json:"customer"`
	Topic     string `json:"topic"`
	Detail    string `json:"detail"`
	Amount    int64  `json:"amount"`
	AdminId   int64  `json:"adminId"`
	CreatedAt int64  `json:"createdAt"`
}

// PanelDashboard is the bot at a glance.
type PanelDashboard struct {
	Enabled      bool         `json:"enabled"`
	HasToken     bool         `json:"hasToken"`
	Owner        int64        `json:"owner"`
	Customers    int          `json:"customers"`
	Blocked      int          `json:"blocked"`
	Sellers      int          `json:"sellers"`
	Pending      int64        `json:"pending"`
	TodayOrders  int64        `json:"todayOrders"`
	TodayRevenue int64        `json:"todayRevenue"`
	MonthOrders  int64        `json:"monthOrders"`
	MonthRevenue int64        `json:"monthRevenue"`
	Wallets      int64        `json:"wallets"`
	WalletTotal  int64        `json:"walletTotal"`
	Owed         int64        `json:"owed"`
	Orders       []PanelOrder `json:"orders"`
}

// PanelBotDashboard counts what the owner panel of the bot shows, for the
// web panel.
func PanelBotDashboard() (PanelDashboard, error) {
	cfg, err := loadConfig()
	if err != nil {
		return PanelDashboard{}, err
	}
	b := newBot(cfg)
	d := PanelDashboard{Enabled: cfg.Enable, HasToken: strings.TrimSpace(cfg.Token) != "", Owner: cfg.Owner, Orders: []PanelOrder{}}
	customers := b.customerList()
	names := map[int64]string{}
	for _, c := range customers {
		if c.Blocked {
			d.Blocked++
		}
		names[c.TgId] = strings.TrimSpace(nonempty(c.Name, c.Username))
	}
	d.Customers = len(customers)
	for _, id := range b.sellers() {
		d.Sellers++
		_, _, owed := unsettled(id)
		d.Owed += owed
	}
	now := time.Now()
	d.Pending = countOrders(0, true, orderPending, 0)
	t := sumSales(0, dayStart(b.loc, now), 0)
	m := sumSales(0, now.AddDate(0, 0, -30).Unix(), 0)
	d.TodayOrders, d.TodayRevenue, d.MonthOrders, d.MonthRevenue = t.Orders, t.Revenue, m.Orders, m.Revenue
	d.Wallets, d.WalletTotal = walletsTotal()
	var rows []model.BotOrder
	database.GetDB().Where("status = ?", orderPending).Order("id desc").Limit(20).Find(&rows)
	for _, o := range rows {
		d.Orders = append(d.Orders, PanelOrder{Id: o.Id, TgId: o.TgId, Customer: names[o.TgId], Topic: o.Topic, Detail: o.Detail, Amount: o.Amount, AdminId: o.AdminId, CreatedAt: o.CreatedAt})
	}
	return d, nil
}

// PanelAddAdmin makes id a bot administrator: a seller limited to group (a
// name made from theirs when group is ""), or with full access.
func PanelAddAdmin(ctx context.Context, cs *service.ConfigService, actor, host string, id int64, full bool, group string) (string, error) {
	group = strings.TrimSpace(group)
	if !full && group == "" {
		group = autoGroupName("", id)
		if b, err := panelBot(); err == nil && b.cfg.Token != "" {
			nctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			group = autoGroupName(b.adminNames(nctx, []int64{id})[id], id)
			cancel()
		}
	}
	err := panelEditAccess(cs, actor, host, func(d *accessDoc) error {
		if err := d.add(id); err != nil {
			return err
		}
		if full {
			return d.setRole(id, role{})
		}
		return d.setRole(id, role{group: group})
	})
	if full {
		group = ""
	}
	return group, err
}

// PanelRemoveAdmin takes all access from an administrator, and their limit.
func PanelRemoveAdmin(cs *service.ConfigService, actor, host string, id int64) error {
	err := panelEditAccess(cs, actor, host, func(d *accessDoc) error { return d.remove(id) })
	if err != nil {
		return err
	}
	if q, ok, err := loadQuota(id); err == nil && ok {
		if err := deleteQuota(id); err != nil {
			return err
		}
		recordQuotaAs(actor, id, "del", q.Total, -1)
	}
	return nil
}

// PanelSetAdminRole gives an administrator full access ("full"), a group
// ("group") or some sections ("sections").
func PanelSetAdminRole(cs *service.ConfigService, actor, host string, id int64, kind, group string, sections []string) error {
	var r role
	switch kind {
	case "full":
	case "group":
		r.group = strings.TrimSpace(group)
		if r.group == "" {
			return errBadGroup
		}
	case "sections":
		r.sections = sectionSet{}
		for _, k := range sections {
			if sectionKnown(k) {
				r.sections[k] = true
			}
		}
	default:
		return errors.New("unknown role")
	}
	return panelEditAccess(cs, actor, host, func(d *accessDoc) error {
		if !d.member(id) {
			return errNoSuchAdmin
		}
		return d.setRole(id, r)
	})
}

// panelSeller checks that id is an administrator other than the owner.
func panelSeller(id int64) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	a := cfg.access()
	switch {
	case a.isOwner(id):
		return errIsOwner
	case !a.isMember(id):
		return errNoSuchAdmin
	}
	return nil
}

// PanelSetQuota changes an administrator's volume limit: "set" it to gb,
// "add" gb to it, "reset" what was used, or "remove" it.
func PanelSetQuota(actor string, id int64, op string, gb int64) error {
	if err := panelSeller(id); err != nil {
		return err
	}
	switch op {
	case "set", "add":
		if gb <= 0 && op == "add" {
			return errors.New("the volume must be more than zero")
		}
		var q model.BotQuota
		var err error
		action := "set"
		if op == "add" {
			var found bool
			q, found, err = shiftQuotaTotal(id, gb*gib)
			if err == nil && found {
				action = "add"
			} else if err == nil {
				q, err = setQuotaTotal(id, gb*gib)
			}
		} else {
			q, err = setQuotaTotal(id, max(gb, 0)*gib)
		}
		if err != nil {
			return err
		}
		recordQuotaAs(actor, id, action, q.Total, -1)
	case "reset":
		q, ok, err := loadQuota(id)
		if err != nil || !ok {
			return errors.New("this seller has no volume limit")
		}
		if err := service.BotQuotaRestart(database.GetDB(), id); err != nil {
			return err
		}
		recordQuotaAs(actor, id, "reset", q.Total, -1)
	case "remove":
		q, ok, err := loadQuota(id)
		if err != nil || !ok {
			return err
		}
		if err := deleteQuota(id); err != nil {
			return err
		}
		recordQuotaAs(actor, id, "del", q.Total, -1)
	default:
		return errors.New("unknown action")
	}
	return nil
}

// PanelSellerTerms is what the owner decides about a seller.
type PanelSellerTerms struct {
	Suspended    bool  `json:"suspended"`
	RequireOwner bool  `json:"requireOwner"`
	MinPerGB     int64 `json:"minPerGB"`
	RatePerGB    int64 `json:"ratePerGB"`
	// ReceiptTo: 0 the seller and the owner, -1 the seller alone, else the
	// administrator who approves the seller's orders.
	ReceiptTo int64 `json:"receiptTo"`
}

// PanelSetSellerTerms saves the owner's terms with a seller.
func PanelSetSellerTerms(id int64, t PanelSellerTerms) error {
	if err := panelSeller(id); err != nil {
		return err
	}
	to := t.ReceiptTo
	switch {
	case to == id:
		to = receiptToSelf
	case to < receiptToSelf:
		to = 0
	case to > 0:
		cfg, err := loadConfig()
		if err != nil {
			return err
		}
		if !cfg.access().isMember(to) {
			return errors.New("the chosen approver is not an administrator of the bot")
		}
	}
	_, err := patchAdminMeta(id, func(m *model.BotAdminMeta) {
		m.Suspended, m.RequireOwner, m.ReceiptTo = t.Suspended, t.RequireOwner, to
		m.MinPerGB, m.RatePerGB = max(t.MinPerGB, 0), max(t.RatePerGB, 0)
	})
	return err
}

// PanelSettle records a settlement of every sale of the seller not settled yet.
func PanelSettle(id int64) (model.BotSettlement, error) {
	if err := panelSeller(id); err != nil {
		return model.BotSettlement{}, err
	}
	return settle(id)
}

// PanelGrantPartner grants a trusted person global bot administration while
// retaining their shop, sales and quota. It never changes the configured owner.
func PanelGrantPartner(cs *service.ConfigService, actor, host string, id int64) error {
	return panelEditAccess(cs, actor, host, func(d *accessDoc) error {
		if d.owner <= 0 {
			return errors.New("save the bot owner Telegram ID first")
		}
		if id <= 0 {
			return errBadID
		}
		if d.isOwner(id) {
			return errIsOwner
		}
		if adminMeta(id).Suspended {
			return errors.New("activate this administrator before granting full access")
		}
		if group := d.roleOf(id).group; group != "" {
			raw, err := (&service.SettingService{}).GetTgBotShop()
			if err != nil {
				return err
			}
			var shop map[string]json.RawMessage
			if err = json.Unmarshal([]byte(raw), &shop); err != nil {
				return err
			}
			if shop == nil {
				shop = map[string]json.RawMessage{}
			}
			desks := map[string]map[string]json.RawMessage{}
			if v := shop["desks"]; len(v) > 0 {
				if err = json.Unmarshal(v, &desks); err != nil {
					return err
				}
			}
			if desks == nil {
				desks = map[string]map[string]json.RawMessage{}
			}
			key := strconv.FormatInt(id, 10)
			desk := desks[key]
			if desk == nil {
				desk = map[string]json.RawMessage{}
			}
			// The enforced scope is the seller's actual sales group before promotion.
			desk["salesGroup"], _ = json.Marshal(group)
			desks[key] = desk
			shop["desks"], err = json.Marshal(desks)
			if err != nil {
				return err
			}
			data, err := json.Marshal(shop)
			if err != nil {
				return err
			}
			text := string(data)
			d.shop = &text
		}
		if !d.member(id) {
			if err := d.add(id); err != nil {
				return err
			}
		}
		return d.setRole(id, role{})
	})
}
