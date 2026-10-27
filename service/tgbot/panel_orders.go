package tgbot

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/logger"
	"github.com/Danialrostamani/drnetwork-panel/service"

	"gorm.io/gorm"
)

// The web panel lists every order of the bot, not only the ones waiting, and
// any of them can be corrected there: the amount, the description and the
// seller, and the status. Approving an order that was rejected delivers it as
// approving it in Telegram would; rejecting one that was approved keeps the
// account it made but gives a wallet payment back.

// PanelOrderRow is one order as the orders list shows it.
type PanelOrderRow struct {
	PanelOrder
	Status     string `json:"status"`
	ClientId   uint   `json:"clientId"`
	ClientName string `json:"clientName"`
	UpdatedAt  int64  `json:"updatedAt"`
	WalletPaid bool   `json:"walletPaid"`
	Ref        string `json:"ref"`
	Settled    bool   `json:"settled"`
	Receipt    bool   `json:"receipt"`
}

// PanelOrderQuery picks the orders: by status ("" for all), seller (-1 for
// all, 0 for the owner's own), and words in the customer, detail or client.
type PanelOrderQuery struct {
	Status string
	Seller int64
	Search string
	Limit  int
	Offset int
}

// PanelOrderPage is a page of orders with how many there are of each status.
type PanelOrderPage struct {
	Rows   []PanelOrderRow  `json:"rows"`
	Total  int64            `json:"total"`
	Counts map[string]int64 `json:"counts"`
}

var orderStatuses = []string{orderPending, orderApproved, orderRejected, orderCancelled, orderReset}

func knownOrderStatus(s string) bool {
	for _, k := range orderStatuses {
		if s == k {
			return true
		}
	}
	return false
}

// PanelOrders lists orders newest first.
func PanelOrders(q PanelOrderQuery) (PanelOrderPage, error) {
	out := PanelOrderPage{Rows: []PanelOrderRow{}, Counts: map[string]int64{}}
	if q.Status != "" && !knownOrderStatus(q.Status) {
		return out, errors.New("unknown status")
	}
	if q.Limit <= 0 || q.Limit > 200 {
		q.Limit = 50
	}
	if q.Offset < 0 {
		q.Offset = 0
	}
	db := database.GetDB()
	names := map[int64]string{}
	var customers []model.BotCustomer
	if err := db.Select("tg_id", "name", "username").Find(&customers).Error; err != nil {
		return out, err
	}
	for _, c := range customers {
		names[c.TgId] = strings.TrimSpace(nonempty(c.Name, c.Username))
	}
	base := db.Model(&model.BotOrder{})
	if q.Seller >= 0 {
		base = base.Where("admin_id = ?", q.Seller)
	}
	if s := strings.TrimSpace(q.Search); s != "" {
		like := "%" + strings.ToLower(s) + "%"
		cond := "LOWER(detail) LIKE ? OR LOWER(hint) LIKE ? OR LOWER(ref) LIKE ? OR CAST(tg_id AS TEXT) LIKE ? OR CAST(id AS TEXT) = ?"
		args := []any{like, like, like, like, strings.TrimPrefix(s, "#")}
		var hit []int64
		for id, n := range names {
			if strings.Contains(strings.ToLower(n), strings.ToLower(s)) {
				hit = append(hit, id)
			}
		}
		var clientIDs []uint
		db.Model(&model.Client{}).Where("LOWER(name) LIKE ?", like).Pluck("id", &clientIDs)
		if len(hit) > 0 {
			cond += " OR tg_id IN ?"
			args = append(args, hit)
		}
		if len(clientIDs) > 0 {
			cond += " OR client_id IN ?"
			args = append(args, clientIDs)
		}
		base = base.Where("("+cond+")", args...)
	}
	type count struct {
		Status string
		N      int64
	}
	var counts []count
	if err := base.Session(&gorm.Session{}).Select("status, COUNT(*) AS n").Group("status").Scan(&counts).Error; err != nil {
		return out, err
	}
	for _, c := range counts {
		out.Counts[c.Status] = c.N
		if q.Status == "" || q.Status == c.Status {
			out.Total += c.N
		}
	}
	list := base.Session(&gorm.Session{})
	if q.Status != "" {
		list = list.Where("status = ?", q.Status)
	}
	var rows []model.BotOrder
	if err := list.Order("id desc").Limit(q.Limit).Offset(q.Offset).Find(&rows).Error; err != nil {
		return out, err
	}
	clientNames := map[uint]string{}
	var ids []uint
	for _, o := range rows {
		if o.ClientId != 0 {
			ids = append(ids, o.ClientId)
		}
	}
	if len(ids) > 0 {
		var cs []model.Client
		db.Select("id", "name").Where("id IN ?", uniqueIDs(ids)).Find(&cs)
		for _, c := range cs {
			clientNames[c.Id] = c.Name
		}
	}
	for _, o := range rows {
		out.Rows = append(out.Rows, panelOrderRow(&o, names[o.TgId], clientNames[o.ClientId]))
	}
	return out, nil
}

func panelOrderRow(o *model.BotOrder, customer, client string) PanelOrderRow {
	return PanelOrderRow{
		PanelOrder: PanelOrder{Id: o.Id, TgId: o.TgId, Customer: customer, Topic: o.Topic, Detail: o.Detail, Amount: o.Amount, AdminId: o.AdminId, CreatedAt: o.CreatedAt},
		Status:     o.Status, ClientId: o.ClientId, ClientName: client, UpdatedAt: o.UpdatedAt, WalletPaid: o.WalletPaid,
		Ref: o.Ref, Settled: o.SettlementId != 0, Receipt: o.ReceiptID != "",
	}
}

// PanelOrderEdit is a change to one order. Nil fields stay as they are.
// Status is "" to keep it, "approved" or "rejected". Notify tells the
// customer of a status change through the bot.
type PanelOrderEdit struct {
	Id     uint
	Amount *int64
	Detail *string
	Seller *int64
	Status string
	Notify bool
}

var panelOrderMu sync.Mutex

var (
	errOrderSettled = errors.New("this order is in a settlement with the seller already; its amount, seller and status can no longer change")
	errOrderStatus  = errors.New("this order cannot go to that status")
)

// PanelEditOrder applies e and returns the order as it is now.
func PanelEditOrder(ctx context.Context, cs *service.ConfigService, actor string, e PanelOrderEdit) (PanelOrderRow, error) {
	panelOrderMu.Lock()
	defer panelOrderMu.Unlock()
	o, err := loadOrder(e.Id)
	if err != nil {
		return PanelOrderRow{}, errors.New("order not found")
	}
	b, err := panelBot()
	if err != nil {
		return PanelOrderRow{}, err
	}
	b.configService = cs
	before := *o
	if e.Status == o.Status {
		e.Status = ""
	}
	settled := o.SettlementId != 0
	if settled && (e.Status != "" || (e.Amount != nil && *e.Amount != o.Amount) || (e.Seller != nil && *e.Seller != o.AdminId)) {
		return PanelOrderRow{}, errOrderSettled
	}
	if e.Seller != nil && *e.Seller != o.AdminId {
		if *e.Seller < 0 || (*e.Seller != 0 && !b.access().isMember(*e.Seller)) {
			return PanelOrderRow{}, errors.New("that seller is not an administrator of the bot")
		}
		o.AdminId = *e.Seller
	}
	if e.Amount != nil && *e.Amount != o.Amount {
		if *e.Amount < 0 {
			return PanelOrderRow{}, errors.New("the amount cannot be negative")
		}
		// Money that already moved in a wallet moved at the old amount.
		if o.WalletPaid && o.Status == orderApproved || o.Topic == "wallet" && o.Status == orderApproved {
			return PanelOrderRow{}, errors.New("the wallet was already charged with this amount; change the status first or correct the wallet")
		}
		o.Amount = *e.Amount
	}
	if e.Detail != nil {
		o.Detail = truncate(strings.TrimSpace(*e.Detail), 300)
	}
	approver := b
	if a := b.access(); a.owner != 0 {
		approver = b.as(a.owner)
	}
	notify := ""
	switch e.Status {
	case "":
		if err := b.saveOrder(o); err != nil {
			return PanelOrderRow{}, err
		}
	case orderApproved:
		if o.Status != orderPending && o.Status != orderRejected && o.Status != orderCancelled {
			return PanelOrderRow{}, errOrderStatus
		}
		// A wallet payment given back when the order was rejected is taken
		// again; without the money it stays as it was.
		recharge := o.WalletPaid && o.Status != orderPending && o.Amount > 0
		if recharge {
			ok, err := walletSpend(o.TgId, o.Amount, walletMemo{kind: walletPay, order: o.Id})
			if err != nil {
				return PanelOrderRow{}, err
			}
			if !ok {
				return PanelOrderRow{}, errors.New("the customer's wallet no longer has the amount this order was paid with")
			}
		}
		if err := approver.approveOrder(o); err != nil {
			if recharge {
				_, _ = walletCredit(o.TgId, o.Amount, walletMemo{kind: walletRefund, order: o.Id})
			}
			return PanelOrderRow{}, err
		}
		notify = orderApproved
	case orderRejected:
		switch o.Status {
		case orderPending, orderCancelled:
			if o.WalletPaid && o.Amount > 0 {
				_, _ = walletCredit(o.TgId, o.Amount, walletMemo{kind: walletRefund, order: o.Id})
			}
		case orderApproved:
			if o.WalletPaid && o.Amount > 0 {
				_, _ = walletCredit(o.TgId, o.Amount, walletMemo{kind: walletRefund, order: o.Id})
			}
			if o.Topic == "wallet" && o.Amount > 0 {
				// The top-up comes off the balance, as far as it is still there.
				_, _ = walletCredit(o.TgId, -o.Amount, walletMemo{kind: walletManual, order: o.Id, note: "order #" + strconv.FormatUint(uint64(o.Id), 10) + " rejected"})
			}
		default:
			return PanelOrderRow{}, errOrderStatus
		}
		o.Status = orderRejected
		if err := b.saveOrder(o); err != nil {
			return PanelOrderRow{}, err
		}
		notify = orderRejected
	default:
		return PanelOrderRow{}, errOrderStatus
	}
	logger.Infof("telegram bot: %s edited order #%d (%s %d -> %s %d, seller %d -> %d)", actor, o.Id, before.Status, before.Amount, o.Status, o.Amount, before.AdminId, o.AdminId)
	if notify != "" && e.Notify && !customerBlocked(o.TgId) && b.cfg.Enable && b.cfg.Token != "" {
		ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		approver.notifyCustomerOrder(ctx, o, notify == orderApproved)
		if notify == orderApproved {
			approver.maybeQuotaWarn(ctx, o.AdminId)
		}
		cancel()
	}
	customer := ""
	if c, ok := customerRow(o.TgId); ok {
		customer = strings.TrimSpace(nonempty(c.Name, c.Username))
	}
	client := ""
	if o.ClientId != 0 {
		var c model.Client
		if database.GetDB().Select("id", "name").First(&c, o.ClientId).Error == nil {
			client = c.Name
		}
	}
	return panelOrderRow(o, customer, client), nil
}
