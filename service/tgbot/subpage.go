package tgbot

import (
	"context"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/service"
)

// runningName is the @username of the bot that is running now, "" while none
// is: the subscription page links to it.
var runningName atomic.Value

func setRunningName(name string) { runningName.Store(strings.TrimPrefix(strings.TrimSpace(name), "@")) }

// RunningName is the username of the running bot, without the @.
func RunningName() string {
	v, _ := runningName.Load().(string)
	return v
}

// SubPagePlan is a plan the subscription page offers.
type SubPagePlan struct {
	Name  string `json:"name,omitempty"`
	Days  int    `json:"days"`
	GB    int    `json:"gb"`
	Price string `json:"price,omitempty"`
}

// SubPageInfo is what the subscription page of a client takes from the bot:
// the bot to renew through, and the support, channel and plans of the
// administrator who sells to the client.
type SubPageInfo struct {
	Bot     string
	Support string
	Channel string
	Plans   []SubPagePlan
}

// SubPageExtras is the bot's part of a client's page; empty while the bot is
// not running.
func SubPageExtras(c *model.Client) SubPageInfo {
	var out SubPageInfo
	name := RunningName()
	if name == "" || c == nil {
		return out
	}
	b, err := panelBot()
	if err != nil || !b.cfg.Enable || b.cfg.Token == "" {
		return out
	}
	out = b.subPageInfo(*c)
	out.Bot = name
	return out
}

// subPageInfo is the seller's part of a client's page.
func (b *bot) subPageInfo(c model.Client) SubPageInfo {
	var out SubPageInfo
	seller := b.clientSeller(c)
	raw := b.loadShopRaw()
	shop := raw.forAdmin(seller)
	if d, ok := raw.Desks[strconv.FormatInt(seller, 10)]; ok && seller != 0 {
		out.Support = service.PageLink(d.Support)
		out.Channel = service.PageLink(d.Channel)
	}
	for _, p := range shop.Plans {
		if strings.TrimSpace(p.Name) == "" && p.Days == 0 && p.GB == 0 {
			continue
		}
		out.Plans = append(out.Plans, SubPagePlan{Name: p.Name, Days: p.Days, GB: p.GB, Price: p.Price})
	}
	return out
}

// clientSeller is the administrator who sells to a client: its customer's
// seller when it is bound to Telegram, else the administrator whose group
// it is in, else the owner.
func (b *bot) clientSeller(c model.Client) int64 {
	if c.TgId != 0 {
		return b.customerAdminID(c.TgId)
	}
	a := b.access()
	if g := strings.TrimSpace(c.Group); g != "" {
		for _, id := range a.order {
			if strings.EqualFold(strings.TrimSpace(a.roleOf(id).group), g) && (a.isOwner(id) || !adminMeta(id).Suspended) {
				return id
			}
		}
	}
	return a.owner
}

// The subscription page opens the bot with /start and one of these in front
// of the link's last part (base64url): renew it, or connect it to the chat
// for its alerts.
const (
	startRenew   = "rn_"
	startConnect = "cn_"
)

// startSubLink handles a /start from the subscription page; false when the
// argument is not one.
func (b *bot) startSubLink(ctx context.Context, chatID int64, from *fromUser, arg string) bool {
	var renew bool
	switch {
	case strings.HasPrefix(arg, startRenew):
		renew = true
	case strings.HasPrefix(arg, startConnect):
	default:
		return false
	}
	raw, err := base64.RawURLEncoding.DecodeString(arg[len(startRenew):])
	if err != nil || len(raw) == 0 {
		return false
	}
	c, ok := clientBySubID(string(raw))
	if !ok {
		b.sendMenu(ctx, chatID, b.tr(
			"❌ این اشتراک پیدا نشد. لینک اشتراک را از «اتصال لینک» بفرستید.",
			"❌ That subscription was not found. Send its link from “Connect link”."),
			b.customerMenu(from.ID))
		return true
	}
	if c.TgId == from.ID {
		if renew {
			b.pend.clear(chatID)
			b.customerBuy(ctx, chatID, from, strconv.FormatUint(uint64(c.Id), 10))
			return true
		}
		b.bindClientRequest(ctx, chatID, from, c)
		return true
	}
	if c.TgId == 0 && b.customerNeedsRef(from.ID) {
		b.customerAskRef(ctx, chatID, from, "bind:"+strconv.FormatUint(uint64(c.Id), 10))
		return true
	}
	if c.TgId == 0 && renew {
		b.send(ctx, chatID, b.tr(
			"♻️ برای تمدید، اول این اشتراک به حسابتان وصل می‌شود؛ بعد از تأیید ادمین از «تمدید» استفاده کنید.",
			"♻️ To renew, this subscription is first connected to your account; once an admin approves, use “Renew”."))
	}
	b.bindClientRequest(ctx, chatID, from, c)
	return true
}

// clientByIDString is the client with a database ID written out.
func clientByIDString(id string) (model.Client, bool) {
	n, err := strconv.ParseUint(strings.TrimSpace(id), 10, 32)
	if err != nil || n == 0 {
		return model.Client{}, false
	}
	var c model.Client
	if database.GetDB().First(&c, uint(n)).Error != nil {
		return model.Client{}, false
	}
	return c, true
}

// SubPageLastPlan is the last plan a client was sold or renewed with through
// the bot, for its page; nil when there is none.
type SubPageLastPlan struct {
	Name string `json:"name,omitempty"`
	GB   int    `json:"gb"`
	Days int    `json:"days"`
	At   int64  `json:"at"`
	Kind string `json:"kind"`
}

// LastPlanOf reads it from the approved orders; the bot need not run.
func LastPlanOf(c *model.Client) *SubPageLastPlan {
	if c == nil || c.Id == 0 || database.GetDB() == nil {
		return nil
	}
	var o model.BotOrder
	err := database.GetDB().Where("client_id = ? AND status = ? AND topic IN ?", c.Id, orderApproved, []string{"buy", "renew", "test"}).
		Order("id desc").First(&o).Error
	if err != nil {
		return nil
	}
	at := o.UpdatedAt
	if at == 0 {
		at = o.CreatedAt
	}
	out := &SubPageLastPlan{GB: o.PlanGB, Days: o.PlanDays, At: at, Kind: o.Topic}
	b := &bot{}
	if p := b.loadShopRaw().forAdmin(o.AdminId).plan(o.PlanID); p != nil {
		out.Name = p.Name
		if !o.PlanSet {
			out.GB, out.Days = p.GB, p.Days
		}
	}
	return out
}

// errNoReport is a report the bot cannot pass on.
var errNoReport = errors.New("the Telegram bot is not running")

// SubPageReport passes a problem report from a client's page to its seller
// and the owner.
func SubPageReport(ctx context.Context, c *model.Client, ip, text string) error {
	if RunningName() == "" || c == nil {
		return errNoReport
	}
	b, err := panelBot()
	if err != nil || !b.cfg.Enable || b.cfg.Token == "" {
		return errNoReport
	}
	who := "<code>" + esc(c.Name) + "</code>"
	if c.TgId != 0 {
		who += " · TG <code>" + strconv.FormatInt(c.TgId, 10) + "</code>"
		if row, ok := customerRow(c.TgId); ok {
			if n := strings.TrimSpace(nonempty(row.Name, row.Username)); n != "" {
				who += " (" + esc(n) + ")"
			}
		}
	}
	msg := b.tr("🆘 <b>گزارش مشکل از صفحه اشتراک</b>", "🆘 <b>Problem report from the subscription page</b>") +
		"\n👤 " + who
	if ip != "" {
		msg += "\n🌐 IP: <code>" + esc(ip) + "</code>"
	}
	msg += "\n\n" + esc(text)
	b.notifyIDs(ctx, b.sellerAndOwner(b.clientSeller(*c)), msg, nil)
	return nil
}
