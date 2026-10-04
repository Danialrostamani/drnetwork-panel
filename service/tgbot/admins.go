package tgbot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/service"
)

// The owner's Admins screen: add and remove administrators and decide what
// each may do. Everybody else only sees the list.
//
// The screen edits the same three settings the web panel does (tgBotAdmins,
// tgBotScopes, tgBotPerms) through the panel's own settings save, so the change
// is recorded in the history like any other, the web panel shows it, and it is
// applied to the running bot at once.

var (
	errBadID        = errors.New("bad telegram id")
	errAdminExists  = errors.New("already an administrator")
	errIsOwner      = errors.New("the owner cannot be edited")
	errNoSuchAdmin  = errors.New("no such administrator")
	errBadGroup     = errors.New("bad group name")
	errGroupLimited = errors.New("group-limited administrator")
	errNotOwner     = errors.New("only the owner can edit administrators")
)

// accessErrText explains the errors of this file; it is "" for any other.
func (b *bot) accessErrText(err error) string {
	switch {
	case errors.Is(err, errBadID):
		return b.tr("شناسه تلگرام یک عدد مثبت است؛ همان عددی که /id نشان می‌دهد.", "A Telegram ID is a positive number, the one /id shows.")
	case errors.Is(err, errAdminExists):
		return b.tr("این شناسه از قبل ادمین است.", "That ID is already an admin.")
	case errors.Is(err, errIsOwner):
		return b.tr("مالک ربات قابل ویرایش نیست.", "The bot owner cannot be edited.")
	case errors.Is(err, errNoSuchAdmin):
		return b.tr("این ادمین پیدا نشد.", "That admin was not found.")
	case errors.Is(err, errBadGroup):
		return b.tr("نام گروه معتبر نیست.", "That group name is not valid.")
	case errors.Is(err, errGroupLimited):
		return b.tr("این ادمین به یک گروه محدود است؛ از پیش‌تنظیم‌ها استفاده کنید.", "This admin is limited to a group; use the presets.")
	case errors.Is(err, errNotOwner):
		return b.tr("فقط مالک ربات می‌تواند مدیران را ویرایش کند.", "Only the bot owner can edit admins.")
	}
	return ""
}

// ---- the access settings, in an editable form ----

// accessDoc is the access settings in a form the owner's screens edit. It is
// read from the settings, changed, and written back by editAccess.
type accessDoc struct {
	owner  int64
	admins []int64
	scopes map[int64]string
	perms  map[int64]sectionSet
}

// docFromConfig takes the roles the configuration gives out. Going through
// access means the document holds what the bot enforces: an ID whose line is
// unusable is "no access" in it, so writing the document back cannot widen
// anybody's rights.
func docFromConfig(c botConfig) *accessDoc {
	a := c.access()
	d := &accessDoc{owner: c.Owner, admins: append([]int64(nil), c.Admins...), scopes: map[int64]string{}, perms: map[int64]sectionSet{}}
	for _, id := range a.order {
		if a.isOwner(id) {
			continue
		}
		switch r := a.roles[id]; {
		case r.group != "":
			d.scopes[id] = r.group
		case r.sections != nil:
			d.perms[id] = r.sections.clone()
		}
	}
	return d
}

func (d *accessDoc) build() *access {
	return buildAccess(d.owner, d.admins, d.scopes, d.perms, nil)
}

func (d *accessDoc) isOwner(id int64) bool { return d.owner != 0 && id == d.owner }

func (d *accessDoc) member(id int64) bool { return d.build().isMember(id) }

func (d *accessDoc) roleOf(id int64) role { return d.build().roleOf(id) }

// add makes id an administrator with no access; the owner then grants some.
func (d *accessDoc) add(id int64) error {
	switch {
	case id <= 0:
		return errBadID
	case d.isOwner(id):
		return errIsOwner
	case d.member(id):
		return errAdminExists
	}
	d.admins = append(d.admins, id)
	d.perms[id] = sectionSet{}
	return nil
}

// remove takes all access from id.
func (d *accessDoc) remove(id int64) error {
	switch {
	case d.isOwner(id):
		return errIsOwner
	case !d.member(id):
		return errNoSuchAdmin
	}
	kept := make([]int64, 0, len(d.admins))
	for _, a := range d.admins {
		if a != id {
			kept = append(kept, a)
		}
	}
	d.admins = kept
	delete(d.scopes, id)
	delete(d.perms, id)
	return nil
}

// setRole gives an existing administrator a role: full access, a group, or a
// set of sections.
func (d *accessDoc) setRole(id int64, r role) error {
	switch {
	case d.isOwner(id):
		return errIsOwner
	case !d.member(id):
		return errNoSuchAdmin
	case r.group != "" && !scopeGroupUsable(r.group):
		return errBadGroup
	}
	delete(d.scopes, id)
	delete(d.perms, id)
	switch {
	case r.group != "":
		d.scopes[id] = r.group
	case r.sections != nil:
		d.perms[id] = r.sections.clone()
	case !hasID(d.admins, id):
		// A full administrator lives in the admin list.
		d.admins = append(d.admins, id)
	}
	return nil
}

// settings is the text of the three settings the document stands for.
func (d *accessDoc) settings() (map[string]string, error) {
	ids := make([]string, 0, len(d.admins))
	for _, id := range d.admins {
		ids = append(ids, strconv.FormatInt(id, 10))
	}
	var scopes, perms []string
	for _, id := range sortedIDs(d.scopes) {
		if !scopeGroupUsable(d.scopes[id]) {
			return nil, errBadGroup
		}
		scopes = append(scopes, fmt.Sprintf("%d=%s", id, d.scopes[id]))
	}
	for _, id := range sortedIDs(d.perms) {
		perms = append(perms, fmt.Sprintf("%d=%s", id, strings.Join(d.perms[id].keys(), ",")))
	}
	return map[string]string{
		"tgBotAdmins": strings.Join(ids, ", "),
		"tgBotScopes": strings.Join(scopes, "\n"),
		"tgBotPerms":  strings.Join(perms, "\n"),
	}, nil
}

// editAccess changes who may do what: it applies mutate to the access settings
// as they are now, saves them, and makes the running bot use the result at
// once. Only the owner's screens call it, and it checks that again against the
// settings it has just read.
func (b *bot) editAccess(mutate func(d *accessDoc) error) error {
	if b.acc == nil || b.configService == nil {
		return errors.New("the panel is not ready to save settings")
	}
	b.acc.mu.Lock()
	defer b.acc.mu.Unlock()
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	if cfg.Owner == 0 || cfg.Owner != b.self || !b.owner {
		return errNotOwner
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
	if _, err := b.configService.Save("settings", "", raw, "", "telegram", b.host()); err != nil {
		return err
	}
	fresh, err := loadConfig()
	if err != nil {
		return err
	}
	b.setAccess(fresh.access())
	return nil
}

// ---- names ----

const (
	nameTTL     = 10 * time.Minute
	nameMissTTL = time.Minute
)

// adminNames looks the administrators' names up through Telegram, which only
// knows people who have started the bot. Names are cached; an ID Telegram does
// not answer for is shown by its number alone.
func (b *bot) adminNames(ctx context.Context, ids []int64) map[int64]string {
	out := map[int64]string{}
	if b.acc == nil {
		return out
	}
	var missing []int64
	b.acc.nameMu.Lock()
	for _, id := range ids {
		if e, ok := b.acc.names[id]; ok && time.Since(e.at) < map[bool]time.Duration{true: nameTTL, false: nameMissTTL}[e.name != ""] {
			out[id] = e.name
		} else {
			missing = append(missing, id)
		}
	}
	b.acc.nameMu.Unlock()
	if len(missing) == 0 {
		return out
	}
	lookup, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	var (
		wg sync.WaitGroup
		mu sync.Mutex
	)
	for _, id := range missing {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			name := b.fetchName(lookup, id)
			mu.Lock()
			out[id] = name
			mu.Unlock()
			if lookup.Err() == nil {
				b.acc.nameMu.Lock()
				b.acc.names[id] = nameEntry{name: name, at: time.Now()}
				b.acc.nameMu.Unlock()
			}
		}(id)
	}
	wg.Wait()
	return out
}

func (b *bot) fetchName(ctx context.Context, id int64) string {
	var c struct {
		First    string `json:"first_name"`
		Last     string `json:"last_name"`
		Username string `json:"username"`
		Title    string `json:"title"`
	}
	if err := b.call(ctx, "getChat", map[string]any{"chat_id": id}, &c); err != nil {
		return ""
	}
	name := strings.TrimSpace(c.First + " " + c.Last)
	if name == "" {
		name = strings.TrimSpace(c.Title)
	}
	if name == "" && c.Username != "" {
		name = "@" + c.Username
	}
	return truncate(strings.Join(strings.Fields(name), " "), 40)
}

// ---- screens ----

// adminLine describes one administrator in a list.
func (b *bot) adminLine(a *access, id int64, name string) string {
	r := a.roleOf(id)
	icon, label := "🟢", b.tr("دسترسی کامل", "full access")
	switch {
	case a.isOwner(id):
		icon, label = "👑", b.tr("مالک", "owner")
	case r.group != "":
		icon, label = "🏷", b.tr("گروه: ", "group: ")+"<b>"+esc(r.group)+"</b>"
	case r.sections != nil && len(r.sections) == 0:
		icon, label = "⛔", b.tr("بدون دسترسی", "no access")
	case r.sections != nil:
		var icons []string
		for _, d := range sectionDefs {
			if r.sections[d.key] {
				icons = append(icons, d.icon)
			}
		}
		icon, label = "🎛", strings.Join(icons, " ")
	}
	who := "<code>" + strconv.FormatInt(id, 10) + "</code>"
	if name != "" {
		who = "<b>" + esc(name) + "</b> " + who
	}
	return icon + " " + who + " — " + label
}

// adminsScreen lists the panel's own users and the bot's administrators. The
// owner also gets the buttons to edit them.
func (b *bot) adminsScreen(ctx context.Context) (string, [][]button) {
	users, err := (&service.UserService{}).GetUsers()
	if err != nil {
		return b.t("failed", esc(err.Error())), [][]button{b.menuRow()}
	}
	lines := []string{b.header("👮", b.tr("مدیران پنل", "Panel admins"))}
	for _, u := range *users {
		line := "👤 <b>" + esc(u.Username) + "</b>"
		if u.LastLogins != "" {
			line += " — " + esc(truncate(u.LastLogins, 60))
		}
		lines = append(lines, line)
	}
	lines = append(lines, "", b.tr("🔒 نام کاربری و رمز عبور فقط از پنل وب تغییر می‌کند (برای امنیت، رمز از طریق تلگرام فرستاده نمی‌شود).", "🔒 Credentials are changed in the web panel only (passwords are never sent through Telegram)."))

	a := b.access()
	names := b.adminNames(ctx, a.order)
	lines = append(lines, "", "🤖 <b>"+b.tr("ادمین‌های ربات", "Bot admins")+"</b>")
	for _, id := range a.order {
		lines = append(lines, b.adminLine(a, id, names[id]))
	}
	switch {
	case a.owner == 0:
		lines = append(lines, "", b.tr("ℹ️ برای ویرایش ادمین‌ها از داخل ربات، شناسه تلگرام مالک را در پنل وب وارد کنید (تنظیمات ← ربات تلگرام ← مالک ربات).", "ℹ️ To edit admins from this bot, set the owner's Telegram ID in the web panel (Settings → Telegram bot → Bot owner)."))
	case !b.owner:
		lines = append(lines, "", b.tr("ℹ️ فقط مالک ربات می‌تواند ادمین‌ها را ویرایش کند.", "ℹ️ Only the bot owner can edit admins."))
	}

	var kb [][]button
	if b.owner {
		const maxButtons = 40
		var btns []button
		for _, id := range a.order {
			if a.isOwner(id) {
				continue
			}
			if len(btns) == maxButtons {
				lines = append(lines, b.t("andMore", len(a.order)-1-maxButtons))
				break
			}
			label := names[id]
			if label == "" {
				label = strconv.FormatInt(id, 10)
			}
			btns = append(btns, button{Text: "✏️ " + truncate(label, 24), Data: "a:e:" + strconv.FormatInt(id, 10)})
		}
		kb = append(rows2(btns), []button{{Text: b.tr("➕ ادمین جدید", "➕ Add admin"), Data: "a:add"}})
	}
	return strings.Join(lines, "\n"), append(kb, b.menuRow())
}

// adminEditScreen is where the owner decides what one administrator may do.
func (b *bot) adminEditScreen(ctx context.Context, id int64) (string, [][]button) {
	a := b.access()
	if !a.isMember(id) || a.isOwner(id) {
		return b.adminsScreen(ctx)
	}
	sid := strconv.FormatInt(id, 10)
	r := a.roleOf(id)
	lines := []string{
		b.header("✏️", b.tr("دسترسی ادمین", "Admin access")),
		b.adminLine(a, id, b.adminNames(ctx, []int64{id})[id]),
		"",
	}
	var kb [][]button
	switch {
	case r.group != "":
		lines = append(lines, b.tr("فقط کلاینت‌های این گروه را می‌بیند و مدیریت می‌کند؛ هیچ بخش دیگری را نه. برای بخش‌های دیگر یکی از پیش‌تنظیم‌ها را بزنید.", "They see and manage the clients of this group only, and no other section. Use a preset below for anything else."))
	default:
		set := r.sections
		if set == nil {
			set = allSections()
			lines = append(lines, b.tr("دسترسی کامل به همه بخش‌ها. با زدن هر بخش آن را برایش خاموش می‌کنید.", "Full access to every section. Tap a section to switch it off for them."))
		} else {
			lines = append(lines, b.tr("بخش‌های روشن را می‌تواند استفاده کند. روی هر بخش بزنید تا روشن یا خاموش شود.", "They can use the sections that are on. Tap a section to switch it on or off."))
		}
		var toggles []button
		for _, d := range sectionDefs {
			toggles = append(toggles, button{Text: onOff(set[d.key]) + " " + d.icon + " " + b.tr(d.fa, d.en), Data: "a:t:" + sid + ":" + d.key})
		}
		kb = rows2(toggles)
	}
	groupLabel := b.tr("🏷 محدود به یک گروه", "🏷 Limit to a group")
	if r.group != "" {
		groupLabel = b.tr("🏷 تغییر گروه", "🏷 Change group")
	}
	kb = append(kb,
		[]button{
			{Text: b.tr("🟢 کامل", "🟢 Full"), Data: "a:p:" + sid + ":full"},
			{Text: b.tr("👥 فقط کلاینت‌ها", "👥 Clients only"), Data: "a:p:" + sid + ":cl"},
			{Text: b.tr("⛔ هیچ", "⛔ None"), Data: "a:p:" + sid + ":none"},
		},
		[]button{{Text: groupLabel, Data: "a:g:" + sid}},
		[]button{{Text: b.tr("🗑 حذف ادمین", "🗑 Remove admin"), Data: "a:rm:" + sid}},
		b.navRow("a:ls"))
	lines = append(lines, "", b.tr(
		"ℹ️ «قوانین/DNS» شامل قوانین، DNS و تنظیمات پایه است. «هسته» ریستارت هسته و حالت نگهداری است. «پشتیبان» کل دیتابیس را می‌دهد. «مدیران» فقط فهرست را نشان می‌دهد؛ ویرایش مدیران فقط برای مالک است. تغییرها همان لحظه اعمال می‌شود.",
		"ℹ️ “Routing” covers Rules, DNS and Basics. “Core” is restarting the core and maintenance mode. “Backup” hands out the whole database. “Admins” only shows the list; editing admins is for the owner alone. Changes apply at once."))
	return strings.Join(lines, "\n"), kb
}

// adminsCallback handles the buttons of the Admins screen. Showing the list is
// open to everybody who may see the screen; every other button is the owner's.
func (b *bot) adminsCallback(ctx context.Context, cbID string, chatID, msgID int64, parts []string) {
	show := func(text string, kb [][]button) { b.edit(ctx, chatID, msgID, text, kb) }
	verb := parts[1]
	if verb == "ls" {
		b.answer(ctx, cbID, "")
		text, kb := b.adminsScreen(ctx)
		show(text, kb)
		return
	}
	if !b.owner {
		b.answer(ctx, cbID, b.denied())
		return
	}
	if verb == "add" {
		b.answer(ctx, cbID, "")
		b.pend.set(chatID, &pending{kind: "ad.add", msgID: msgID, back: "a:ls", data: map[string]string{}})
		show("➕ "+b.tr("شناسه عددی تلگرام ادمین جدید را بفرستید. او می‌تواند با /id آن را ببیند. ادمین جدید در ابتدا هیچ دسترسی ندارد؛ بعد از افزودن، دسترسی‌هایش را روشن کنید.", "Send the numeric Telegram ID of the new admin; they can see it with /id. A new admin starts with no access: switch their sections on after adding."),
			[][]button{b.navRow("a:ls"), b.cancelRow()})
		return
	}
	if len(parts) < 3 {
		b.answer(ctx, cbID, "")
		return
	}
	id, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		b.answer(ctx, cbID, "")
		return
	}
	sid := strconv.FormatInt(id, 10)
	a := b.access()
	if a.isOwner(id) {
		b.answer(ctx, cbID, b.accessErrText(errIsOwner))
		return
	}
	if !a.isMember(id) {
		b.answer(ctx, cbID, b.accessErrText(errNoSuchAdmin))
		text, kb := b.adminsScreen(ctx)
		show(text, kb)
		return
	}
	// apply edits the settings and shows the result; on a failure it tells the
	// owner and leaves the screen as it was.
	apply := func(mutate func(d *accessDoc) error, next func() (string, [][]button)) {
		if err := b.editAccess(mutate); err != nil {
			b.answer(ctx, cbID, b.t("failed", b.errText(err)))
			return
		}
		b.answer(ctx, cbID, "")
		text, kb := next()
		show(text, kb)
		b.syncCommandMenus(ctx)
	}
	edited := func() (string, [][]button) { return b.adminEditScreen(ctx, id) }

	switch verb {
	case "e":
		b.answer(ctx, cbID, "")
		text, kb := b.adminEditScreen(ctx, id)
		show(text, kb)
	case "t":
		sec := ""
		if len(parts) > 3 {
			sec = parts[3]
		}
		if !sectionKnown(sec) {
			b.answer(ctx, cbID, "")
			return
		}
		apply(func(d *accessDoc) error {
			r := d.roleOf(id)
			if r.group != "" {
				return errGroupLimited
			}
			set := r.sections.clone()
			if r.sections == nil {
				set = allSections()
			}
			if set[sec] {
				delete(set, sec)
			} else {
				set[sec] = true
			}
			return d.setRole(id, role{sections: set})
		}, edited)
	case "p":
		var r role
		switch {
		case len(parts) > 3 && parts[3] == "full":
		case len(parts) > 3 && parts[3] == "cl":
			r.sections = sectionSet{"clients": true}
		case len(parts) > 3 && parts[3] == "none":
			r.sections = sectionSet{}
		default:
			b.answer(ctx, cbID, "")
			return
		}
		apply(func(d *accessDoc) error { return d.setRole(id, r) }, edited)
	case "g":
		b.answer(ctx, cbID, "")
		var kb [][]button
		for _, row := range b.groupChoices("a:sg:" + sid + ":") {
			// "No group" would leave the admin unlimited; it is the presets' job.
			if len(row) == 1 && strings.HasSuffix(row[0].Data, ":-") {
				continue
			}
			kb = append(kb, row)
		}
		kb = append(kb, []button{{Text: b.tr("➕ گروه جدید", "➕ New group"), Data: "a:sg:" + sid + ":#new"}}, b.navRow("a:e:"+sid), b.cancelRow())
		b.pend.set(chatID, &pending{kind: "ad.grp", msgID: msgID, back: "a:e:" + sid, data: map[string]string{"id": sid}})
		show(b.header("🏷", b.tr("محدود به یک گروه", "Limit to a group"))+"\n"+b.tr("گروه را از دکمه‌ها انتخاب کنید یا نام یک گروه را بفرستید. این ادمین فقط کلاینت‌های همان گروه را می‌بیند و مدیریت می‌کند.", "Pick a group below, or send a group name. This admin will see and manage the clients of that group only."), kb)
	case "sg":
		sel := ""
		if len(parts) > 3 {
			sel = parts[3]
		}
		if sel == "#new" {
			b.answer(ctx, cbID, "")
			b.pend.set(chatID, &pending{kind: "ad.grp", msgID: msgID, back: "a:e:" + sid, data: map[string]string{"id": sid}})
			text, kb := b.newGroupPrompt("a:g:" + sid)
			show(text, kb)
			return
		}
		groups := b.clientGroups()
		idx, err := strconv.Atoi(sel)
		if err != nil || idx < 0 || idx >= len(groups) {
			b.answer(ctx, cbID, b.t("notFound"))
			return
		}
		group := groups[idx]
		apply(func(d *accessDoc) error { return d.setRole(id, role{group: group}) }, edited)
	case "rm":
		b.answer(ctx, cbID, "")
		a := b.access()
		show("⚠️ "+b.tr("این ادمین حذف شود؟ دسترسی‌اش به ربات کامل برداشته می‌شود:", "Remove this admin? They lose all access to the bot:")+"\n"+b.adminLine(a, id, b.adminNames(ctx, []int64{id})[id]),
			[][]button{{{Text: b.t("btnConfirm"), Data: "a:rmy:" + sid}, {Text: b.t("btnCancel"), Data: "a:e:" + sid}}})
	case "rmy":
		apply(func(d *accessDoc) error { return d.remove(id) }, func() (string, [][]button) {
			b.pend.clear(id)
			text, kb := b.adminsScreen(ctx)
			return b.t("done") + "\n\n" + text, kb
		})
	default:
		b.answer(ctx, cbID, "")
	}
}

// parseTelegramID reads the ID of a new administrator.
func parseTelegramID(text string) (int64, error) {
	n, err := strconv.ParseInt(strings.TrimSpace(text), 10, 64)
	if err != nil || n <= 0 || n >= 1<<52 {
		return 0, errBadID
	}
	return n, nil
}

// adminPending consumes the typed answers of the Admins screen: the ID of a
// new administrator, and a group name to limit one to.
func (b *bot) adminPending(ctx context.Context, chatID int64, p *pending, text string, retry func(error), finish func(string, [][]button)) {
	switch p.kind {
	case "ad.add":
		id, err := parseTelegramID(text)
		if err == nil {
			err = b.editAccess(func(d *accessDoc) error { return d.add(id) })
		}
		if err != nil {
			retry(err)
			return
		}
		b.syncCommandMenus(ctx)
		t, kb := b.adminEditScreen(ctx, id)
		finish(b.t("done")+"\n\n"+t, kb)
	case "ad.grp":
		id, err := strconv.ParseInt(p.data["id"], 10, 64)
		if err != nil {
			b.pend.clear(chatID)
			return
		}
		group, err := b.normalizeGroup(text)
		if err == nil {
			err = b.editAccess(func(d *accessDoc) error { return d.setRole(id, role{group: group}) })
		}
		if err != nil {
			retry(err)
			return
		}
		b.syncCommandMenus(ctx)
		t, kb := b.adminEditScreen(ctx, id)
		finish(b.t("done")+"\n\n"+t, kb)
	}
}
