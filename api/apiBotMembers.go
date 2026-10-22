package api

import (
	"strconv"
	"strings"

	"github.com/Danialrostamani/drnetwork-panel/service"
	"github.com/Danialrostamani/drnetwork-panel/service/tgbot"
	"github.com/Danialrostamani/drnetwork-panel/util/common"

	"github.com/gin-gonic/gin"
)

// GetBotMembers is everybody who used the Telegram bot's shop, with the
// introducer words and the sellers the owner may give them.
func (a *ApiService) GetBotMembers(c *gin.Context) {
	ms := &service.BotMemberService{}
	members, err := ms.List()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	sellers, _ := tgbot.PanelSellers(c.Request.Context())
	if sellers == nil {
		sellers = []tgbot.PanelSeller{}
	}
	jsonObj(c, gin.H{"members": members, "refs": ms.Refs(), "sellers": sellers}, nil)
}

func formTgID(c *gin.Context) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(c.Request.FormValue("tgId")), 10, 64)
	if err != nil || id <= 0 {
		return 0, common.NewError("bad telegram id")
	}
	return id, nil
}

// BotMemberAction changes one bot member: "ref" sets or removes the
// introducer, "block" blocks or unblocks, "seller" gives them to a seller and
// "wallet" adds to or takes from their wallet.
func (a *ApiService) BotMemberAction(c *gin.Context, what string) {
	id, err := formTgID(c)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	switch what {
	case "ref":
		err = (&service.BotMemberService{}).SetRef(id, c.Request.FormValue("word"))
	case "block":
		err = tgbot.PanelSetBlocked(id, c.Request.FormValue("blocked") == "true")
	case "seller":
		var seller int64
		if seller, err = strconv.ParseInt(strings.TrimSpace(c.Request.FormValue("seller")), 10, 64); err != nil || seller < 0 {
			err = common.NewError("bad seller id")
			break
		}
		err = tgbot.PanelSetSeller(id, seller)
	case "wallet":
		var delta int64
		if delta, err = strconv.ParseInt(strings.TrimSpace(c.Request.FormValue("delta")), 10, 64); err != nil || delta == 0 || delta > 1e15 || delta < -1e15 {
			jsonMsg(c, "", common.NewError("bad amount"))
			return
		}
		bal, werr := tgbot.PanelWallet(c.Request.Context(), id, delta, c.Request.FormValue("note"), c.Request.FormValue("notify") == "true")
		jsonObj(c, gin.H{"balance": bal}, werr)
		return
	default:
		err = common.NewError("unknown action: ", what)
	}
	jsonMsg(c, "", err)
}

// BotMembersMessage sends one message, through the bot, to the members whose
// Telegram IDs are listed, comma separated.
func (a *ApiService) BotMembersMessage(c *gin.Context) {
	var ids []int64
	for _, s := range strings.Split(c.Request.FormValue("ids"), ",") {
		if s = strings.TrimSpace(s); s == "" {
			continue
		}
		id, err := strconv.ParseInt(s, 10, 64)
		if err != nil || id <= 0 {
			jsonMsg(c, "", common.NewError("bad telegram id: ", s))
			return
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		jsonMsg(c, "", common.NewError("no members chosen"))
		return
	}
	n, err := tgbot.PanelMessage(ids, c.Request.FormValue("text"))
	jsonObj(c, gin.H{"queued": n}, err)
}
