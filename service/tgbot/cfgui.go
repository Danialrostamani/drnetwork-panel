package tgbot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/Danialrostamani/drnetwork-panel/service"
)

// The routing rules, rule sets, DNS servers/rules and the basic options all live
// inside the sing-box base config. These screens edit that document in place.

type cfgSec struct {
	code   string
	path   []string
	icon   string
	fa     string
	en     string
	sample string
}

var cfgSecs = []cfgSec{
	{"rl", []string{"route", "rules"}, "📏", "قوانین مسیریابی", "Routing rules", `{"action":"route","outbound":"direct","domain_suffix":["example.com"]}`},
	{"rs", []string{"route", "rule_set"}, "📚", "مجموعه قوانین", "Rule sets", `{"type":"remote","tag":"geosite-example","format":"binary","url":"https://example.com/rules.srs","download_detour":"direct"}`},
	{"dn", []string{"dns", "servers"}, "🌐", "سرورهای DNS", "DNS servers", `{"type":"udp","tag":"google","server":"8.8.8.8"}`},
	{"dr", []string{"dns", "rules"}, "🧭", "قوانین DNS", "DNS rules", `{"action":"route","server":"google","domain_suffix":["example.com"]}`},
}

func cfgSecByCode(code string) *cfgSec {
	for i := range cfgSecs {
		if cfgSecs[i].code == code {
			return &cfgSecs[i]
		}
	}
	return nil
}

func (b *bot) loadConfigMap() (map[string]interface{}, error) {
	raw, err := (&service.SettingService{}).GetConfig()
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	var m map[string]interface{}
	if err := dec.Decode(&m); err != nil {
		return nil, err
	}
	if m == nil {
		m = map[string]interface{}{}
	}
	return m, nil
}

func (b *bot) saveConfigMap(m map[string]interface{}) error {
	if b.configService == nil {
		return fmt.Errorf("config service unavailable")
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return err
	}
	_, err = b.configService.Save("config", "", raw, "", b.actor(), b.host())
	return err
}

func getPath(m map[string]interface{}, path []string) []interface{} {
	cur := m
	for i, p := range path {
		v := cur[p]
		if i == len(path)-1 {
			arr, _ := v.([]interface{})
			return arr
		}
		next, _ := v.(map[string]interface{})
		if next == nil {
			return nil
		}
		cur = next
	}
	return nil
}

func setPath(m map[string]interface{}, path []string, arr []interface{}) {
	cur := m
	for i, p := range path {
		if i == len(path)-1 {
			cur[p] = arr
			return
		}
		next, _ := cur[p].(map[string]interface{})
		if next == nil {
			next = map[string]interface{}{}
			cur[p] = next
		}
		cur = next
	}
}

var ruleSkip = map[string]bool{"action": true, "outbound": true, "server": true, "method": true, "strategy": true, "tag": true, "type": true, "disable_cache": true, "rewrite_ttl": true, "client_subnet": true, "invert": true, "mode": true}

func itemSummary(sec *cfgSec, v interface{}) string {
	m, _ := v.(map[string]interface{})
	if m == nil {
		return truncate(scalar(v), 60)
	}
	if sec.code == "rs" || sec.code == "dn" {
		s := scalar(m["tag"]) + " · " + scalar(m["type"])
		if u := scalar(m["url"]); u != "" {
			s += " · " + u
		} else if sv := scalar(m["server"]); sv != "" {
			s += " · " + sv
		} else if p := scalar(m["path"]); p != "" {
			s += " · " + p
		}
		return truncate(s, 70)
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		if !ruleSkip[k] {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		val := ""
		switch x := m[k].(type) {
		case []interface{}:
			if len(x) > 0 {
				val = scalar(x[0])
				if len(x) > 1 {
					val += fmt.Sprintf("+%d", len(x)-1)
				}
			}
		case map[string]interface{}:
			val = "{…}"
		default:
			val = scalar(x)
		}
		parts = append(parts, k+"="+val)
		if len(parts) == 2 {
			break
		}
	}
	target := scalar(m["outbound"])
	if target == "" {
		target = scalar(m["server"])
	}
	if target == "" {
		target = scalar(m["action"])
	}
	if len(parts) == 0 {
		parts = []string{"*"}
	}
	return truncate(strings.Join(parts, " ")+" → "+target, 70)
}

func (b *bot) cfgChips(active string) []button {
	var chips []button
	for _, s := range cfgSecs {
		text := s.icon + " " + b.tr(s.fa, s.en)
		if s.code == active {
			text = "◉ " + text
		}
		chips = append(chips, button{Text: text, Data: "g:" + s.code + ":ls:0"})
	}
	return chips
}

func (b *bot) cfgListScreen(sec *cfgSec, page int) (string, [][]button) {
	m, err := b.loadConfigMap()
	if err != nil {
		return b.t("failed", esc(err.Error())), [][]button{b.menuRow()}
	}
	items := getPath(m, sec.path)
	from, to, page, pages := pageSlice(len(items), page, objPageSize)
	lines := []string{b.header(sec.icon, fmt.Sprintf("%s (%d)", b.tr(sec.fa, sec.en), len(items)))}
	if route := asMap(m["route"]); sec.code == "rl" || sec.code == "rs" {
		if f := scalar(route["final"]); f != "" {
			lines = append(lines, "🏁 final: <code>"+esc(f)+"</code>")
		}
	}
	if dns := asMap(m["dns"]); sec.code == "dn" || sec.code == "dr" {
		if f := scalar(dns["final"]); f != "" {
			lines = append(lines, "🏁 final: <code>"+esc(f)+"</code>")
		}
	}
	if len(items) == 0 {
		lines = append(lines, b.tr("موردی تعریف نشده است.", "Nothing defined yet."))
	}
	var btns []button
	for i := from; i < to; i++ {
		lines = append(lines, fmt.Sprintf("<b>%d.</b> %s", i+1, esc(itemSummary(sec, items[i]))))
		btns = append(btns, button{Text: strconv.Itoa(i + 1), Data: fmt.Sprintf("g:%s:v:%d", sec.code, i)})
	}
	var kb [][]button
	for i := 0; i < len(btns); i += 5 {
		end := i + 5
		if end > len(btns) {
			end = len(btns)
		}
		kb = append(kb, btns[i:end])
	}
	if pg := pager("g:"+sec.code+":ls", page, pages); pg != nil {
		kb = append(kb, pg)
	}
	chips := b.cfgChips(sec.code)
	opt := "route"
	group := chips[:2]
	if sec.code == "dn" || sec.code == "dr" {
		opt = "dns"
		group = chips[2:]
	}
	kb = append(kb, group, []button{{Text: b.tr("➕ جدید", "➕ New"), Data: "g:" + sec.code + ":n:0"}, {Text: b.tr("⚙️ تنظیمات", "⚙️ Options"), Data: "g:" + opt + ":eo:0"}}, b.menuRow())
	return strings.Join(lines, "\n"), kb
}

func (b *bot) cfgItemScreen(sec *cfgSec, idx int) (string, [][]button) {
	m, err := b.loadConfigMap()
	if err != nil {
		return b.t("failed", esc(err.Error())), [][]button{b.menuRow()}
	}
	items := getPath(m, sec.path)
	if idx < 0 || idx >= len(items) {
		return b.t("notFound"), [][]button{b.navRow("g:" + sec.code + ":ls:0")}
	}
	raw, _ := json.MarshalIndent(items[idx], "", "  ")
	text := b.header(sec.icon, fmt.Sprintf("%s #%d", b.tr(sec.fa, sec.en), idx+1)) + "\n<pre>" + esc(truncate(string(raw), 3400)) + "</pre>"
	pre := fmt.Sprintf("g:%s:", sec.code)
	i := strconv.Itoa(idx)
	return text, [][]button{
		{{Text: b.tr("✏️ ویرایش", "✏️ Edit"), Data: pre + "e:" + i}, {Text: "⬆️", Data: pre + "up:" + i}, {Text: "⬇️", Data: pre + "dn:" + i}, {Text: b.tr("🗑 حذف", "🗑 Delete"), Data: pre + "d:" + i}},
		{{Text: b.tr("⬅️ فهرست", "⬅️ List"), Data: pre + "ls:0"}, {Text: "🏠", Data: "m:menu"}},
	}
}

// options sections: JSON objects without the big arrays
var optionSecs = map[string]struct {
	keep []string // keys left alone when the whole section is replaced
	fa   string
	en   string
}{
	"route": {[]string{"rules", "rule_set"}, "تنظیمات مسیریابی", "Route options"},
	"dns":   {[]string{"servers", "rules"}, "تنظیمات DNS", "DNS options"},
	"log":   {nil, "لاگ", "Log"},
	"ntp":   {nil, "NTP", "NTP"},
	"exp":   {nil, "تجربی (Clash API / کش)", "Experimental (Clash API / cache)"},
}

func secKey(sec string) string {
	if sec == "exp" {
		return "experimental"
	}
	return sec
}

func (b *bot) optionJSON(sec string) (map[string]interface{}, error) {
	m, err := b.loadConfigMap()
	if err != nil {
		return nil, err
	}
	cur := asMap(m[secKey(sec)])
	out := map[string]interface{}{}
	skip := map[string]bool{}
	for _, k := range optionSecs[sec].keep {
		skip[k] = true
	}
	for k, v := range cur {
		if !skip[k] {
			out[k] = v
		}
	}
	return out, nil
}

func (b *bot) saveOption(sec string, nm map[string]interface{}) error {
	m, err := b.loadConfigMap()
	if err != nil {
		return err
	}
	cur := asMap(m[secKey(sec)])
	for _, k := range optionSecs[sec].keep {
		if v, ok := cur[k]; ok {
			nm[k] = v
		}
	}
	if len(nm) == 0 && sec != "route" && sec != "dns" {
		delete(m, secKey(sec))
	} else {
		m[secKey(sec)] = nm
	}
	return b.saveConfigMap(m)
}

func (b *bot) basicsScreen() (string, [][]button) {
	m, err := b.loadConfigMap()
	if err != nil {
		return b.t("failed", esc(err.Error())), [][]button{b.menuRow()}
	}
	lines := []string{b.header("⚙️", b.tr("تنظیمات پایه", "Basics"))}
	log := asMap(m["log"])
	level := scalar(log["level"])
	if level == "" {
		level = "info"
	}
	lines = append(lines, "📜 <b>Log</b>: "+esc(level)+" · "+esc(scalar(log["output"])))
	if ntp := asMap(m["ntp"]); len(ntp) > 0 {
		lines = append(lines, fmt.Sprintf("🕰 <b>NTP</b>: %s %s", onOff(ntp["enabled"] == true), esc(scalar(ntp["server"]))))
	} else {
		lines = append(lines, "🕰 <b>NTP</b>: ⬜")
	}
	exp := asMap(m["experimental"])
	if c := asMap(exp["clash_api"]); len(c) > 0 {
		lines = append(lines, "🔌 <b>Clash API</b>: <code>"+esc(scalar(c["external_controller"]))+"</code>")
	}
	if c := asMap(exp["cache_file"]); len(c) > 0 {
		lines = append(lines, "🗄 <b>Cache file</b>: "+onOff(c["enabled"] == true))
	}
	lvl := func(l string) button {
		t := l
		if l == level {
			t = "◉ " + l
		}
		return button{Text: t, Data: "g:log:lv:" + l}
	}
	return strings.Join(lines, "\n"), [][]button{
		{lvl("debug"), lvl("info"), lvl("warn"), lvl("error")},
		{{Text: "✏️ Log", Data: "g:log:eo:0"}, {Text: "✏️ NTP", Data: "g:ntp:eo:0"}, {Text: "✏️ Experimental", Data: "g:exp:eo:0"}},
		b.menuRow(),
	}
}

func (b *bot) cfgCallback(ctx context.Context, cbID string, chatID, msgID int64, parts []string) {
	if len(parts) < 3 {
		b.answer(ctx, cbID, "")
		return
	}
	code, verb := parts[1], parts[2]
	arg := 0
	if len(parts) > 3 {
		arg, _ = strconv.Atoi(parts[3])
	}
	show := func(text string, kb [][]button) { b.edit(ctx, chatID, msgID, text, kb) }
	// option sections
	if o, ok := optionSecs[code]; ok {
		switch verb {
		case "eo":
			b.answer(ctx, cbID, "")
			cur, err := b.optionJSON(code)
			if err != nil {
				b.fail(ctx, chatID, err)
				return
			}
			b.sendJSON(ctx, chatID, code, cur)
			back := "g:rl:ls:0"
			if code == "dns" {
				back = "g:dn:ls:0"
			} else if code != "route" {
				back = "g:basics:ls:0"
			}
			b.pend.set(chatID, &pending{kind: "cfg.opt", key: code, msgID: msgID, back: back, data: map[string]string{}})
			show("✏️ <b>"+b.tr(o.fa, o.en)+"</b>\n"+b.tr("JSON ویرایش‌شده را بفرستید (متن یا فایل). {} در بخش‌های اختیاری آن را پاک می‌کند.", "Send the edited JSON (text or file). {} clears an optional section."), [][]button{b.cancelRow()})
		case "lv":
			m, err := b.loadConfigMap()
			if err == nil {
				log := asMap(m["log"])
				log["level"] = parts[3]
				m["log"] = log
				err = b.saveConfigMap(m)
			}
			if err != nil {
				b.answer(ctx, cbID, b.t("failed", b.errText(err)))
				return
			}
			b.answer(ctx, cbID, b.t("done"))
			text, kb := b.basicsScreen()
			show(text, kb)
		}
		return
	}
	if code == "basics" {
		b.answer(ctx, cbID, "")
		text, kb := b.basicsScreen()
		show(text, kb)
		return
	}
	sec := cfgSecByCode(code)
	if sec == nil {
		b.answer(ctx, cbID, "")
		return
	}
	mutate := func(fn func(items []interface{}) ([]interface{}, error), after func()) {
		m, err := b.loadConfigMap()
		if err == nil {
			var items []interface{}
			items, err = fn(getPath(m, sec.path))
			if err == nil {
				setPath(m, sec.path, items)
				err = b.saveConfigMap(m)
			}
		}
		if err != nil {
			b.answer(ctx, cbID, b.t("failed", b.errText(err)))
			return
		}
		b.answer(ctx, cbID, b.t("done"))
		after()
	}
	switch verb {
	case "ls":
		b.answer(ctx, cbID, "")
		text, kb := b.cfgListScreen(sec, arg)
		show(text, kb)
	case "v":
		b.answer(ctx, cbID, "")
		text, kb := b.cfgItemScreen(sec, arg)
		show(text, kb)
	case "n":
		b.answer(ctx, cbID, "")
		b.send(ctx, chatID, "<pre>"+esc(sec.sample)+"</pre>")
		b.pend.set(chatID, &pending{kind: "cfg.new", key: sec.code, msgID: msgID, back: "g:" + sec.code + ":ls:0", data: map[string]string{}})
		show("✏️ "+b.tr("JSON مورد جدید را بفرستید (نمونه بالا).", "Send the JSON of the new entry (see the sample above)."), [][]button{b.cancelRow()})
	case "e":
		b.answer(ctx, cbID, "")
		m, err := b.loadConfigMap()
		items := getPath(m, sec.path)
		if err != nil || arg >= len(items) {
			b.answer(ctx, cbID, b.t("notFound"))
			return
		}
		b.sendJSON(ctx, chatID, sec.code, items[arg])
		b.pend.set(chatID, &pending{kind: "cfg.edit", key: sec.code, id: uint(arg), msgID: msgID, back: fmt.Sprintf("g:%s:v:%d", sec.code, arg), data: map[string]string{}})
		show("✏️ "+b.tr("JSON ویرایش‌شده را بفرستید.", "Send the edited JSON."), [][]button{b.cancelRow()})
	case "up", "dn":
		delta := -1
		if verb == "dn" {
			delta = 1
		}
		mutate(func(items []interface{}) ([]interface{}, error) {
			j := arg + delta
			if arg < 0 || arg >= len(items) || j < 0 || j >= len(items) {
				return items, nil
			}
			items[arg], items[j] = items[j], items[arg]
			return items, nil
		}, func() {
			text, kb := b.cfgItemScreen(sec, arg+delta)
			show(text, kb)
		})
	case "d":
		b.answer(ctx, cbID, "")
		show("⚠️ "+b.tr("این مورد حذف شود؟ (هسته ریستارت می‌شود)", "Delete this entry? (the core restarts)"), [][]button{{b.btn("btnConfirm", fmt.Sprintf("g:%s:dy:%d", sec.code, arg)), {Text: b.tr("✖️ انصراف", "✖️ Cancel"), Data: fmt.Sprintf("g:%s:v:%d", sec.code, arg)}}})
	case "dy":
		mutate(func(items []interface{}) ([]interface{}, error) {
			if arg < 0 || arg >= len(items) {
				return items, nil
			}
			return append(items[:arg:arg], items[arg+1:]...), nil
		}, func() {
			text, kb := b.cfgListScreen(sec, 0)
			show(text, kb)
		})
	default:
		b.answer(ctx, cbID, "")
	}
}

func decodeJSONValue(text string) (interface{}, error) {
	dec := json.NewDecoder(bytes.NewReader([]byte(stripFence(text))))
	dec.UseNumber()
	var v interface{}
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	return v, nil
}
