package tgbot

import (
	"context"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/logger"
)

// Who may do what in the bot.
//
// The owner (setting tgBotOwner) can do everything, and is the only one who
// can edit the other administrators, from the Admins screen. Everybody else
// has one of three roles:
//
//   - full access: every section of the bot, except editing administrators;
//   - group-limited: the clients of one group only (see scope.go);
//   - custom: only the sections the owner switched on.
//
// The roles live in three settings that the web panel and the owner's Admins
// screen both edit: tgBotAdmins (who), tgBotScopes (group limits) and
// tgBotPerms (custom sections). A line that cannot be understood never widens
// anyone's access: it leaves that ID with no access at all.
//
// An administrator's rights are checked in three places, so a screen that
// forgets one does not open a hole: the router (allowed, allowedCommand,
// pendingAllowed), the keyboards the bot sends (filterKeyboard hides buttons
// that would be refused), and, for group limits, the data accessors in scope.go.

// sectionDef is one part of the bot an administrator can be given or denied.
type sectionDef struct{ key, icon, fa, en string }

var sectionDefs = []sectionDef{
	{"home", "🏠", "خانه", "Home"},
	{"clients", "👥", "کلاینت‌ها", "Clients"},
	{"inbounds", "📡", "اینباندها", "Inbounds"},
	{"outbounds", "📤", "اوت‌باندها", "Outbounds"},
	{"endpoints", "🔌", "اندپوینت‌ها", "Endpoints"},
	{"services", "🛠", "سرویس‌ها", "Services"},
	{"tls", "🔐", "TLS", "TLS"},
	{"nodes", "🖥", "نودها", "Nodes"},
	{"routing", "📏", "قوانین/DNS", "Routing"},
	{"settings", "🔧", "تنظیمات", "Settings"},
	{"stats", "📊", "آمار", "Stats"},
	{"logs", "📜", "لاگ‌ها", "Logs"},
	{"core", "♻️", "هسته", "Core"},
	{"backup", "💾", "پشتیبان", "Backup"},
	{"admins", "👮", "مدیران", "Admins"},
}

func sectionKnown(key string) bool {
	for _, d := range sectionDefs {
		if d.key == key {
			return true
		}
	}
	return false
}

// sectionSet is a set of section keys.
type sectionSet map[string]bool

func allSections() sectionSet {
	s := sectionSet{}
	for _, d := range sectionDefs {
		s[d.key] = true
	}
	return s
}

func (s sectionSet) clone() sectionSet {
	out := sectionSet{}
	for k, v := range s {
		if v {
			out[k] = true
		}
	}
	return out
}

// keys lists the sections in the order the bot shows them.
func (s sectionSet) keys() []string {
	var out []string
	for _, d := range sectionDefs {
		if s[d.key] {
			out = append(out, d.key)
		}
	}
	return out
}

// parsePerms reads the "admin ID = section, section" lines of the sections
// setting. Unknown section names are ignored, so a typo can only narrow
// someone's access. As with parseScopes, a line that starts with an ID but is
// not a usable line (no "=") puts that ID in bad, unless a good line for it
// exists: such an ID gets no access until the line is fixed. An empty list
// ("123=") is a good line that grants nothing. The first line of an ID wins.
func parsePerms(raw string) (perms map[int64]sectionSet, bad []int64) {
	perms = map[int64]sectionSet{}
	broken := map[int64]bool{}
	for _, line := range strings.Split(raw, "\n") {
		head, list, hasEquals := strings.Cut(line, "=")
		fields := strings.Fields(head)
		if len(fields) == 0 {
			continue
		}
		n, err := strconv.ParseInt(fields[0], 10, 64)
		if err != nil || n == 0 {
			continue
		}
		if !hasEquals || len(fields) > 1 {
			broken[n] = true
			continue
		}
		if _, dup := perms[n]; dup {
			continue
		}
		set := sectionSet{}
		for _, k := range strings.FieldsFunc(list, func(r rune) bool {
			return r == ',' || r == ';' || r == ' ' || r == '\t' || r == '\r'
		}) {
			if k = strings.ToLower(k); sectionKnown(k) {
				set[k] = true
			}
		}
		perms[n] = set
	}
	for n := range broken {
		if _, ok := perms[n]; !ok {
			bad = append(bad, n)
		}
	}
	sort.Slice(bad, func(i, j int) bool { return bad[i] < bad[j] })
	return perms, bad
}

// role is what one administrator may do.
type role struct {
	// group, when set, limits the administrator to the clients of that group.
	group string
	// sections, when not nil, limits the administrator to those sections.
	sections sectionSet
}

func (r role) full() bool { return r.group == "" && r.sections == nil }

// access is everybody's role. It is replaced as a whole and never modified, so
// the handlers and the alert watcher read it without locks.
type access struct {
	owner int64
	// order lists the owner, then every other administrator in display order.
	order []int64
	roles map[int64]role
}

func (a *access) isMember(id int64) bool {
	_, ok := a.roles[id]
	return ok
}

func (a *access) roleOf(id int64) role { return a.roles[id] }

// allows tells whether an administrator without a group limit may use a
// section. Group-limited administrators only have their clients.
func (a *access) allows(id int64, section string) bool {
	r, ok := a.roles[id]
	return ok && r.group == "" && (r.sections == nil || r.sections[section])
}

// with lists the administrators who may use at least one of the sections, in
// display order. It is how the alerts find their readers.
func (a *access) with(sections ...string) []int64 {
	var out []int64
	for _, id := range a.order {
		for _, sec := range sections {
			if a.allows(id, sec) {
				out = append(out, id)
				break
			}
		}
	}
	return out
}

// isOwner tells whether id is the owner.
func (a *access) isOwner(id int64) bool { return a.owner != 0 && id == a.owner }

func sortedIDs[V any](m map[int64]V) []int64 {
	ids := make([]int64, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

// buildAccess works out everybody's role from the settings. An ID that
// appears in any of the lists is an administrator, so a forgotten entry in the
// admin list cannot leave a limit off. The owner is never limited. A locked ID
// (its limit line was unusable) is an administrator with no access, never one
// with full access.
func buildAccess(owner int64, admins []int64, scopes map[int64]string, perms map[int64]sectionSet, locked []int64) *access {
	a := &access{owner: owner, roles: map[int64]role{}}
	add := func(id int64) {
		if id == 0 {
			return
		}
		if _, dup := a.roles[id]; dup {
			return
		}
		var r role
		if id != owner {
			if group, ok := scopes[id]; ok {
				r.group = group
			} else if set, ok := perms[id]; ok {
				r.sections = set
			}
			if hasID(locked, id) {
				r = role{sections: sectionSet{}}
			}
		}
		a.roles[id] = r
		a.order = append(a.order, id)
	}
	add(owner)
	for _, id := range admins {
		add(id)
	}
	for _, id := range sortedIDs(scopes) {
		add(id)
	}
	for _, id := range sortedIDs(perms) {
		add(id)
	}
	for _, id := range locked {
		add(id)
	}
	return a
}

// access builds the roles the configuration describes.
func (c botConfig) access() *access {
	return buildAccess(c.Owner, c.Admins, c.Scopes, c.Perms, c.Locked)
}

// accessState is the live access shared by the bot and every per-update copy
// of it. A change of the administrators' settings applies to the running bot
// by replacing the snapshot, without restarting it.
type accessState struct {
	// mu serializes the edits and reloads of the access settings.
	mu  sync.Mutex
	cur atomic.Pointer[access]

	// menus are the command lists the bot has set per chat (as JSON), so an
	// unchanged menu is not sent again.
	cmdMu sync.Mutex
	menus map[int64]string

	nameMu sync.Mutex
	names  map[int64]nameEntry
}

type nameEntry struct {
	name string
	at   time.Time
}

func newAccessState(a *access) *accessState {
	st := &accessState{menus: map[int64]string{}, names: map[int64]nameEntry{}}
	st.cur.Store(a)
	return st
}

// access returns the current roles.
func (b *bot) access() *access {
	if b.acc != nil {
		if a := b.acc.cur.Load(); a != nil {
			return a
		}
	}
	return b.cfg.access()
}

// setAccess replaces the roles of the running bot.
func (b *bot) setAccess(a *access) {
	if b.acc == nil {
		b.acc = newAccessState(a)
		return
	}
	b.acc.cur.Store(a)
}

// isAdmin tells whether an ID may use the bot as an administrator.
func (b *bot) isAdmin(id int64) bool { return b.access().isMember(id) }

// reloadAccess re-reads the administrators' settings into the running bot.
func (b *bot) reloadAccess() error {
	if b.acc == nil {
		return nil
	}
	b.acc.mu.Lock()
	defer b.acc.mu.Unlock()
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	b.setAccess(cfg.access())
	return nil
}

// ---- what a role may use ----

// Marks callbackSection can return instead of a section.
const (
	secNone  = ""         // anybody
	secOwner = "#owner"   // the owner only
	secDeny  = "#unknown" // not a button of a section: administrators with full access only
)

// kindSection maps the object kinds of the bot (kindList) to their sections.
var kindSection = map[string]string{
	"in": "inbounds", "out": "outbounds", "ep": "endpoints", "sv": "services", "tl": "tls", "nd": "nodes",
}

// callbackSection names the section the button whose callback data is parts
// (already split on ":") belongs to.
func callbackSection(parts []string) string {
	if len(parts) < 2 {
		return secDeny
	}
	switch parts[0] {
	case "c", "w":
		return "clients"
	case "m":
		switch parts[1] {
		case "menu":
			return secNone
		case "status":
			return "home"
		case "clients", "online":
			return "clients"
		case "nodes", "sync":
			return "nodes"
		case "inbounds":
			return "inbounds"
		case "traffic":
			return "stats"
		case "logs":
			return "logs"
		case "backup":
			return "backup"
		case "restart", "restarty", "maint":
			return "core"
		case "prest", "prestY":
			return "settings"
		}
	case "o":
		if sec, ok := kindSection[parts[1]]; ok {
			return sec
		}
	case "g":
		return "routing"
	case "s":
		return "settings"
	case "h":
		return "home"
	case "t":
		return "stats"
	case "x":
		if parts[1] == "cancel" {
			return secNone
		}
		return "stats"
	case "a":
		if parts[1] == "ls" {
			return "admins"
		}
		return secOwner
	}
	return secDeny
}

// commandSections are the commands that belong to a section; the others
// (start, menu, help, id, unknown ones) are open to every administrator.
var commandSections = map[string]string{
	"home": "home", "status": "home",
	"stats": "stats", "traffic": "stats", "changes": "stats",
	"settings": "settings",
	"clients":  "clients", "client": "clients", "online": "clients", "ips": "clients",
	"add": "clients", "addbulk": "clients", "enable": "clients", "disable": "clients",
	"reset": "clients", "volume": "clients", "expiry": "clients", "limitip": "clients",
	"del": "clients", "delete": "clients", "sub": "clients", "bind": "clients", "unbind": "clients",
	"nodes": "nodes", "sync": "nodes",
	"inbounds": "inbounds",
	"backup":   "backup",
	"logs":     "logs",
	"restart":  "core", "maintenance": "core",
}

// pendingSection names the section a typed answer the bot waits for belongs to.
func pendingSection(p *pending) string {
	switch {
	case strings.HasPrefix(p.kind, "ad."):
		return secOwner
	case strings.HasPrefix(p.kind, "cl."), strings.HasPrefix(p.kind, "bk."), p.kind == "wiz":
		return "clients"
	case p.kind == "in.port":
		return "inbounds"
	case p.kind == "obj.edit", p.kind == "obj.new":
		if sec, ok := kindSection[p.key]; ok {
			return sec
		}
	case strings.HasPrefix(p.kind, "cfg."):
		return "routing"
	case p.kind == "set":
		return "settings"
	}
	return secDeny
}

// restricted tells whether the administrator being served has less than full
// access.
func (b *bot) restricted() bool { return b.scope != "" || b.sections != nil }

// can reports whether the administrator being served may use a section. A
// group-limited administrator only has the clients, and only of their group.
func (b *bot) can(section string) bool {
	if b.scope != "" {
		return section == "clients"
	}
	return b.sections == nil || b.sections[section]
}

func (b *bot) allowedBy(sec string) bool {
	switch sec {
	case secNone:
		return true
	case secOwner:
		return b.owner
	case secDeny:
		return !b.restricted()
	}
	return b.can(sec)
}

// allowed tells whether the administrator being served may press the button
// whose callback data is parts.
func (b *bot) allowed(parts []string) bool {
	if b.scope != "" {
		return scopedCallbackAllowed(parts)
	}
	return b.allowedBy(callbackSection(parts))
}

func (b *bot) allowedCommand(cmd string) bool {
	if b.scope != "" {
		return scopedCommands[cmd]
	}
	sec, gated := commandSections[cmd]
	return !gated || b.can(sec)
}

// pendingAllowed tells whether the administrator being served may still answer
// the prompt the bot is waiting on. Rights can change between the question and
// the answer.
func (b *bot) pendingAllowed(p *pending) bool {
	if b.scope != "" {
		return scopedPendingKind(p.kind)
	}
	return b.allowedBy(pendingSection(p))
}

// denied is what an administrator is told on a refused request.
func (b *bot) denied() string {
	if b.scope != "" {
		return b.t("scopeDenied")
	}
	return b.t("noAccess")
}

// filterKeyboard drops the buttons the administrator being served may not use,
// and the rows left empty by that.
func (b *bot) filterKeyboard(kb [][]button) [][]button {
	if kb == nil || !b.restricted() {
		return kb
	}
	out := make([][]button, 0, len(kb))
	for _, row := range kb {
		var keep []button
		for _, btn := range row {
			if !strings.Contains(btn.Data, ":") || b.allowed(strings.Split(btn.Data, ":")) {
				keep = append(keep, btn)
			}
		}
		if len(keep) > 0 {
			out = append(out, keep)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ---- command menus ----

// commandsFor is the command list Telegram shows an administrator.
func (b *bot) commandsFor(id int64) []commandInfo {
	r := b.access().roleOf(id)
	var out []commandInfo
	for _, c := range adminCommands {
		switch {
		case r.group != "":
			if !scopedCommands[c.name] {
				continue
			}
		case r.sections != nil:
			if sec, gated := commandSections[c.name]; gated && !r.sections[sec] {
				continue
			}
		}
		out = append(out, c)
	}
	return append(out, userCommands[2])
}

func (b *bot) commandPayload(list []commandInfo) []map[string]string {
	out := make([]map[string]string, 0, len(list))
	for _, c := range list {
		desc := c.en
		if b.cfg.Lang == "fa" {
			desc = c.fa
		}
		out = append(out, map[string]string{"command": c.name, "description": desc})
	}
	return out
}

// registerCommands sets the command lists at start-up: the one every user
// sees, and each administrator's own.
func (b *bot) registerCommands(ctx context.Context) {
	if err := b.call(ctx, "setMyCommands", map[string]any{"commands": b.commandPayload(userCommands)}, nil); err != nil && ctx.Err() == nil {
		logger.Warning("telegram bot: set commands: ", err)
	}
	b.syncCommandMenus(ctx)
}

// syncCommandMenus makes every administrator's command list match their
// rights, and takes the list away from whoever stopped being an
// administrator. Lists that did not change are not sent again.
func (b *bot) syncCommandMenus(ctx context.Context) {
	if b.acc == nil {
		return
	}
	a := b.access()
	b.acc.cmdMu.Lock()
	defer b.acc.cmdMu.Unlock()
	for _, id := range a.order {
		if ctx.Err() != nil {
			return
		}
		raw, err := json.Marshal(b.commandPayload(b.commandsFor(id)))
		if err != nil || b.acc.menus[id] == string(raw) {
			continue
		}
		if err := b.call(ctx, "setMyCommands", map[string]any{
			"commands": json.RawMessage(raw),
			"scope":    map[string]any{"type": "chat", "chat_id": id},
		}, nil); err == nil {
			b.acc.menus[id] = string(raw)
		}
	}
	for id := range b.acc.menus {
		if a.isMember(id) || ctx.Err() != nil {
			continue
		}
		if err := b.call(ctx, "deleteMyCommands", map[string]any{
			"scope": map[string]any{"type": "chat", "chat_id": id},
		}, nil); err == nil {
			delete(b.acc.menus, id)
		}
	}
}

// sectionLabels lists the names of some sections, in display order.
func (b *bot) sectionLabels(set sectionSet) string {
	var out []string
	for _, d := range sectionDefs {
		if set[d.key] {
			out = append(out, d.icon+" "+b.tr(d.fa, d.en))
		}
	}
	return strings.Join(out, " · ")
}

// menuScreen is the main menu with only the buttons the administrator may use.
func (b *bot) menuScreen() (string, [][]button) {
	kb := b.filterKeyboard(b.mainMenu())
	if kb == nil {
		return b.t("noAccessYet"), nil
	}
	return b.t("menuTitle"), kb
}

// restrictedHelp is /help for an administrator with custom sections: what they
// have, and the commands that go with it.
func (b *bot) restrictedHelp() string {
	lines := []string{"🤖 <b>" + b.tr("ربات مدیریت DrNetwork", "DrNetwork management bot") + "</b>"}
	if len(b.sections) == 0 {
		return strings.Join(append(lines, "", b.t("noAccessYet")), "\n")
	}
	lines = append(lines, "🎛 "+b.sectionLabels(b.sections), "")
	for _, c := range b.commandsFor(b.self) {
		desc := c.en
		if b.cfg.Lang == "fa" {
			desc = c.fa
		}
		lines = append(lines, "/"+c.name+" — "+desc)
	}
	return strings.Join(lines, "\n")
}
