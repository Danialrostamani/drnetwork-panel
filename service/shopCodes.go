package service

import (
	"errors"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"

	"gorm.io/gorm"
)

// Codes come in two kinds. A discount code takes a percentage off a
// purchase; it may be limited to some plans, to purchases or renewals, and to
// one use per customer. A gift code adds an amount to the wallet of whoever
// redeems it, once per customer.

var discountCodeRe = regexp.MustCompile(`^[A-Za-z0-9_-]{2,32}$`)

// maxGift caps what one gift code can add.
const maxGift = 1_000_000_000_000

func (s *ShopService) Discounts() ([]model.ShopDiscount, error) {
	var out []model.ShopDiscount
	err := database.GetDB().Order("id").Find(&out).Error
	return out, err
}

// SaveDiscount creates or updates a discount or gift code.
func (s *ShopService) SaveDiscount(d *model.ShopDiscount) error {
	d.Code = strings.ToUpper(strings.TrimSpace(d.Code))
	if !discountCodeRe.MatchString(d.Code) {
		return errors.New("code: 2-32 letters, digits, _ or -")
	}
	if d.MaxUses < 0 || d.Used < 0 || d.Expiry < 0 {
		return errors.New("uses and expiry cannot be negative")
	}
	switch d.Kind {
	case model.CodeGift:
		if d.Amount <= 0 || d.Amount > maxGift {
			return errors.New("a gift code needs an amount above 0")
		}
		// A gift is for the wallet, once per customer.
		d.Percent, d.PlanIds, d.Kinds, d.OncePerUser = 0, "", "", true
	case "", model.CodeDiscount:
		d.Kind = model.CodeDiscount
		if d.Percent < 1 || d.Percent > 100 {
			return errors.New("percent must be 1-100")
		}
		d.Amount = 0
		ids, err := normalizePlanIds(d.PlanIds)
		if err != nil {
			return err
		}
		d.PlanIds = ids
		d.Kinds = strings.ToLower(strings.TrimSpace(d.Kinds))
		if d.Kinds != "" && d.Kinds != model.OrderBuy && d.Kinds != model.OrderRenew {
			return errors.New("a discount is for buy, renew or both")
		}
	default:
		return errors.New("unknown code kind")
	}
	db := database.GetDB()
	var n int64
	db.Model(&model.ShopDiscount{}).Where("code = ? AND id <> ?", d.Code, d.Id).Count(&n)
	if n > 0 {
		return errors.New("the code exists")
	}
	if d.Id == 0 {
		return db.Create(d).Error
	}
	return db.Select("*").Save(d).Error
}

// normalizePlanIds checks a comma separated list of plan ids and writes it
// sorted, without repeats.
func normalizePlanIds(v string) (string, error) {
	var ids []uint
	for _, f := range strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == ' ' || r == '،' }) {
		n, err := strconv.ParseUint(f, 10, 32)
		if err != nil || n == 0 {
			return "", errors.New("plans: a list of plan numbers, like 1,3")
		}
		if !slices.Contains(ids, uint(n)) {
			ids = append(ids, uint(n))
		}
	}
	if len(ids) > 50 {
		return "", errors.New("plans: at most 50")
	}
	slices.Sort(ids)
	var out []string
	db := database.GetDB()
	for _, id := range ids {
		var n int64
		db.Model(&model.ShopPlan{}).Where("id = ?", id).Count(&n)
		if n == 0 {
			return "", errors.New("plans: there is no plan #" + strconv.FormatUint(uint64(id), 10))
		}
		out = append(out, strconv.FormatUint(uint64(id), 10))
	}
	return strings.Join(out, ","), nil
}

// codeForPlan tells whether a discount may be used on this plan.
func codeForPlan(d *model.ShopDiscount, planID uint) bool {
	if strings.TrimSpace(d.PlanIds) == "" {
		return true
	}
	for _, f := range strings.Split(d.PlanIds, ",") {
		if n, err := strconv.ParseUint(strings.TrimSpace(f), 10, 32); err == nil && uint(n) == planID {
			return true
		}
	}
	return false
}

// ParseDiscountLine reads a code the bot was sent:
//
//	CODE percent [max uses] [days valid] [once]
//	gift CODE amount [max uses] [days valid]
func ParseDiscountLine(line string) (*model.ShopDiscount, error) {
	f := strings.Fields(line)
	bad := errors.New("format: CODE percent [max uses] [days] [once], or gift CODE amount [max uses] [days]")
	if len(f) == 0 {
		return nil, bad
	}
	d := &model.ShopDiscount{Enable: true, Kind: model.CodeDiscount}
	if k := strings.ToLower(f[0]); k == "gift" || k == "هدیه" {
		d.Kind = model.CodeGift
		f = f[1:]
	} else if n := len(f); n > 2 {
		if w := strings.ToLower(f[n-1]); w == "once" || w == "یکبار" || w == "یکبار‌مصرف" {
			d.OncePerUser = true
			f = f[:n-1]
		}
	}
	if len(f) < 2 || len(f) > 4 {
		return nil, bad
	}
	d.Code = f[0]
	var err error
	if d.Kind == model.CodeGift {
		if d.Amount, err = strconv.ParseInt(f[1], 10, 64); err != nil || d.Amount <= 0 {
			return nil, bad
		}
	} else if d.Percent, err = strconv.Atoi(f[1]); err != nil {
		return nil, bad
	}
	if len(f) > 2 {
		if d.MaxUses, err = strconv.Atoi(f[2]); err != nil || d.MaxUses < 0 {
			return nil, bad
		}
	}
	if len(f) > 3 {
		days, err := strconv.Atoi(f[3])
		if err != nil || days < 0 || days > 3650 {
			return nil, bad
		}
		if days > 0 {
			d.Expiry = time.Now().Add(time.Duration(days) * 24 * time.Hour).Unix()
		}
	}
	return d, nil
}

func (s *ShopService) DeleteDiscount(id uint) error {
	return database.GetDB().Delete(&model.ShopDiscount{}, id).Error
}

var (
	// ErrBadCode is returned for an unknown, used up or expired code.
	ErrBadCode = errors.New("invalid discount code")
	// ErrCodeNotHere is returned for a discount of other plans or orders.
	ErrCodeNotHere = errors.New("this code is not for this plan")
	// ErrCodeUsed is returned to a customer who had their one use.
	ErrCodeUsed = errors.New("you have already used this code")
	// ErrGiftCode is returned for a gift code given as a discount.
	ErrGiftCode = errors.New("this is a gift code for the wallet")
	// ErrNotGift is returned for a discount code given as a gift.
	ErrNotGift = errors.New("this is a discount code, not a gift code")
)

// findCode finds a code that is on, not expired and not used up.
func findCode(code string) (*model.ShopDiscount, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return nil, ErrBadCode
	}
	var d model.ShopDiscount
	if err := database.GetDB().Where("code = ?", code).First(&d).Error; err != nil {
		return nil, ErrBadCode
	}
	if !d.Enable || (d.MaxUses > 0 && d.Used >= d.MaxUses) || (d.Expiry > 0 && d.Expiry < time.Now().Unix()) {
		return nil, ErrBadCode
	}
	return &d, nil
}

// Discount finds a usable discount code, whatever it is for.
func (s *ShopService) Discount(code string) (*model.ShopDiscount, error) {
	d, err := findCode(code)
	if err != nil {
		return nil, err
	}
	if d.IsGift() {
		return nil, ErrGiftCode
	}
	return d, nil
}

// DiscountFor finds a discount code this customer may use on this plan and
// kind of order ("buy" or "renew").
func (s *ShopService) DiscountFor(code string, tgID int64, planID uint, kind string) (*model.ShopDiscount, error) {
	d, err := s.Discount(code)
	if err != nil {
		return nil, err
	}
	if !codeForPlan(d, planID) || (d.Kinds != "" && d.Kinds != kind) {
		return nil, ErrCodeNotHere
	}
	if d.OncePerUser && codeUsedBy(database.GetDB(), d, tgID) {
		return nil, ErrCodeUsed
	}
	return d, nil
}

// codeUsedBy tells whether a customer used a code, or has an order with it
// that is still open.
func codeUsedBy(db *gorm.DB, d *model.ShopDiscount, tgID int64) bool {
	var n int64
	db.Model(&model.ShopCodeUse{}).Where("code_id = ? AND tg_id = ?", d.Id, tgID).Count(&n)
	if n > 0 {
		return true
	}
	if d.IsGift() {
		return false
	}
	db.Model(&model.ShopOrder{}).Where("tg_id = ? AND code = ? AND status IN ?", tgID, d.Code,
		[]string{model.OrderPending, model.OrderApproved}).Count(&n)
	return n > 0
}

// giftMu makes checking and redeeming a gift code one step.
var giftMu sync.Mutex

// RedeemGift adds a gift code's amount to a customer's wallet, once per
// customer. It returns the amount and the new balance.
func (s *ShopService) RedeemGift(tgID int64, code string) (amount, balance int64, err error) {
	giftMu.Lock()
	defer giftMu.Unlock()
	d, err := findCode(code)
	if err != nil {
		return 0, 0, err
	}
	if !d.IsGift() {
		return 0, 0, ErrNotGift
	}
	err = database.GetDB().Transaction(func(tx *gorm.DB) error {
		if codeUsedBy(tx, d, tgID) {
			return ErrCodeUsed
		}
		q := tx.Model(&model.ShopDiscount{}).Where("id = ? AND enable = ?", d.Id, true)
		if d.MaxUses > 0 {
			q = q.Where("used < max_uses")
		}
		r := q.Update("used", gorm.Expr("used + 1"))
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected == 0 {
			return ErrBadCode
		}
		if err := tx.Create(&model.ShopCodeUse{CodeId: d.Id, Code: d.Code, TgId: tgID, CreatedAt: time.Now().Unix()}).Error; err != nil {
			return err
		}
		var err error
		balance, err = s.addFunds(tx, tgID, d.Amount, "gift", 0, 0)
		return err
	})
	if err != nil {
		return 0, 0, err
	}
	return d.Amount, balance, nil
}

// useCode counts the use of an approved order's code.
func (s *ShopService) useCode(o *model.ShopOrder) {
	db := database.GetDB()
	var d model.ShopDiscount
	if db.Where("code = ?", o.Code).Limit(1).Find(&d).Error != nil || d.Id == 0 {
		return
	}
	db.Model(&model.ShopDiscount{}).Where("id = ?", d.Id).Update("used", gorm.Expr("used + 1"))
	db.Create(&model.ShopCodeUse{CodeId: d.Id, Code: d.Code, TgId: o.TgId, OrderId: o.Id, CreatedAt: time.Now().Unix()})
}
