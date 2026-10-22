package tgbot

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/service"
)

// The panel's view of the bot's administrators and their groups. A group
// limits an administrator to its clients; what they sell goes into it. The
// owner is never limited and keeps seeing everything.

// PanelAdmin is one bot administrator as the panel shows them.
type PanelAdmin struct {
	Id        int64    `json:"id"`
	Name      string   `json:"name"`
	Role      string   `json:"role"` // owner, full, sections, group or locked
	Group     string   `json:"group"`
	Sections  []string `json:"sections"`
	Clients   int      `json:"clients"`
	Customers int      `json:"customers"`
	Sales     int      `json:"sales"`
	Revenue   int64    `json:"revenue"`
	// Outside counts the clients of their customers, sales and creations that
	// are not in their group yet.
	Outside int `json:"outside"`
	// The owner's terms with them, their volume limit (nil when none) and
	// what they sold since the last settlement.
	Suspended       bool        `json:"suspended"`
	RequireOwner    bool        `json:"requireOwner"`
	MinPerGB        int64       `json:"minPerGB"`
	RatePerGB       int64       `json:"ratePerGB"`
	Quota           *PanelQuota `json:"quota"`
	UnsettledOrders int64       `json:"unsettledOrders"`
	UnsettledGB     int64       `json:"unsettledGB"`
	Owed            int64       `json:"owed"`
}

// PanelAdminsView is the administrators and the client groups there are.
type PanelAdminsView struct {
	Owner  int64        `json:"owner"`
	Admins []PanelAdmin `json:"admins"`
	Groups []string     `json:"groups"`
	// Sections are the parts of the bot an administrator can be given.
	Sections []PanelSection `json:"sections"`
}

// PanelAdmins lists the bot's administrators, the owner first.
func PanelAdmins(ctx context.Context) (PanelAdminsView, error) {
	cfg, err := loadConfig()
	if err != nil {
		return PanelAdminsView{}, err
	}
	b := newBot(cfg)
	a := b.access()
	view := PanelAdminsView{Owner: a.owner, Admins: []PanelAdmin{}, Groups: []string{}, Sections: PanelSections()}
	names := map[int64]string{}
	if cfg.Token != "" && len(a.order) > 0 {
		nctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		names = b.adminNames(nctx, a.order)
		cancel()
	}
	clients := allClients()
	groupSet := map[string]string{}
	for _, c := range clients {
		g := strings.TrimSpace(c.Group)
		if g != "" && !sameGroup(g, service.ClusterGroup) {
			groupSet[strings.ToLower(g)] = g
		}
	}
	sellerOf := b.customerSellers()
	for _, id := range a.order {
		r := a.roleOf(id)
		pa := PanelAdmin{Id: id, Name: names[id], Group: r.group, Sections: []string{}}
		switch {
		case a.isOwner(id):
			pa.Role = "owner"
		case hasID(cfg.Locked, id):
			pa.Role = "locked"
		case r.group != "":
			pa.Role = "group"
			groupSet[strings.ToLower(r.group)] = r.group
		case r.sections != nil:
			pa.Role = "sections"
			pa.Sections = r.sections.keys()
		default:
			pa.Role = "full"
		}
		if r.group != "" {
			for _, c := range clients {
				if sameGroup(c.Group, r.group) {
					pa.Clients++
				}
			}
		}
		if !a.isOwner(id) {
			for _, seller := range sellerOf {
				if seller == id {
					pa.Customers++
				}
			}
			if r.group != "" {
				if ids, err := b.adminClientIDs(id, a, sellerOf); err == nil {
					pa.Outside = len(ids)
				}
			}
		}
		var sales []model.BotOrder
		database.GetDB().Select("amount").Where("admin_id = ? AND status = ? AND topic IN ?", id, orderApproved, []string{"buy", "renew"}).Find(&sales)
		pa.Sales = len(sales)
		for _, o := range sales {
			pa.Revenue += o.Amount
		}
		b.panelAdminExtras(&pa)
		view.Admins = append(view.Admins, pa)
	}
	for _, g := range groupSet {
		view.Groups = append(view.Groups, g)
	}
	sort.Slice(view.Groups, func(i, j int) bool { return strings.ToLower(view.Groups[i]) < strings.ToLower(view.Groups[j]) })
	return view, nil
}

// panelEditAccess changes the bot's administrator settings from the panel,
// through the same save as the settings page, so the history records actor.
func panelEditAccess(cs *service.ConfigService, actor, host string, mutate func(d *accessDoc) error) error {
	if cs == nil {
		return fmt.Errorf("the panel is not ready to save settings")
	}
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	d := docFromConfig(cfg)
	if err := mutate(d); err != nil {
		return err
	}
	values, err := d.settings()
	if err != nil {
		return err
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return err
	}
	_, err = cs.Save("settings", "", raw, "", actor, host)
	return err
}

// customerSellers is the seller of every customer, worked out once.
func (b *bot) customerSellers() map[int64]int64 {
	out := map[int64]int64{}
	for _, c := range b.customerList() {
		out[c.TgId] = b.customerAdminID(c.TgId)
	}
	return out
}

// PanelSetAdminGroup limits an administrator to a group, or with "" gives
// them full access again. The owner cannot be limited.
func PanelSetAdminGroup(cs *service.ConfigService, actor, host string, id int64, group string) error {
	group = strings.TrimSpace(group)
	return panelEditAccess(cs, actor, host, func(d *accessDoc) error {
		if !d.member(id) {
			return errNoSuchAdmin
		}
		return d.setRole(id, role{group: group})
	})
}

var groupNameJunk = regexp.MustCompile(`[^\p{L}\p{N} _.\-]+`)

// autoGroupName is a group name made from an administrator's Telegram name,
// or "admin-<ID>" when there is none usable.
func autoGroupName(name string, id int64) string {
	g := strings.Join(strings.Fields(groupNameJunk.ReplaceAllString(name, " ")), " ")
	if r := []rune(g); len(r) > 40 {
		g = strings.TrimSpace(string(r[:40]))
	}
	if g == "" || !scopeGroupUsable(g) {
		g = "admin-" + strconv.FormatInt(id, 10)
	}
	return g
}

// PanelAutoGroup is what PanelAutoGroups did for one administrator.
type PanelAutoGroup struct {
	Id    int64  `json:"id"`
	Group string `json:"group"`
	New   bool   `json:"new"`
	Moved int    `json:"moved"`
}

// PanelAutoGroups gives every administrator but the owner who has no group
// one of their own, named after them and not used by anybody else. With move,
// the clients of each limited administrator's customers, sales and
// creations are then moved into their group.
func PanelAutoGroups(ctx context.Context, cs *service.ConfigService, actor, host string, move bool) ([]PanelAutoGroup, error) {
	cfg, err := loadConfig()
	if err != nil {
		return nil, err
	}
	b := newBot(cfg)
	a := b.access()
	names := map[int64]string{}
	if cfg.Token != "" {
		nctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		names = b.adminNames(nctx, a.order)
		cancel()
	}
	taken := map[string]bool{}
	for _, c := range allClients() {
		taken[strings.ToLower(strings.TrimSpace(c.Group))] = true
	}
	for _, id := range a.order {
		if g := a.roleOf(id).group; g != "" {
			taken[strings.ToLower(g)] = true
		}
	}
	var out []PanelAutoGroup
	fresh := map[int64]string{}
	for _, id := range a.order {
		if a.isOwner(id) {
			continue
		}
		if g := a.roleOf(id).group; g != "" {
			out = append(out, PanelAutoGroup{Id: id, Group: g})
			continue
		}
		g := autoGroupName(names[id], id)
		if taken[strings.ToLower(g)] {
			g = autoGroupName(g+"-"+strconv.FormatInt(id, 10), id)
		}
		taken[strings.ToLower(g)] = true
		fresh[id] = g
		out = append(out, PanelAutoGroup{Id: id, Group: g, New: true})
	}
	if len(fresh) > 0 {
		err := panelEditAccess(cs, actor, host, func(d *accessDoc) error {
			for id, g := range fresh {
				if err := d.setRole(id, role{group: g}); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	if move {
		for i := range out {
			n, err := PanelMoveAdminClients(cs, actor, host, out[i].Id)
			if err != nil {
				return out, err
			}
			out[i].Moved = n
		}
	}
	return out, nil
}

// adminClientIDs are the clients that belong with a limited administrator
// but are not in their group: those bound to their customers, those of their
// sales and those they created through the bot. Clients of the cluster group
// and of another administrator's group are left where they are.
func (b *bot) adminClientIDs(id int64, a *access, sellerOf map[int64]int64) ([]uint, error) {
	group := a.roleOf(id).group
	if group == "" || a.isOwner(id) {
		return nil, nil
	}
	others := map[string]bool{}
	for _, other := range a.order {
		if g := a.roleOf(other).group; g != "" && other != id {
			others[strings.ToLower(strings.TrimSpace(g))] = true
		}
	}
	want := map[uint]bool{}
	mine := map[int64]bool{}
	for customer, seller := range sellerOf {
		if seller == id {
			mine[customer] = true
		}
	}
	var sold []model.BotOrder
	if err := database.GetDB().Select("client_id").Where("admin_id = ? AND status = ? AND client_id <> 0", id, orderApproved).Find(&sold).Error; err != nil {
		return nil, err
	}
	for _, o := range sold {
		want[o.ClientId] = true
	}
	made, err := service.ClientsCreatedBy(database.GetDB(), service.TelegramActor+":"+strconv.FormatInt(id, 10))
	if err != nil {
		return nil, err
	}
	for _, cid := range made {
		want[cid] = true
	}
	var ids []uint
	for _, c := range allClients() {
		if !want[c.Id] && !(c.TgId != 0 && mine[c.TgId]) {
			continue
		}
		g := strings.ToLower(strings.TrimSpace(c.Group))
		if sameGroup(c.Group, group) || sameGroup(c.Group, service.ClusterGroup) || others[g] {
			continue
		}
		ids = append(ids, c.Id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids, nil
}

// PanelMoveAdminClients moves the clients that belong with a limited
// administrator into their group, in one save the history records, and
// returns how many moved.
func PanelMoveAdminClients(cs *service.ConfigService, actor, host string, id int64) (int, error) {
	if cs == nil {
		return 0, fmt.Errorf("the panel is not ready to save clients")
	}
	cfg, err := loadConfig()
	if err != nil {
		return 0, err
	}
	b := newBot(cfg)
	a := b.access()
	if !a.isMember(id) || a.isOwner(id) {
		return 0, errNoSuchAdmin
	}
	group := a.roleOf(id).group
	if group == "" {
		return 0, fmt.Errorf("this administrator has no group")
	}
	ids, err := b.adminClientIDs(id, a, b.customerSellers())
	if err != nil || len(ids) == 0 {
		return 0, err
	}
	var list []model.Client
	if err := database.GetDB().Where("id IN ?", ids).Find(&list).Error; err != nil {
		return 0, err
	}
	for i := range list {
		list[i].Group = group
	}
	raw, err := json.Marshal(list)
	if err != nil {
		return 0, err
	}
	if _, err := cs.Save("clients", "editbulk", raw, "", actor, host); err != nil {
		return 0, err
	}
	return len(list), nil
}
