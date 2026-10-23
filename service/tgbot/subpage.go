package tgbot

import (
	"context"
	"encoding/base64"
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
