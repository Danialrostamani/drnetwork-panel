package tgbot

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/logger"
)

// What the panel's bot members page does through the bot: the same changes
// the owner makes from a customer's profile in Telegram.

// PanelSeller is a bot administrator a customer can be given to.
type PanelSeller struct {
	Id    int64  `json:"id"`
	Name  string `json:"name"`
	Owner bool   `json:"owner"`
}

var errBotOff = errors.New("the Telegram bot is off or has no token")

// panelBot is a bot for one request from the panel, with the current
// settings. It does not poll; it only calls Telegram when asked to send.
func panelBot() (*bot, error) {
	cfg, err := loadConfig()
	if err != nil {
		return nil, err
	}
	return newBot(cfg), nil
}

// PanelSellers are the administrators a customer can be given to, the owner
// first, with their Telegram names when the bot can look them up.
func PanelSellers(ctx context.Context) ([]PanelSeller, error) {
	b, err := panelBot()
	if err != nil {
		return nil, err
	}
	a := b.access()
	var ids []int64
	if a.owner != 0 {
		ids = append(ids, a.owner)
	}
	for _, id := range a.order {
		if id != a.owner {
			ids = append(ids, id)
		}
	}
	names := map[int64]string{}
	if b.cfg.Token != "" && len(ids) > 0 {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		names = b.adminNames(ctx, ids)
		cancel()
	}
	out := make([]PanelSeller, 0, len(ids))
	for _, id := range ids {
		out = append(out, PanelSeller{Id: id, Name: names[id], Owner: a.isOwner(id)})
	}
	return out, nil
}

// PanelSetBlocked blocks a customer from the shop, or lets them back in.
func PanelSetBlocked(tgID int64, blocked bool) error {
	if tgID <= 0 {
		return errors.New("bad telegram id")
	}
	c, _ := customerRow(tgID)
	c.TgId, c.Blocked = tgID, blocked
	if c.FirstSeen == 0 {
		c.FirstSeen = time.Now().Unix()
	}
	return saveCustomer(c)
}

// PanelSetSeller gives a customer to a seller; 0 takes them back, so their
// introducer decides again.
func PanelSetSeller(tgID, seller int64) error {
	if tgID <= 0 {
		return errors.New("bad telegram id")
	}
	if seller != 0 {
		b, err := panelBot()
		if err != nil {
			return err
		}
		if !b.access().isMember(seller) {
			return errors.New("that seller is not an administrator of the bot")
		}
	}
	c, _ := customerRow(tgID)
	c.TgId, c.AdminId = tgID, seller
	if c.FirstSeen == 0 {
		c.FirstSeen = time.Now().Unix()
	}
	return saveCustomer(c)
}

// PanelWallet adds delta, negative to take, to a customer's wallet and
// returns the new balance. With notify the customer is told, as when the
// owner changes it from Telegram.
func PanelWallet(ctx context.Context, tgID, delta int64, note string, notify bool) (int64, error) {
	if tgID <= 0 {
		return 0, errors.New("bad telegram id")
	}
	if delta == 0 {
		return walletBalance(tgID), nil
	}
	note = truncate(strings.TrimSpace(note), 80)
	bal, err := walletCredit(tgID, delta, walletMemo{kind: walletManual, note: note})
	if err != nil {
		return 0, err
	}
	if notify {
		if b, err := panelBot(); err == nil && b.cfg.Enable && b.cfg.Token != "" {
			amount := delta
			msg := b.tr("🏦 به کیف پول شما اضافه شد: ", "🏦 Added to your wallet: ")
			if delta < 0 {
				amount = -delta
				msg = b.tr("🏦 از کیف پول شما کم شد: ", "🏦 Taken from your wallet: ")
			}
			msg += "<b>" + formatMoney(amount) + "</b>"
			if note != "" {
				msg += "\n" + esc(note)
			}
			msg += "\n" + b.tr("موجودی: ", "Balance: ") + "<b>" + formatMoney(bal) + "</b>"
			ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			b.send(ctx, tgID, msg)
			cancel()
		}
	}
	return bal, nil
}

// PanelMessage sends text to these customers in the background, a little
// apart, skipping blocked ones, and tells the owner how it went. It returns
// how many will get it.
func PanelMessage(ids []int64, text string) (int, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, errors.New("the message is empty")
	}
	if len([]rune(text)) > maxMessageRunes {
		return 0, errors.New("the message is too long")
	}
	b, err := panelBot()
	if err != nil {
		return 0, err
	}
	if !b.cfg.Enable || b.cfg.Token == "" {
		return 0, errBotOff
	}
	seen := map[int64]bool{}
	var to []int64
	for _, id := range ids {
		if id <= 0 || seen[id] || customerBlocked(id) {
			continue
		}
		seen[id] = true
		to = append(to, id)
	}
	if len(to) == 0 {
		return 0, nil
	}
	body := "📣 " + esc(text)
	runBroadcast(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
		defer cancel()
		sent, failed := 0, 0
		for i, id := range to {
			if i > 0 && broadcastGap > 0 {
				time.Sleep(broadcastGap)
			}
			if b.call(ctx, "sendMessage", map[string]any{"chat_id": id, "text": body, "parse_mode": "HTML", "disable_web_page_preview": true}, nil) != nil {
				failed++
			} else {
				sent++
			}
		}
		logger.Info("telegram bot: panel message sent to ", sent, ", failed for ", failed)
		b.notifyIDs(ctx, b.ownerIDs(), "📣 "+b.tr("پیام پنل به اعضای ربات تمام شد", "The panel's message to bot members finished")+
			": ✅ "+strconv.Itoa(sent)+" · ❌ "+strconv.Itoa(failed), nil)
	})
	return len(to), nil
}
