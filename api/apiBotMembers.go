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
	jsonObj(c, gin.H{"members": members, "refs": ms.Refs(), "sellers": sellers, "clients": ms.Clients()}, nil)
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
// "wallet" adds to or takes from their wallet, "edit" sets the note and the
// bound clients, "trialReset" lets them take another test account and
// "delete" forgets them.
func (a *ApiService) BotMemberAction(c *gin.Context, what, actor string) {
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
	case "edit":
		var ids []uint
		set := c.Request.FormValue("setClients") == "true"
		if set {
			for _, s := range strings.Split(c.Request.FormValue("clients"), ",") {
				if s = strings.TrimSpace(s); s == "" {
					continue
				}
				n, perr := strconv.ParseUint(s, 10, 32)
				if perr != nil || n == 0 {
					jsonMsg(c, "", common.NewError("bad client id: ", s))
					return
				}
				ids = append(ids, uint(n))
			}
		}
		err = tgbot.PanelEditCustomer(id, c.Request.FormValue("note"), set, ids)
	case "trialReset":
		out, rerr := tgbot.PanelResetTrial(c.Request.Context(), &a.ConfigService, actor, getHostname(c), id,
			c.Request.FormValue("deleteClient") == "true", c.Request.FormValue("notify") == "true")
		jsonObj(c, out, rerr)
		return
	case "delete":
		out, derr := tgbot.PanelDeleteCustomer(&a.ConfigService, actor, getHostname(c), id, c.Request.FormValue("deleteClients") == "true")
		jsonObj(c, out, derr)
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

// GetBotAdmins is the bot's administrators with their groups.
func (a *ApiService) GetBotAdmins(c *gin.Context) {
	view, err := tgbot.PanelAdmins(c.Request.Context())
	jsonObj(c, view, err)
}

// BotAdminAction changes the bot's administrators' groups: "group" limits
// one to a group (or frees them with an empty one), "move" puts the clients
// of one into their group, and "auto" gives every administrator without a
// group one of their own, moving their clients when move is true.
func (a *ApiService) BotAdminAction(c *gin.Context, what, actor string) {
	host := getHostname(c)
	if what == "auto" {
		out, err := tgbot.PanelAutoGroups(c.Request.Context(), &a.ConfigService, actor, host, c.Request.FormValue("move") == "true")
		jsonObj(c, out, err)
		return
	}
	id, err := strconv.ParseInt(strings.TrimSpace(c.Request.FormValue("id")), 10, 64)
	if err != nil || id <= 0 {
		jsonMsg(c, "", common.NewError("bad telegram id"))
		return
	}
	switch what {
	case "group":
		err = tgbot.PanelSetAdminGroup(&a.ConfigService, actor, host, id, c.Request.FormValue("group"))
		if err == nil && c.Request.FormValue("move") == "true" && strings.TrimSpace(c.Request.FormValue("group")) != "" {
			n, merr := tgbot.PanelMoveAdminClients(&a.ConfigService, actor, host, id)
			jsonObj(c, gin.H{"moved": n}, merr)
			return
		}
		jsonObj(c, gin.H{"moved": 0}, err)
	case "move":
		n, merr := tgbot.PanelMoveAdminClients(&a.ConfigService, actor, host, id)
		jsonObj(c, gin.H{"moved": n}, merr)
	case "partner":
		jsonMsg(c, "", tgbot.PanelGrantPartner(&a.ConfigService, actor, host, id))
	case "add":
		group, aerr := tgbot.PanelAddAdmin(c.Request.Context(), &a.ConfigService, actor, host, id, c.Request.FormValue("full") == "true", c.Request.FormValue("group"))
		jsonObj(c, gin.H{"group": group}, aerr)
	case "remove":
		jsonMsg(c, "", tgbot.PanelRemoveAdmin(&a.ConfigService, actor, host, id))
	case "role":
		var sections []string
		for _, s := range strings.Split(c.Request.FormValue("sections"), ",") {
			if s = strings.TrimSpace(s); s != "" {
				sections = append(sections, s)
			}
		}
		jsonMsg(c, "", tgbot.PanelSetAdminRole(&a.ConfigService, actor, host, id, c.Request.FormValue("role"), c.Request.FormValue("group"), sections))
	case "quota":
		gb, _ := strconv.ParseInt(strings.TrimSpace(c.Request.FormValue("gb")), 10, 64)
		jsonMsg(c, "", tgbot.PanelSetQuota(actor, id, c.Request.FormValue("op"), gb))
	case "terms":
		num := func(k string) int64 {
			n, _ := strconv.ParseInt(strings.TrimSpace(c.Request.FormValue(k)), 10, 64)
			return n
		}
		jsonMsg(c, "", tgbot.PanelSetSellerTerms(id, tgbot.PanelSellerTerms{
			Suspended:    c.Request.FormValue("suspended") == "true",
			RequireOwner: c.Request.FormValue("requireOwner") == "true",
			MinPerGB:     num("minPerGB"),
			RatePerGB:    num("ratePerGB"),
			ReceiptTo:    num("receiptTo"),
		}))
	case "settle":
		rec, serr := tgbot.PanelSettle(id)
		jsonObj(c, rec, serr)
	default:
		jsonMsg(c, "", common.NewError("unknown action: ", what))
	}
}

// GetBotDashboard is the Telegram bot at a glance.
func (a *ApiService) GetBotDashboard(c *gin.Context) {
	d, err := tgbot.PanelBotDashboard()
	jsonObj(c, d, err)
}

// GetBotOrders lists the bot's orders: status, seller (-1 or empty for all),
// q to search, limit and offset.
func (a *ApiService) GetBotOrders(c *gin.Context) {
	q := tgbot.PanelOrderQuery{Status: c.Query("status"), Seller: -1, Search: c.Query("q")}
	if v := strings.TrimSpace(c.Query("seller")); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			jsonMsg(c, "", common.NewError("invalid seller"))
			return
		}
		q.Seller = n
	}
	q.Limit, _ = strconv.Atoi(c.Query("limit"))
	q.Offset, _ = strconv.Atoi(c.Query("offset"))
	page, err := tgbot.PanelOrders(q)
	jsonObj(c, page, err)
}

// BotOrderEdit corrects one order: amount, detail and seller when given, and
// status ("approved" or "rejected") with notify to tell the customer.
func (a *ApiService) BotOrderEdit(c *gin.Context, loginUser string) {
	id, err := strconv.ParseUint(c.PostForm("id"), 10, 32)
	if err != nil || id == 0 {
		jsonMsg(c, "botOrderEdit", common.NewError("invalid order id"))
		return
	}
	e := tgbot.PanelOrderEdit{Id: uint(id), Status: c.PostForm("status"), Notify: c.PostForm("notify") == "true"}
	if v, ok := c.GetPostForm("amount"); ok {
		n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		if err != nil {
			jsonMsg(c, "botOrderEdit", common.NewError("invalid amount"))
			return
		}
		e.Amount = &n
	}
	if v, ok := c.GetPostForm("detail"); ok {
		e.Detail = &v
	}
	if v, ok := c.GetPostForm("seller"); ok {
		n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		if err != nil {
			jsonMsg(c, "botOrderEdit", common.NewError("invalid seller"))
			return
		}
		e.Seller = &n
	}
	row, err := tgbot.PanelEditOrder(c.Request.Context(), &a.ConfigService, loginUser, e)
	jsonMsgObj(c, "botOrderEdit", row, err)
}
