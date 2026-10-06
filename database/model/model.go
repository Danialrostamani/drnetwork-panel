package model

import "encoding/json"

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
}

type Client struct {
	Id     uint `json:"id" form:"id" gorm:"primaryKey;autoIncrement"`
	Enable bool `json:"enable" form:"enable"`
	// Name is the join key on every hot path: the stats job and every
	// subscription fetch resolve a client by it. Uniqueness stays in
	// ClientService.validateClientName -- a unique index would need existing
	// duplicates renamed, and a name is the subscription ID, so renaming one
	// breaks that user's link.
	Name     string          `json:"name" form:"name" gorm:"index"`
	Config   json.RawMessage `json:"config,omitempty" form:"config"`
	Inbounds json.RawMessage `json:"inbounds" form:"inbounds"`
	Links    json.RawMessage `json:"links,omitempty" form:"links"`
	Volume   int64           `json:"volume" form:"volume"`
	Expiry   int64           `json:"expiry" form:"expiry"`
	Down     int64           `json:"down" form:"down"`
	Up       int64           `json:"up" form:"up"`
	Desc     string          `json:"desc" form:"desc"`
	Group    string          `json:"group" form:"group"`
	Remark   string          `json:"remark" form:"remark"`

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
}

// TableName pins the name, for the same reason as BotQuota's.
func (BotQuotaClient) TableName() string { return "bot_quota_clients" }

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
}
