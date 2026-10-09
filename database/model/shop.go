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
	// Extra is what a card payment adds to Paid to make the amount unique
	// among the open orders; it goes to the wallet when the order is
	// approved. The customer transfers Paid + Extra.
	Extra int64 `json:"extra" gorm:"default:0;not null"`
	// Card is the payment card the order was given, of the shop's cards.
	Card string `json:"card"`
	// ReceiptKey identifies the receipt (the photo's or file's unique id, or
	// the digits of a tracking number), so one receipt pays for one order.
	ReceiptKey string `json:"receiptKey" gorm:"index"`
	// Auto marks a renewal the wallet paid for on its own.
	Auto bool `json:"auto"`
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

// Code kinds: a discount on a purchase, or a gift that tops up the wallet.
const (
	CodeDiscount = "discount"
	CodeGift     = "gift"
)

// ShopDiscount is a discount code or a gift code.
type ShopDiscount struct {
	Id      uint   `json:"id" gorm:"primaryKey;autoIncrement"`
	Code    string `json:"code" gorm:"uniqueIndex"`
	Percent int    `json:"percent"`
	// MaxUses 0 is unlimited; Expiry 0 never.
	MaxUses int   `json:"maxUses"`
	Used    int   `json:"used"`
	Expiry  int64 `json:"expiry"`
	Enable  bool  `json:"enable" gorm:"default:true;not null"`
	// Kind is CodeDiscount ("" in older rows) or CodeGift; a gift code adds
	// Amount to the wallet of whoever redeems it, once per customer.
	Kind   string `json:"kind"`
	Amount int64  `json:"amount" gorm:"default:0;not null"`
	// PlanIds limits a discount to some plans (comma separated ids, "" is
	// every plan); Kinds to purchases or renewals ("buy", "renew", "" both).
	PlanIds string `json:"planIds"`
	Kinds   string `json:"kinds"`
	// OncePerUser lets each customer use the discount once.
	OncePerUser bool `json:"oncePerUser"`
}

func (ShopDiscount) TableName() string { return "shop_discounts" }

// IsGift tells a gift code.
func (d *ShopDiscount) IsGift() bool { return d.Kind == CodeGift }

// ShopCodeUse is one use of a code by a customer.
type ShopCodeUse struct {
	Id        uint   `json:"id" gorm:"primaryKey;autoIncrement"`
	CodeId    uint   `json:"codeId" gorm:"index"`
	Code      string `json:"code" gorm:"index"`
	TgId      int64  `json:"tgId" gorm:"index"`
	OrderId   uint   `json:"orderId"`
	CreatedAt int64  `json:"createdAt"`
}

func (ShopCodeUse) TableName() string { return "shop_code_uses" }

// SMS states.
const (
	SmsApproved  = "approved"
	SmsUnmatched = "unmatched"
	SmsIgnored   = "ignored"
	SmsDuplicate = "duplicate"
	SmsError     = "error"
)

// ShopSms is a bank message the panel received, and what came of it.
type ShopSms struct {
	Id     uint   `json:"id" gorm:"primaryKey;autoIncrement"`
	Hash   string `json:"-" gorm:"uniqueIndex"`
	Sender string `json:"sender"`
	Text   string `json:"text"`
	// Amount is the deposit read from the text, in the shop's currency.
	Amount    int64  `json:"amount"`
	OrderId   uint   `json:"orderId"`
	Status    string `json:"status"`
	Note      string `json:"note"`
	CreatedAt int64  `json:"createdAt" gorm:"index"`
}

func (ShopSms) TableName() string { return "shop_sms" }

// ShopAutoRenew is a client its owner wants renewed from the wallet.
type ShopAutoRenew struct {
	ClientId uint  `json:"clientId" gorm:"primaryKey;autoIncrement:false"`
	TgId     int64 `json:"tgId" gorm:"index"`
	PlanId   uint  `json:"planId"`
	Enable   bool  `json:"enable"`
	// LastAt is the last renewal, LastFailAt the last failure told and Fails
	// how many were told since the last renewal.
	LastAt     int64 `json:"lastAt"`
	LastFailAt int64 `json:"lastFailAt"`
	Fails      int   `json:"fails" gorm:"default:0;not null"`
}

func (ShopAutoRenew) TableName() string { return "shop_auto_renews" }

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
