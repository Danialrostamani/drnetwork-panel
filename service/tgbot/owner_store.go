package tgbot

import (
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// The owner's records: preferences, what the owner decided about each seller
// and each customer, and the money figures the owner screens show.

func ownerDB() *gorm.DB { return database.GetDB() }

// ---- preferences ----

// Owner alert switches and their defaults.
const (
	prefStale     = "al.stale"  // pending orders waiting too long
	prefStaleHrs  = "al.staleh" // how long is too long, in hours
	prefQuotaFull = "al.q100"   // a seller's volume limit is used up
	prefAdminActs = "al.acts"   // a seller deletes clients or gives volume
	prefApprovals = "al.appr"   // a seller approves an order
	prefNewCust   = "al.newcu"  // a new customer starts the bot
)

var prefDefaults = map[string]string{
	prefStale: "1", prefStaleHrs: "3", prefQuotaFull: "1", prefAdminActs: "1", prefApprovals: "1", prefNewCust: "0",
	prefRenew: "1", prefRenewDays: "3",
}

func getPref(key string) string {
	db := ownerDB()
	if db != nil {
		var p model.BotPref
		if db.Where("`key` = ?", key).Limit(1).Find(&p).Error == nil && p.Key != "" {
			return p.Value
		}
	}
	return prefDefaults[key]
}

func setPref(key, value string) error {
	return ownerDB().Clauses(clause.OnConflict{UpdateAll: true}).Create(&model.BotPref{Key: key, Value: value}).Error
}

func prefOn(key string) bool { return getPref(key) == "1" }

func prefInt(key string, fallback int) int {
	n, err := strconv.Atoi(getPref(key))
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

// ---- sellers ----

func adminMeta(id int64) model.BotAdminMeta {
	m := model.BotAdminMeta{TgId: id}
	db := ownerDB()
	if db == nil || id == 0 {
		return m
	}
	_ = db.Where("tg_id = ?", id).Limit(1).Find(&m).Error
	m.TgId = id
	return m
}

func saveAdminMeta(m model.BotAdminMeta) error {
	return ownerDB().Save(&m).Error
}

func patchAdminMeta(id int64, fn func(m *model.BotAdminMeta)) (model.BotAdminMeta, error) {
	m := adminMeta(id)
	fn(&m)
	return m, saveAdminMeta(m)
}

// ---- customers ----

// customerSeen throttles how often a customer's visit is written, and how
// often the customer list is rebuilt from the shop's records.
type customerSeen struct {
	at     sync.Map
	syncMu sync.Mutex
	synced time.Time
}

const customerSyncEvery = 30 * time.Second

const customerTouchEvery = 5 * time.Minute

func customerRow(tgID int64) (model.BotCustomer, bool) {
	var c model.BotCustomer
	db := ownerDB()
	if db == nil {
		return c, false
	}
	if db.Where("tg_id = ?", tgID).Limit(1).Find(&c).Error != nil || c.TgId == 0 {
		return model.BotCustomer{TgId: tgID}, false
	}
	return c, true
}

func customerBlocked(tgID int64) bool {
	c, ok := customerRow(tgID)
	return ok && c.Blocked
}

func displayName(f *fromUser) string {
	return strings.TrimSpace(f.FirstName + " " + f.LastName)
}

// touchCustomer records that a customer used the bot. It reports whether
// they are new.
func (b *bot) touchCustomer(f *fromUser) (fresh bool) {
	if f == nil || f.ID <= 0 || ownerDB() == nil {
		return false
	}
	now := time.Now()
	if b.seen != nil {
		if last, ok := b.seen.at.Load(f.ID); ok && now.Sub(last.(time.Time)) < customerTouchEvery {
			return false
		}
		b.seen.at.Store(f.ID, now)
	}
	c, ok := customerRow(f.ID)
	if !ok {
		c = model.BotCustomer{TgId: f.ID, FirstSeen: now.Unix()}
		fresh = true
	}
	if n := displayName(f); n != "" {
		c.Name = n
	}
	if f.Username != "" {
		c.Username = f.Username
	}
	c.LastSeen = now.Unix()
	if c.FirstSeen == 0 {
		c.FirstSeen = now.Unix()
	}
	if err := ownerDB().Save(&c).Error; err != nil {
		return false
	}
	return fresh
}

func saveCustomer(c model.BotCustomer) error { return ownerDB().Save(&c).Error }

// syncCustomers makes sure everybody the shop knows of has a customer row:
// people who ordered, have a wallet, an introducer, or a bound client.
func (b *bot) syncCustomers() {
	db := ownerDB()
	if db == nil {
		return
	}
	if b.seen != nil {
		b.seen.syncMu.Lock()
		defer b.seen.syncMu.Unlock()
		if time.Since(b.seen.synced) < customerSyncEvery {
			return
		}
		defer func() { b.seen.synced = time.Now() }()
	}
	ids := map[int64]int64{}
	add := func(id, at int64) {
		if id <= 0 {
			return
		}
		if cur, ok := ids[id]; !ok || at > cur {
			ids[id] = at
		}
	}
	var orders []model.BotOrder
	db.Select("tg_id", "created_at").Find(&orders)
	for _, o := range orders {
		add(o.TgId, o.CreatedAt)
	}
	var wallets []model.BotWallet
	db.Find(&wallets)
	for _, w := range wallets {
		add(w.TgId, w.UpdatedAt)
	}
	var refs []model.BotRef
	db.Find(&refs)
	for _, r := range refs {
		add(r.TgId, 0)
	}
	for _, c := range allClients() {
		add(c.TgId, 0)
	}
	a := b.access()
	for id, at := range ids {
		if a.isMember(id) {
			continue
		}
		_ = db.Clauses(clause.OnConflict{DoNothing: true}).
			Create(&model.BotCustomer{TgId: id, FirstSeen: at, LastSeen: at}).Error
	}
}

// customerList is every customer, the most recently seen first, without the
// bot's administrators.
func (b *bot) customerList() []model.BotCustomer {
	b.syncCustomers()
	var rows []model.BotCustomer
	ownerDB().Order("last_seen desc, tg_id desc").Find(&rows)
	a := b.access()
	out := rows[:0]
	for _, c := range rows {
		if !a.isMember(c.TgId) {
			out = append(out, c)
		}
	}
	return out
}

// findCustomers matches a Telegram ID, a @username, a name or the name of a
// client bound to the customer.
func (b *bot) findCustomers(q string) []model.BotCustomer {
	q = strings.TrimSpace(latinDigits(q))
	if q == "" {
		return nil
	}
	all := b.customerList()
	if id, err := strconv.ParseInt(q, 10, 64); err == nil {
		for _, c := range all {
			if c.TgId == id {
				return []model.BotCustomer{c}
			}
		}
	}
	needle := strings.ToLower(strings.TrimPrefix(q, "@"))
	byClient := map[int64]bool{}
	for _, cl := range allClients() {
		if cl.TgId != 0 && strings.Contains(strings.ToLower(cl.Name), needle) {
			byClient[cl.TgId] = true
		}
	}
	var out []model.BotCustomer
	for _, c := range all {
		if strings.Contains(strings.ToLower(c.Username), needle) || strings.Contains(strings.ToLower(c.Name), needle) || byClient[c.TgId] {
			out = append(out, c)
		}
	}
	return out
}

// customerHasActive tells a customer with an enabled, unexpired client.
func customerHasActive(tgID int64, now int64) bool {
	for _, c := range boundClients(tgID) {
		if c.Enable && (c.Expiry == 0 || c.Expiry > now) {
			return true
		}
	}
	return false
}

// ---- money ----

// salesSum is what approved buys and renews brought in.
type salesSum struct {
	Orders  int64
	Revenue int64
	GB      int64
	Wallet  int64 // of the revenue, paid from wallets
}

// sumSales adds up the approved buys and renews of one seller (0: all
// sellers) approved at or after since (0: ever) and before until (0: now).
func sumSales(adminID int64, since, until int64) salesSum {
	q := ownerDB().Model(&model.BotOrder{}).
		Where("status = ? AND topic IN ?", orderApproved, []string{"buy", "renew"})
	if adminID != 0 {
		q = q.Where("admin_id = ?", adminID)
	}
	if since > 0 {
		q = q.Where("updated_at >= ?", since)
	}
	if until > 0 {
		q = q.Where("updated_at < ?", until)
	}
	var rows []model.BotOrder
	q.Find(&rows)
	return sumOf(rows)
}

// sumTopUps is what approved wallet top-ups brought in.
func sumTopUps(adminID int64, since int64) (n, amount int64) {
	q := ownerDB().Model(&model.BotOrder{}).Where("status = ? AND topic = ?", orderApproved, "wallet")
	if adminID != 0 {
		q = q.Where("admin_id = ?", adminID)
	}
	if since > 0 {
		q = q.Where("updated_at >= ?", since)
	}
	var rows []model.BotOrder
	q.Find(&rows)
	for _, o := range rows {
		n++
		amount += o.Amount
	}
	return n, amount
}

func walletsTotal() (n, total int64) {
	var rows []model.BotWallet
	ownerDB().Where("balance > 0").Find(&rows)
	for _, w := range rows {
		n++
		total += w.Balance
	}
	return n, total
}

func lastSettlement(adminID int64) (model.BotSettlement, bool) {
	var s model.BotSettlement
	if ownerDB().Where("admin_id = ?", adminID).Order("id desc").Limit(1).Find(&s).Error != nil || s.Id == 0 {
		return s, false
	}
	return s, true
}

func settlements(adminID int64, limit int) []model.BotSettlement {
	var out []model.BotSettlement
	ownerDB().Where("admin_id = ?", adminID).Order("id desc").Limit(limit).Find(&out)
	return out
}

// unsettledOrders are a seller's approved sales no settlement covered yet.
func unsettledOrders(tx *gorm.DB, adminID int64) []model.BotOrder {
	var rows []model.BotOrder
	tx.Where("admin_id = ? AND status = ? AND topic IN ? AND settlement_id = 0", adminID, orderApproved, []string{"buy", "renew"}).
		Order("id").Find(&rows)
	return rows
}

func sumOf(rows []model.BotOrder) salesSum {
	var s salesSum
	for _, o := range rows {
		s.Orders++
		s.Revenue += o.Amount
		s.GB += int64(o.PlanGB)
		if o.WalletPaid {
			s.Wallet += o.Amount
		}
	}
	return s
}

// unsettled is what a seller sold since the last settlement, and what they
// owe the owner for it at their rate per GB.
func unsettled(adminID int64) (from int64, s salesSum, owed int64) {
	if last, ok := lastSettlement(adminID); ok {
		from = last.To
	}
	s = sumOf(unsettledOrders(ownerDB(), adminID))
	owed = s.GB * adminMeta(adminID).RatePerGB
	return from, s, owed
}

// settle records a settlement of every sale not settled yet.
func settle(adminID int64) (model.BotSettlement, error) {
	var rec model.BotSettlement
	from, _, _ := unsettled(adminID)
	rate := adminMeta(adminID).RatePerGB
	err := ownerDB().Transaction(func(tx *gorm.DB) error {
		rows := unsettledOrders(tx, adminID)
		s := sumOf(rows)
		now := time.Now().Unix()
		rec = model.BotSettlement{
			AdminId: adminID, Orders: s.Orders, Revenue: s.Revenue, GB: s.GB, Amount: s.GB * rate,
			From: from, To: now, CreatedAt: now,
		}
		if err := tx.Create(&rec).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		ids := make([]uint, 0, len(rows))
		for _, o := range rows {
			ids = append(ids, o.Id)
		}
		return tx.Model(&model.BotOrder{}).Where("id IN ?", ids).Update("settlement_id", rec.Id).Error
	})
	return rec, err
}

func dayStart(loc *time.Location, now time.Time) int64 {
	if loc != nil {
		now = now.In(loc)
	}
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Unix()
}
