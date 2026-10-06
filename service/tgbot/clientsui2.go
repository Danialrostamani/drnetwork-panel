package tgbot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/service"
)

// This file holds the client screens that mirror the rest of the web panel's
// client form: every basic field, the per-protocol credentials ("Config"
// tab), the external / subscription links ("Links" tab), whole-client JSON and
// the bulk-edit dialog.

func sid(id uint) string { return strconv.FormatUint(uint64(id), 10) }

func inboundTagMap() map[uint]string {
	var inbounds []model.Inbound
	_ = database.GetDB().Select("id", "tag").Find(&inbounds).Error
	out := make(map[uint]string, len(inbounds))
	for _, in := range inbounds {
		out[in.Id] = in.Tag
	}
	return out
}

func clientInboundIDs(c model.Client) []uint {
	var ids []uint
	_ = json.Unmarshal(c.Inbounds, &ids)
	return ids
}

// randomClientName suggests an unused 8-character name, like the panel's own
// "Add client" form does.
func (b *bot) randomClientName() string {
	for i := 0; i < 20; i++ {
		n := randomSeq(8)
		if rawFindClientByName(n) == nil {
			return n
		}
	}
	return randomSeq(12)
}

func (b *bot) clientAnswerErr(key string) error { return fmt.Errorf("%s", b.t(key)) }

// ---- edit screen (Basics tab) ----

func (b *bot) clientEditScreen(c model.Client) (string, [][]button) {
	v := func(s string) string {
		if strings.TrimSpace(s) == "" {
			return "—"
		}
		return esc(s)
	}
	now := time.Now()
	vol := b.t("unlimited")
	if c.Volume > 0 {
		vol = humanBytes(c.Volume)
	}
	exp := b.expiryText(c, now)
	lines := []string{
		b.header("✏️", b.tr("ویرایش ", "Edit ")+esc(c.Name)),
		fmt.Sprintf("👤 %s: <b>%s</b>", b.tr("نام", "Name"), esc(c.Name)),
		fmt.Sprintf("📝 %s: %s", b.tr("توضیح", "Description"), v(c.Desc)),
		fmt.Sprintf("🗒 %s: %s", b.tr("یادداشت", "Remark"), v(c.Remark)),
		fmt.Sprintf("🏷 %s: %s", b.t("group"), v(c.Group)),
		fmt.Sprintf("📦 %s: %s", b.tr("حجم", "Volume"), vol),
		fmt.Sprintf("⏳ %s: %s", b.t("expiry"), exp),
		fmt.Sprintf("📱 %s: %d", b.t("ipLimit"), c.LimitIp),
		fmt.Sprintf("%s %s", onOff(c.DelayStart), b.tr("شروع با اولین مصرف (Delay start)", "Delay start")),
		fmt.Sprintf("%s %s", onOff(c.AutoReset), b.tr("ریست خودکار (Auto reset)", "Auto reset")),
	}
	if c.DelayStart || c.AutoReset {
		lines = append(lines, fmt.Sprintf("🔁 %s: %d", b.tr("روز (Reset days)", "Reset days"), c.ResetDays))
	}
	if c.AutoReset && c.NextReset > 0 {
		lines = append(lines, fmt.Sprintf("🗓 %s: %s", b.tr("ریست بعدی", "Next reset"), b.stamp(c.NextReset)))
	}
	id := sid(c.Id)
	a := func(fa, en, f string) button { return button{Text: b.tr(fa, en), Data: "c:ask:" + f + ":" + id} }
	// A limited administrator cannot move a client to another group.
	volRow := []button{a("🏷 گروه", "🏷 Group", "grp"), a("📦 حجم", "📦 Volume", "vol"), a("📱 IP", "📱 IP limit", "ip")}
	if b.scope != "" {
		volRow = volRow[1:]
	}
	kb := [][]button{
		{a("👤 نام", "👤 Name", "name"), a("📝 توضیح", "📝 Description", "desc"), a("🗒 یادداشت", "🗒 Remark", "rem")},
		volRow,
		{a("⏳ روز تا انقضا", "⏳ Days left", "days"), a("📅 تاریخ انقضا", "📅 Expiry date", "exp")},
		{{Text: onOff(c.DelayStart) + " " + b.tr("شروع با اولین مصرف", "Delay start"), Data: "c:dly:" + id}, {Text: onOff(c.AutoReset) + " " + b.tr("ریست خودکار", "Auto reset"), Data: "c:ar:" + id}},
	}
	if c.DelayStart || c.AutoReset {
		kb = append(kb, []button{a("🔁 روزهای ریست", "🔁 Reset days", "rd")})
	}
	kb = append(kb, []button{{Text: b.tr("⬅️ کلاینت", "⬅️ Client"), Data: "c:view:" + id}, {Text: "🏠", Data: "m:menu"}})
	return strings.Join(lines, "\n"), kb
}

func parseExpiryDate(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "0" || s == "-" {
		return 0, nil
	}
	for _, layout := range []string{"2006-01-02 15:04", "2006-01-02T15:04", "2006-01-02", "2006/01/02"} {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t.Unix(), nil
		}
	}
	return 0, errors.New("date format: YYYY-MM-DD [HH:MM]")
}

func (b *bot) toggleDelayStart(id uint) error {
	return b.editClient(id, func(c *model.Client) error {
		if !c.DelayStart && c.Up+c.Down > 0 {
			return errors.New(b.tr("این کلاینت قبلاً مصرف داشته است.", "This client already has traffic."))
		}
		c.DelayStart = !c.DelayStart
		if c.DelayStart {
			if c.ResetDays <= 0 {
				c.ResetDays = 1
			}
			if !c.AutoReset {
				c.Expiry = 0
			}
		} else if !c.AutoReset {
			c.ResetDays = 0
		}
		return nil
	})
}

func (b *bot) toggleAutoReset(id uint) error {
	return b.editClient(id, func(c *model.Client) error {
		c.AutoReset = !c.AutoReset
		if c.AutoReset {
			if c.ResetDays <= 0 {
				c.ResetDays = 1
			}
		} else {
			c.NextReset = 0
			if !c.DelayStart {
				c.ResetDays = 0
			}
		}
		return nil
	})
}

func (b *bot) setResetDays(id uint, days int) error {
	return b.editClient(id, func(c *model.Client) error {
		if !c.DelayStart && !c.AutoReset {
			return errors.New(b.tr("اول شروع با اولین مصرف یا ریست خودکار را روشن کنید.", "Turn on delay start or auto reset first."))
		}
		if days < 1 {
			days = 1
		}
		if c.NextReset > 0 {
			c.NextReset += int64(days-c.ResetDays) * 86400
		}
		c.ResetDays = days
		return nil
	})
}

// ---- Config tab: per-protocol credentials ----

type clientCfg map[string]map[string]interface{}

func parseClientCfg(raw json.RawMessage) clientCfg {
	cfg := clientCfg{}
	_ = json.Unmarshal(raw, &cfg)
	return cfg
}

func (cfg clientCfg) keys() []string {
	out := make([]string, 0, len(cfg))
	for k := range cfg {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func shuffleCfg(cfg clientCfg, only string) {
	for k, m := range cfg {
		if only != "" && k != only {
			continue
		}
		if m == nil {
			continue
		}
		switch k {
		case "mixed", "socks", "http", "anytls", "trojan", "naive", "hysteria2":
			m["password"] = randomSeq(10)
		case "shadowsocks", "shadowtls":
			m["password"] = randomBase64(32)
		case "shadowsocks16":
			m["password"] = randomBase64(16)
		case "hysteria":
			m["auth_str"] = randomSeq(10)
		case "snell":
			m["userkey"] = randomSeq(32)
		case "tuic":
			m["password"] = randomSeq(10)
			m["uuid"] = randomUUID()
		case "vmess", "vless":
			m["uuid"] = randomUUID()
		}
	}
}

func (b *bot) clientConfigScreen(c model.Client) (string, [][]button) {
	cfg := parseClientCfg(c.Config)
	keys := cfg.keys()
	lines := []string{b.header("🔑", b.tr("پیکربندی ", "Config of ")+esc(c.Name))}
	if len(keys) == 0 {
		lines = append(lines, b.tr("پیکربندی‌ای ثبت نشده است.", "No credentials stored."))
	}
	var btns []button
	id := sid(c.Id)
	for i, k := range keys {
		lines = append(lines, "• <b>"+esc(k)+"</b>")
		fields := make([]string, 0, len(cfg[k]))
		for f := range cfg[k] {
			fields = append(fields, f)
		}
		sort.Strings(fields)
		for _, f := range fields {
			lines = append(lines, fmt.Sprintf("    %s: <code>%s</code>", esc(f), esc(fmt.Sprint(cfg[k][f]))))
		}
		btns = append(btns, button{Text: "🔄 " + k, Data: "c:cfgp:" + id + ":" + strconv.Itoa(i)})
	}
	kb := rows2(btns)
	kb = append(kb,
		[]button{{Text: b.tr("🔄 همه را بازسازی کن", "🔄 Regenerate all"), Data: "c:cfga:" + id}, {Text: b.tr("✏️ ویرایش JSON", "✏️ Edit JSON"), Data: "c:ask:cfgj:" + id}},
		[]button{{Text: b.tr("⬅️ کلاینت", "⬅️ Client"), Data: "c:view:" + id}, {Text: "🏠", Data: "m:menu"}})
	return strings.Join(lines, "\n"), kb
}

func (b *bot) shuffleClientConfig(id uint, idx int) error {
	return b.editClient(id, func(c *model.Client) error {
		cfg := parseClientCfg(c.Config)
		only := ""
		if idx >= 0 {
			keys := cfg.keys()
			if idx >= len(keys) {
				return errors.New("unknown protocol")
			}
			only = keys[idx]
		}
		shuffleCfg(cfg, only)
		c.Config, _ = json.Marshal(cfg)
		return nil
	})
}

// ---- Links tab: external and subscription links ----

type clientLink struct {
	raw map[string]interface{}
	idx int // position in the stored array
}

func (b *bot) editableLinks(raw json.RawMessage) ([]map[string]interface{}, []clientLink) {
	var all []map[string]interface{}
	_ = json.Unmarshal(raw, &all)
	var nodes []model.Node
	_ = database.GetDB().Select("name").Find(&nodes).Error
	nodeOwned := func(remark string) bool {
		for _, n := range nodes {
			if strings.HasPrefix(remark, "["+n.Name+"] ") {
				return true
			}
		}
		return false
	}
	var out []clientLink
	for i, l := range all {
		typ, _ := l["type"].(string)
		remark, _ := l["remark"].(string)
		if typ == "sub" || (typ == "external" && !nodeOwned(remark)) {
			out = append(out, clientLink{raw: l, idx: i})
		}
	}
	return all, out
}

func (b *bot) clientLinksScreen(c model.Client) (string, [][]button) {
	_, links := b.editableLinks(c.Links)
	id := sid(c.Id)
	lines := []string{b.header("🔗", b.tr("لینک‌های خارجی و اشتراک ", "External & sub links of ")+esc(c.Name))}
	if len(links) == 0 {
		lines = append(lines, b.tr("لینک خارجی یا اشتراکی ثبت نشده است.", "No external or subscription links."))
	}
	var btns []button
	for i, l := range links {
		typ, _ := l.raw["type"].(string)
		uri, _ := l.raw["uri"].(string)
		lines = append(lines, fmt.Sprintf("%d. [%s] <code>%s</code>", i+1, esc(typ), esc(truncate(uri, 120))))
		btns = append(btns, button{Text: "🗑 " + strconv.Itoa(i+1), Data: "c:xld:" + id + ":" + strconv.Itoa(i)})
	}
	var kb [][]button
	for i := 0; i < len(btns); i += 4 {
		end := i + 4
		if end > len(btns) {
			end = len(btns)
		}
		kb = append(kb, btns[i:end])
	}
	kb = append(kb,
		[]button{{Text: b.tr("➕ لینک خارجی", "➕ External link"), Data: "c:ask:xl:" + id}, {Text: b.tr("➕ لینک اشتراک", "➕ Sub link"), Data: "c:ask:sl:" + id}},
		[]button{{Text: b.tr("⬅️ کلاینت", "⬅️ Client"), Data: "c:view:" + id}, {Text: "🏠", Data: "m:menu"}})
	return strings.Join(lines, "\n"), kb
}

func (b *bot) addClientLink(id uint, typ, uri string) error {
	uri = strings.TrimSpace(uri)
	switch typ {
	case "external":
		if !strings.Contains(uri, "://") {
			return errors.New("<protocol>://<data>")
		}
	case "sub":
		if !strings.HasPrefix(uri, "http://") && !strings.HasPrefix(uri, "https://") {
			return errors.New("http[s]://<domain>[:port]/<path>")
		}
	}
	return b.editClient(id, func(c *model.Client) error {
		var all []map[string]interface{}
		_ = json.Unmarshal(c.Links, &all)
		all = append(all, map[string]interface{}{"type": typ, "uri": uri})
		c.Links, _ = json.Marshal(all)
		return nil
	})
}

func (b *bot) deleteClientLink(id uint, n int) error {
	return b.editClient(id, func(c *model.Client) error {
		all, links := b.editableLinks(c.Links)
		if n < 0 || n >= len(links) {
			return errors.New("unknown link")
		}
		drop := links[n].idx
		out := make([]map[string]interface{}, 0, len(all))
		for i, l := range all {
			if i != drop {
				out = append(out, l)
			}
		}
		c.Links, _ = json.Marshal(out)
		return nil
	})
}

// ---- whole-client JSON ----

var protectedClientJSON = map[string]bool{"id": true, "up": true, "down": true, "totalUp": true, "totalDown": true, "tgId": true, "createdAt": true, "onlineAt": true, "nextReset": true}

func (b *bot) applyClientJSON(id uint, text string) error {
	v, err := decodeJSONValue(text)
	if err != nil {
		return err
	}
	obj, ok := v.(map[string]interface{})
	if !ok {
		return errors.New("a JSON object is required")
	}
	for k := range protectedClientJSON {
		delete(obj, k)
	}
	return b.editClient(id, func(c *model.Client) error {
		old := c.Name
		raw, _ := json.Marshal(obj)
		if err := json.Unmarshal(raw, c); err != nil {
			return err
		}
		if c.Group == service.ClusterGroup {
			return errors.New(b.tr("این نام گروه رزرو شده است.", "That group name is reserved."))
		}
		if c.Name != old {
			if !clientNameRe.MatchString(c.Name) {
				return b.clientAnswerErr("badName")
			}
			if o := rawFindClientByName(c.Name); o != nil && o.Id != id {
				return errors.New(b.tr("این نام قبلاً استفاده شده است.", "That name is already in use."))
			}
		}
		if len(c.Inbounds) == 0 || string(c.Inbounds) == "null" {
			c.Inbounds = json.RawMessage("[]")
		}
		return nil
	})
}

const newClientTemplate = `{
  "name": "user1",
  "enable": true,
  "desc": "",
  "remark": "",
  "group": "",
  "volumeGB": 10,
  "days": 30,
  "limitIp": 0,
  "delayStart": false,
  "autoReset": false,
  "resetDays": 0,
  "inbounds": []
}`

// createClientFromJSON accepts the template above; "volumeGB" and "days" are
// conveniences, "volume" (bytes) and "expiry" (unix seconds) also work. When
// "inbounds" is absent or empty the client is attached to every inbound.
func (b *bot) createClientFromJSON(text string) (string, error) {
	v, err := decodeJSONValue(text)
	if err != nil {
		return "", err
	}
	obj, ok := v.(map[string]interface{})
	if !ok {
		return "", errors.New("a JSON object is required")
	}
	num := func(k string) (float64, bool) {
		x, ok := obj[k]
		if !ok {
			return 0, false
		}
		n, ok := x.(json.Number)
		if !ok {
			return 0, false
		}
		f, err := n.Float64()
		return f, err == nil
	}
	gb, hasGB := num("volumeGB")
	days, hasDays := num("days")
	delete(obj, "volumeGB")
	delete(obj, "days")
	for k := range protectedClientJSON {
		delete(obj, k)
	}
	c := model.Client{Enable: true}
	if e, ok := obj["enable"]; ok {
		if eb, isBool := e.(bool); isBool {
			c.Enable = eb
		}
	}
	raw, _ := json.Marshal(obj)
	if err := json.Unmarshal(raw, &c); err != nil {
		return "", err
	}
	if !clientNameRe.MatchString(c.Name) {
		return "", b.clientAnswerErr("badName")
	}
	if b.scope != "" {
		// A limited administrator's clients all go to their group.
		g, err := b.groupForCreate(c.Group)
		if err != nil {
			return "", err
		}
		c.Group = g
	} else if c.Group != "" {
		g, err := b.normalizeGroup(c.Group)
		if err != nil {
			return "", err
		}
		c.Group = g
	}
	if rawFindClientByName(c.Name) != nil {
		return "", errors.New(b.tr("این نام قبلاً استفاده شده است.", "That name is already in use."))
	}
	if hasGB && gb > 0 {
		c.Volume = int64(gb * float64(gib))
	}
	if hasDays && days > 0 && !c.DelayStart {
		c.Expiry = time.Now().Add(time.Duration(days*24) * time.Hour).Unix()
	}
	if c.DelayStart && !c.AutoReset {
		c.Expiry = 0
	}
	if (c.DelayStart || c.AutoReset) && c.ResetDays < 1 {
		c.ResetDays = 1
	}
	if len(c.Config) == 0 || string(c.Config) == "null" {
		c.Config = newClientConfig(c.Name)
	}
	if len(c.Links) == 0 || string(c.Links) == "null" {
		c.Links = json.RawMessage("[]")
	}
	var ids []uint
	_ = json.Unmarshal(c.Inbounds, &ids)
	if len(ids) == 0 {
		if err := database.GetDB().Model(&model.Inbound{}).Order("id").Pluck("id", &ids).Error; err != nil {
			return "", err
		}
	}
	if ids == nil {
		ids = []uint{}
	}
	c.Inbounds, _ = json.Marshal(ids)
	if err := b.save("new", c); err != nil {
		return "", err
	}
	return c.Name, nil
}

// ---- bulk edit (the panel's "edit selected clients" dialog) ----

// clientGroups lists the groups the panel's clients use, sorted. The reserved
// cluster group (clients pushed to nodes) is not a hand-made group.
func (b *bot) clientGroups() []string {
	seen := map[string]bool{}
	var out []string
	for _, c := range b.loadClients() {
		if c.Group != "" && c.Group != service.ClusterGroup && !seen[c.Group] {
			seen[c.Group] = true
			out = append(out, c.Group)
		}
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i]) < strings.ToLower(out[j]) })
	return out
}

// normalizeGroup validates a typed group name. A name that only differs in
// case from an existing group reuses that group instead of making a twin.
func (b *bot) normalizeGroup(text string) (string, error) {
	g := strings.TrimSpace(text)
	switch {
	case g == "":
		return "", errors.New(b.tr("نام گروه خالی است.", "The group name is empty."))
	case utf8.RuneCountInString(g) > maxGroupRunes || strings.ContainsAny(g, "\r\n"):
		return "", errors.New(b.tr("نام گروه باید تک‌خط و حداکثر ۶۴ نویسه باشد.", "A group name is one line of at most 64 characters."))
	case strings.EqualFold(g, service.ClusterGroup):
		return "", errors.New(b.tr("این نام گروه رزرو شده است.", "That group name is reserved."))
	}
	for _, existing := range b.clientGroups() {
		if strings.EqualFold(existing, g) {
			return existing, nil
		}
	}
	return g, nil
}

// groupChoices builds the group buttons shared by the new-client wizard and
// the client editor: every existing group with its client count, plus "no
// group". prefix is the callback data the group index (or "-") is appended to.
func (b *bot) groupChoices(prefix string) [][]button {
	counts := map[string]int{}
	for _, c := range b.loadClients() {
		counts[c.Group]++
	}
	var btns []button
	for i, g := range b.clientGroups() {
		if i == 40 {
			break
		}
		btns = append(btns, button{Text: fmt.Sprintf("🏷 %s (%d)", truncate(g, 20), counts[g]), Data: prefix + strconv.Itoa(i)})
	}
	kb := rows2(btns)
	return append(kb, []button{{Text: b.tr("➖ بدون گروه", "➖ No group"), Data: prefix + "-"}})
}

// groupPicker is the group chooser of the new-client wizard and the client
// editor: the existing groups, "no group", and a button to create a new group
// (which then asks for its name). prefix is as for groupChoices.
func (b *bot) groupPicker(prefix string) [][]button {
	kb := b.groupChoices(prefix)
	kb = append(kb, []button{{Text: b.tr("➕ گروه جدید", "➕ New group"), Data: prefix + "#new"}})
	return append(kb, b.cancelRow())
}

// newGroupPrompt asks for the name of a group to create; back returns to the
// chooser it was opened from.
func (b *bot) newGroupPrompt(back string) (string, [][]button) {
	return "🏷 " + b.tr("نام گروه جدید را بفرستید (حداکثر ۶۴ نویسه). گروه با اولین کلاینتی که در آن قرار بگیرد ساخته می‌شود.", "Send the name of the new group (up to 64 characters). The group exists once a client is in it."),
		[][]button{{{Text: b.tr("⬅️ گروه‌ها", "⬅️ Groups"), Data: back}}, b.cancelRow()}
}

// groupsScreen lists the groups with their client counts; each opens the
// client list filtered to it.
func (b *bot) groupsScreen() (string, [][]button) {
	counts := map[string]int{}
	for _, c := range b.loadClients() {
		counts[c.Group]++
	}
	groups := b.clientGroups()
	lines := []string{b.header("🏷", b.tr("گروه‌ها", "Groups"))}
	var btns []button
	for i, g := range groups {
		lines = append(lines, fmt.Sprintf("• <b>%s</b> — %d", esc(g), counts[g]))
		if i < 40 {
			btns = append(btns, button{Text: fmt.Sprintf("🏷 %s (%d)", truncate(g, 20), counts[g]), Data: "c:ls:g" + strconv.Itoa(i) + ":0"})
		}
	}
	if len(groups) == 0 {
		lines = append(lines, b.tr("هنوز گروهی ساخته نشده. هنگام ساخت کلاینت یا از «✏️ ویرایش ← 🏷 گروه» می‌توانید گروه بسازید.", "No groups yet. Create one while adding a client or from ✏️ Edit → 🏷 Group."))
	}
	kb := rows2(btns)
	if n := counts[""]; n > 0 {
		lines = append(lines, fmt.Sprintf("• %s — %d", b.tr("بدون گروه", "No group"), n))
		kb = append(kb, []button{{Text: fmt.Sprintf("%s (%d)", b.tr("➖ بدون گروه", "➖ No group"), n), Data: "c:ls:u:0"}})
	}
	kb = append(kb, []button{{Text: b.tr("⬅️ کلاینت‌ها", "⬅️ Clients"), Data: "c:ls:a:0"}, {Text: "🏠", Data: "m:menu"}})
	return strings.Join(lines, "\n"), kb
}

func (b *bot) scopeName(scope string) string {
	if strings.HasPrefix(scope, "g") {
		idx, _ := strconv.Atoi(scope[1:])
		groups := b.clientGroups()
		if idx >= 0 && idx < len(groups) {
			return "🏷 " + groups[idx]
		}
		return "🏷 ?"
	}
	for _, f := range clientFilters {
		if "f"+f.code == scope {
			return b.tr(f.fa, f.en)
		}
	}
	return scope
}

// scopeClients resolves a scope ("fa", "fe", … or "g<i>") to full client rows.
func (b *bot) scopeClients(scope string) []model.Client {
	all := b.loadClients()
	var picked []model.Client
	switch {
	case strings.HasPrefix(scope, "g"):
		idx, _ := strconv.Atoi(scope[1:])
		groups := b.clientGroups()
		if idx < 0 || idx >= len(groups) {
			return nil
		}
		for _, c := range all {
			if c.Group == groups[idx] {
				picked = append(picked, c)
			}
		}
	case strings.HasPrefix(scope, "f"):
		picked = b.filterClients(scope[1:], all)
	}
	if len(picked) == 0 {
		return nil
	}
	ids := make([]uint, 0, len(picked))
	for _, c := range picked {
		ids = append(ids, c.Id)
	}
	var full []model.Client
	if err := database.GetDB().Where("id IN ?", ids).Order("id").Find(&full).Error; err != nil {
		return nil
	}
	return full
}

func (b *bot) bulkScopeScreen() (string, [][]button) {
	lines := []string{b.header("🛠", b.tr("ویرایش گروهی کلاینت‌ها", "Bulk edit clients")), b.tr("کلاینت‌های هدف را انتخاب کنید:", "Choose which clients to change:")}
	var btns []button
	for _, f := range clientFilters {
		btns = append(btns, button{Text: b.tr(f.fa, f.en), Data: "c:bk:f" + f.code})
	}
	for i, g := range b.clientGroups() {
		if i == 20 {
			break
		}
		btns = append(btns, button{Text: "🏷 " + truncate(g, 22), Data: "c:bk:g" + strconv.Itoa(i)})
	}
	kb := rows2(btns)
	if b.scope == "" {
		kb = append(kb, []button{{Text: b.tr("🔗 افزودن همه اینباندها به همه کلاینت‌ها", "🔗 Add all inbounds to all clients"), Data: "c:att"}})
	}
	kb = append(kb, b.navRow("c:ls:a:0"))
	return strings.Join(lines, "\n"), kb
}

// attachAllScreen asks before every inbound that takes clients is added to
// every client, the button the panel has in its clients tools menu.
func (b *bot) attachAllScreen() (string, [][]button) {
	head := b.header("🔗", b.tr("افزودن همه اینباندها به همه کلاینت‌ها", "Add all inbounds to all clients"))
	n, m, err := (&service.ClientService{}).AttachAllPreview()
	if err != nil {
		return head + "\n" + b.t("failed", esc(b.errText(err))), [][]button{b.navRow("c:bulk")}
	}
	if n == 0 {
		return head + "\n" + b.tr("همهٔ کلاینت‌ها از قبل همهٔ اینباندها را دارند.", "Every client already has every inbound."), [][]button{b.navRow("c:bulk")}
	}
	text := head + "\n" + b.tr(
		fmt.Sprintf("به %d کلاینت بعضی از %d اینباندی که کلاینت می‌پذیرند (اینباندهای روی نودها هم) اضافه نشده است. موارد کم‌شده اضافه می‌شود؛ اینباندهایی که کلاینت از قبل دارد دست‌نخورده می‌ماند.", n, m),
		fmt.Sprintf("%d clients lack some of the %d inbounds that take clients (inbounds hosted on nodes included). The missing ones are added; inbounds a client already has stay as they are.", n, m))
	return text, [][]button{{{Text: b.t("btnConfirm"), Data: "c:att:y"}, {Text: b.tr("✖️ انصراف", "✖️ Cancel"), Data: "c:bulk"}}}
}

func (b *bot) bulkActionsScreen(scope, note string) (string, [][]button) {
	n := len(b.scopeClients(scope))
	text := b.header("🛠", b.tr("ویرایش گروهی", "Bulk edit")) + "\n" + fmt.Sprintf("%s: <b>%s</b> (%d)", b.tr("هدف", "Target"), esc(b.scopeName(scope)), n)
	if note != "" {
		text = note + "\n\n" + text
	}
	p := "c:bk:" + scope + ":"
	kb := [][]button{
		{{Text: b.tr("📦 افزودن حجم", "📦 Add volume"), Data: p + "vol"}, {Text: b.tr("⏳ افزودن روز", "⏳ Add days"), Data: p + "days"}},
		{{Text: b.tr("📱 تعیین محدودیت IP", "📱 Set IP limit"), Data: p + "ip"}},
		{{Text: b.tr("🟢 فعال‌سازی", "🟢 Enable"), Data: p + "en"}, {Text: b.tr("🔴 غیرفعال‌سازی", "🔴 Disable"), Data: p + "dis"}},
		{{Text: b.tr("➕ افزودن اینباند", "➕ Add inbound"), Data: p + "ai"}, {Text: b.tr("➖ حذف اینباند", "➖ Remove inbound"), Data: p + "ri"}},
		{{Text: b.tr("♻️ ریست مصرف", "♻️ Reset traffic"), Data: p + "rst"}, {Text: b.tr("🗑 حذف", "🗑 Delete"), Data: p + "del"}},
		b.navRow("c:bulk"),
	}
	return text, kb
}

func parseSignedFloat(s string) (float64, bool) {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return v, err == nil && v > -1e6 && v < 1e6
}

// bulkApply runs one bulk action over the scope's clients and saves them in a
// single editbulk call, exactly like the panel does.
func (b *bot) bulkApply(scope, act, arg string) (int, error) {
	list := b.scopeClients(scope)
	if len(list) == 0 {
		return 0, errors.New(b.t("noClients"))
	}
	if act == "del" {
		ids := make([]uint, 0, len(list))
		for _, c := range list {
			ids = append(ids, c.Id)
		}
		return len(ids), b.deleteClients(ids)
	}
	now := time.Now()
	changed := list[:0]
	for _, c := range list {
		keep := true
		switch act {
		case "vol":
			gb, ok := parseSignedFloat(arg)
			if !ok {
				return 0, b.clientAnswerErr("badNumber")
			}
			nv := c.Volume + int64(gb*float64(gib))
			keep = c.Volume > 0 && nv > 0
			c.Volume = nv
		case "days":
			d, ok := parseSignedFloat(arg)
			if !ok {
				return 0, b.clientAnswerErr("badNumber")
			}
			keep = c.Expiry > 0 && !c.DelayStart
			c.Expiry += int64(d * 86400)
		case "ip":
			n, ok := parseIntArg(arg)
			if !ok {
				return 0, b.clientAnswerErr("badNumber")
			}
			c.LimitIp = n
		case "en":
			c.Enable = !isDepleted(c, now)
		case "dis":
			c.Enable = false
		case "rst":
			c.Up, c.Down = 0, 0
		case "ai", "ri":
			target, _ := strconv.ParseUint(arg, 10, 32)
			ids := clientInboundIDs(c)
			has := false
			out := make([]uint, 0, len(ids)+1)
			for _, id := range ids {
				if id == uint(target) {
					has = true
					if act == "ri" {
						continue
					}
				}
				out = append(out, id)
			}
			if act == "ai" {
				if has {
					keep = false
				} else {
					out = append(out, uint(target))
				}
			} else if !has {
				keep = false
			}
			sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
			c.Inbounds, _ = json.Marshal(out)
		default:
			return 0, errors.New("unknown action")
		}
		if keep {
			changed = append(changed, c)
		}
	}
	if len(changed) == 0 {
		return 0, nil
	}
	return len(changed), b.save("editbulk", changed)
}

// bulkCallback handles "c:bk:<scope>[:<action>[:<arg>]]".
func (b *bot) bulkCallback(ctx context.Context, cbID string, chatID, msgID int64, parts []string) {
	show := func(text string, kb [][]button) { b.edit(ctx, chatID, msgID, text, kb) }
	if len(parts) < 3 {
		b.answer(ctx, cbID, "")
		return
	}
	scope := parts[2]
	if len(parts) == 3 {
		b.answer(ctx, cbID, "")
		t, kb := b.bulkActionsScreen(scope, "")
		show(t, kb)
		return
	}
	act := parts[3]
	arg := ""
	if len(parts) > 4 {
		arg = parts[4]
	}
	back := "c:bk:" + scope
	done := func(n int, err error) {
		if err != nil {
			b.answer(ctx, cbID, b.t("failed", b.errText(err)))
			return
		}
		b.answer(ctx, cbID, b.t("done"))
		t, kb := b.bulkActionsScreen(scope, "✅ "+b.tr(fmt.Sprintf("%d کلاینت تغییر کرد", n), fmt.Sprintf("%d clients changed", n)))
		show(t, kb)
	}
	switch act {
	case "vol", "days", "ip":
		b.answer(ctx, cbID, "")
		prompts := map[string][2]string{
			"vol":  {"مقدار حجمی که اضافه شود (GB؛ منفی = کم کردن) را بفرستید. فقط کلاینت‌های دارای حجم محدود تغییر می‌کنند.", "Send the volume to add in GB (negative subtracts). Only clients with a volume limit change."},
			"days": {"تعداد روزی که اضافه شود (منفی = کم کردن) را بفرستید. فقط کلاینت‌های دارای تاریخ انقضا تغییر می‌کنند.", "Send the days to add (negative subtracts). Only clients with an expiry date change."},
			"ip":   {"محدودیت جدید IP را بفرستید (۰ = نامحدود).", "Send the new IP limit (0 = unlimited)."},
		}
		prompt := b.tr(prompts[act][0], prompts[act][1])
		if line := b.quotaLine(); act == "vol" && line != "" {
			prompt += "\n\n" + line
		}
		b.ask(ctx, chatID, msgID, "bk."+act, scope, 0, back, prompt)
	case "en", "dis", "rst", "del":
		b.answer(ctx, cbID, "")
		if arg == "y" {
			n, err := b.bulkApply(scope, act, "")
			done(n, err)
			return
		}
		n := len(b.scopeClients(scope))
		label := map[string][2]string{
			"en":  {"فعال شوند؟", "be enabled?"},
			"dis": {"غیرفعال شوند؟", "be disabled?"},
			"rst": {"مصرفشان ریست شود؟", "have their traffic reset?"},
			"del": {"حذف شوند؟", "be deleted?"},
		}[act]
		show(fmt.Sprintf("⚠️ %s <b>%s</b> (%d) %s", b.tr("کلاینت‌های", "Clients in"), esc(b.scopeName(scope)), n, b.tr(label[0], label[1])),
			[][]button{{{Text: b.t("btnConfirm"), Data: back + ":" + act + ":y"}, {Text: b.tr("✖️ انصراف", "✖️ Cancel"), Data: back}}})
	case "ai", "ri":
		if arg != "" {
			n, err := b.bulkApply(scope, act, arg)
			done(n, err)
			return
		}
		b.answer(ctx, cbID, "")
		var inbounds []model.Inbound
		_ = database.GetDB().Select("id", "tag").Order("id").Find(&inbounds).Error
		var btns []button
		for i, in := range inbounds {
			if i == 40 {
				break
			}
			btns = append(btns, button{Text: truncate(in.Tag, 24), Data: back + ":" + act + ":" + sid(in.Id)})
		}
		title := b.tr("اینباندی را که به همه کلاینت‌های هدف اضافه شود انتخاب کنید:", "Pick the inbound to add to all target clients:")
		if act == "ri" {
			title = b.tr("اینباندی را که از همه کلاینت‌های هدف حذف شود انتخاب کنید:", "Pick the inbound to remove from all target clients:")
		}
		show(b.header("🛠", b.scopeName(scope))+"\n"+title, append(rows2(btns), b.navRow(back)))
	default:
		b.answer(ctx, cbID, "")
	}
}

func (b *bot) bulkPending(ctx context.Context, chatID int64, p *pending, text string, retry func(error), finish func(string, [][]button)) {
	act := strings.TrimPrefix(p.kind, "bk.")
	n, err := b.bulkApply(p.key, act, text)
	if err != nil {
		retry(err)
		return
	}
	t, kb := b.bulkActionsScreen(p.key, "✅ "+b.tr(fmt.Sprintf("%d کلاینت تغییر کرد", n), fmt.Sprintf("%d clients changed", n)))
	finish(t, kb)
}

// ---- callbacks for the screens above ----

// clientCallbackExt handles the callbacks of this file; it returns false for
// everything else so clientCallback can carry on.
func (b *bot) clientCallbackExt(ctx context.Context, cbID string, chatID, msgID int64, parts []string) bool {
	action := parts[1]
	show := func(text string, kb [][]button) { b.edit(ctx, chatID, msgID, text, kb) }
	switch action {
	case "bulk":
		b.answer(ctx, cbID, "")
		t, kb := b.bulkScopeScreen()
		show(t, kb)
		return true
	case "bk":
		b.bulkCallback(ctx, cbID, chatID, msgID, parts)
		return true
	case "grps":
		b.answer(ctx, cbID, "")
		t, kb := b.groupsScreen()
		show(t, kb)
		return true
	case "att":
		if b.scope != "" {
			// It would reach every client of the panel.
			b.answer(ctx, cbID, b.t("scopeDenied"))
			return true
		}
		if len(parts) > 2 && parts[2] == "y" {
			n, _, err := (&service.ClientService{}).AttachAllPreview()
			if err == nil {
				err = b.save("attachall", struct{}{})
			}
			if err != nil {
				b.answer(ctx, cbID, b.t("failed", b.errText(err)))
				return true
			}
			b.answer(ctx, cbID, b.t("done"))
			show("✅ "+b.tr(fmt.Sprintf("اینباندهای کم‌شده به %d کلاینت اضافه شد.", n), fmt.Sprintf("Added the missing inbounds to %d clients.", n)), [][]button{b.navRow("c:bulk")})
			return true
		}
		b.answer(ctx, cbID, "")
		t, kb := b.attachAllScreen()
		show(t, kb)
		return true
	case "sort":
		next := "desc"
		if clientsNewestFirst() {
			next = "asc"
		}
		if err := (&service.SettingService{}).SetTgBotClientSort(next); err != nil {
			b.answer(ctx, cbID, b.t("failed", b.errText(err)))
			return true
		}
		b.answer(ctx, cbID, "")
		filter := "a"
		if len(parts) > 2 {
			filter = parts[2]
		}
		t, kb := b.clientsScreen(filter, 0)
		show(t, kb)
		return true
	case "newj":
		b.answer(ctx, cbID, "")
		b.pend.set(chatID, &pending{kind: "cl.newj", msgID: msgID, back: "c:ls:a:0", data: map[string]string{}})
		tpl := strings.Replace(newClientTemplate, `"user1"`, `"`+b.randomClientName()+`"`, 1)
		if b.scope != "" {
			g, _ := json.Marshal(b.scopeGroup())
			tpl = strings.Replace(tpl, `"group": ""`, `"group": `+string(g), 1)
		}
		show(b.header("➕", b.tr("کلاینت جدید با JSON", "New client from JSON"))+"\n"+
			b.tr("این قالب را ویرایش و بفرستید (متن یا فایل .json). فیلدهای حذف‌شده مقدار پیش‌فرض می‌گیرند؛ اگر inbounds خالی باشد به همه اینباندها وصل می‌شود.", "Edit this template and send it back (text or .json file). Omitted fields use defaults; an empty inbounds list attaches the client to every inbound.")+
			"\n<pre>"+esc(tpl)+"</pre>", [][]button{b.cancelRow()})
		return true
	case "edit", "cfg", "cfga", "cfgp", "xl", "xld", "json", "dly", "ar", "sg":
	default:
		return false
	}
	if len(parts) < 3 {
		b.answer(ctx, cbID, "")
		return true
	}
	id64, err := strconv.ParseUint(parts[2], 10, 32)
	if err != nil {
		b.answer(ctx, cbID, "")
		return true
	}
	id := uint(id64)
	client := b.clientByID(id)
	if client == nil {
		b.answer(ctx, cbID, b.t("notFound"))
		return true
	}
	mutate := func(err error, after func()) {
		if err != nil {
			b.answer(ctx, cbID, b.t("failed", b.errText(err)))
			return
		}
		b.answer(ctx, cbID, b.t("done"))
		after()
	}
	reload := func(f func(model.Client) (string, [][]button)) func() {
		return func() {
			if c := b.clientByID(id); c != nil {
				t, kb := f(*c)
				show(t, kb)
			}
		}
	}
	idx := -1
	if len(parts) > 3 {
		idx, _ = strconv.Atoi(parts[3])
	}
	switch action {
	case "edit":
		b.answer(ctx, cbID, "")
		t, kb := b.clientEditScreen(*client)
		show(t, kb)
	case "cfg":
		b.answer(ctx, cbID, "")
		t, kb := b.clientConfigScreen(*client)
		show(t, kb)
	case "cfga":
		mutate(b.shuffleClientConfig(id, -1), reload(b.clientConfigScreen))
	case "cfgp":
		mutate(b.shuffleClientConfig(id, idx), reload(b.clientConfigScreen))
	case "xl":
		b.answer(ctx, cbID, "")
		t, kb := b.clientLinksScreen(*client)
		show(t, kb)
	case "xld":
		mutate(b.deleteClientLink(id, idx), reload(b.clientLinksScreen))
	case "sg":
		name := ""
		if len(parts) < 4 {
			b.answer(ctx, cbID, "")
			return true
		}
		if b.scope != "" {
			b.answer(ctx, cbID, b.t("scopeDenied"))
			return true
		}
		if parts[3] == "#new" {
			// The router drops a pending input on any navigation; arm it again
			// so the next text message is the new group's name.
			b.answer(ctx, cbID, "")
			b.pend.set(chatID, &pending{kind: "cl.grp", key: "grp", id: id, msgID: msgID, back: "c:view:" + sid(id), data: map[string]string{}})
			text, kb := b.newGroupPrompt("c:ask:grp:" + sid(id))
			show(text, kb)
			return true
		}
		if parts[3] != "-" {
			groups := b.clientGroups()
			if idx < 0 || idx >= len(groups) {
				b.answer(ctx, cbID, b.t("notFound"))
				return true
			}
			name = groups[idx]
		}
		b.pend.clear(chatID)
		mutate(b.editClient(id, func(c *model.Client) error { c.Group = name; return nil }), func() {
			if c := b.clientByID(id); c != nil {
				t, kb := b.card(*c)
				show(t, kb)
			}
		})
	case "dly":
		mutate(b.toggleDelayStart(id), reload(b.clientEditScreen))
	case "ar":
		mutate(b.toggleAutoReset(id), reload(b.clientEditScreen))
	case "json":
		b.answer(ctx, cbID, "")
		full, err := b.fullClient(id)
		if err != nil {
			b.fail(ctx, chatID, err)
			return true
		}
		view := map[string]interface{}{}
		raw, _ := json.Marshal(full)
		_ = json.Unmarshal(raw, &view)
		for _, k := range []string{"links", "id", "up", "down", "totalUp", "totalDown", "tgId", "createdAt", "onlineAt", "nextReset"} {
			delete(view, k)
		}
		b.sendJSON(ctx, chatID, "client-"+full.Name, view)
		b.ask(ctx, chatID, msgID, "cl.json", "json", id, "c:view:"+sid(id), b.tr("JSON ویرایش‌شده را بفرستید (متن یا فایل .json). فقط فیلدهای ارسالی تغییر می‌کنند؛ مصرف و شناسه‌ها محافظت می‌شوند.", "Send the edited JSON (text or .json file). Only the fields you send change; usage and IDs are protected."))
	}
	return true
}
