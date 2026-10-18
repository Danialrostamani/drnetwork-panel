package tgbot

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Customer wallets.
//
// A customer tops up by paying a wallet amount and sending the receipt; when
// staff approve the order the amount is added to the balance. A plan whose
// price is a number can then be paid from the balance: the price is taken at
// once and the order is approved without waiting for staff, since the money
// was approved already. If the sale cannot be delivered the price goes back.

// defaultWallets is offered when the shop lists no wallet amounts.
var defaultWallets = []shopWallet{
	{ID: "50", Label: "50,000", Amount: "50000"},
	{ID: "100", Label: "100,000", Amount: "100000"},
	{ID: "200", Label: "200,000", Amount: "200000"},
	{ID: "500", Label: "500,000", Amount: "500000"},
}

func (s shopConfig) walletList() []shopWallet {
	if len(s.Wallets) > 0 {
		return s.Wallets
	}
	return defaultWallets
}

var moneyDigits = strings.NewReplacer(
	"۰", "0", "۱", "1", "۲", "2", "۳", "3", "۴", "4", "۵", "5", "۶", "6", "۷", "7", "۸", "8", "۹", "9",
	"٠", "0", "١", "1", "٢", "2", "٣", "3", "٤", "4", "٥", "5", "٦", "6", "٧", "7", "٨", "8", "٩", "9",
)

var moneyNumber = regexp.MustCompile(`([0-9][0-9,٬،]*)(\.[0-9]+)?\s*(هزار|میلیون|thousand|million|k\b|K\b)?`)

// parseMoney reads an amount such as "150,000 تومان", "۱۵۰ هزار" or "1.5
// million" as a whole number of currency units. ok is false when the text
// has no positive number.
func parseMoney(text string) (n int64, ok bool) {
	m := moneyNumber.FindStringSubmatch(moneyDigits.Replace(text))
	if m == nil {
		return 0, false
	}
	whole := strings.NewReplacer(",", "", "٬", "", "،", "").Replace(m[1])
	v, err := strconv.ParseFloat(whole+m[2], 64)
	if err != nil {
		return 0, false
	}
	switch strings.ToLower(m[3]) {
	case "هزار", "thousand", "k":
		v *= 1e3
	case "میلیون", "million":
		v *= 1e6
	}
	if v < 1 || v > 1e15 {
		return 0, false
	}
	return int64(v + 0.5), true
}

// formatMoney writes n with thousands separators: 1500000 → 1,500,000.
func formatMoney(n int64) string {
	neg := n < 0
	if neg {
		n = -n
	}
	s := strconv.FormatInt(n, 10)
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

func walletBalance(tgID int64) int64 {
	var w model.BotWallet
	if database.GetDB().Where("tg_id = ?", tgID).Limit(1).Find(&w).Error != nil {
		return 0
	}
	return w.Balance
}

// walletMemo says why a balance changes, for the wallet history.
type walletMemo struct {
	kind  string // topup, pay, refund or manual
	order uint
	actor int64
	note  string
}

const (
	walletTopUp  = "topup"
	walletPay    = "pay"
	walletRefund = "refund"
	walletManual = "manual"
)

// walletCredit adds amount, which may be negative to undo a credit, to a
// balance, and returns the new balance. The change goes in the history.
func walletCredit(tgID, amount int64, m walletMemo) (int64, error) {
	var balance int64
	err := database.GetDB().Transaction(func(tx *gorm.DB) error {
		now := time.Now().Unix()
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).
			Create(&model.BotWallet{TgId: tgID, UpdatedAt: now}).Error; err != nil {
			return err
		}
		var before model.BotWallet
		if err := tx.Where("tg_id = ?", tgID).First(&before).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.BotWallet{}).Where("tg_id = ?", tgID).
			Updates(map[string]any{"balance": gorm.Expr("MAX(balance + ?, 0)", amount), "updated_at": now}).Error; err != nil {
			return err
		}
		var after model.BotWallet
		if err := tx.Where("tg_id = ?", tgID).First(&after).Error; err != nil {
			return err
		}
		balance = after.Balance
		return logWalletTx(tx, tgID, after.Balance-before.Balance, after.Balance, m)
	})
	if err != nil {
		return 0, err
	}
	return balance, nil
}

// walletSpend takes amount from a balance when it is enough; ok is false and
// nothing changes when it is not.
func walletSpend(tgID, amount int64, m walletMemo) (ok bool, err error) {
	if amount <= 0 {
		return false, nil
	}
	err = database.GetDB().Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&model.BotWallet{}).
			Where("tg_id = ? AND balance >= ?", tgID, amount).
			Updates(map[string]any{"balance": gorm.Expr("balance - ?", amount), "updated_at": time.Now().Unix()})
		if res.Error != nil || res.RowsAffected != 1 {
			return res.Error
		}
		ok = true
		var after model.BotWallet
		if err := tx.Where("tg_id = ?", tgID).First(&after).Error; err != nil {
			return err
		}
		return logWalletTx(tx, tgID, -amount, after.Balance, m)
	})
	if err != nil {
		return false, err
	}
	return ok, nil
}

func logWalletTx(tx *gorm.DB, tgID, delta, balance int64, m walletMemo) error {
	if delta == 0 {
		return nil
	}
	return tx.Create(&model.BotWalletTx{
		TgId: tgID, Delta: delta, Balance: balance, Kind: m.kind, OrderId: m.order,
		Actor: m.actor, Note: m.note, CreatedAt: time.Now().Unix(),
	}).Error
}

func walletHistory(tgID int64, limit int) []model.BotWalletTx {
	var out []model.BotWalletTx
	database.GetDB().Where("tg_id = ?", tgID).Order("id desc").Limit(limit).Find(&out)
	return out
}
