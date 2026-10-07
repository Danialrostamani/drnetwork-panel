package model

// The shop sells plans through the Telegram bot: customers pay by card
// transfer (an administrator approves the receipt) or from their wallet.

// ShopPlan is something the shop sells: a client with this volume and time.
type ShopPlan struct {
	Id   uint   `json:"id" gorm:"primaryKey;autoIncrement"`
	Name string `json:"name"`
	// Volume is in bytes; 0 is unlimited. Days 0 is no time limit.
	Volume  int64 `json:"volume" gorm:"default:0;not null"`
	Days    int   `json:"days" gorm:"default:0;not null"`
	LimitIp int   `json:"limitIp" gorm:"default:0;not null"`
	// Price is in the shop's currency.
	Price  int64  `json:"price" gorm:"default:0;not null"`
	Group  string `json:"group"`
	Enable bool   `json:"enable" gorm:"default:true;not null"`
	Sort   int    `json:"sort" gorm:"default:0;not null"`
}

func (ShopPlan) TableName() string { return "shop_plans" }

// Order kinds and states.
const (
	OrderBuy   = "buy"
	OrderRenew = "renew"
	OrderTopup = "topup"

	OrderPending  = "pending"
	OrderApproved = "approved"
	OrderRejected = "rejected"
	OrderCanceled = "canceled"

	PayCard   = "card"
	PayWallet = "wallet"
)

// ShopOrder is one purchase, renewal or wallet top-up.
type ShopOrder struct {
	Id       uint   `json:"id" gorm:"primaryKey;autoIncrement"`
	TgId     int64  `json:"tgId" gorm:"index;not null"`
	TgName   string `json:"tgName"`
	Kind     string `json:"kind"`
	PlanId   uint   `json:"planId"`
	PlanName string `json:"planName"`
	// ClientId is the client renewed, or the one the purchase created.
	ClientId   uint   `json:"clientId"`
	ClientName string `json:"clientName"`
	Volume     int64  `json:"volume"`
	Days       int    `json:"days"`
	// Amount is the price before the discount; Paid what the customer pays.
	Amount    int64  `json:"amount"`
	Discount  int64  `json:"discount"`
	Paid      int64  `json:"paid"`
	Code      string `json:"code"`
	Method    string `json:"method"`
	Receipt   string `json:"receipt"`
	Status    string `json:"status" gorm:"index"`
	CreatedAt int64  `json:"createdAt" gorm:"index"`
	DecidedAt int64  `json:"decidedAt"`
	DecidedBy int64  `json:"decidedBy"`
	// Reseller is set when a bot administrator bought it for a customer;
	// Group is the client group a purchase goes to.
	Reseller bool   `json:"reseller"`
	Group    string `json:"group"`
}

func (ShopOrder) TableName() string { return "shop_orders" }

// ShopWallet is the balance of one Telegram user.
type ShopWallet struct {
	TgId    int64 `json:"tgId" gorm:"primaryKey;autoIncrement:false"`
	Balance int64 `json:"balance" gorm:"default:0;not null"`
}

func (ShopWallet) TableName() string { return "shop_wallets" }

// ShopWalletTx is one change of a wallet.
type ShopWalletTx struct {
	Id        uint   `json:"id" gorm:"primaryKey;autoIncrement"`
	TgId      int64  `json:"tgId" gorm:"index;not null"`
	Amount    int64  `json:"amount"`
	Balance   int64  `json:"balance"`
	Reason    string `json:"reason"`
	OrderId   uint   `json:"orderId"`
	By        int64  `json:"by"`
	CreatedAt int64  `json:"createdAt"`
}

func (ShopWalletTx) TableName() string { return "shop_wallet_txs" }

// ShopDiscount is a discount code.
type ShopDiscount struct {
	Id      uint   `json:"id" gorm:"primaryKey;autoIncrement"`
	Code    string `json:"code" gorm:"uniqueIndex"`
	Percent int    `json:"percent"`
	// MaxUses 0 is unlimited; Expiry 0 never.
	MaxUses int   `json:"maxUses"`
	Used    int   `json:"used"`
	Expiry  int64 `json:"expiry"`
	Enable  bool  `json:"enable" gorm:"default:true;not null"`
}

func (ShopDiscount) TableName() string { return "shop_discounts" }

// ShopUser is somebody who started the bot.
type ShopUser struct {
	TgId       int64  `json:"tgId" gorm:"primaryKey;autoIncrement:false"`
	Name       string `json:"name"`
	Username   string `json:"username"`
	ReferredBy int64  `json:"referredBy" gorm:"index"`
	// RefPaid is set once the referrer got their reward.
	RefPaid  bool  `json:"refPaid"`
	TrialAt  int64 `json:"trialAt"`
	JoinedAt int64 `json:"joinedAt"`
	// Blocked users cannot use the shop; Gone ones blocked the bot, so
	// broadcasts skip them.
	Blocked bool `json:"blocked"`
	Gone    bool `json:"gone"`
	// ResellerPercent is the discount a bot administrator gets as a reseller.
	ResellerPercent int `json:"resellerPercent"`
}

func (ShopUser) TableName() string { return "shop_users" }
