package tgbot

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/service"
)

// clientHead is the top of a client's card: who they are, whether they are
// enabled (and online right now), and the rule under it.
func (b *bot) clientHead(c model.Client, online bool) []string {
	state := "🟢 " + b.t("enabled")
	if !c.Enable {
		state = "🔴 " + b.t("disabled")
	}
	if online {
		state += "  🔵 " + b.tr("آنلاین", "online")
	}
	return []string{"👤 <b>" + esc(c.Name) + "</b>  " + state, rule}
}

// usageText is the card's usage block: the bar with what was used of the
// volume, or only the total for a client without one.
func (b *bot) usageText(c model.Client) string {
	used := c.Up + c.Down
	if c.Volume > 0 {
		p := usagePercent(c)
		return fmt.Sprintf("📊 <b>%s</b> %s %d%%\n      %s / %s  (↑ %s ↓ %s)", b.t("usage"), bar(p, 10), p, humanBytes(used), humanBytes(c.Volume), humanBytes(c.Up), humanBytes(c.Down))
	}
	return fmt.Sprintf("📊 <b>%s</b>: %s ∞\n      ↑ %s ↓ %s", b.t("usage"), humanBytes(used), humanBytes(c.Up), humanBytes(c.Down))
}

// subSummary is what the subscription message says about the client above the
// link: how much was used and what is left of the volume and of the time, the
// IP limit, when they were last online and when they were created. It holds
// the client's own numbers only -- no note, group or Telegram binding -- so the
// message can be passed on to the client as it is.
func (b *bot) subSummary(c model.Client, online bool, now time.Time) string {
	lines := append(b.clientHead(c, online), b.usageText(c))
	if c.Volume > 0 {
		lines = append(lines, fmt.Sprintf("📦 <b>%s</b>: %s", b.t("volLeft"), humanBytes(max(c.Volume-c.Up-c.Down, 0))))
	}
	lines = append(lines, fmt.Sprintf("⏳ <b>%s</b>: %s", b.t("expiry"), b.expiryText(c, now)))
	if c.LimitIp > 0 {
		lines = append(lines, fmt.Sprintf("📱 %s: %d", b.t("ipLimit"), c.LimitIp))
	}
	lines = append(lines, fmt.Sprintf("🕒 %s: %s", b.t("lastOnline"), b.stamp(c.OnlineAt)), fmt.Sprintf("📅 %s: %s", b.t("createdAt"), b.stamp(c.CreatedAt)))
	return strings.Join(lines, "\n")
}

func (b *bot) clientDetail(c model.Client, online bool, now time.Time) string {
	lines := b.clientHead(c, online)
	if c.Desc != "" {
		lines = append(lines, "📝 <i>"+esc(c.Desc)+"</i>")
	}
	lines = append(lines, b.usageText(c))
	lines = append(lines, fmt.Sprintf("⏳ <b>%s</b>: %s", b.t("expiry"), b.expiryText(c, now)))
	if c.LimitIp > 0 {
		lines = append(lines, fmt.Sprintf("📱 %s: %d", b.t("ipLimit"), c.LimitIp))
	}
	if c.Remark != "" {
		lines = append(lines, "🗒 "+b.tr("یادداشت", "Remark")+": "+esc(c.Remark))
	}
	if c.Group != "" {
		lines = append(lines, fmt.Sprintf("🏷 %s: %s", b.t("group"), esc(c.Group)))
	}
	if c.DelayStart || c.AutoReset {
		var parts []string
		if c.DelayStart {
			parts = append(parts, b.tr("شروع با اولین مصرف", "delay start"))
		}
		if c.AutoReset {
			parts = append(parts, b.tr("ریست خودکار", "auto reset"))
		}
		line := fmt.Sprintf("🔁 %s (%d %s)", strings.Join(parts, " + "), c.ResetDays, b.tr("روز", "days"))
		if c.AutoReset && c.NextReset > 0 {
			line += " — " + b.tr("بعدی: ", "next: ") + b.stamp(c.NextReset)
		}
		lines = append(lines, line)
	}
	if c.AutoReset || c.TotalUp+c.TotalDown > 0 {
		lines = append(lines, fmt.Sprintf("Σ %s: ↑ %s ↓ %s", b.tr("مصرف کل", "Lifetime"), humanBytes(c.TotalUp+c.Up), humanBytes(c.TotalDown+c.Down)))
	}
	if ids := clientInboundIDs(c); len(ids) > 0 {
		tags := inboundTagMap()
		names := make([]string, 0, len(ids))
		for i, id := range ids {
			if i == 8 {
				names = append(names, "…")
				break
			}
			if t, ok := tags[id]; ok {
				names = append(names, esc(t))
			}
		}
		lines = append(lines, "📡 "+strings.Join(names, ", "))
	}
	if c.TgId != 0 {
		lines = append(lines, fmt.Sprintf("🔔 %s: <code>%d</code>", b.t("boundTo"), c.TgId))
	}
	lines = append(lines, fmt.Sprintf("🕒 %s: %s", b.t("lastOnline"), b.stamp(c.OnlineAt)), fmt.Sprintf("📅 %s: %s", b.t("createdAt"), b.stamp(c.CreatedAt)))
	// The subscription link closes the card, in a code span so that a tap
	// copies it. The button under the card still sends it with its QR code.
	if link, err := b.subLink(c.Name); err == nil {
		lines = append(lines, "🔗 "+b.t("subLine")+":\n<code>"+esc(link)+"</code>")
	}
	return strings.Join(lines, "\n")
}

func (b *bot) clientKeyboard(c model.Client) [][]button {
	id := strconv.FormatUint(uint64(c.Id), 10)
	toggle := b.btn("btnDisable", "c:tog:"+id)
	if !c.Enable {
		toggle = b.btn("btnEnable", "c:tog:"+id)
	}
	return [][]button{
		{toggle, b.btn("btnReset", "c:rst:"+id)},
		{b.btn("btnAddVol", "c:gb10:"+id, 10), b.btn("btnAddVol", "c:gb50:"+id, 50), b.btn("btnAddDays", "c:d30:"+id, 30)},
		{{Text: b.tr("✏️ ویرایش", "✏️ Edit"), Data: "c:edit:" + id}, {Text: b.tr("🔑 پیکربندی", "🔑 Config"), Data: "c:cfg:" + id}, {Text: b.tr("🔗 لینک خارجی", "🔗 Ext. links"), Data: "c:xl:" + id}},
		{{Text: b.tr("📡 اینباندها", "📡 Inbounds"), Data: "c:inb:" + id}, b.btn("btnSub", "c:sub:"+id)},
		{{Text: b.tr("📋 لینک‌ها", "📋 Links"), Data: "c:lnk:" + id}, b.btn("btnIps", "c:ips:"+id), {Text: b.tr("⏏️ قطع", "⏏️ Kick"), Data: "c:kick:" + id}},
		{{Text: b.tr("🔔 تلگرام", "🔔 Telegram"), Data: "c:ask:tg:" + id}, {Text: "{ } JSON", Data: "c:json:" + id}, b.btn("btnDelete", "c:del:"+id)},
		{b.btn("btnRefresh", "c:view:"+id), {Text: b.tr("⬅️ کلاینت‌ها", "⬅️ Clients"), Data: "c:ls:a:0"}, {Text: b.tr("🏠", "🏠"), Data: "m:menu"}},
	}
}

func isDepleted(c model.Client, now time.Time) bool {
	return (c.Volume > 0 && c.Up+c.Down >= c.Volume) || (c.Expiry > 0 && c.Expiry <= now.Unix())
}

var clientFilters = []struct {
	code, fa, en string
}{
	{"a", "همه", "All"}, {"e", "🟢 فعال", "🟢 Active"}, {"d", "🔴 غیرفعال", "🔴 Disabled"},
	{"n", "⚠️ نزدیک اتمام", "⚠️ Near limit"}, {"x", "⏰ تمام‌شده", "⏰ Depleted"}, {"o", "🔵 آنلاین", "🔵 Online"},
}

func (b *bot) filterClients(filter string, all []model.Client) []model.Client {
	now := time.Now()
	online := map[string]bool{}
	if filter == "o" {
		for _, n := range b.onlineUsers() {
			online[n] = true
		}
	}
	// "g<i>" selects the i-th group of b.clientGroups(), "u" the clients
	// that have no group.
	groupFilter, groupName := false, ""
	if len(filter) > 1 && filter[0] == 'g' {
		idx, err := strconv.Atoi(filter[1:])
		groups := b.clientGroups()
		if err != nil || idx < 0 || idx >= len(groups) {
			return nil
		}
		groupFilter, groupName = true, groups[idx]
	}
	var out []model.Client
	for _, c := range all {
		keep := true
		switch filter {
		case "u":
			keep = c.Group == ""
		case "e":
			keep = c.Enable
		case "d":
			keep = !c.Enable
		case "n":
			p := usagePercent(c)
			days := 1 << 30
			if c.Expiry > 0 {
				days = int(time.Unix(c.Expiry, 0).Sub(now).Hours() / 24)
			}
			keep = (p >= 80 || days <= 7) && !isDepleted(c, now)
		case "x":
			keep = isDepleted(c, now)
		case "o":
			keep = online[c.Name]
		}
		if groupFilter {
			keep = c.Group == groupName
		}
		if keep {
			out = append(out, c)
		}
	}
	sortClients(out, clientsNewestFirst())
	return out
}

// clientsNewestFirst reports the saved list order. The default is the order the
// clients were created in (oldest first), like the panel's own table.
func clientsNewestFirst() bool {
	return (&service.SettingService{}).GetTgBotClientSort() == "desc"
}

// sortClients orders clients by creation time, then by id. Rows that predate
// the creation-time column have createdAt 0 and fall back to id order, which
// is creation order too.
func sortClients(list []model.Client, newestFirst bool) {
	sort.SliceStable(list, func(i, j int) bool {
		a, c := list[i], list[j]
		if a.CreatedAt != c.CreatedAt {
			if newestFirst {
				return a.CreatedAt > c.CreatedAt
			}
			return a.CreatedAt < c.CreatedAt
		}
		if newestFirst {
			return a.Id > c.Id
		}
		return a.Id < c.Id
	})
}

// filterLabel names the group filters in the list header ("" for the others).
func (b *bot) filterLabel(filter string) string {
	if filter == "u" {
		return " — " + b.tr("➖ بدون گروه", "➖ no group")
	}
	if len(filter) > 1 && filter[0] == 'g' {
		if idx, err := strconv.Atoi(filter[1:]); err == nil {
			if groups := b.clientGroups(); idx >= 0 && idx < len(groups) {
				return " — 🏷 " + esc(groups[idx])
			}
		}
	}
	return ""
}

func (b *bot) clientSortLabel() string {
	if clientsNewestFirst() {
		return b.tr("جدیدترین اول", "Newest first")
	}
	return b.tr("قدیمی‌ترین اول", "Oldest first")
}

const clientsPageSize = 10

func (b *bot) clientsScreen(filter string, page int) (string, [][]button) {
	all := b.loadClients()
	list := b.filterClients(filter, all)
	from, to, page, pages := pageSlice(len(list), page, clientsPageSize)
	now := time.Now()
	online := map[string]bool{}
	for _, n := range b.onlineUsers() {
		online[n] = true
	}
	lines := []string{b.header("👥", fmt.Sprintf("%s%s (%d/%d)", b.tr("کلاینت‌ها", "Clients"), b.filterLabel(filter), len(list), len(all))),
		"📅 " + b.tr("مرتب‌شده بر اساس تاریخ ساخت — ", "Sorted by creation date — ") + b.clientSortLabel()}
	if line := b.quotaLine(); line != "" {
		lines = append(lines, line)
	}
	if len(list) == 0 {
		lines = append(lines, b.t("noClients"))
	}
	var btns []button
	for _, c := range list[from:to] {
		line := b.clientLine(c, now)
		if c.CreatedAt > 0 {
			line += " · 📅 " + time.Unix(c.CreatedAt, 0).In(b.loc).Format("2006-01-02")
		}
		if online[c.Name] {
			line += " 🔵"
		}
		lines = append(lines, line)
		icon := "🟢"
		if !c.Enable {
			icon = "🔴"
		}
		btns = append(btns, button{Text: icon + " " + truncate(c.Name, 24), Data: "c:view:" + strconv.FormatUint(uint64(c.Id), 10)})
	}
	kb := rows2(btns)
	if pg := pager("c:ls:"+filter, page, pages); pg != nil {
		kb = append(kb, pg)
	}
	var chips []button
	for _, f := range clientFilters {
		text := b.tr(f.fa, f.en)
		if f.code == filter {
			text = "◉ " + text
		}
		chips = append(chips, button{Text: text, Data: "c:ls:" + f.code + ":0"})
	}
	kb = append(kb, chips[:3], chips[3:])
	sortRow := []button{{Text: "🔃 " + b.clientSortLabel(), Data: "c:sort:" + filter}}
	if b.scope == "" {
		sortRow = append(sortRow, button{Text: b.tr("🏷 گروه‌ها", "🏷 Groups"), Data: "c:grps"})
	}
	kb = append(kb, sortRow)
	kb = append(kb,
		[]button{{Text: b.tr("➕ کلاینت جدید", "➕ New client"), Data: "c:new"}, {Text: b.tr("📄 جدید با JSON", "📄 New via JSON"), Data: "c:newj"}},
		[]button{{Text: b.tr("🔎 جستجو", "🔎 Search"), Data: "c:srch"}, {Text: b.tr("🛠 ویرایش گروهی", "🛠 Bulk edit"), Data: "c:bulk"}, {Text: b.tr("🧹 پاکسازی", "🧹 Cleanup"), Data: "c:clean"}},
		b.menuRow())
	return strings.Join(lines, "\n"), kb
}

// ---- prompts ----

var clientAsks = map[string]struct{ fa, en string }{
	"vol":  {"حجم کل را به گیگابایت بفرستید (۰ = نامحدود).", "Send the total volume in GB (0 = unlimited)."},
	"days": {"تعداد روز از همین الان تا انقضا را بفرستید (۰ = نامحدود).", "Send the number of days from now until expiry (0 = unlimited)."},
	"ip":   {"محدودیت تعداد IP همزمان را بفرستید (۰ = نامحدود).", "Send the concurrent IP limit (0 = unlimited)."},
	"desc": {"توضیح را بفرستید (- برای پاک کردن).", "Send the note (- to clear)."},
	"grp":  {"گروه را از دکمه‌ها انتخاب کنید یا نام گروه جدید را بفرستید تا ساخته شود.", "Pick a group below or send a new group name to create it."},
	"name": {"نام جدید را بفرستید. توجه: لینک اشتراک کلاینت با نام عوض می‌شود.", "Send the new name. Note: the client's subscription link changes with its name."},
	"rem":  {"یادداشت (Remark) را بفرستید (- برای پاک کردن).", "Send the remark (- to clear)."},
	"exp":  {"تاریخ انقضا را به شکل YYYY-MM-DD یا YYYY-MM-DD HH:MM بفرستید (۰ = نامحدود).", "Send the expiry date as YYYY-MM-DD or YYYY-MM-DD HH:MM (0 = unlimited)."},
	"rd":   {"تعداد روزهای ریست / شروع با اولین مصرف را بفرستید (حداقل ۱).", "Send the reset / delay-start days (at least 1)."},
	"cfgj": {"JSON پیکربندی پروتکل‌ها را بفرستید (متن یا فایل .json)، مثل {\"vless\":{\"name\":\"x\",\"uuid\":\"…\"}}.", "Send the protocol credentials JSON (text or .json file), e.g. {\"vless\":{\"name\":\"x\",\"uuid\":\"…\"}}."},
	"xl":   {"لینک خارجی را بفرستید: <protocol>://<data>", "Send the external link: <protocol>://<data>"},
	"sl":   {"لینک اشتراک را بفرستید: http[s]://دامنه[:پورت]/مسیر", "Send the subscription link: http[s]://domain[:port]/path"},
	"tg":   {"شناسه عددی تلگرام کاربر را بفرستید (۰ = قطع اتصال).", "Send the user's numeric Telegram ID (0 = unbind)."},
}

func (b *bot) ask(ctx context.Context, chatID, msgID int64, kind, key string, id uint, back string, prompt string) {
	b.pend.set(chatID, &pending{kind: kind, key: key, id: id, msgID: msgID, back: back, data: map[string]string{}})
	b.edit(ctx, chatID, msgID, "✏️ "+prompt, [][]button{b.cancelRow()})
}

func (b *bot) askClient(ctx context.Context, chatID, msgID int64, field string, id uint) {
	a, ok := clientAsks[field]
	if !ok {
		return
	}
	if field == "grp" {
		if b.scope != "" {
			// A limited administrator cannot move a client to another group.
			b.edit(ctx, chatID, msgID, b.t("scopeDenied"), [][]button{b.menuRow()})
			return
		}
		// Existing groups as buttons; a typed name, or the "New group"
		// button, creates a new group.
		sid := strconv.FormatUint(uint64(id), 10)
		b.pend.set(chatID, &pending{kind: "cl.grp", key: "grp", id: id, msgID: msgID, back: "c:view:" + sid, data: map[string]string{}})
		b.edit(ctx, chatID, msgID, "🏷 "+b.tr(a.fa, a.en), b.groupPicker("c:sg:"+sid+":"))
		return
	}
	b.ask(ctx, chatID, msgID, "cl."+field, field, id, "c:view:"+strconv.FormatUint(uint64(id), 10), b.tr(a.fa, a.en))
}

// applyClientAnswer applies a typed answer to a client field and returns the
// user-facing error text, if any.
func (b *bot) applyClientAnswer(field string, id uint, text string) error {
	text = strings.TrimSpace(text)
	clear := text == "-"
	switch field {
	case "vol":
		gb, ok := parseFloatArg(text)
		if !ok {
			return fmt.Errorf("%s", b.t("badNumber"))
		}
		return b.setVolume(id, int64(gb*float64(gib)))
	case "days":
		d, ok := parseIntArg(text)
		if !ok {
			return fmt.Errorf("%s", b.t("badNumber"))
		}
		return b.setExpiryDays(id, d)
	case "ip":
		n, ok := parseIntArg(text)
		if !ok {
			return fmt.Errorf("%s", b.t("badNumber"))
		}
		return b.setLimitIP(id, n)
	case "desc":
		return b.editClient(id, func(c *model.Client) error {
			c.Desc = text
			if clear {
				c.Desc = ""
			}
			return nil
		})
	case "rem":
		return b.editClient(id, func(c *model.Client) error {
			c.Remark = text
			if clear {
				c.Remark = ""
			}
			return nil
		})
	case "exp":
		ts, err := parseExpiryDate(text)
		if err != nil {
			return err
		}
		return b.editClient(id, func(c *model.Client) error {
			if c.DelayStart && !c.AutoReset {
				return fmt.Errorf("%s", b.tr("برای شروع با اولین مصرف تاریخ ثابت معنی ندارد.", "A fixed date does not apply to delay start."))
			}
			c.Expiry = ts
			return nil
		})
	case "rd":
		d, ok := parseIntArg(text)
		if !ok {
			return fmt.Errorf("%s", b.t("badNumber"))
		}
		return b.setResetDays(id, d)
	case "cfgj":
		v, err := decodeJSONValue(text)
		if err != nil {
			return err
		}
		m, ok := v.(map[string]interface{})
		if !ok {
			return fmt.Errorf("a JSON object is required")
		}
		for k, x := range m {
			if _, isObj := x.(map[string]interface{}); !isObj {
				return fmt.Errorf("%q must be an object", k)
			}
		}
		raw, _ := json.Marshal(m)
		return b.editClient(id, func(c *model.Client) error { c.Config = raw; return nil })
	case "xl":
		return b.addClientLink(id, "external", text)
	case "sl":
		return b.addClientLink(id, "sub", text)
	case "json":
		return b.applyClientJSON(id, text)
	case "grp":
		group := ""
		if !clear {
			g, err := b.normalizeGroup(text)
			if err != nil {
				return err
			}
			group = g
		}
		return b.editClient(id, func(c *model.Client) error {
			c.Group = group
			return nil
		})
	case "name":
		if !clientNameRe.MatchString(text) {
			return fmt.Errorf("%s", b.t("badName"))
		}
		return b.editClient(id, func(c *model.Client) error { c.Name = text; return nil })
	case "tg":
		n, err := strconv.ParseInt(text, 10, 64)
		if err != nil || n < 0 {
			return fmt.Errorf("%s", b.t("badNumber"))
		}
		return b.bindClient(id, n)
	}
	return nil
}

// ---- inbound assignment, links, kick ----

func (b *bot) clientInboundsScreen(c model.Client) (string, [][]button) {
	var assigned []uint
	_ = json.Unmarshal(b.mustFullInbounds(c.Id), &assigned)
	has := map[uint]bool{}
	for _, id := range assigned {
		has[id] = true
	}
	var inbounds []model.Inbound
	_ = database.GetDB().Select("id", "tag", "type", "node_id").Order("id").Find(&inbounds).Error
	cid := strconv.FormatUint(uint64(c.Id), 10)
	var btns []button
	for i, in := range inbounds {
		if i == 40 {
			break
		}
		btns = append(btns, button{Text: onOff(has[in.Id]) + " " + truncate(in.Tag, 22), Data: "c:ti:" + cid + ":" + strconv.FormatUint(uint64(in.Id), 10)})
	}
	text := b.header("📡", b.tr("اینباندهای ", "Inbounds of ")+esc(c.Name)) + "\n" + b.tr("برای افزودن یا حذف روی هر اینباند بزنید.", "Tap an inbound to add or remove it.")
	kb := append(rows2(btns), []button{{Text: b.tr("⬅️ کلاینت", "⬅️ Client"), Data: "c:view:" + cid}, {Text: "🏠", Data: "m:menu"}})
	return text, kb
}

func (b *bot) toggleClientInbound(clientID, inboundID uint) error {
	return b.editClient(clientID, func(c *model.Client) error {
		var ids []uint
		_ = json.Unmarshal(c.Inbounds, &ids)
		out := make([]uint, 0, len(ids)+1)
		found := false
		for _, id := range ids {
			if id == inboundID {
				found = true
				continue
			}
			out = append(out, id)
		}
		if !found {
			out = append(out, inboundID)
		}
		sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
		c.Inbounds, _ = json.Marshal(out)
		return nil
	})
}

func (b *bot) sendLinks(ctx context.Context, chatID int64, id uint) {
	c, err := b.fullClient(id)
	if err != nil {
		b.fail(ctx, chatID, err)
		return
	}
	var links []struct {
		Remark string `json:"remark"`
		URI    string `json:"uri"`
	}
	_ = json.Unmarshal(c.Links, &links)
	if len(links) == 0 {
		b.send(ctx, chatID, b.tr("این کلاینت لینکی ندارد (اینباندی به آن اختصاص داده نشده).", "This client has no links (no inbound assigned)."))
		return
	}
	lines := []string{b.header("📋", b.tr("لینک‌های ", "Links of ")+esc(c.Name))}
	for _, l := range links {
		if l.Remark != "" {
			lines = append(lines, "• <b>"+esc(l.Remark)+"</b>")
		}
		lines = append(lines, "<code>"+esc(l.URI)+"</code>")
	}
	b.send(ctx, chatID, strings.Join(lines, "\n"))
}

// ---- cleanup ----

func (b *bot) depletedClients() []model.Client {
	return b.filterClients("x", b.loadClients())
}

func (b *bot) deleteClients(ids []uint) error {
	return b.save("delbulk", ids)
}

// ---- bulk create ----

func (b *bot) createBulk(prefix, group string, count int, volume int64, days, limitIP int) error {
	if !clientNameRe.MatchString(prefix) || count < 1 || count > 200 {
		return fmt.Errorf("%s", b.t("badNumber"))
	}
	group, err := b.groupForCreate(group)
	if err != nil {
		return err
	}
	if b.scope == "" && strings.TrimSpace(group) != "" {
		g, err := b.normalizeGroup(group)
		if err != nil {
			return err
		}
		group = g
	}
	var ids []uint
	if err := database.GetDB().Model(&model.Inbound{}).Order("id").Pluck("id", &ids).Error; err != nil {
		return err
	}
	if ids == nil {
		ids = []uint{}
	}
	inbounds, _ := json.Marshal(ids)
	var clients []model.Client
	for i := 1; i <= count; i++ {
		name := fmt.Sprintf("%s%d", prefix, i)
		c := model.Client{Enable: true, Name: name, Config: newClientConfig(name), Inbounds: inbounds, Links: json.RawMessage(`[]`), Volume: volume, LimitIp: limitIP, Group: group}
		if days > 0 {
			c.Expiry = time.Now().Add(time.Duration(days) * 24 * time.Hour).Unix()
		}
		clients = append(clients, c)
	}
	return b.save("addbulk", clients)
}

// ---- callbacks ----

func (b *bot) clientCallback(ctx context.Context, cbID string, chatID, msgID int64, parts []string) {
	action := parts[1]
	arg := func(i int) string {
		if i < len(parts) {
			return parts[i]
		}
		return ""
	}
	show := func(text string, kb [][]button) { b.edit(ctx, chatID, msgID, text, kb) }
	if b.clientCallbackExt(ctx, cbID, chatID, msgID, parts) {
		return
	}
	switch action {
	case "ls":
		page, _ := strconv.Atoi(arg(3))
		b.answer(ctx, cbID, "")
		text, kb := b.clientsScreen(arg(2), page)
		show(text, kb)
		return
	case "srch":
		b.answer(ctx, cbID, "")
		b.ask(ctx, chatID, msgID, "cl.search", "", 0, "c:ls:a:0", b.tr("نام یا بخشی از نام کلاینت را بفرستید.", "Send the client's name or part of it."))
		return
	case "new":
		// An administrator whose volume limit is used up is told before the
		// questions, not after the last one.
		if err := b.quotaGate("new", model.Client{}); err != nil {
			b.answer(ctx, cbID, b.t("failed", b.errText(err)))
			return
		}
		b.answer(ctx, cbID, "")
		p := &pending{kind: "wiz", key: "name", msgID: msgID, back: "c:ls:a:0", data: map[string]string{}}
		b.pend.set(chatID, p)
		t, kb := b.wizardPrompt(p)
		show(t, kb)
		return
	case "clean":
		b.answer(ctx, cbID, "")
		list := b.depletedClients()
		if len(list) == 0 {
			show(b.tr("کلاینت تمام‌شده‌ای وجود ندارد.", "There are no depleted clients."), [][]button{b.navRow("c:ls:a:0")})
			return
		}
		names := make([]string, 0, 15)
		for i, c := range list {
			if i == 15 {
				names = append(names, "…")
				break
			}
			names = append(names, "• "+esc(c.Name))
		}
		show(fmt.Sprintf("⚠️ %s\n%s", b.tr(fmt.Sprintf("%d کلاینت با حجم یا زمان تمام‌شده حذف شوند؟", len(list)), fmt.Sprintf("Delete %d clients that are out of volume or time?", len(list))), strings.Join(names, "\n")),
			[][]button{{b.btn("btnConfirm", "c:cleany"), {Text: b.tr("✖️ انصراف", "✖️ Cancel"), Data: "c:ls:a:0"}}})
		return
	case "cleany":
		var ids []uint
		for _, c := range b.depletedClients() {
			ids = append(ids, c.Id)
		}
		if len(ids) == 0 {
			b.answer(ctx, cbID, b.t("done"))
			return
		}
		if err := b.deleteClients(ids); err != nil {
			b.answer(ctx, cbID, b.t("failed", b.errText(err)))
			return
		}
		b.answer(ctx, cbID, b.t("done"))
		text, kb := b.clientsScreen("a", 0)
		show(text, kb)
		return
	}

	id64, err := strconv.ParseUint(arg(len(parts)-1), 10, 32)
	if action == "ask" || action == "ti" {
		if action == "ask" {
			id64, err = strconv.ParseUint(arg(3), 10, 32)
		} else {
			id64, err = strconv.ParseUint(arg(2), 10, 32)
		}
	}
	if err != nil {
		b.answer(ctx, cbID, "")
		return
	}
	id := uint(id64)
	client := b.clientByID(id)
	if client == nil {
		b.answer(ctx, cbID, b.t("notFound"))
		return
	}
	showCard := func(note string) {
		if c := b.clientByID(id); c != nil {
			text, kb := b.card(*c)
			if note != "" {
				text = note + "\n\n" + text
			}
			show(text, kb)
		}
	}
	mutate := func(err error, after func()) {
		if err != nil {
			b.answer(ctx, cbID, b.t("failed", b.errText(err)))
			return
		}
		b.answer(ctx, cbID, b.t("done"))
		after()
	}
	sid := strconv.FormatUint(uint64(id), 10)
	switch action {
	case "view":
		b.answer(ctx, cbID, "")
		showCard("")
	case "ask":
		b.answer(ctx, cbID, "")
		b.askClient(ctx, chatID, msgID, arg(2), id)
	case "tog":
		err := b.setEnabled(id, !client.Enable)
		mutate(err, func() {
			note := ""
			if !client.Enable {
				if c, _ := b.fullClient(id); c != nil && isDepleted(*c, time.Now()) {
					note = strings.TrimSpace(b.t("stillDepleted"))
				}
			}
			showCard(note)
		})
	case "rst":
		b.answer(ctx, cbID, "")
		show(b.t("confirmReset", esc(client.Name)), b.confirmKeyboard("c:rsty:", id))
	case "rsty":
		mutate(b.resetTraffic(id), func() { showCard("") })
	case "gb10":
		mutate(b.addVolume(id, 10*gib), func() { showCard("") })
	case "gb50":
		mutate(b.addVolume(id, 50*gib), func() { showCard("") })
	case "d30":
		mutate(b.addDays(id, 30), func() { showCard("") })
	case "sub":
		b.answer(ctx, cbID, "")
		b.sendSub(ctx, chatID, *client)
	case "lnk":
		b.answer(ctx, cbID, "")
		b.sendLinks(ctx, chatID, id)
	case "ips":
		b.answer(ctx, cbID, "")
		show(b.ipsText(client.Name), [][]button{{b.btn("btnBack", "c:view:"+sid)}, b.menuRow()})
	case "kick":
		err := (&service.StatsService{}).CloseClusterSessions(client.Name)
		mutate(err, func() {})
	case "inb":
		b.answer(ctx, cbID, "")
		text, kb := b.clientInboundsScreen(*client)
		show(text, kb)
	case "ti":
		inb, _ := strconv.ParseUint(arg(3), 10, 32)
		mutate(b.toggleClientInbound(id, uint(inb)), func() {
			if c := b.clientByID(id); c != nil {
				text, kb := b.clientInboundsScreen(*c)
				show(text, kb)
			}
		})
	case "del":
		b.answer(ctx, cbID, "")
		show(b.t("confirmDelete", esc(client.Name)), b.confirmKeyboard("c:dely:", id))
	case "dely":
		mutate(b.deleteClient(id), func() {
			show(b.t("deleted", esc(client.Name)), [][]button{b.navRow("c:ls:a:0")})
		})
	default:
		b.answer(ctx, cbID, "")
	}
}
