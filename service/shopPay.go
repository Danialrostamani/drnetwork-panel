package service

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/logger"
	"github.com/Danialrostamani/drnetwork-panel/util/common"

	"gorm.io/gorm"
)

// Card payments without an administrator: every order gets an amount no
// other open order has (the price plus a few units, which go to the wallet),
// a phone forwards the bank's deposit messages to the panel, and the order
// whose amount came in is approved. The shop's cards take turns, and one
// receipt can pay for only one order.

// openOrderWindow is how long a card order waits for its money.
const openOrderWindow = 24 * time.Hour

// ShopCards splits the card setting into its cards: texts separated by a
// line of "---".
func ShopCards(setting string) []string {
	var cards []string
	var cur []string
	flush := func() {
		if c := strings.TrimSpace(strings.Join(cur, "\n")); c != "" {
			cards = append(cards, c)
		}
		cur = nil
	}
	for _, line := range strings.Split(strings.ReplaceAll(setting, "\r\n", "\n"), "\n") {
		if strings.TrimSpace(line) == "---" {
			flush()
			continue
		}
		cur = append(cur, line)
	}
	flush()
	return cards
}

// pickCard is the card used longest ago, so the cards take turns.
func (s *ShopService) pickCard(cards []string) string {
	if len(cards) <= 1 {
		if len(cards) == 1 {
			return cards[0]
		}
		return ""
	}
	db := database.GetDB()
	best, bestLast := cards[0], int64(-1)
	for _, c := range cards {
		var last int64
		db.Model(&model.ShopOrder{}).Where("card = ?", c).Select("COALESCE(MAX(id), 0)").Scan(&last)
		if bestLast < 0 || last < bestLast {
			best, bestLast = c, last
		}
	}
	return best
}

// uniqueExtra is an addition to paid, 1 to most, that no open card order
// pays; 0 when every one is taken.
func (s *ShopService) uniqueExtra(paid int64, most int) int64 {
	var taken []int64
	database.GetDB().Model(&model.ShopOrder{}).
		Where("status = ? AND method = ? AND created_at >= ? AND paid + extra > ? AND paid + extra <= ?",
			model.OrderPending, model.PayCard, time.Now().Add(-openOrderWindow).Unix(), paid, paid+int64(most)).
		Pluck("paid + extra", &taken)
	used := make(map[int64]bool, len(taken))
	for _, t := range taken {
		used[t-paid] = true
	}
	if len(used) >= most {
		return 0
	}
	// A random start, so the amounts do not tell how many orders there are.
	start := common.RandomInt(most)
	for i := 0; i < most; i++ {
		e := int64((start+i)%most + 1)
		if !used[e] {
			return e
		}
	}
	return 0
}

// ToPay is what the customer transfers for a card order.
func ToPay(o *model.ShopOrder) int64 { return o.Paid + o.Extra }

// ---- receipts ----

// ErrReceiptUsed is returned for a receipt another order already has.
type ErrReceiptUsed struct{ Order uint }

func (e ErrReceiptUsed) Error() string {
	return fmt.Sprintf("this receipt was already sent for order #%d", e.Order)
}

var receiptDigitsRe = regexp.MustCompile(`\d{6,}`)

// ReceiptKey identifies a receipt: the unique id Telegram gives a photo or
// file, or the longest run of digits of a typed tracking number. "" when it
// has nothing to tell it by.
func ReceiptKey(kind, uniqueID, text string) string {
	switch kind {
	case "photo", "doc":
		if uniqueID != "" {
			return kind + ":" + uniqueID
		}
	case "text":
		best := ""
		for _, d := range receiptDigitsRe.FindAllString(asciiDigits(text), -1) {
			if len(d) > len(best) {
				best = d
			}
		}
		if best != "" {
			return "text:" + best
		}
	}
	return ""
}

// receiptWindow is how far back a receipt is looked for among the orders.
const receiptWindow = 90 * 24 * time.Hour

// SetReceiptKeyed saves the receipt of a pending order unless an open or
// approved order of the last receiptWindow already has the same receipt.
func (s *ShopService) SetReceiptKeyed(id uint, receipt, key string) error {
	return database.GetDB().Transaction(func(tx *gorm.DB) error {
		if key != "" {
			var other model.ShopOrder
			err := tx.Where("receipt_key = ? AND id <> ? AND status IN ? AND created_at >= ?", key, id,
				[]string{model.OrderPending, model.OrderApproved}, time.Now().Add(-receiptWindow).Unix()).
				Order("id").Limit(1).Find(&other).Error
			if err != nil {
				return err
			}
			if other.Id != 0 {
				return ErrReceiptUsed{Order: other.Id}
			}
		}
		return tx.Model(&model.ShopOrder{}).Where("id = ? AND status = ?", id, model.OrderPending).
			Updates(map[string]any{"receipt": receipt, "receipt_key": key}).Error
	})
}

// ---- bank messages ----

// asciiDigits turns Persian and Arabic digits into ASCII ones.
func asciiDigits(s string) string {
	var sb strings.Builder
	for _, r := range s {
		switch {
		case r >= '۰' && r <= '۹':
			sb.WriteRune('0' + (r - '۰'))
		case r >= '٠' && r <= '٩':
			sb.WriteRune('0' + (r - '٠'))
		default:
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// normalizeSms makes the spellings banks use comparable: ASCII digits,
// Persian letters for Arabic ones, plain commas, no direction marks.
func normalizeSms(text string) string {
	r := strings.NewReplacer(
		"ي", "ی", "ى", "ی", "ك", "ک", "ة", "ه",
		"٬", ",", "،", ",", "\u066b", ".",
		"\u200c", "", "\u200d", "", "\u200e", "", "\u200f", "",
		"\u202a", "", "\u202b", "", "\u202c", "", "\u202d", "", "\u202e", "", "\ufeff", "",
		"\r\n", "\n", "\r", "\n",
	)
	return strings.ToLower(r.Replace(asciiDigits(text)))
}

var (
	smsDepositWords  = []string{"واریز", "deposit", "credited", "credit", "received", "+"}
	smsWithdrawWords = []string{"برداشت", "withdraw", "debit", "خرید", "پرداخت", "انتقال از"}
	smsBalanceWords  = []string{"مانده", "موجودی", "balance", "باقیمانده", "bal:"}
	// Words a number of the line is not an amount after: account, card and
	// reference numbers.
	smsRefWords = []string{"حساب", "کارت", "شماره", "شبا", "پیگیری", "مرجع", "سند", "ترمینال", "کد", "ref", "acc", "card", "no.", "iban", "ش.ح", "ش ح"}
	smsNumberRe = regexp.MustCompile(`\d{1,3}(?:,\d{3})+|\d+`)
)

func containsAny(s string, words []string) bool {
	for _, w := range words {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}

type smsAmount struct {
	value          int64
	sign           byte // '+', '-' or 0
	line           int
	deposit, debit bool // the words of its line
}

// smsAmounts lists the numbers of a message that can be an amount of money.
func smsAmounts(lines []string) []smsAmount {
	var out []smsAmount
	for li, line := range lines {
		if containsAny(line, smsBalanceWords) {
			continue
		}
		deposit := containsAny(line, smsDepositWords[:len(smsDepositWords)-1])
		debit := containsAny(line, smsWithdrawWords)
		for _, m := range smsNumberRe.FindAllStringIndex(line, -1) {
			raw := line[m[0]:m[1]]
			digits := strings.ReplaceAll(raw, ",", "")
			if len(digits) >= 13 {
				continue // a card, account or IBAN number
			}
			before, after := "", ""
			if m[0] > 0 {
				before = line[:m[0]]
			}
			if m[1] < len(line) {
				after = line[m[1]:]
			}
			// Masked numbers, dates and times.
			if strings.HasSuffix(before, "*") || strings.HasPrefix(after, "*") ||
				strings.HasSuffix(before, "x") || strings.HasPrefix(after, "x") ||
				strings.HasSuffix(before, "/") || strings.HasPrefix(after, "/") ||
				strings.HasSuffix(before, ":") && len(before) > 1 && before[len(before)-2] >= '0' && before[len(before)-2] <= '9' ||
				strings.HasPrefix(after, ":") && len(after) > 1 && after[1] >= '0' && after[1] <= '9' ||
				strings.HasPrefix(after, ".") && len(after) > 1 && after[1] >= '0' && after[1] <= '9' {
				continue
			}
			var sign byte
			trimmed := strings.TrimRight(before, " \t")
			if strings.HasSuffix(trimmed, "+") {
				sign = '+'
			} else if strings.HasSuffix(trimmed, "-") {
				// "1402-07-15" is a date, "-1,000" a debit.
				prev := strings.TrimSuffix(trimmed, "-")
				if prev != "" && prev[len(prev)-1] >= '0' && prev[len(prev)-1] <= '9' {
					continue
				}
				sign = '-'
			}
			if strings.HasPrefix(after, "-") && len(after) > 1 && after[1] >= '0' && after[1] <= '9' {
				continue
			}
			// Written right to left, the sign can come after the digits.
			if sign == 0 {
				switch {
				case strings.HasPrefix(after, "+"):
					sign = '+'
				case strings.HasPrefix(after, "-"):
					sign = '-'
				}
			}
			// A plain number right after an account or reference word is
			// that number, not money.
			if !strings.Contains(raw, ",") && sign == 0 {
				tail := []rune(before)
				if len(tail) > 14 {
					tail = tail[len(tail)-14:]
				}
				if containsAny(string(tail), smsRefWords) {
					continue
				}
			}
			v, err := strconv.ParseInt(digits, 10, 64)
			if err != nil || v < 1000 {
				continue
			}
			out = append(out, smsAmount{value: v, sign: sign, line: li, deposit: deposit, debit: debit})
		}
	}
	return out
}

// ParseDepositSms reads the deposit a bank message tells of. rialToToman
// takes amounts without a unit, or in rials, as rials and returns tomans.
// ok is false, with the reason, for a message that is not a deposit or
// whose amount is not clear.
func ParseDepositSms(text string, rialToToman bool) (amount int64, ok bool, why string) {
	norm := normalizeSms(text)
	lines := strings.Split(norm, "\n")
	for i := range lines {
		lines[i] = strings.TrimSpace(lines[i])
	}
	all := smsAmounts(lines)
	var picked []smsAmount
	for _, a := range all {
		if a.sign == '-' || (a.debit && !a.deposit && a.sign != '+') {
			continue
		}
		if a.sign == '+' || a.deposit {
			picked = append(picked, a)
		}
	}
	if len(picked) == 0 {
		// "واریز" on a line of its own, the amount on the next one.
		for li, line := range lines {
			if !containsAny(line, smsDepositWords[:len(smsDepositWords)-1]) || containsAny(line, smsWithdrawWords) {
				continue
			}
			for _, a := range all {
				if a.line == li+1 && a.sign != '-' && !a.debit {
					picked = append(picked, a)
				}
			}
		}
	}
	if len(picked) == 0 {
		if containsAny(norm, smsWithdrawWords) || strings.Contains(norm, "-") && !containsAny(norm, smsDepositWords[:len(smsDepositWords)-1]) {
			return 0, false, "not a deposit"
		}
		return 0, false, "no deposit amount in the message"
	}
	value := picked[0].value
	for _, a := range picked[1:] {
		if a.value != value {
			return 0, false, "more than one amount in the message"
		}
	}
	toman := strings.Contains(norm, "تومان") || strings.Contains(norm, "toman")
	rial := strings.Contains(norm, "ریال") || strings.Contains(norm, "rial") || strings.Contains(norm, "irr")
	if rialToToman && (rial || !toman) {
		if value%10 != 0 {
			return 0, false, "the rial amount is not a whole number of tomans"
		}
		value /= 10
	}
	return value, true, ""
}

// SmsResult is what came of one bank message.
type SmsResult struct {
	Status  string `json:"status"`
	Amount  int64  `json:"amount"`
	OrderId uint   `json:"orderId"`
	Note    string `json:"note"`
}

// smsMaxText caps what is kept of a message.
const smsMaxText = 2000

// HandleSms reads a bank message and approves the one open card order whose
// amount it says came in. A message is handled once: the phone may send it
// again.
func (s *ShopService) HandleSms(text, sender string) (*SmsResult, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, errors.New("empty message")
	}
	if utf8.RuneCountInString(text) > smsMaxText {
		text = string([]rune(text)[:smsMaxText])
	}
	if utf8.RuneCountInString(sender) > 64 {
		sender = string([]rune(sender)[:64])
	}
	sum := sha256.Sum256([]byte(normalizeSms(text)))
	rec := model.ShopSms{Hash: hex.EncodeToString(sum[:]), Sender: sender, Text: text, CreatedAt: time.Now().Unix()}
	db := database.GetDB()
	res := db.Where("hash = ?", rec.Hash).Attrs(rec).FirstOrCreate(&rec)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return &SmsResult{Status: model.SmsDuplicate, Amount: rec.Amount, OrderId: rec.OrderId}, nil
	}
	result := s.matchSms(text)
	rec.Status, rec.Amount, rec.OrderId, rec.Note = result.Status, result.Amount, result.OrderId, result.Note
	if err := db.Model(&model.ShopSms{}).Where("id = ?", rec.Id).
		Updates(map[string]any{"status": rec.Status, "amount": rec.Amount, "order_id": rec.OrderId, "note": rec.Note}).Error; err != nil {
		logger.Warning("shop: save bank message: ", err)
	}
	// Only the newest messages are kept.
	db.Where("id <= ?", int64(rec.Id)-1000).Delete(&model.ShopSms{})
	return result, nil
}

func (s *ShopService) matchSms(text string) *SmsResult {
	st := s.Settings()
	amount, ok, why := ParseDepositSms(text, st.SmsRial)
	if !ok {
		return &SmsResult{Status: model.SmsIgnored, Note: why}
	}
	var orders []model.ShopOrder
	database.GetDB().Where("status = ? AND method = ? AND created_at >= ? AND paid + extra = ?",
		model.OrderPending, model.PayCard, time.Now().Add(-openOrderWindow).Unix(), amount).Order("id").Limit(3).Find(&orders)
	switch len(orders) {
	case 0:
		return &SmsResult{Status: model.SmsUnmatched, Amount: amount, Note: "no open order of this amount"}
	case 1:
	default:
		return &SmsResult{Status: model.SmsUnmatched, Amount: amount, Note: "more than one open order of this amount"}
	}
	o := orders[0]
	if ShopDecider == nil {
		return &SmsResult{Status: model.SmsError, Amount: amount, OrderId: o.Id, Note: ErrBotDown.Error()}
	}
	if _, err := ShopDecider(o.Id, true); err != nil {
		return &SmsResult{Status: model.SmsError, Amount: amount, OrderId: o.Id, Note: err.Error()}
	}
	if ShopSmsApproved != nil {
		ShopSmsApproved(o.Id, amount)
	}
	return &SmsResult{Status: model.SmsApproved, Amount: amount, OrderId: o.Id}
}

// ShopSmsApproved, set by the running bot, tells the order approvers about an
// order a bank message approved.
var ShopSmsApproved func(orderID uint, amount int64)

// SmsLog is the newest bank messages, newest first.
func (s *ShopService) SmsLog(limit int) ([]model.ShopSms, error) {
	var out []model.ShopSms
	err := database.GetDB().Order("id DESC").Limit(limit).Find(&out).Error
	return out, err
}
