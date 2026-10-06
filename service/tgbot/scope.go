package tgbot

import (
	"fmt"
	"strings"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/logger"

	"gorm.io/gorm"
)

// Group-limited administrators.
//
// An administrator listed in the "tgBotScopes" setting (one "ID=Group" line
// each) only sees and manages the clients of that group. Everything such an
// administrator can reach goes through three layers, so a missing check in one
// screen does not open a hole:
//
//  1. the router only lets the commands and buttons in scopedCommands and
//     scopedCallbackAllowed through;
//  2. the data accessors below (loadClients, fullClient, ...) return the
//     group's clients only, so a screen cannot show or open anyone else;
//  3. b.save, the one way the bot writes clients, refuses a change that
//     touches a client outside the group or moves one out of it.
//
// A limited administrator is a per-update copy of the bot (see as); the bot the
// supervisor and the alert watcher use has no scope.

// scopedCommands are the commands a limited administrator may use: client
// management, and nothing that reaches the server, the core, the nodes, the
// panel settings or other groups' data.
var scopedCommands = map[string]bool{
	"start": true, "menu": true, "help": true, "home": true, "status": true,
	"clients": true, "client": true, "online": true, "ips": true,
	"add": true, "addbulk": true, "enable": true, "disable": true, "reset": true,
	"del": true, "delete": true, "volume": true, "expiry": true, "limitip": true,
	"bind": true, "unbind": true, "sub": true,
}

// scopedCallbackAllowed tells whether a limited administrator may press the
// button whose callback data is parts (already split on ":").
func scopedCallbackAllowed(parts []string) bool {
	if len(parts) < 2 {
		return false
	}
	switch parts[0] {
	case "c":
		switch parts[1] {
		case "att", "grps", "sg":
			// Every client of the panel / group management.
			return false
		case "ask":
			return len(parts) > 2 && parts[2] != "grp"
		}
		return true
	case "m":
		return parts[1] == "menu" || parts[1] == "clients" || parts[1] == "online"
	case "w", "h":
		return true
	case "x":
		return parts[1] == "cancel"
	}
	return false
}

// as returns the bot as seen by one administrator: a copy that carries what
// that administrator may do (their group, their sections, whether they are the
// owner). The copy shares the HTTP client, the pending-input store and the
// live access with the original. Somebody who is not an administrator gets a
// copy with no access at all.
func (b *bot) as(id int64) *bot {
	sb := *b
	sb.scope, sb.sections, sb.owner, sb.self = "", sectionSet{}, false, id
	a := b.access()
	if a.isMember(id) {
		r := a.roleOf(id)
		sb.scope, sb.sections = r.group, r.sections
		sb.owner = a.isOwner(id)
	}
	return &sb
}

func sameGroup(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

// inScope reports whether a client group belongs to the administrator being
// served. Group names compare without regard to case, like the bot's own
// group handling.
func (b *bot) inScope(group string) bool {
	return b.scope == "" || sameGroup(group, b.scope)
}

// scopeGroup is the group a limited administrator's new clients go to: the
// spelling the group's clients already use, else the configured one. It is ""
// for an administrator without a limit.
func (b *bot) scopeGroup() string {
	if b.scope == "" {
		return ""
	}
	for _, c := range b.loadClients() {
		if g := strings.TrimSpace(c.Group); g != "" {
			return g
		}
	}
	return b.scope
}

// groupForCreate is the group a client created by this administrator gets. A
// limited administrator can only create clients in their own group, and an
// empty group means exactly that.
func (b *bot) groupForCreate(group string) (string, error) {
	if b.scope == "" {
		return group, nil
	}
	if strings.TrimSpace(group) != "" && !b.inScope(group) {
		return "", b.outOfScope()
	}
	return b.scopeGroup(), nil
}

func (b *bot) outOfScope() error {
	return fmt.Errorf(b.tr("فقط کلاینت‌های گروه «%s» در اختیار شماست.", "You can only manage clients of the “%s” group."), b.scope)
}

// ---- data accessors ----

// allClients lists every client (the columns the lists need). Use the bot's
// loadClients for anything an administrator sees.
func allClients() []model.Client {
	var clients []model.Client
	err := database.GetDB().Model(model.Client{}).
		Select("`id`, `enable`, `name`, `desc`, `group`, `up`, `down`, `volume`, `expiry`, `created_at`, `online_at`, `limit_ip`, `tg_id`, `delay_start`, `reset_days`").
		Scan(&clients).Error
	if err != nil {
		logger.Warning("telegram bot: load clients: ", err)
		return nil
	}
	return clients
}

func rawFullClient(id uint) (*model.Client, error) {
	var c model.Client
	if err := database.GetDB().Where("id = ?", id).First(&c).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

// rawFindClientByName looks a name up among every client, whatever group it is
// in: names are unique across the panel, so a name check must not be narrowed
// to an administrator's group.
func rawFindClientByName(name string) *model.Client {
	for _, c := range allClients() {
		if strings.EqualFold(c.Name, name) {
			c := c
			return &c
		}
	}
	return nil
}

// loadClients lists the clients this administrator may see.
func (b *bot) loadClients() []model.Client {
	all := allClients()
	if b.scope == "" {
		return all
	}
	out := make([]model.Client, 0, len(all))
	for _, c := range all {
		if b.inScope(c.Group) {
			out = append(out, c)
		}
	}
	return out
}

// fullClient loads one whole client row; a client outside the administrator's
// group does not exist as far as they are concerned.
func (b *bot) fullClient(id uint) (*model.Client, error) {
	c, err := rawFullClient(id)
	if err != nil {
		return nil, err
	}
	if !b.inScope(c.Group) {
		return nil, gorm.ErrRecordNotFound
	}
	return c, nil
}

func (b *bot) clientByID(id uint) *model.Client {
	c, err := b.fullClient(id)
	if err != nil {
		return nil
	}
	return c
}

func (b *bot) findClientByName(name string) *model.Client {
	for _, c := range b.loadClients() {
		if strings.EqualFold(c.Name, name) {
			c := c
			return &c
		}
	}
	return nil
}

func (b *bot) mustFullInbounds(id uint) []byte {
	c, err := b.fullClient(id)
	if err != nil || len(c.Inbounds) == 0 {
		return []byte("[]")
	}
	return c.Inbounds
}

// onlineUsers lists the online clients this administrator may see.
func (b *bot) onlineUsers() []string {
	users := allOnlineUsers()
	if b.scope == "" {
		return users
	}
	mine := map[string]bool{}
	for _, c := range b.loadClients() {
		mine[c.Name] = true
	}
	out := make([]string, 0, len(users))
	for _, u := range users {
		if mine[u] {
			out = append(out, u)
		}
	}
	return out
}

// ---- write guard ----

// storedInScope checks the clients as they are stored now: all of them exist
// and belong to the administrator's group.
func (b *bot) storedInScope(ids ...uint) bool {
	if len(ids) == 0 {
		return false
	}
	seen := map[uint]bool{}
	unique := make([]uint, 0, len(ids))
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			unique = append(unique, id)
		}
	}
	var rows []struct {
		Id    uint
		Group string
	}
	if err := database.GetDB().Model(&model.Client{}).Select("`id`, `group`").Where("id IN ?", unique).Scan(&rows).Error; err != nil || len(rows) != len(unique) {
		return false
	}
	for _, r := range rows {
		if !b.inScope(r.Group) {
			return false
		}
	}
	return true
}

// guardSave refuses a client write that reaches outside a limited
// administrator's group: creating, changing or deleting a client of another
// group, or moving a client out of the group. Everything the bot writes passes
// through here (see save).
func (b *bot) guardSave(act string, payload interface{}) error {
	if b.scope == "" {
		return nil
	}
	denied := b.outOfScope()
	switch act {
	case "new":
		c, ok := payload.(model.Client)
		if !ok || c.Id != 0 || !b.inScope(c.Group) {
			return denied
		}
	case "addbulk":
		list, ok := payload.([]model.Client)
		if !ok {
			return denied
		}
		for _, c := range list {
			if c.Id != 0 || !b.inScope(c.Group) {
				return denied
			}
		}
	case "edit":
		c, ok := payload.(*model.Client)
		if !ok || c == nil || c.Id == 0 || !b.inScope(c.Group) || !b.storedInScope(c.Id) {
			return denied
		}
	case "editbulk":
		list, ok := payload.([]model.Client)
		if !ok {
			return denied
		}
		ids := make([]uint, 0, len(list))
		for _, c := range list {
			if c.Id == 0 || !b.inScope(c.Group) {
				return denied
			}
			ids = append(ids, c.Id)
		}
		if !b.storedInScope(ids...) {
			return denied
		}
	case "del":
		id, ok := payload.(uint)
		if !ok || !b.storedInScope(id) {
			return denied
		}
	case "delbulk":
		ids, ok := payload.([]uint)
		if !ok || !b.storedInScope(ids...) {
			return denied
		}
	default:
		// "attachall" touches every client of the panel.
		return denied
	}
	return nil
}

// ---- screens ----

// scopedMenu is the button menu of a limited administrator.
func (b *bot) scopedMenu() [][]button {
	return [][]button{
		{{Text: b.tr("🏠 خانه", "🏠 Home"), Data: "h:home"}, {Text: b.tr("👥 کلاینت‌ها", "👥 Clients"), Data: "c:ls:a:0"}},
		{{Text: b.tr("➕ کلاینت جدید", "➕ New client"), Data: "c:new"}, {Text: b.tr("🔵 آنلاین‌ها", "🔵 Online"), Data: "m:online"}},
	}
}

// scopedHomeText is the Home page of a limited administrator: their group's
// numbers, nothing about the server.
func (b *bot) scopedHomeText() string {
	clients := b.loadClients()
	enabled := 0
	var up, down int64
	for _, c := range clients {
		if c.Enable {
			enabled++
		}
		up += c.Up
		down += c.Down
	}
	lines := []string{
		b.header("🏠", "DrNetwork"),
		"🏷 " + b.t("group") + ": <b>" + esc(b.scopeGroup()) + "</b>",
		fmt.Sprintf("👥 %s: %d (%s: %d) · 🟢 %s: %d", b.tr("کلاینت", "Clients"), len(clients), b.tr("فعال", "active"), enabled, b.tr("آنلاین", "online"), len(b.onlineUsers())),
		fmt.Sprintf("📈 %s: ↑ %s ↓ %s", b.tr("مصرف کل", "Total"), humanBytes(up), humanBytes(down)),
	}
	if line := b.quotaLine(); line != "" {
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func (b *bot) scopedHomeKeyboard() [][]button {
	return [][]button{
		{{Text: b.tr("🔄 به‌روزرسانی", "🔄 Refresh"), Data: "h:home"}, {Text: b.tr("🏠 منو", "🏠 Menu"), Data: "m:menu"}},
		{{Text: b.tr("👥 کلاینت‌ها", "👥 Clients"), Data: "c:ls:a:0"}, {Text: b.tr("➕ کلاینت جدید", "➕ New client"), Data: "c:new"}},
	}
}
