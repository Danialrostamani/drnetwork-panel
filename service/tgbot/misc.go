package tgbot

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/service"
)

// ---- settings ----

type settingGroup struct {
	code, icon, fa, en string
	keys               []string
}

var settingGroups = []settingGroup{
	{"p", "🌐", "پنل", "Panel", []string{"webListen", "webDomain", "webPort", "webCertFile", "webKeyFile", "webPath", "webURI", "sessionMaxAge", "trafficAge", "statsBucketSeconds", "timeLocation"}},
	{"s", "🔗", "اشتراک", "Subscription", []string{"subListen", "subPort", "subPath", "subDomain", "subCertFile", "subKeyFile", "subUpdates", "subEncode", "subShowInfo", "subURI", "subJsonExt", "subClashExt", "subClashNoDefGrp", "subClashSprtAll", "subClashUdp"}},
	{"m", "🧰", "سایر", "Other", []string{"globalReset"}},
}

func settingGroupByCode(code string) *settingGroup {
	for i := range settingGroups {
		if settingGroups[i].code == code {
			return &settingGroups[i]
		}
	}
	return nil
}

func isBoolValue(v string) bool { return v == "true" || v == "false" }

func (b *bot) settingsHome() (string, [][]button) {
	var row []button
	for _, g := range settingGroups {
		row = append(row, button{Text: g.icon + " " + b.tr(g.fa, g.en), Data: "s:g:" + g.code})
	}
	return b.header("🔧", b.tr("تنظیمات پنل", "Panel settings")) + "\n" + b.tr("یک بخش را انتخاب کنید. تنظیمات ربات تلگرام را از پنل وب تغییر دهید.", "Pick a group. Telegram bot settings are changed in the web panel."),
		[][]button{row, {{Text: b.tr("♻️ ریستارت پنل", "♻️ Restart panel"), Data: "m:prest"}}, b.menuRow()}
}

func (b *bot) settingGroupScreen(g *settingGroup) (string, [][]button) {
	all, err := (&service.SettingService{}).GetAllSetting()
	if err != nil {
		return b.t("failed", esc(err.Error())), [][]button{b.menuRow()}
	}
	lines := []string{b.header(g.icon, b.tr(g.fa, g.en))}
	var btns []button
	for _, k := range g.keys {
		v, ok := (*all)[k]
		if !ok {
			continue
		}
		if isBoolValue(v) {
			lines = append(lines, fmt.Sprintf("%s %s", onOff(v == "true"), k))
			btns = append(btns, button{Text: onOff(v == "true") + " " + k, Data: "s:tg:" + k})
			continue
		}
		shown := v
		if shown == "" {
			shown = "—"
		}
		lines = append(lines, fmt.Sprintf("▫️ %s: <code>%s</code>", k, esc(truncate(shown, 50))))
		btns = append(btns, button{Text: "✏️ " + k, Data: "s:ask:" + k})
	}
	lines = append(lines, "", b.tr("ℹ️ تغییر پورت، دامنه و گواهی بعد از ریستارت پنل اعمال می‌شود.", "ℹ️ Port, domain and certificate changes apply after a panel restart."))
	kb := rows2(btns)
	kb = append(kb, []button{{Text: b.tr("♻️ ریستارت پنل", "♻️ Restart panel"), Data: "m:prest"}}, b.navRow("s:ls"))
	return strings.Join(lines, "\n"), kb
}

func (b *bot) saveSetting(key, value string) error {
	if b.configService == nil {
		return fmt.Errorf("config service unavailable")
	}
	raw, _ := json.Marshal(map[string]string{key: value})
	_, err := b.configService.Save("settings", "", raw, "", "telegram", b.host())
	return err
}

func groupOfKey(key string) *settingGroup {
	for i := range settingGroups {
		for _, k := range settingGroups[i].keys {
			if k == key {
				return &settingGroups[i]
			}
		}
	}
	return nil
}

func (b *bot) settingsCallback(ctx context.Context, cbID string, chatID, msgID int64, parts []string) {
	verb := parts[1]
	arg := ""
	if len(parts) > 2 {
		arg = parts[2]
	}
	show := func(text string, kb [][]button) { b.edit(ctx, chatID, msgID, text, kb) }
	switch verb {
	case "ls":
		b.answer(ctx, cbID, "")
		text, kb := b.settingsHome()
		show(text, kb)
	case "g":
		b.answer(ctx, cbID, "")
		if g := settingGroupByCode(arg); g != nil {
			text, kb := b.settingGroupScreen(g)
			show(text, kb)
		}
	case "tg":
		g := groupOfKey(arg)
		all, err := (&service.SettingService{}).GetAllSetting()
		if g == nil || err != nil {
			b.answer(ctx, cbID, "")
			return
		}
		next := "true"
		if (*all)[arg] == "true" {
			next = "false"
		}
		if err := b.saveSetting(arg, next); err != nil {
			b.answer(ctx, cbID, b.t("failed", b.errText(err)))
			return
		}
		b.answer(ctx, cbID, b.t("done"))
		text, kb := b.settingGroupScreen(g)
		show(text, kb)
	case "ask":
		g := groupOfKey(arg)
		if g == nil {
			b.answer(ctx, cbID, "")
			return
		}
		b.answer(ctx, cbID, "")
		b.pend.set(chatID, &pending{kind: "set", key: arg, msgID: msgID, back: "s:g:" + g.code, data: map[string]string{}})
		show("✏️ "+b.tr("مقدار جدید «"+arg+"» را بفرستید (- برای خالی کردن).", "Send the new value of “"+arg+"” (- to clear)."), [][]button{b.cancelRow()})
	default:
		b.answer(ctx, cbID, "")
	}
}

// ---- stats ----

var statRanges = []struct {
	code   string
	secs   int64
	bucket int64
	fa, en string
}{
	{"d1", 24 * 3600, 3600, "۲۴ ساعت", "24 h"},
	{"d7", 7 * 24 * 3600, 24 * 3600, "۷ روز", "7 days"},
	{"d30", 30 * 24 * 3600, 24 * 3600, "۳۰ روز", "30 days"},
}

func (b *bot) statsScreen(code string) (string, [][]button) {
	rng := statRanges[0]
	for _, r := range statRanges {
		if r.code == code {
			rng = r
		}
	}
	since := time.Now().Unix() - rng.secs
	db := database.GetDB()
	var rows []struct {
		Resource  string
		Tag       string
		Direction bool
		Total     int64
	}
	_ = db.Model(model.Stats{}).Select("resource, tag, direction, SUM(traffic) AS total").Where("date_time > ?", since).Group("resource, tag, direction").Scan(&rows).Error
	type ud struct{ up, down int64 }
	by := map[string]map[string]*ud{}
	for _, r := range rows {
		if by[r.Resource] == nil {
			by[r.Resource] = map[string]*ud{}
		}
		e := by[r.Resource][r.Tag]
		if e == nil {
			e = &ud{}
			by[r.Resource][r.Tag] = e
		}
		if r.Direction {
			e.up += r.Total
		} else {
			e.down += r.Total
		}
	}
	lines := []string{b.header("📊", b.tr("آمار ترافیک — ", "Traffic — ")+b.tr(rng.fa, rng.en))}
	for _, res := range []struct{ key, icon, fa, en string }{{"user", "👥", "کاربران", "Users"}, {"inbound", "📡", "اینباندها", "Inbounds"}, {"outbound", "📤", "اوت‌باندها", "Outbounds"}} {
		tags := by[res.key]
		if len(tags) == 0 {
			continue
		}
		names := make([]string, 0, len(tags))
		for t := range tags {
			names = append(names, t)
		}
		sort.Slice(names, func(i, j int) bool {
			return tags[names[i]].up+tags[names[i]].down > tags[names[j]].up+tags[names[j]].down
		})
		lines = append(lines, "", fmt.Sprintf("%s <b>%s</b>", res.icon, b.tr(res.fa, res.en)))
		for i, n := range names {
			if i == 6 {
				break
			}
			e := tags[n]
			lines = append(lines, fmt.Sprintf("  • %s — %s  (↑ %s ↓ %s)", esc(n), humanBytes(e.up+e.down), humanBytes(e.up), humanBytes(e.down)))
		}
	}
	// timeline
	buckets := int(rng.secs / rng.bucket)
	var tl []struct {
		B     int64
		Total int64
	}
	_ = db.Raw("SELECT (date_time / ?) * ? AS b, SUM(traffic) AS total FROM stats WHERE resource = 'inbound' AND date_time > ? GROUP BY b ORDER BY b", rng.bucket, rng.bucket, since).Scan(&tl).Error
	if len(tl) > 0 {
		vals := make([]int64, buckets)
		start := (time.Now().Unix()/rng.bucket - int64(buckets) + 1) * rng.bucket
		for _, p := range tl {
			if i := int((p.B - start) / rng.bucket); i >= 0 && i < buckets {
				vals[i] = p.Total
			}
		}
		lines = append(lines, "", "📈 "+spark(vals))
	}
	if len(rows) == 0 {
		lines = append(lines, "", b.tr("هنوز آماری ثبت نشده است.", "No statistics recorded yet."))
	}
	if sessions, err := (&service.StatsService{}).GetSessions("", ""); err == nil {
		lines = append(lines, "", fmt.Sprintf("🔌 %s: %d", b.tr("اتصال‌های زنده", "Live connections"), len(sessions)))
	}
	var chips []button
	for _, r := range statRanges {
		t := b.tr(r.fa, r.en)
		if r.code == rng.code {
			t = "◉ " + t
		}
		chips = append(chips, button{Text: t, Data: "t:" + r.code})
	}
	return strings.Join(lines, "\n"), [][]button{chips, b.menuRow()}
}

// ---- changes ----

func (b *bot) changesScreen() (string, [][]button) {
	changes := b.configService.GetChanges("", "", "15")
	lines := []string{b.header("🧾", b.tr("آخرین تغییرات", "Recent changes"))}
	if len(changes) == 0 {
		lines = append(lines, b.tr("تغییری ثبت نشده است.", "No changes recorded."))
	}
	for _, c := range changes {
		what := ""
		var m map[string]interface{}
		if json.Unmarshal(c.Obj, &m) == nil {
			what = scalar(m["name"])
			if what == "" {
				what = scalar(m["tag"])
			}
		} else {
			what = scalar(string(c.Obj))
		}
		line := fmt.Sprintf("🕒 %s · <b>%s</b> · %s.%s", time.Unix(c.DateTime, 0).In(b.loc).Format("01-02 15:04"), esc(c.Actor), esc(c.Key), esc(c.Action))
		if what != "" {
			line += " · " + esc(truncate(what, 30))
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n"), [][]button{b.menuRow()}
}

// ---- logs ----

func (b *bot) logsScreen(level string) (string, [][]button) {
	if level == "" {
		level = "info"
	}
	var chips []button
	for _, l := range []string{"debug", "info", "warn", "err"} {
		t := l
		if l == level {
			t = "◉ " + l
		}
		chips = append(chips, button{Text: t, Data: "m:logs:" + l})
	}
	return b.logsText(level), [][]button{chips, {{Text: b.tr("🔄 به‌روزرسانی", "🔄 Refresh"), Data: "m:logs:" + level}}, b.menuRow()}
}
