package tgbot

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/logger"
)

// memberFor is how long a customer found in the shop's channel is not asked
// again.
const memberFor = 10 * time.Minute

// memberCache remembers the customers found in the shop's channel.
type memberCache struct {
	mu sync.Mutex
	ok map[string]time.Time
}

// gate stops a customer the bot is not open to right now: while the shop is
// in maintenance, and until they join the shop's channel. It tells whether
// it answered. data is the message text or the button's data.
func (b *bot) gate(ctx context.Context, chatID int64, from *fromUser, cbID string, msgID int64, data string) bool {
	if b.isAdmin(from.ID) {
		return false
	}
	t := b.tr
	st := b.shopSvc().Settings()
	reply := func(text string, kb [][]button) {
		if cbID != "" {
			b.answer(ctx, cbID, "")
			b.edit(ctx, chatID, msgID, text, kb)
			return
		}
		b.sendKeyboard(ctx, chatID, text, kb)
	}
	if st.Maintenance != "" {
		reply("🛠 "+esc(st.Maintenance), nil)
		return true
	}
	if st.Channel == "" {
		return false
	}
	joined := data == "p:joined"
	if b.isMember(ctx, st.Channel, from.ID, joined) {
		if !joined {
			return false
		}
		b.answer(ctx, cbID, "✅")
		if b.shopOpen(from.ID) {
			b.shopStart(ctx, chatID, from, "")
		} else {
			b.edit(ctx, chatID, msgID, t("✅ عضویت شما تایید شد. /start را بزنید.", "✅ You are in. Send /start."), nil)
		}
		return true
	}
	if joined {
		b.answer(ctx, cbID, t("هنوز عضو کانال نشده‌اید.", "You have not joined the channel yet."))
		return true
	}
	// A referral link is kept for when they come back.
	if cmd, arg := parseCommand(data); cmd == "start" && strings.HasPrefix(arg, "ref") && b.shopOpen(from.ID) {
		ref, _ := strconv.ParseInt(strings.TrimPrefix(arg, "ref"), 10, 64)
		if _, err := b.shopSvc().Touch(from.ID, strings.TrimSpace(from.FirstName+" "+from.LastName), from.Username, ref); err != nil {
			logger.Warning("telegram bot: shop user: ", err)
		}
	}
	reply(t("📢 برای استفاده از ربات ابتدا عضو کانال ما شوید، سپس «✅ عضو شدم» را بزنید.", "📢 Join our channel to use the bot, then tap “✅ I joined”."),
		[][]button{{{Text: t("📢 عضویت در کانال", "📢 Join the channel"), URL: st.ChannelLink}}, {{Text: t("✅ عضو شدم", "✅ I joined"), Data: "p:joined"}}})
	return true
}

// isMember tells whether a user is in the channel. fresh asks Telegram even
// when they were found there a moment ago. A channel the bot cannot read
// locks nobody out: the bot has to be one of its administrators.
func (b *bot) isMember(ctx context.Context, chat string, user int64, fresh bool) bool {
	c := b.members
	key := chat + "|" + strconv.FormatInt(user, 10)
	if c != nil && !fresh {
		c.mu.Lock()
		until, ok := c.ok[key]
		c.mu.Unlock()
		if ok && time.Now().Before(until) {
			return true
		}
	}
	var m struct {
		Status   string `json:"status"`
		IsMember bool   `json:"is_member"`
	}
	in := true
	if err := b.call(ctx, "getChatMember", map[string]any{"chat_id": chat, "user_id": user}, &m); err != nil {
		logger.Warning("telegram bot: channel check (is the bot an administrator of ", chat, "?): ", b.scrub(err.Error()))
	} else {
		in = m.Status == "creator" || m.Status == "administrator" || m.Status == "member" || m.Status == "restricted" && m.IsMember
	}
	if c != nil {
		c.mu.Lock()
		if c.ok == nil || len(c.ok) > 50000 {
			c.ok = map[string]time.Time{}
		}
		if in {
			c.ok[key] = time.Now().Add(memberFor)
		} else {
			delete(c.ok, key)
		}
		c.mu.Unlock()
	}
	return in
}
