package model

import (
	"crypto/rand"
	"encoding/json"
	"math/big"

	"gorm.io/gorm"
)

type Setting struct {
	Id    uint   `json:"id" form:"id" gorm:"primaryKey;autoIncrement"`
	Key   string `json:"key" form:"key"`
	Value string `json:"value" form:"value"`
}

type Tls struct {
	Id     uint            `json:"id" form:"id" gorm:"primaryKey;autoIncrement"`
	Name   string          `json:"name" form:"name"`
	Server json.RawMessage `json:"server" form:"server"`
	Client json.RawMessage `json:"client" form:"client"`
}

type User struct {
	Id         uint   `json:"id" form:"id" gorm:"primaryKey;autoIncrement"`
	Username   string `json:"username" form:"username"`
	Password   string `json:"password" form:"password"`
	LastLogins string `json:"lastLogin"`
	// Totp is the two-factor secret; empty while two-factor login is off.
	Totp string `json:"-" form:"-"`
}

type Client struct {
	Id     uint `json:"id" form:"id" gorm:"primaryKey;autoIncrement"`
	Enable bool `json:"enable" form:"enable"`
	// Name is the join key on every hot path: the stats job and every
	// subscription fetch resolve a client by it. Uniqueness stays in
	// ClientService.validateClientName -- a unique index would need existing
	// duplicates renamed, and a name is the subscription ID, so renaming one
	// breaks that user's link.
	Name string `json:"name" form:"name" gorm:"index"`
	// SubToken is the random ID of the client's subscription link. The name
	// served as that ID before, and a name anybody can guess opened anybody's
	// configs; the panel makes the token itself (BeforeCreate) and only
	// ClientService.ResetSubToken changes it.
	SubToken string `json:"subToken" form:"subToken" gorm:"index;default:'';not null"`
	// UseSubToken puts the token in the client's link in place of its name,
	// and then the link by name no longer opens the subscription. Off, the
	// link is the name, as before tokens.
	UseSubToken bool            `json:"useSubToken" form:"useSubToken" gorm:"default:false;not null"`
	Config      json.RawMessage `json:"config,omitempty" form:"config"`
	Inbounds    json.RawMessage `json:"inbounds" form:"inbounds"`
	Links       json.RawMessage `json:"links,omitempty" form:"links"`
	Volume      int64           `json:"volume" form:"volume"`
	Expiry      int64           `json:"expiry" form:"expiry"`
	Down        int64           `json:"down" form:"down"`
	Up          int64           `json:"up" form:"up"`
	Desc        string          `json:"desc" form:"desc"`
	Group       string          `json:"group" form:"group"`
	Remark      string          `json:"remark" form:"remark"`

	// Timestamps (unix seconds): creation time and last time the client had traffic
	CreatedAt int64 `json:"createdAt" form:"createdAt" gorm:"default:0;not null"`
	OnlineAt  int64 `json:"onlineAt" form:"onlineAt" gorm:"default:0;not null"`

	// Maximum concurrently connected source IP identities; 0 means unlimited.
	LimitIp int `json:"limitIp" form:"limitIp" gorm:"default:0;not null;index"`

	// Telegram user allowed to look this client up in the bot; 0 = nobody. It is
	// bound through the bot, so the panel's own client form does not carry it
	// and ClientService.preserveServerOwnedFields keeps an edit from erasing it.
	TgId int64 `json:"tgId" form:"tgId" gorm:"default:0;not null;index"`

	// Delay start and periodic reset
	DelayStart bool  `json:"delayStart" form:"delayStart" gorm:"default:false;not null"`
	AutoReset  bool  `json:"autoReset" form:"autoReset" gorm:"default:false;not null"`
	ResetDays  int   `json:"resetDays" form:"resetDays" gorm:"default:0;not null"`
	NextReset  int64 `json:"nextReset" form:"nextReset" gorm:"default:0;not null"`
	TotalUp    int64 `json:"totalUp" form:"totalUp" gorm:"default:0;not null"`
	TotalDown  int64 `json:"totalDown" form:"totalDown" gorm:"default:0;not null"`

	// Daily limits: the bytes (DailyVolume) and the seconds online (DailyTime)
	// the client may use in one day of the panel's time zone; 0 is no limit.
	// Past one the client is disabled with DayLocked set, and enabled again
	// when the day changes.
	DailyVolume int64 `json:"dailyVolume" form:"dailyVolume" gorm:"default:0;not null"`
	DailyTime   int64 `json:"dailyTime" form:"dailyTime" gorm:"default:0;not null"`
	// What the client used on DayKey (yyyymmdd), kept by the stats jobs.
	DayUsed   int64 `json:"dayUsed" form:"dayUsed" gorm:"default:0;not null"`
	DayOnline int64 `json:"dayOnline" form:"dayOnline" gorm:"default:0;not null"`
	DayKey    int   `json:"dayKey" form:"dayKey" gorm:"default:0;not null"`
	DayLocked bool  `json:"dayLocked" form:"dayLocked" gorm:"default:false;not null"`
}

// ClientDay is what a client moved in one day (yyyymmdd, the panel's time
// zone), on every server together: the history the subscription page shows
// the last 30 days of.
type ClientDay struct {
	ClientId uint  `json:"clientId" gorm:"primaryKey;autoIncrement:false"`
	Day      int   `json:"day" gorm:"primaryKey;autoIncrement:false;index"`
	Up       int64 `json:"up"`
	Down     int64 `json:"down"`
}

// SubTokenLength is how long a subscription token is: 24 letters and digits,
// far beyond guessing.
const SubTokenLength = 24

const subTokenAlphabet = "abcdefghijklmnopqrstuvwxyz0123456789"

// NewSubToken makes a subscription token from crypto/rand.
func NewSubToken() string {
	out := make([]byte, SubTokenLength)
	max := big.NewInt(int64(len(subTokenAlphabet)))
	for i := range out {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			panic("subscription token: " + err.Error())
		}
		out[i] = subTokenAlphabet[n.Int64()]
	}
	return string(out)
}

// BeforeCreate gives a new client its subscription token.
func (c *Client) BeforeCreate(tx *gorm.DB) error {
	if c.SubToken == "" {
		c.SubToken = NewSubToken()
	}
	return nil
}

type Stats struct {
	Id uint64 `json:"id" gorm:"primaryKey;autoIncrement"`
	// date_time sits third in idx_stats_bucket, so that index cannot serve the
	// retention purge, which filters on date_time alone.
	DateTime  int64  `json:"dateTime" gorm:"uniqueIndex:idx_stats_bucket,priority:3;index:idx_stats_date_time"`
	Resource  string `json:"resource" gorm:"uniqueIndex:idx_stats_bucket,priority:1"`
	Tag       string `json:"tag" gorm:"uniqueIndex:idx_stats_bucket,priority:2"`
	Direction bool   `json:"direction" gorm:"uniqueIndex:idx_stats_bucket,priority:4"`
	Traffic   int64  `json:"traffic"`
}

type Changes struct {
	Id       uint64          `json:"id" gorm:"primaryKey;autoIncrement"`
	DateTime int64           `json:"dateTime"`
	Actor    string          `json:"actor"`
	Key      string          `json:"key"`
	Action   string          `json:"action"`
	Obj      json.RawMessage `json:"obj"`
}

// BotQuota is the volume limit of a Telegram bot administrator: the most
// traffic the clients counted as theirs may consume. The owner sets Total from
// the bot's Admins screen. What is used is not stored as a running balance: it
// is worked out from the clients themselves (see BotQuotaClient) plus Banked,
// the usage of counted clients that were deleted since. An administrator
// without a row is not limited. The row lives in the database, not in the
// settings, because it changes whenever one of their clients is deleted.
type BotQuota struct {
	TgId  int64 `json:"tgId" gorm:"primaryKey;autoIncrement:false"`
	Total int64 `json:"total" gorm:"default:0;not null"`
	// Banked is the traffic of counted clients that were deleted.
	Banked int64 `json:"banked" gorm:"default:0;not null"`
	// Sold is volume given out when this admin's shop sales are approved
	// (plan GB). It counts against the limit immediately, not as traffic.
	Sold int64 `json:"sold" gorm:"default:0;not null"`
	// Warn80 is set after the owner has been told this admin reached 80% of
	// their limit, so the same crossing is not announced again until usage
	// drops back below 80%.
	Warn80 bool `json:"warn80" gorm:"default:false;not null"`
	// Warn100 is the same for the limit being used up.
	Warn100 bool `json:"warn100" gorm:"default:false;not null"`
	// Adopted is set once the clients the administrator created through the
	// bot before the limit existed have been looked up in the change history.
	Adopted bool `json:"adopted" gorm:"default:false;not null"`
}

// TableName pins the name: left to GORM's pluralizer, "quota" stays singular
// and the table would not match the one the backup lists.
func (BotQuota) TableName() string { return "bot_quotas" }

// BotQuotaClient says that a client counts against an administrator's volume
// limit, and from where: Base is the client's lifetime traffic (what it used
// before its last reset included) when it started to count, so what counts is
// the traffic it used since. A client belongs to at most one administrator.
type BotQuotaClient struct {
	ClientId uint  `json:"clientId" gorm:"primaryKey;autoIncrement:false"`
	TgId     int64 `json:"tgId" gorm:"index;not null"`
	Base     int64 `json:"base" gorm:"default:0;not null"`
	// Prepaid marks a client sold through the administrator's shop: its plan
	// volume was already taken from the limit (BotQuota.Sold) when the sale
	// was approved, so its traffic is not counted a second time.
	Prepaid bool `json:"prepaid" gorm:"default:false;not null"`
}

// TableName pins the name, for the same reason as BotQuota's.
func (BotQuotaClient) TableName() string { return "bot_quota_clients" }

// BotOrder is a customer shop request: buy, renew, wallet or trial. The owner
// approves or rejects it from Telegram; the customer sees the status.
type BotOrder struct {
	Id        uint   `json:"id" gorm:"primaryKey;autoIncrement"`
	TgId      int64  `json:"tgId" gorm:"index;not null"`
	Topic     string `json:"topic" gorm:"index"`
	PlanID    string `json:"planId"`
	Extra     string `json:"extra"`
	Detail    string `json:"detail"`
	Status    string `json:"status" gorm:"index"`
	ReceiptID string `json:"receiptId"`
	ClientId  uint   `json:"clientId"`
	AdminId   int64  `json:"adminId" gorm:"index"`
	Ref       string `json:"ref"`
	// Hint is a readable client name taken from the customer's Telegram
	// username (or a sanitized first name) when the order is placed.
	Hint string `json:"hint"`
	// The plan as the customer saw it when the order was placed, so a plan
	// edited or deleted before approval cannot change what is delivered.
	// PlanSet tells these apart from orders placed by older versions.
	PlanSet   bool   `json:"planSet" gorm:"default:false;not null"`
	PlanGB    int    `json:"planGB" gorm:"default:0;not null"`
	PlanDays  int    `json:"planDays" gorm:"default:0;not null"`
	PlanGroup string `json:"planGroup"`
	// Amount is the top-up of a wallet order, and the plan price in wallet
	// units for a buy or renew (0 when the price is not a number).
	Amount int64 `json:"amount" gorm:"default:0;not null"`
	// WalletPaid is set when a buy or renew was paid from the wallet balance.
	WalletPaid bool `json:"walletPaid" gorm:"default:false;not null"`
	// SettlementId is the settlement with the seller that covered this sale.
	SettlementId uint `json:"settlementId" gorm:"default:0;not null;index"`
	// Reminded is set once the owner was told the order waits too long.
	Reminded  bool  `json:"reminded" gorm:"default:false;not null"`
	CreatedAt int64 `json:"createdAt" gorm:"index"`
	UpdatedAt int64 `json:"updatedAt"`
}

func (BotOrder) TableName() string { return "bot_orders" }

// BotRef is the introducer word a customer sent, so they do not type it again
// and new clients go into that introducer's group.
type BotRef struct {
	TgId  int64  `json:"tgId" gorm:"primaryKey;autoIncrement:false"`
	Word  string `json:"word" gorm:"not null"`
	Group string `json:"group"`
}

func (BotRef) TableName() string { return "bot_refs" }

// BotWallet is a shop customer's wallet balance, in the shop's currency
// unit. It is credited when a top-up order is approved and spent when the
// customer pays a plan from it.
type BotWallet struct {
	TgId      int64 `json:"tgId" gorm:"primaryKey;autoIncrement:false"`
	Balance   int64 `json:"balance" gorm:"default:0;not null"`
	UpdatedAt int64 `json:"updatedAt"`
}

func (BotWallet) TableName() string { return "bot_wallets" }

// BotWalletTx is one change of a wallet balance: a top-up, a payment, a
// refund or a manual correction by the owner.
type BotWalletTx struct {
	Id      uint   `json:"id" gorm:"primaryKey;autoIncrement"`
	TgId    int64  `json:"tgId" gorm:"index;not null"`
	Delta   int64  `json:"delta" gorm:"default:0;not null"`
	Balance int64  `json:"balance" gorm:"default:0;not null"`
	Kind    string `json:"kind"`
	OrderId uint   `json:"orderId"`
	// Actor is the Telegram ID of who made a manual change (0 otherwise).
	Actor     int64  `json:"actor"`
	Note      string `json:"note"`
	CreatedAt int64  `json:"createdAt" gorm:"index"`
}

func (BotWalletTx) TableName() string { return "bot_wallet_txs" }

// BotCustomer is somebody who used the shop: who they are, when they were
// seen, and what the owner decided about them.
type BotCustomer struct {
	TgId     int64  `json:"tgId" gorm:"primaryKey;autoIncrement:false"`
	Name     string `json:"name"`
	Username string `json:"username" gorm:"index"`
	Blocked  bool   `json:"blocked" gorm:"default:false;not null"`
	// AdminId, when set, is the seller the owner gave this customer to; it
	// wins over the introducer.
	AdminId   int64  `json:"adminId" gorm:"default:0;not null"`
	Note      string `json:"note"`
	FirstSeen int64  `json:"firstSeen"`
	LastSeen  int64  `json:"lastSeen" gorm:"index"`
}

func (BotCustomer) TableName() string { return "bot_customers" }

// BotAdminMeta is what the owner decided about one bot administrator as a
// seller: suspended, approvals only by the owner, the lowest price per GB
// they may sell at and the rate per GB they owe the owner.
type BotAdminMeta struct {
	TgId         int64 `json:"tgId" gorm:"primaryKey;autoIncrement:false"`
	Suspended    bool  `json:"suspended" gorm:"default:false;not null"`
	RequireOwner bool  `json:"requireOwner" gorm:"default:false;not null"`
	MinPerGB     int64 `json:"minPerGB" gorm:"default:0;not null"`
	RatePerGB    int64 `json:"ratePerGB" gorm:"default:0;not null"`
	// ReceiptTo is who gets the seller's orders and receipts to approve:
	// 0 the seller and the owner, -1 the seller alone, else that
	// administrator's Telegram ID. RequireOwner takes precedence.
	ReceiptTo int64 `json:"receiptTo" gorm:"default:0;not null"`
}

func (BotAdminMeta) TableName() string { return "bot_admin_meta" }

// BotSettlement is a settling of accounts between the owner and a seller:
// what was sold from From to To and the amount that was settled.
type BotSettlement struct {
	Id        uint  `json:"id" gorm:"primaryKey;autoIncrement"`
	AdminId   int64 `json:"adminId" gorm:"index;not null"`
	Orders    int64 `json:"orders"`
	Revenue   int64 `json:"revenue"`
	GB        int64 `json:"gb"`
	Amount    int64 `json:"amount"`
	From      int64 `json:"from"`
	To        int64 `json:"to"`
	CreatedAt int64 `json:"createdAt"`
}

func (BotSettlement) TableName() string { return "bot_settlements" }

// BotPref is a small bot preference of the owner, such as an alert switch.
type BotPref struct {
	Key   string `json:"key" gorm:"primaryKey"`
	Value string `json:"value"`
}

func (BotPref) TableName() string { return "bot_prefs" }

// BotRenewNote is what the bot already reminded a customer about for one
// client in its current term: a renewal (new expiry or volume) starts over.
type BotRenewNote struct {
	ClientId uint  `json:"clientId" gorm:"primaryKey;autoIncrement:false"`
	Expiry   int64 `json:"expiry"`
	Volume   int64 `json:"volume"`
	Soon     bool  `json:"soon"`
	Low      bool  `json:"low"`
	Ended    bool  `json:"ended"`
	At       int64 `json:"at"`
}

func (BotRenewNote) TableName() string { return "bot_renew_notes" }

// BotClientAlert is where the client alerts stand for one client: the term
// they measure the time against (Start, from when its Expiry was set) and
// the alerts already sent in it (Sent, comma separated), so a restart does
// not send them again.
type BotClientAlert struct {
	ClientId uint   `json:"clientId" gorm:"primaryKey;autoIncrement:false"`
	Expiry   int64  `json:"expiry"`
	Start    int64  `json:"start"`
	Sent     string `json:"sent"`
}

func (BotClientAlert) TableName() string { return "bot_client_alerts" }

type Tokens struct {
	Id     uint   `json:"id" form:"id" gorm:"primaryKey;autoIncrement"`
	Desc   string `json:"desc" form:"desc"`
	Token  string `json:"token" form:"token"`
	Expiry int64  `json:"expiry" form:"expiry"`
	UserId uint   `json:"userId" form:"userId"`
	User   *User  `json:"user" gorm:"foreignKey:UserId;references:Id"`
}

// Node is a remote DrNetwork panel managed over its token-authenticated API.
type Node struct {
	Id        uint            `json:"id" form:"id" gorm:"primaryKey;autoIncrement"`
	Enable    bool            `json:"enable" form:"enable" gorm:"default:true;not null"`
	Name      string          `json:"name" form:"name" gorm:"uniqueIndex"`
	BaseUrl   string          `json:"baseUrl" form:"baseUrl"`
	WebPath   string          `json:"webPath" form:"webPath"`
	Token     string          `json:"token,omitempty" form:"token"`
	Insecure  bool            `json:"insecure" form:"insecure" gorm:"default:false;not null"`
	CertPin   string          `json:"certPin" form:"certPin"`
	Desc      string          `json:"desc" form:"desc"`
	LastSeen  int64           `json:"lastSeen" form:"lastSeen" gorm:"default:0;not null"`
	Dirty     bool            `json:"dirty" gorm:"default:false;not null"`
	LastSync  int64           `json:"lastSync" gorm:"default:0;not null"`
	Baselines json.RawMessage `json:"-"`

	// Labels for the Nodes page only: they sort and filter it and change
	// nothing on the node.
	Tags      []string `json:"tags" gorm:"serializer:json"`
	Country   string   `json:"country"`
	SortOrder int      `json:"sortOrder" gorm:"default:0;not null"`
	// Alerts holds the node's Telegram alert thresholds.
	Alerts NodeAlerts `json:"alerts" gorm:"serializer:json"`
	// Cap is the node's monthly traffic allowance.
	Cap NodeCap `json:"cap" gorm:"serializer:json"`
	// HideDown leaves the node's links out of subscriptions while it is down.
	HideDown bool `json:"hideDown" gorm:"default:false;not null"`
	// Access limits which clients the node serves; empty serves them all.
	Access NodeAccess `json:"access" gorm:"serializer:json"`

	// Bookkeeping only the master writes; a save from the panel leaves it be.
	NetBase    json.RawMessage `json:"-"`
	CapState   json.RawMessage `json:"-"`
	SyncReport json.RawMessage `json:"-"`
}
