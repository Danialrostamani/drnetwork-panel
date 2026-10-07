package tgbot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/service"
	"github.com/Danialrostamani/drnetwork-panel/util"
)

type kindInfo struct {
	code, obj, icon, fa, en string
}

var kindList = []kindInfo{
	{"in", "inbounds", "📡", "اینباندها", "Inbounds"},
	{"out", "outbounds", "📤", "اوت‌باندها", "Outbounds"},
	{"ep", "endpoints", "🔌", "اندپوینت‌ها", "Endpoints"},
	{"sv", "services", "🛠", "سرویس‌ها", "Services"},
	{"tl", "tls", "🔐", "TLS", "TLS"},
	{"nd", "nodes", "🖥", "نودها", "Nodes"},
}

func kindByCode(code string) *kindInfo {
	for i := range kindList {
		if kindList[i].code == code {
			return &kindList[i]
		}
	}
	return nil
}

func (k *kindInfo) title(b *bot) string { return b.tr(k.fa, k.en) }

type objItem struct {
	ID        uint
	Tag, Type string
	Sub       string
	Icon      string
}

// normalize turns any service value into a plain JSON map; numbers stay exact.
func normalize(v interface{}) map[string]interface{} {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var m map[string]interface{}
	if dec.Decode(&m) != nil {
		return nil
	}
	return m
}

func scalar(v interface{}) string {
	switch x := v.(type) {
	case nil:
		return ""
	case json.Number:
		return x.String()
	case string:
		return x
	case bool:
		return strconv.FormatBool(x)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	}
	return ""
}

func idOf(m map[string]interface{}) uint {
	n, _ := strconv.ParseUint(scalar(m["id"]), 10, 32)
	return uint(n)
}

func (b *bot) objRows(k *kindInfo) ([]map[string]interface{}, error) {
	var out []map[string]interface{}
	add := func(v interface{}) {
		if m := normalize(v); m != nil {
			out = append(out, m)
		}
	}
	switch k.code {
	case "in":
		rows, err := (&service.InboundService{}).GetAll()
		if err != nil {
			return nil, err
		}
		for _, r := range *rows {
			add(r)
		}
	case "out":
		rows, err := (&service.OutboundService{}).GetAll()
		if err != nil {
			return nil, err
		}
		for _, r := range *rows {
			add(r)
		}
	case "ep":
		rows, err := (&service.EndpointService{}).GetAll()
		if err != nil {
			return nil, err
		}
		for _, r := range *rows {
			add(r)
		}
	case "sv":
		rows, err := (&service.ServicesService{}).GetAll()
		if err != nil {
			return nil, err
		}
		for _, r := range *rows {
			add(r)
		}
	case "tl":
		rows, err := (&service.TlsService{}).GetAll()
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			add(r)
		}
	case "nd":
		rows, err := (&service.NodeService{}).GetAll()
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			add(r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return idOf(out[i]) < idOf(out[j]) })
	return out, nil
}

func (b *bot) objItems(k *kindInfo) ([]objItem, error) {
	rows, err := b.objRows(k)
	if err != nil {
		return nil, err
	}
	var onlineIn map[string]bool
	var nodeStatus map[uint]service.NodeStatus
	switch k.code {
	case "in":
		onlineIn = map[string]bool{}
		if o, err := (&service.StatsService{}).GetClusterOnlines(); err == nil {
			for _, t := range o.Inbound {
				onlineIn[t] = true
			}
		}
	case "nd":
		nodeStatus = (&service.NodeService{}).GetStatuses()
	}
	items := make([]objItem, 0, len(rows))
	for _, m := range rows {
		it := objItem{ID: idOf(m), Tag: scalar(m["tag"]), Type: scalar(m["type"]), Icon: k.icon}
		switch k.code {
		case "in":
			it.Icon = "⚪️"
			if onlineIn[it.Tag] {
				it.Icon = "🟢"
			}
			if p := scalar(m["listen_port"]); p != "" {
				it.Sub = ":" + p
			}
			if scalar(m["tls_id"]) != "" && scalar(m["tls_id"]) != "0" {
				it.Sub += " 🔒"
			}
			if m["node_id"] != nil {
				it.Sub += " ↗️"
			}
			if u, ok := m["users"].([]interface{}); ok {
				it.Sub += fmt.Sprintf(" 👥%d", len(u))
			}
		case "out", "sv", "ep":
			if s := scalar(m["server"]); s != "" {
				it.Sub = s
				if p := scalar(m["server_port"]); p != "" {
					it.Sub += ":" + p
				}
			} else if p := scalar(m["listen_port"]); p != "" {
				it.Sub = ":" + p
			}
		case "tl":
			it.Tag = scalar(m["name"])
			it.Type = "tls"
			srv := asMap(m["server"])
			if r := asMap(srv["reality"]); r["enabled"] == true {
				it.Type = "reality"
			} else if srv["acme"] != nil {
				it.Type = "acme"
			}
			it.Sub = scalar(srv["server_name"])
		case "nd":
			it.Tag = scalar(m["name"])
			it.Type = "node"
			it.Sub = scalar(m["baseUrl"])
			it.Icon = "🔴"
			switch {
			case m["enable"] == false:
				it.Icon = "⚪️"
			case nodeStatus[it.ID].State == "online":
				it.Icon = "🟢"
			case nodeStatus[it.ID].State == "core-stopped":
				it.Icon = "🟠"
			}
		}
		items = append(items, it)
	}
	return items, nil
}

const objPageSize = 10

func (b *bot) objListScreen(k *kindInfo, page int) (string, [][]button) {
	items, err := b.objItems(k)
	if err != nil {
		return b.t("failed", esc(err.Error())), [][]button{b.menuRow()}
	}
	from, to, page, pages := pageSlice(len(items), page, objPageSize)
	lines := []string{b.header(k.icon, fmt.Sprintf("%s (%d)", k.title(b), len(items)))}
	if len(items) == 0 {
		lines = append(lines, b.tr("موردی تعریف نشده است.", "Nothing defined yet."))
	}
	var btns []button
	for _, it := range items[from:to] {
		line := fmt.Sprintf("%s <b>%s</b> — %s", it.Icon, esc(it.Tag), esc(it.Type))
		if it.Sub != "" {
			line += " " + esc(it.Sub)
		}
		lines = append(lines, line)
		btns = append(btns, button{Text: it.Icon + " " + truncate(it.Tag, 22), Data: fmt.Sprintf("o:%s:v:%d", k.code, it.ID)})
	}
	kb := rows2(btns)
	if pg := pager("o:"+k.code+":ls", page, pages); pg != nil {
		kb = append(kb, pg)
	}
	actions := []button{{Text: b.tr("➕ جدید", "➕ New"), Data: "o:" + k.code + ":n:0"}}
	if k.code == "nd" {
		actions = append(actions, button{Text: b.tr("🔁 همگام‌سازی همه", "🔁 Sync all"), Data: "m:sync"}, button{Text: b.tr("🩺 بررسی", "🩺 Probe"), Data: "o:nd:probe:0"})
	}
	kb = append(kb, actions, b.menuRow())
	return strings.Join(lines, "\n"), kb
}

func (b *bot) objFull(k *kindInfo, id uint) (map[string]interface{}, error) {
	if k.code == "in" {
		rows, err := (&service.InboundService{}).Get(strconv.FormatUint(uint64(id), 10))
		if err != nil {
			return nil, err
		}
		if rows != nil && len(*rows) > 0 {
			return normalize((*rows)[0]), nil
		}
		return nil, fmt.Errorf("%s", b.t("notFound"))
	}
	rows, err := b.objRows(k)
	if err != nil {
		return nil, err
	}
	for _, m := range rows {
		if idOf(m) == id {
			return m, nil
		}
	}
	return nil, fmt.Errorf("%s", b.t("notFound"))
}

var secretKeys = []string{"password", "private_key", "token", "secret", "psk", "auth_str", "pre_shared"}

func isSecretKey(k string) bool {
	lk := strings.ToLower(k)
	for _, s := range secretKeys {
		if strings.Contains(lk, s) {
			return true
		}
	}
	return false
}

// summarize lists the top-level fields of a config object, one per line.
func summarize(m map[string]interface{}, skip ...string) []string {
	skipSet := map[string]bool{"id": true, "tag": true, "type": true, "out_json": true, "name": false}
	for _, s := range skip {
		skipSet[s] = true
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		if !skipSet[k] {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	var lines []string
	for _, k := range keys {
		if len(lines) >= 18 {
			lines = append(lines, "…")
			break
		}
		v := m[k]
		var val string
		switch x := v.(type) {
		case map[string]interface{}:
			val = fmt.Sprintf("{%d}", len(x))
		case []interface{}:
			simple := true
			parts := make([]string, 0, len(x))
			for _, e := range x {
				s := scalar(e)
				if s == "" {
					simple = false
					break
				}
				parts = append(parts, s)
			}
			if simple && len(x) > 0 {
				val = truncate(strings.Join(parts, ", "), 60)
			} else {
				val = fmt.Sprintf("[%d]", len(x))
			}
		default:
			val = scalar(v)
			if val == "" {
				continue
			}
			if isSecretKey(k) {
				val = "••••"
			}
		}
		lines = append(lines, fmt.Sprintf("▫️ %s: <code>%s</code>", esc(k), esc(truncate(val, 60))))
	}
	return lines
}

func (b *bot) objCard(k *kindInfo, id uint) (string, [][]button) {
	m, err := b.objFull(k, id)
	if err != nil {
		return b.t("failed", esc(err.Error())), [][]button{b.navRow("o:" + k.code + ":ls:0")}
	}
	sid := strconv.FormatUint(uint64(id), 10)
	name := scalar(m["tag"])
	if name == "" {
		name = scalar(m["name"])
	}
	head := fmt.Sprintf("%s <b>%s</b>", k.icon, esc(name))
	if t := scalar(m["type"]); t != "" {
		head += " · " + esc(t)
	}
	lines := []string{head, rule}
	pre := "o:" + k.code + ":"
	var actions [][]button
	switch k.code {
	case "in":
		skip := []string{"users", "addrs"}
		if u, ok := m["users"].([]interface{}); ok {
			lines = append(lines, fmt.Sprintf("👥 %s: %d", b.tr("کلاینت‌ها", "Clients"), len(u)))
		}
		lines = append(lines, summarize(m, skip...)...)
		actions = append(actions, []button{{Text: b.tr("🔢 پورت", "🔢 Port"), Data: pre + "port:" + sid}, {Text: b.tr("👥 کلاینت‌ها", "👥 Clients"), Data: pre + "cl:" + sid}})
	case "out":
		lines = append(lines, summarize(m)...)
		actions = append(actions, []button{{Text: b.tr("⚡ تست تأخیر", "⚡ Latency test"), Data: pre + "test:" + sid}})
	case "nd":
		lines = append(lines, b.nodeLines(id, m)...)
		tog := b.tr("⛔ غیرفعال", "⛔ Disable")
		if m["enable"] == false {
			tog = b.tr("✅ فعال", "✅ Enable")
		}
		actions = append(actions, []button{{Text: tog, Data: pre + "tog:" + sid}, {Text: b.tr("🔁 همگام‌سازی", "🔁 Sync"), Data: pre + "sync:" + sid}, {Text: b.tr("🩺 تست", "🩺 Test"), Data: pre + "probe:" + sid}})
		if m["enable"] != false {
			more := []button{{Text: b.tr("♻️ ری‌استارت هسته", "♻️ Restart core"), Data: pre + "rsb:" + sid}}
			if b.can("backup") {
				more = append(more, button{Text: b.tr("💾 بکاپ", "💾 Backup"), Data: pre + "bk:" + sid})
			}
			actions = append(actions, more)
		}
	default:
		lines = append(lines, summarize(m)...)
	}
	kb := append(actions,
		[]button{{Text: "📄 JSON", Data: pre + "j:" + sid}, {Text: b.tr("✏️ ویرایش", "✏️ Edit"), Data: pre + "e:" + sid}, {Text: b.tr("🗑 حذف", "🗑 Delete"), Data: pre + "d:" + sid}},
		[]button{{Text: b.tr("⬅️ فهرست", "⬅️ List"), Data: pre + "ls:0"}, {Text: "🏠", Data: "m:menu"}})
	return strings.Join(lines, "\n"), kb
}

func (b *bot) nodeLines(id uint, m map[string]interface{}) []string {
	st := (&service.NodeService{}).GetStatuses()[id]
	state := map[string]string{"online": "🟢 " + b.t("nodeOnline"), "core-stopped": "🟠 " + b.t("nodeCore")}[st.State]
	if m["enable"] == false {
		state = "⚪️ " + b.t("disabled")
	} else if state == "" {
		state = "🔴 " + b.t("nodeOffline")
	}
	if st.Maintenance && m["enable"] != false {
		state += " · 🛠 " + b.tr("حالت تعمیر", "maintenance")
	}
	lines := []string{state, "🔗 <code>" + esc(scalar(m["baseUrl"])+scalar(m["webPath"])) + "</code>"}
	var meta []string
	if flag := flagEmoji(scalar(m["country"])); flag != "" {
		meta = append(meta, flag)
	}
	if tags := stringList(m["tags"]); len(tags) > 0 {
		meta = append(meta, "🏷 "+esc(strings.Join(tags, ", ")))
	}
	if len(meta) > 0 {
		lines = append(lines, strings.Join(meta, " · "))
	}
	if m["enable"] != false && (st.State == "online" || st.State == "core-stopped") {
		lines = append(lines,
			fmt.Sprintf("🧠 CPU %s %.0f%%", bar(int(st.Cpu), 10), st.Cpu),
			fmt.Sprintf("💾 RAM %s %d%%", bar(pctOf(uint64(max(st.Mem.Current, 0)), uint64(max(st.Mem.Total, 0))), 10), pctOf(uint64(max(st.Mem.Current, 0)), uint64(max(st.Mem.Total, 0)))))
		if st.Disk.Total > 0 {
			p := pctOf(uint64(max(st.Disk.Current, 0)), uint64(st.Disk.Total))
			lines = append(lines, fmt.Sprintf("🗄 %s %s %d%%", b.tr("دیسک", "Disk"), bar(p, 10), p))
		}
		version := st.AppFull
		if version == "" {
			version = st.AppVersion
		}
		lines = append(lines, fmt.Sprintf("⏱ %d ms · v%s · core %s", st.Latency, esc(version), esc(st.CoreVersion)))
		lines = append(lines, fmt.Sprintf("👥 %s: %d · ⬆️ %s/s ⬇️ %s/s", b.tr("آنلاین", "Online"), st.Online, humanBytes(st.NetUp), humanBytes(st.NetDown)))
	} else if st.Error != "" && m["enable"] != false {
		lines = append(lines, "⚠️ "+esc(st.Error))
	}
	if st.Uptime24 >= 0 || st.Uptime7d >= 0 {
		lines = append(lines, fmt.Sprintf("📈 %s: 24h %s · 7d %s", b.tr("آپتایم", "Uptime"), uptimeText(st.Uptime24), uptimeText(st.Uptime7d)))
	}
	if t := st.Traffic; t != nil {
		lines = append(lines, fmt.Sprintf("📦 %s %s · %s %s", b.tr("امروز", "Today"), humanBytes(t.TodayUp+t.TodayDown), b.tr("این ماه", "This month"), humanBytes(t.MonthUp+t.MonthDown)))
		if t.CapLimit > 0 {
			p := int(percentOfInt(t.CapUsed, t.CapLimit))
			lines = append(lines, fmt.Sprintf("🎚 %s %s %d%% · %s / %s · %s %s", b.tr("سقف", "Cap"), bar(p, 10), p, humanBytes(t.CapUsed), humanBytes(t.CapLimit), b.tr("ریست", "resets"), b.stamp(t.CapEnd)))
		}
	}
	switch st.Hidden {
	case "down":
		lines = append(lines, "🙈 "+b.tr("لینک‌ها به‌خاطر قطعی از سابسکریپشن‌ها برداشته شده‌اند", "Links taken out of the subscriptions while it is down"))
	case "cap":
		lines = append(lines, "🙈 "+b.tr("لینک‌ها به‌خاطر پر شدن سقف از سابسکریپشن‌ها برداشته شده‌اند", "Links taken out of the subscriptions: the cap is reached"))
	case "filtered":
		lines = append(lines, "🙈 "+b.tr("لینک‌ها به‌خاطر فیلتر از سابسکریپشن‌ها برداشته شده‌اند", "Links taken out of the subscriptions: filtered in Iran"))
	}
	if st.Filtered && st.Hidden != "filtered" {
		lines = append(lines, "🚫 "+b.tr("از ایران در دسترس نیست (احتمال فیلتر)", "Not reachable from Iran (likely filtered)"))
	}
	for _, w := range st.Warnings {
		lines = append(lines, "⚠️ "+b.warningShort(w))
	}
	if ls := int64(toFloat(jsonFloat(m["lastSeen"]))); ls > 0 {
		lines = append(lines, "👁 "+b.t("lastSeen")+": "+b.stamp(ls))
	}
	if sync := int64(toFloat(jsonFloat(m["lastSync"]))); sync > 0 {
		lines = append(lines, "🔁 "+b.tr("آخرین همگام‌سازی", "Last sync")+": "+b.stamp(sync))
	}
	if m["dirty"] == true {
		lines = append(lines, "⏳ "+b.tr("تغییرات در انتظار همگام‌سازی", "Changes waiting to sync"))
	}
	if desc := scalar(m["desc"]); desc != "" {
		lines = append(lines, "📝 "+esc(desc))
	}
	return lines
}

// warningShort is one threshold warning on a node card.
func (b *bot) warningShort(w service.NodeWarning) string {
	switch w.Key {
	case "cpu":
		return fmt.Sprintf("CPU %.0f%% ≥ %.0f%%", w.Value, w.Limit)
	case "mem":
		return fmt.Sprintf("RAM %.0f%% ≥ %.0f%%", w.Value, w.Limit)
	case "disk":
		return fmt.Sprintf("%s %.0f%% ≥ %.0f%%", b.tr("دیسک", "Disk"), w.Value, w.Limit)
	case "ping":
		return fmt.Sprintf("%s %.0f ms ≥ %.0f ms", b.tr("پینگ", "Ping"), w.Value, w.Limit)
	case "cert":
		if w.Value <= 0 {
			return b.tr("گواهی TLS منقضی شده", "TLS certificate expired")
		}
		return fmt.Sprintf(b.tr("گواهی TLS: %.0f روز مانده", "TLS certificate: %.0f days left"), w.Value)
	case "version":
		return fmt.Sprintf(b.tr("نسخه قدیمی: %s", "Old version: %s"), esc(w.Info))
	}
	return esc(w.Key)
}

func uptimeText(p float64) string {
	if p < 0 {
		return "—"
	}
	if p >= 99.95 && p < 100 {
		return "99.9%"
	}
	return strconv.FormatFloat(p, 'f', 1, 64) + "%"
}

func percentOfInt(cur, total int64) float64 {
	if total <= 0 {
		return 0
	}
	p := float64(cur) * 100 / float64(total)
	if p < 0 {
		return 0
	}
	return p
}

// flagEmoji turns a two-letter country code into its flag.
func flagEmoji(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	if len(code) != 2 || code[0] < 'A' || code[0] > 'Z' || code[1] < 'A' || code[1] > 'Z' {
		return ""
	}
	return string([]rune{rune(code[0]) - 'A' + 0x1F1E6, rune(code[1]) - 'A' + 0x1F1E6})
}

func stringList(v interface{}) []string {
	var out []string
	switch x := v.(type) {
	case []string:
		out = append(out, x...)
	case []interface{}:
		for _, e := range x {
			if s, ok := e.(string); ok && s != "" {
				out = append(out, s)
			}
		}
	}
	return out
}

func jsonFloat(v interface{}) interface{} {
	if n, ok := v.(json.Number); ok {
		f, _ := n.Float64()
		return f
	}
	return v
}

// prettyJSON is the editable JSON of an object.
func (b *bot) objEditable(k *kindInfo, id uint) (map[string]interface{}, error) {
	m, err := b.objFull(k, id)
	if err != nil {
		return nil, err
	}
	delete(m, "out_json")
	delete(m, "users")
	delete(m, "tokenSet")
	delete(m, "lastSeen")
	delete(m, "lastSync")
	delete(m, "dirty")
	delete(m, "inboundCount")
	delete(m, "clientCount")
	delete(m, "syncReport")
	return m, nil
}

func (b *bot) sendJSON(ctx context.Context, chatID int64, name string, v interface{}) {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		b.fail(ctx, chatID, err)
		return
	}
	if len([]rune(string(raw))) <= 3300 {
		b.send(ctx, chatID, "<pre>"+esc(string(raw))+"</pre>")
		return
	}
	if err := b.upload(ctx, "sendDocument", "document", chatID, name+".json", "", raw); err != nil {
		b.fail(ctx, chatID, err)
	}
}

// ---- saving ----

func (b *bot) saveObj(k *kindInfo, act string, data interface{}) error {
	if b.configService == nil {
		return fmt.Errorf("config service unavailable")
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	if _, err := b.configService.Save(k.obj, act, raw, "", b.actor(), b.host()); err != nil {
		return err
	}
	if k.code == "in" || k.code == "tl" {
		b.fanOut()
	}
	return nil
}

func (b *bot) deleteObj(k *kindInfo, id uint) error {
	m, err := b.objFull(k, id)
	if err != nil {
		return err
	}
	if k.code == "tl" || k.code == "nd" {
		return b.saveObj(k, "del", id)
	}
	return b.saveObj(k, "del", scalar(m["tag"]))
}

func stripFence(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```")
		if i := strings.IndexByte(s, '\n'); i >= 0 && !strings.HasPrefix(strings.TrimSpace(s[:i]), "{") {
			s = s[i+1:]
		}
		s = strings.TrimSuffix(strings.TrimSpace(s), "```")
	}
	return strings.TrimSpace(s)
}

// parseObjectInput reads the JSON an administrator typed or uploaded. An
// outbound can also be given as a share link (vless://, ss://, ...).
func parseObjectInput(k *kindInfo, text string) (map[string]interface{}, error) {
	text = stripFence(text)
	if k.code == "out" && !strings.HasPrefix(text, "{") && strings.Contains(text, "://") {
		m, _, err := util.GetOutbound(strings.TrimSpace(text), 0)
		if err != nil {
			return nil, err
		}
		return normalize(*m), nil
	}
	dec := json.NewDecoder(strings.NewReader(text))
	dec.UseNumber()
	var m map[string]interface{}
	if err := dec.Decode(&m); err != nil {
		return nil, err
	}
	return m, nil
}

func (b *bot) objPortEdit(id uint, port string) error {
	n, err := strconv.Atoi(strings.TrimSpace(port))
	if err != nil || n < 1 || n > 65535 {
		return fmt.Errorf("%s", b.t("badNumber"))
	}
	k := kindByCode("in")
	m, err := b.objEditable(k, id)
	if err != nil {
		return err
	}
	m["listen_port"] = n
	return b.saveObj(k, "edit", m)
}

func (b *bot) inboundClients(id uint) []string {
	var names []string
	_ = database.GetDB().Raw("SELECT clients.name FROM clients, json_each(clients.inbounds) AS je WHERE je.value = ? ORDER BY clients.name", id).Scan(&names).Error
	return names
}

// ---- templates for "new" ----

var templates = map[string]map[string]string{
	"in": {
		"mixed":       `{"type":"mixed","tag":"mixed-in","listen":"::","listen_port":2080,"tls_id":0}`,
		"vless":       `{"type":"vless","tag":"vless-in","listen":"::","listen_port":8443,"tls_id":0}`,
		"vmess":       `{"type":"vmess","tag":"vmess-in","listen":"::","listen_port":8444,"tls_id":0}`,
		"trojan":      `{"type":"trojan","tag":"trojan-in","listen":"::","listen_port":8445,"tls_id":0}`,
		"hysteria2":   `{"type":"hysteria2","tag":"hy2-in","listen":"::","listen_port":8446,"tls_id":0}`,
		"tuic":        `{"type":"tuic","tag":"tuic-in","listen":"::","listen_port":8447,"tls_id":0}`,
		"shadowsocks": `{"type":"shadowsocks","tag":"ss-in","listen":"::","listen_port":8388,"method":"2022-blake3-aes-128-gcm","password":"%PW16%","tls_id":0}`,
		"direct":      `{"type":"direct","tag":"direct-in","listen":"::","listen_port":8080,"tls_id":0}`,
	},
	"out": {
		"direct":      `{"type":"direct","tag":"direct-out"}`,
		"socks":       `{"type":"socks","tag":"socks-out","server":"1.2.3.4","server_port":1080}`,
		"http":        `{"type":"http","tag":"http-out","server":"1.2.3.4","server_port":8080}`,
		"shadowsocks": `{"type":"shadowsocks","tag":"ss-out","server":"1.2.3.4","server_port":8388,"method":"aes-128-gcm","password":"password"}`,
		"selector":    `{"type":"selector","tag":"select","outbounds":["direct"]}`,
		"urltest":     `{"type":"urltest","tag":"auto","outbounds":["direct"]}`,
	},
	"ep": {
		"wireguard": `{"type":"wireguard","tag":"wg-ep","address":["10.0.0.2/32"],"private_key":"","peers":[{"address":"1.2.3.4","port":51820,"public_key":"","allowed_ips":["0.0.0.0/0"]}],"ext":{}}`,
	},
	"sv": {},
	"tl": {
		"tls": `{"name":"my-tls","server":{"enabled":true,"server_name":"example.com","certificate_path":"/path/fullchain.pem","key_path":"/path/privkey.pem"},"client":{}}`,
	},
	"nd": {
		"node": `{"name":"node1","baseUrl":"https://1.2.3.4:2095","webPath":"/app/","token":"API_TOKEN","enable":true,"insecure":false}`,
	},
}

func templateJSON(kind, name string) string {
	t := templates[kind][name]
	if strings.Contains(t, "%PW16%") {
		t = strings.ReplaceAll(t, "%PW16%", randomBase64(16))
	}
	return t
}

func (b *bot) newObjScreen(k *kindInfo) (string, [][]button) {
	names := make([]string, 0, len(templates[k.code]))
	for n := range templates[k.code] {
		names = append(names, n)
	}
	sort.Strings(names)
	var btns []button
	for _, n := range names {
		btns = append(btns, button{Text: n, Data: "o:" + k.code + ":nt:" + n})
	}
	text := b.header(k.icon, b.tr("جدید — ", "New — ")+k.title(b))
	hint := b.tr("یک قالب انتخاب کنید یا مستقیم JSON کامل را بفرستید (متن یا فایل ‎.json).", "Pick a template, or send the full JSON directly (text or a .json file).")
	if k.code == "out" {
		hint += "\n" + b.tr("لینک اشتراکی (vless://، ss://، ...) هم پذیرفته می‌شود.", "A share link (vless://, ss://, ...) is accepted too.")
	}
	if k.code == "in" {
		hint += "\n" + b.tr("پس از ساخت، کلاینت‌ها را از کارت هر کلاینت ← اینباندها به آن اضافه کنید.", "After creating, attach clients from each client card → Inbounds.")
	}
	kb := rows2(btns)
	kb = append(kb, b.navRow("o:"+k.code+":ls:0"))
	return text + "\n" + hint, kb
}

func (b *bot) objCallback(ctx context.Context, cbID string, chatID, msgID int64, parts []string) {
	if len(parts) < 3 {
		b.answer(ctx, cbID, "")
		return
	}
	k := kindByCode(parts[1])
	if k == nil {
		b.answer(ctx, cbID, "")
		return
	}
	verb := parts[2]
	arg := ""
	if len(parts) > 3 {
		arg = parts[3]
	}
	id64, _ := strconv.ParseUint(arg, 10, 32)
	id := uint(id64)
	show := func(text string, kb [][]button) { b.edit(ctx, chatID, msgID, text, kb) }
	card := func() {
		text, kb := b.objCard(k, id)
		show(text, kb)
	}
	listBack := "o:" + k.code + ":ls:0"
	switch verb {
	case "ls":
		b.answer(ctx, cbID, "")
		text, kb := b.objListScreen(k, int(id64))
		show(text, kb)
	case "v":
		b.answer(ctx, cbID, "")
		card()
	case "probe":
		b.answer(ctx, cbID, b.tr("در حال بررسی…", "Probing…"))
		if id == 0 {
			(&service.NodeService{}).RefreshAll()
		} else {
			_, _ = (&service.NodeService{}).ProbeNow([]uint{id})
		}
		if id == 0 {
			text, kb := b.objListScreen(k, 0)
			show(text, kb)
			return
		}
		card()
	case "j":
		b.answer(ctx, cbID, "")
		if m, err := b.objEditable(k, id); err == nil {
			b.sendJSON(ctx, chatID, k.obj+"-"+arg, m)
		} else {
			b.fail(ctx, chatID, err)
		}
	case "e":
		b.answer(ctx, cbID, "")
		m, err := b.objEditable(k, id)
		if err != nil {
			b.fail(ctx, chatID, err)
			return
		}
		b.sendJSON(ctx, chatID, k.obj+"-"+arg, m)
		b.pend.set(chatID, &pending{kind: "obj.edit", key: k.code, id: id, msgID: msgID, back: "o:" + k.code + ":v:" + arg, data: map[string]string{}})
		show("✏️ "+b.tr("JSON ویرایش‌شده را کامل بفرستید (متن یا فایل ‎.json). فیلد id را تغییر ندهید.", "Send the full edited JSON (text or a .json file). Do not change the id field."), [][]button{b.cancelRow()})
	case "n":
		b.answer(ctx, cbID, "")
		if len(templates[k.code]) == 0 {
			b.pend.set(chatID, &pending{kind: "obj.new", key: k.code, msgID: msgID, back: listBack, data: map[string]string{}})
			show("✏️ "+b.tr("JSON کامل را بفرستید.", "Send the full JSON."), [][]button{b.cancelRow()})
			return
		}
		b.pend.set(chatID, &pending{kind: "obj.new", key: k.code, msgID: msgID, back: listBack, data: map[string]string{}})
		text, kb := b.newObjScreen(k)
		show(text, append(kb[:len(kb)-1], b.cancelRow()))
	case "nt":
		b.answer(ctx, cbID, "")
		b.send(ctx, chatID, "<pre>"+esc(templateJSON(k.code, arg))+"</pre>")
		b.pend.set(chatID, &pending{kind: "obj.new", key: k.code, msgID: msgID, back: listBack, data: map[string]string{}})
		show("✏️ "+b.tr("قالب بالا را ویرایش کنید و JSON کامل را بفرستید.", "Edit the template above and send the full JSON."), [][]button{b.cancelRow()})
	case "d":
		b.answer(ctx, cbID, "")
		name := arg
		if m, err := b.objFull(k, id); err == nil {
			name = scalar(m["tag"])
			if name == "" {
				name = scalar(m["name"])
			}
		}
		show(fmt.Sprintf("⚠️ %s <b>%s</b>", b.tr("حذف شود؟", "Delete?"), esc(name)), [][]button{{b.btn("btnConfirm", "o:"+k.code+":dy:"+arg), {Text: b.tr("✖️ انصراف", "✖️ Cancel"), Data: "o:" + k.code + ":v:" + arg}}})
	case "dy":
		if err := b.deleteObj(k, id); err != nil {
			b.answer(ctx, cbID, b.t("failed", b.errText(err)))
			return
		}
		b.answer(ctx, cbID, b.t("done"))
		text, kb := b.objListScreen(k, 0)
		show(text, kb)
	case "port":
		b.answer(ctx, cbID, "")
		b.ask(ctx, chatID, msgID, "in.port", "", id, "o:in:v:"+arg, b.tr("پورت جدید را بفرستید (۱ تا ۶۵۵۳۵).", "Send the new port (1-65535)."))
	case "cl":
		b.answer(ctx, cbID, "")
		names := b.inboundClients(id)
		lines := []string{b.header("👥", fmt.Sprintf("%s (%d)", b.tr("کلاینت‌های اینباند", "Inbound clients"), len(names)))}
		for i, n := range names {
			if i == 60 {
				lines = append(lines, b.t("andMore", len(names)-60))
				break
			}
			lines = append(lines, "• "+esc(n))
		}
		show(strings.Join(lines, "\n"), [][]button{b.navRow("o:in:v:" + arg)})
	case "test":
		m, err := b.objFull(k, id)
		if err != nil {
			b.answer(ctx, cbID, b.t("failed", b.errText(err)))
			return
		}
		b.answer(ctx, cbID, b.tr("در حال تست…", "Testing…"))
		res := b.configService.CheckOutbound(scalar(m["tag"]), "https://www.gstatic.com/generate_204")
		msg := fmt.Sprintf("⚡ <b>%s</b>: ✅ %d ms", esc(scalar(m["tag"])), res.Delay)
		if !res.OK {
			msg = fmt.Sprintf("⚡ <b>%s</b>: ❌ %s", esc(scalar(m["tag"])), esc(res.Error))
		}
		b.send(ctx, chatID, msg)
	case "tog":
		m, err := b.objEditable(k, id)
		if err == nil {
			m["enable"] = m["enable"] == false
			err = b.saveObj(k, "edit", m)
		}
		if err != nil {
			b.answer(ctx, cbID, b.t("failed", b.errText(err)))
			return
		}
		b.answer(ctx, cbID, b.t("done"))
		card()
	case "sync":
		b.answer(ctx, cbID, b.tr("همگام‌سازی…", "Syncing…"))
		if err := (&service.NodeSyncService{}).ReconcileNow(id); err != nil {
			b.send(ctx, chatID, b.t("failed", esc(err.Error())))
			return
		}
		card()
	case "rsb":
		b.answer(ctx, cbID, b.tr("ری‌استارت هسته…", "Restarting the core…"))
		res, err := (&service.NodeSyncService{}).NodeAction([]uint{id}, "restartSb", b.actor())
		if err == nil && len(res) > 0 && !res[0].Ok {
			err = fmt.Errorf("%s", res[0].Error)
		}
		if err != nil {
			b.send(ctx, chatID, b.t("failed", esc(err.Error())))
			return
		}
		b.send(ctx, chatID, "♻️ "+b.tr("هسته نود ری‌استارت شد.", "The node's core was restarted."))
		card()
	case "bk":
		if !b.can("backup") {
			b.answer(ctx, cbID, "")
			b.send(ctx, chatID, b.denied())
			return
		}
		b.answer(ctx, cbID, b.tr("در حال گرفتن بکاپ از نود…", "Fetching the node's backup…"))
		// A big database takes a while to come over: the bot keeps answering
		// meanwhile.
		go b.sendNodeBackup(ctx, chatID, id)
	default:
		b.answer(ctx, cbID, "")
	}
}
