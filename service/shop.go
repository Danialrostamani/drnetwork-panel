package service

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"

	"gorm.io/gorm"
)

// ShopService keeps the shop's plans, orders, wallets and discount codes. It
// never touches the core: the Telegram bot turns approved orders into clients
// through the normal client save path.
type ShopService struct{}

// ShopSettings is the shop configuration as stored in the settings table.
type ShopSettings struct {
	Enable bool `json:"enable"`
	// Card is the payment text a customer sees: card number, holder, bank.
	Card     string `json:"card"`
	Currency string `json:"currency"`
	// Trial is "GB days" of the free test account; empty is no trial.
	TrialGB    float64 `json:"trialGB"`
	TrialDays  int     `json:"trialDays"`
	RefPercent int     `json:"refPercent"`
	Support    string  `json:"support"`
	Prefix     string  `json:"prefix"`
	// Cards are the payment texts of Card, one per card: orders take turns.
	Cards []string `json:"cards"`
	// UniqueAmount is the most a card payment may add to its price to be
	// told apart from the other open orders; 0 adds nothing.
	UniqueAmount int `json:"uniqueAmount"`
	// SmsSecret opens the bank message hook; "" keeps it closed. SmsRial
	// reads the messages' amounts as rials against prices in tomans.
	SmsSecret string `json:"-"`
	SmsRial   bool   `json:"smsRial"`
	// AutoRenew lets customers have a service renewed from their wallet.
	AutoRenew bool `json:"autoRenew"`
}

var shopSettingKeys = []string{"shopEnable", "shopCard", "shopCurrency", "shopTrial", "shopRefPercent", "shopSupport", "shopPrefix",
	"shopUniqueAmount", "shopSmsSecret", "shopSmsRial", "shopAutoRenew"}

// IsShopSetting tells the settings that belong to the shop.
func IsShopSetting(key string) bool {
	for _, k := range shopSettingKeys {
		if k == key {
			return true
		}
	}
	return false
}

// ParseTrial reads the "GB days" trial setting.
func ParseTrial(v string) (float64, int, error) {
	f := strings.Fields(strings.TrimSpace(v))
	if len(f) == 0 {
		return 0, 0, nil
	}
	if len(f) != 2 {
		return 0, 0, errors.New("trial must be \"GB days\"")
	}
	gb, err1 := strconv.ParseFloat(f[0], 64)
	days, err2 := strconv.Atoi(f[1])
	if err1 != nil || err2 != nil || gb < 0 || days < 0 || gb > 1000 || days > 365 {
		return 0, 0, errors.New("trial must be \"GB days\"")
	}
	return gb, days, nil
}

var (
	shopPrefixRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,15}$`)
	smsSecretRe  = regexp.MustCompile(`^[A-Za-z0-9_-]{16,64}$`)
)

// CheckShopSetting validates one shop setting.
func CheckShopSetting(key, value string) error {
	switch key {
	case "shopEnable":
		_, err := strconv.ParseBool(value)
		return err
	case "shopTrial":
		_, _, err := ParseTrial(value)
		return err
	case "shopRefPercent":
		n, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || n < 0 || n > 100 {
			return errors.New("percent must be 0-100")
		}
	case "shopPrefix":
		if !shopPrefixRe.MatchString(value) {
			return errors.New("prefix: a letter, then letters, digits, _ or -")
		}
	case "shopCard":
		if len(value) > 3000 {
			return errors.New("too long")
		}
	case "shopSupport":
		if len(value) > 1000 {
			return errors.New("too long")
		}
	case "shopUniqueAmount":
		n, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || n < 0 || n > 9999 {
			return errors.New("the unique amount is 0 (off) to 9999")
		}
	case "shopSmsSecret":
		if value != "" && !smsSecretRe.MatchString(value) {
			return errors.New("the SMS key is 16-64 letters, digits, _ or -")
		}
	case "shopSmsRial", "shopAutoRenew":
		_, err := strconv.ParseBool(value)
		return err
	case "shopCurrency":
		if len(value) > 30 {
			return errors.New("too long")
		}
	default:
		return fmt.Errorf("unknown shop setting %s", key)
	}
	return nil
}

func (s *ShopService) Settings() ShopSettings {
	ss := &SettingService{}
	get := func(k string) string { v, _ := ss.getString(k); return v }
	out := ShopSettings{
		Card:     get("shopCard"),
		Currency: strings.TrimSpace(get("shopCurrency")),
		Support:  get("shopSupport"),
		Prefix:   strings.TrimSpace(get("shopPrefix")),
	}
	out.Enable, _ = strconv.ParseBool(get("shopEnable"))
	out.TrialGB, out.TrialDays, _ = ParseTrial(get("shopTrial"))
	out.RefPercent, _ = strconv.Atoi(strings.TrimSpace(get("shopRefPercent")))
	out.Cards = ShopCards(out.Card)
	out.UniqueAmount, _ = strconv.Atoi(strings.TrimSpace(get("shopUniqueAmount")))
	out.UniqueAmount = max(min(out.UniqueAmount, 9999), 0)
	out.SmsSecret = strings.TrimSpace(get("shopSmsSecret"))
	if !smsSecretRe.MatchString(out.SmsSecret) {
		out.SmsSecret = ""
	}
	out.SmsRial, _ = strconv.ParseBool(get("shopSmsRial"))
	out.AutoRenew, _ = strconv.ParseBool(get("shopAutoRenew"))
	if !shopPrefixRe.MatchString(out.Prefix) {
		out.Prefix = "u"
	}
	return out
}

// RawSettings returns the stored shop settings as text, for editing.
func (s *ShopService) RawSettings() map[string]string {
	ss := &SettingService{}
	out := map[string]string{}
	for _, k := range shopSettingKeys {
		out[k], _ = ss.getString(k)
	}
	return out
}

// SetSetting saves one shop setting after checking it.
func (s *ShopService) SetSetting(key, value string) error {
	if !IsShopSetting(key) {
		return fmt.Errorf("unknown shop setting %s", key)
	}
	value = strings.TrimSpace(value)
	if err := CheckShopSetting(key, value); err != nil {
		return err
	}
	return (&SettingService{}).saveSetting(key, value)
}

// ---- plans ----

func (s *ShopService) Plans(onlyEnabled bool) ([]model.ShopPlan, error) {
	var out []model.ShopPlan
	q := database.GetDB().Order("sort, id")
	if onlyEnabled {
		q = q.Where("enable = ?", true)
	}
	err := q.Find(&out).Error
	return out, err
}

func (s *ShopService) Plan(id uint) (*model.ShopPlan, error) {
	var p model.ShopPlan
	if err := database.GetDB().First(&p, id).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

func checkPlan(p *model.ShopPlan) error {
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" || len([]rune(p.Name)) > 40 {
		return errors.New("plan name: 1-40 characters")
	}
	if p.Volume < 0 || p.Days < 0 || p.Days > 3650 || p.Price < 0 || p.LimitIp < 0 || p.LimitIp > 1000 {
		return errors.New("bad plan numbers")
	}
	return nil
}

// SavePlan creates (Id 0) or updates a plan.
func (s *ShopService) SavePlan(p *model.ShopPlan) error {
	if err := checkPlan(p); err != nil {
		return err
	}
	db := database.GetDB()
	if p.Id == 0 {
		return db.Create(p).Error
	}
	return db.Select("*").Save(p).Error
}

func (s *ShopService) DeletePlan(id uint) error {
	return database.GetDB().Delete(&model.ShopPlan{}, id).Error
}

// ParsePlanLine reads "name | GB | days | price [| IP limit]".
func ParsePlanLine(line string) (*model.ShopPlan, error) {
	f := strings.Split(line, "|")
	if len(f) < 4 || len(f) > 5 {
		return nil, errors.New("format: name | GB | days | price [| IP]")
	}
	for i := range f {
		f[i] = strings.TrimSpace(f[i])
	}
	gb, err1 := strconv.ParseFloat(f[1], 64)
	days, err2 := strconv.Atoi(f[2])
	price, err3 := strconv.ParseInt(strings.ReplaceAll(f[3], ",", ""), 10, 64)
	ip := 0
	var err4 error
	if len(f) == 5 && f[4] != "" {
		ip, err4 = strconv.Atoi(f[4])
	}
	if err1 != nil || err2 != nil || err3 != nil || err4 != nil || gb < 0 || gb > 100000 {
		return nil, errors.New("format: name | GB | days | price [| IP]")
	}
	p := &model.ShopPlan{Name: f[0], Volume: int64(gb * (1 << 30)), Days: days, Price: price, LimitIp: ip, Enable: true}
	return p, checkPlan(p)
}

// ---- discount codes: see shopCodes.go ----

// Price works out an order's price: the plan's, less the reseller's and the
// code's percentages.
func Price(base int64, resellerPercent, codePercent int) (discount, paid int64) {
	p := base
	if resellerPercent > 0 {
		p -= p * int64(min(resellerPercent, 100)) / 100
	}
	if codePercent > 0 {
		p -= p * int64(min(codePercent, 100)) / 100
	}
	return base - p, p
}

// ---- users ----

// Touch records a user who opened the shop; a new one may come with a
// referrer.
func (s *ShopService) Touch(tgID int64, name, username string, referrer int64) (*model.ShopUser, error) {
	db := database.GetDB()
	var u model.ShopUser
	err := db.First(&u, "tg_id = ?", tgID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		u = model.ShopUser{TgId: tgID, Name: name, Username: username, JoinedAt: time.Now().Unix()}
		if referrer != 0 && referrer != tgID {
			var n int64
			db.Model(&model.ShopUser{}).Where("tg_id = ?", referrer).Count(&n)
			if n > 0 {
				u.ReferredBy = referrer
			}
		}
		return &u, db.Create(&u).Error
	} else if err != nil {
		return nil, err
	}
	if u.Name != name || u.Username != username || u.Gone {
		u.Name, u.Username, u.Gone = name, username, false
		err = db.Model(&u).Select("name", "username", "gone").Updates(&u).Error
	}
	return &u, err
}

func (s *ShopService) User(tgID int64) *model.ShopUser {
	var u model.ShopUser
	if database.GetDB().First(&u, "tg_id = ?", tgID).Error != nil {
		return nil
	}
	return &u
}

// SetResellerPercent saves the discount of a reseller.
func (s *ShopService) SetResellerPercent(tgID int64, percent int) error {
	if percent < 0 || percent > 100 {
		return errors.New("percent must be 0-100")
	}
	db := database.GetDB()
	u := model.ShopUser{TgId: tgID, JoinedAt: time.Now().Unix()}
	if err := db.Where("tg_id = ?", tgID).FirstOrCreate(&u).Error; err != nil {
		return err
	}
	return db.Model(&model.ShopUser{}).Where("tg_id = ?", tgID).Update("reseller_percent", percent).Error
}

func (s *ShopService) SetBlocked(tgID int64, blocked bool) error {
	u := model.ShopUser{TgId: tgID, JoinedAt: time.Now().Unix()}
	if err := database.GetDB().Where("tg_id = ?", tgID).FirstOrCreate(&u).Error; err != nil {
		return err
	}
	return database.GetDB().Model(&model.ShopUser{}).Where("tg_id = ?", tgID).Update("blocked", blocked).Error
}

func (s *ShopService) MarkGone(tgID int64) {
	database.GetDB().Model(&model.ShopUser{}).Where("tg_id = ?", tgID).Update("gone", true)
}

// ClaimTrial marks the trial of a user as taken; false if it already was.
func (s *ShopService) ClaimTrial(tgID int64) (bool, error) {
	db := database.GetDB()
	u := model.ShopUser{TgId: tgID, JoinedAt: time.Now().Unix()}
	if err := db.Where("tg_id = ?", tgID).FirstOrCreate(&u).Error; err != nil {
		return false, err
	}
	r := db.Model(&model.ShopUser{}).Where("tg_id = ? AND trial_at = 0", tgID).Update("trial_at", time.Now().Unix())
	return r.RowsAffected == 1, r.Error
}

// ReleaseTrial gives the trial back when making the account failed.
func (s *ShopService) ReleaseTrial(tgID int64) {
	database.GetDB().Model(&model.ShopUser{}).Where("tg_id = ?", tgID).Update("trial_at", 0)
}

// BroadcastTargets lists the Telegram IDs a broadcast goes to: shop users and
// the clients bound to Telegram, but not the blocked or gone ones.
func (s *ShopService) BroadcastTargets() ([]int64, error) {
	db := database.GetDB()
	var ids []int64
	if err := db.Raw(`SELECT tg_id FROM shop_users WHERE blocked = 0 AND gone = 0
		UNION SELECT tg_id FROM clients WHERE tg_id <> 0 AND tg_id NOT IN (SELECT tg_id FROM shop_users WHERE blocked = 1 OR gone = 1)`).Scan(&ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}

// ---- wallets ----

// ErrNoFunds is returned when a wallet cannot pay.
var ErrNoFunds = errors.New("not enough wallet balance")

func (s *ShopService) Balance(tgID int64) int64 {
	var w model.ShopWallet
	if database.GetDB().First(&w, "tg_id = ?", tgID).Error != nil {
		return 0
	}
	return w.Balance
}

// AddFunds changes a wallet by amount (negative takes money out) and records
// why. Taking out more than there is fails with ErrNoFunds. Without tx the
// change and its ledger line are written in a transaction of their own, so the
// balance on every line is the one that change left.
func (s *ShopService) AddFunds(tx *gorm.DB, tgID, amount int64, reason string, orderID uint, by int64) (int64, error) {
	if amount == 0 {
		return s.Balance(tgID), nil
	}
	if tx != nil {
		return s.addFunds(tx, tgID, amount, reason, orderID, by)
	}
	var balance int64
	err := database.GetDB().Transaction(func(tx *gorm.DB) error {
		var err error
		balance, err = s.addFunds(tx, tgID, amount, reason, orderID, by)
		return err
	})
	if err != nil {
		return 0, err
	}
	return balance, nil
}

func (s *ShopService) addFunds(tx *gorm.DB, tgID, amount int64, reason string, orderID uint, by int64) (int64, error) {
	if err := tx.Exec(`INSERT INTO shop_wallets (tg_id, balance) VALUES (?, 0) ON CONFLICT (tg_id) DO NOTHING`, tgID).Error; err != nil {
		return 0, err
	}
	r := tx.Exec(`UPDATE shop_wallets SET balance = balance + ? WHERE tg_id = ? AND balance + ? >= 0`, amount, tgID, amount)
	if r.Error != nil {
		return 0, r.Error
	}
	if r.RowsAffected == 0 {
		return 0, ErrNoFunds
	}
	var w model.ShopWallet
	if err := tx.First(&w, "tg_id = ?", tgID).Error; err != nil {
		return 0, err
	}
	if err := tx.Create(&model.ShopWalletTx{TgId: tgID, Amount: amount, Balance: w.Balance, Reason: reason, OrderId: orderID, By: by, CreatedAt: time.Now().Unix()}).Error; err != nil {
		return 0, err
	}
	return w.Balance, nil
}

func (s *ShopService) WalletTxs(tgID int64, limit int) ([]model.ShopWalletTx, error) {
	var out []model.ShopWalletTx
	err := database.GetDB().Where("tg_id = ?", tgID).Order("id DESC").Limit(limit).Find(&out).Error
	return out, err
}

// ---- orders ----

// cardOrderMu makes choosing a card order's amount and saving the order one
// step.
var cardOrderMu sync.Mutex

func (s *ShopService) CreateOrder(o *model.ShopOrder) error {
	o.Id = 0
	o.Status = model.OrderPending
	o.CreatedAt = time.Now().Unix()
	o.Extra, o.Card = 0, ""
	if o.Method != model.PayCard {
		return database.GetDB().Create(o).Error
	}
	// A card payment gets its card, and an amount no other open order has,
	// in one step: two orders made at once must not take the same amount.
	cardOrderMu.Lock()
	defer cardOrderMu.Unlock()
	st := s.Settings()
	o.Card = s.pickCard(st.Cards)
	if st.UniqueAmount > 0 && o.Paid > 0 {
		o.Extra = s.uniqueExtra(o.Paid, st.UniqueAmount)
	}
	return database.GetDB().Create(o).Error
}

func (s *ShopService) Order(id uint) (*model.ShopOrder, error) {
	var o model.ShopOrder
	if err := database.GetDB().First(&o, id).Error; err != nil {
		return nil, err
	}
	return &o, nil
}

func (s *ShopService) SetReceipt(id uint, receipt string) error {
	return database.GetDB().Model(&model.ShopOrder{}).Where("id = ? AND status = ?", id, model.OrderPending).Update("receipt", receipt).Error
}

// ErrDecided is returned when an order was already approved or rejected.
var ErrDecided = errors.New("the order was already handled")

// Decide moves a pending order to approved, rejected or canceled. Only one
// administrator can decide: the second one gets ErrDecided.
func (s *ShopService) Decide(id uint, status string, by int64) (*model.ShopOrder, error) {
	r := database.GetDB().Model(&model.ShopOrder{}).Where("id = ? AND status = ?", id, model.OrderPending).
		Updates(map[string]any{"status": status, "decided_at": time.Now().Unix(), "decided_by": by})
	if r.Error != nil {
		return nil, r.Error
	}
	if r.RowsAffected == 0 {
		return nil, ErrDecided
	}
	return s.Order(id)
}

// Finish records what the approved order created and uses up its code.
func (s *ShopService) Finish(o *model.ShopOrder) error {
	db := database.GetDB()
	if err := db.Model(&model.ShopOrder{}).Where("id = ?", o.Id).Updates(map[string]any{"client_id": o.ClientId, "client_name": o.ClientName}).Error; err != nil {
		return err
	}
	if o.Code != "" {
		s.useCode(o)
	}
	return nil
}

// Reopen puts an approved order whose delivery failed back to pending.
func (s *ShopService) Reopen(id uint) {
	database.GetDB().Model(&model.ShopOrder{}).Where("id = ?", id).Updates(map[string]any{"status": model.OrderPending, "decided_at": 0, "decided_by": 0})
}

// Orders lists orders, newest first; status "" is any.
func (s *ShopService) Orders(status string, tgID int64, limit int) ([]model.ShopOrder, error) {
	var out []model.ShopOrder
	q := database.GetDB().Order("id DESC").Limit(limit)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if tgID != 0 {
		q = q.Where("tg_id = ?", tgID)
	}
	err := q.Find(&out).Error
	return out, err
}

// ReferralReward pays the referrer of a user for their first approved
// purchase, once. It returns the referrer and what they got.
func (s *ShopService) ReferralReward(o *model.ShopOrder) (int64, int64) {
	if o.Kind == model.OrderTopup || o.Paid <= 0 {
		return 0, 0
	}
	pct := s.Settings().RefPercent
	if pct <= 0 {
		return 0, 0
	}
	db := database.GetDB()
	var u model.ShopUser
	if db.First(&u, "tg_id = ?", o.TgId).Error != nil || u.ReferredBy == 0 || u.RefPaid {
		return 0, 0
	}
	if r := db.Model(&model.ShopUser{}).Where("tg_id = ? AND ref_paid = ?", o.TgId, false).Update("ref_paid", true); r.Error != nil || r.RowsAffected == 0 {
		return 0, 0
	}
	reward := o.Paid * int64(pct) / 100
	if reward <= 0 {
		return 0, 0
	}
	if _, err := s.AddFunds(nil, u.ReferredBy, reward, "referral", o.Id, 0); err != nil {
		return 0, 0
	}
	return u.ReferredBy, reward
}

func (s *ShopService) Referrals(tgID int64) int64 {
	var n int64
	database.GetDB().Model(&model.ShopUser{}).Where("referred_by = ?", tgID).Count(&n)
	return n
}

// ---- reports ----

// ShopDay is the sales of one day.
type ShopDay struct {
	Day     string `json:"day"`
	Revenue int64  `json:"revenue"`
	Orders  int64  `json:"orders"`
}

// ShopStats is the business summary of the panel.
type ShopStats struct {
	Currency      string            `json:"currency"`
	RevenueToday  int64             `json:"revenueToday"`
	Revenue7      int64             `json:"revenue7"`
	Revenue30     int64             `json:"revenue30"`
	RevenueAll    int64             `json:"revenueAll"`
	Orders30      int64             `json:"orders30"`
	Pending       int64             `json:"pending"`
	Users         int64             `json:"users"`
	NewUsers30    int64             `json:"newUsers30"`
	Trials        int64             `json:"trials"`
	WalletTotal   int64             `json:"walletTotal"`
	Clients       int64             `json:"clients"`
	ActiveClients int64             `json:"activeClients"`
	Online24h     int64             `json:"online24h"`
	Expiring3d    int64             `json:"expiring3d"`
	Expired       int64             `json:"expired"`
	Depleted      int64             `json:"depleted"`
	Renewals30    int64             `json:"renewals30"`
	Days          []ShopDay         `json:"days"`
	TopConsumers  []TopUse          `json:"topConsumers"`
	TopPlans      []TopPlan         `json:"topPlans"`
	RecentOrders  []model.ShopOrder `json:"recentOrders"`
}

type TopUse struct {
	Name  string `json:"name"`
	Usage int64  `json:"usage"`
}

type TopPlan struct {
	Name    string `json:"name"`
	Orders  int64  `json:"orders"`
	Revenue int64  `json:"revenue"`
}

// Stats sums up sales and clients. Day boundaries follow loc.
func (s *ShopService) Stats(loc *time.Location, days int) (*ShopStats, error) {
	if loc == nil {
		loc = time.Local
	}
	if days < 1 || days > 366 {
		days = 30
	}
	db := database.GetDB()
	now := time.Now().In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	out := &ShopStats{Currency: s.Settings().Currency}
	sum := func(from int64) int64 {
		var v int64
		db.Raw(`SELECT COALESCE(SUM(paid),0) FROM shop_orders WHERE status = ? AND kind <> ? AND decided_at >= ?`, model.OrderApproved, model.OrderTopup, from).Scan(&v)
		return v
	}
	// Revenue is money that came in for services: wallet top-ups are counted
	// when the wallet pays for something, card purchases when approved.
	out.RevenueToday = sum(today.Unix())
	out.Revenue7 = sum(today.AddDate(0, 0, -6).Unix())
	out.Revenue30 = sum(today.AddDate(0, 0, -29).Unix())
	out.RevenueAll = sum(0)
	from30 := today.AddDate(0, 0, -29).Unix()
	db.Model(&model.ShopOrder{}).Where("status = ? AND kind <> ? AND decided_at >= ?", model.OrderApproved, model.OrderTopup, from30).Count(&out.Orders30)
	db.Model(&model.ShopOrder{}).Where("status = ? AND kind = ? AND decided_at >= ?", model.OrderApproved, model.OrderRenew, from30).Count(&out.Renewals30)
	db.Model(&model.ShopOrder{}).Where("status = ? AND receipt <> ''", model.OrderPending).Count(&out.Pending)
	db.Model(&model.ShopUser{}).Count(&out.Users)
	db.Model(&model.ShopUser{}).Where("joined_at >= ?", from30).Count(&out.NewUsers30)
	db.Model(&model.ShopUser{}).Where("trial_at > 0").Count(&out.Trials)
	db.Raw(`SELECT COALESCE(SUM(balance),0) FROM shop_wallets`).Scan(&out.WalletTotal)
	unix := time.Now().Unix()
	db.Model(&model.Client{}).Count(&out.Clients)
	db.Model(&model.Client{}).Where("enable = ?", true).Count(&out.ActiveClients)
	db.Model(&model.Client{}).Where("online_at >= ?", unix-86400).Count(&out.Online24h)
	db.Model(&model.Client{}).Where("enable = ? AND expiry > ? AND expiry <= ?", true, unix, unix+3*86400).Count(&out.Expiring3d)
	db.Model(&model.Client{}).Where("expiry > 0 AND expiry <= ?", unix).Count(&out.Expired)
	db.Model(&model.Client{}).Where("volume > 0 AND up + down >= volume").Count(&out.Depleted)

	start := today.AddDate(0, 0, -(days - 1))
	var rows []struct {
		DecidedAt int64
		Paid      int64
	}
	db.Model(&model.ShopOrder{}).Select("decided_at, paid").Where("status = ? AND kind <> ? AND decided_at >= ?", model.OrderApproved, model.OrderTopup, start.Unix()).Scan(&rows)
	idx := map[string]int{}
	for d := 0; d < days; d++ {
		day := start.AddDate(0, 0, d).Format("2006-01-02")
		idx[day] = d
		out.Days = append(out.Days, ShopDay{Day: day})
	}
	for _, r := range rows {
		if i, ok := idx[time.Unix(r.DecidedAt, 0).In(loc).Format("2006-01-02")]; ok {
			out.Days[i].Revenue += r.Paid
			out.Days[i].Orders++
		}
	}
	db.Raw(`SELECT name, up + down AS usage FROM clients ORDER BY up + down DESC LIMIT 10`).Scan(&out.TopConsumers)
	db.Raw(`SELECT plan_name AS name, COUNT(*) AS orders, COALESCE(SUM(paid),0) AS revenue FROM shop_orders
		WHERE status = ? AND kind <> ? AND decided_at >= ? GROUP BY plan_name ORDER BY revenue DESC LIMIT 10`, model.OrderApproved, model.OrderTopup, from30).Scan(&out.TopPlans)
	db.Order("id DESC").Limit(20).Find(&out.RecentOrders)
	if out.Days == nil {
		out.Days = []ShopDay{}
	}
	return out, nil
}

// ShopDecider approves (true) or rejects a pending order through the running
// Telegram bot, which delivers it and tells the customer. The bot sets it.
var ShopDecider func(id uint, approve bool) (string, error)

// ErrBotDown is returned when an order needs the bot and it is not running.
var ErrBotDown = errors.New("the Telegram bot is not running")

// BotUsername is the Telegram bot's username once it is running, for links to
// it from the subscription page.
var BotUsername atomic.Value
