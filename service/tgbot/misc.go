package tgbot

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

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
	_, err := b.configService.Save("settings", "", raw, "", b.actor(), b.host())
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
	now := time.Now().Unix()
	// The master's own numbers plus what the nodes counted for its clients and
	// inbounds: a client served by a node moves its traffic through that node.
	sum := (&service.StatsService{}).GetClusterSummary(now-rng.secs, rng.bucket)
	lines := []string{b.header("📊", b.tr("آمار ترافیک — ", "Traffic — ")+b.tr(rng.fa, rng.en))}
	for _, res := range []struct{ key, icon, fa, en string }{{"user", "👥", "کاربران", "Users"}, {"inbound", "📡", "اینباندها", "Inbounds"}, {"outbound", "📤", "اوت‌باندها", "Outbounds"}} {
		// sum.Totals is in order: by resource, the busiest first.
		shown := 0
		for _, t := range sum.Totals {
			if t.Resource != res.key {
				continue
			}
			if shown == 0 {
				lines = append(lines, "", fmt.Sprintf("%s <b>%s</b>", res.icon, b.tr(res.fa, res.en)))
			}
			if shown++; shown > 6 {
				break
			}
			lines = append(lines, fmt.Sprintf("  • %s — %s  (↑ %s ↓ %s)", esc(t.Tag), humanBytes(t.Up+t.Down), humanBytes(t.Up), humanBytes(t.Down)))
		}
	}
	// timeline
	buckets := int(rng.secs / rng.bucket)
	if len(sum.Series) > 0 {
		vals := make([]int64, buckets)
		start := (now/rng.bucket - int64(buckets) + 1) * rng.bucket
		for _, p := range sum.Series {
			if i := int((p.At - start) / rng.bucket); i >= 0 && i < buckets {
				vals[i] += p.Traffic
			}
		}
		lines = append(lines, "", "📈 "+spark(vals))
	}
	if len(sum.Totals) == 0 {
		lines = append(lines, "", b.tr("هنوز آماری ثبت نشده است.", "No statistics recorded yet."))
	}
	if len(sum.Missing) > 0 {
		parts := make([]string, 0, len(sum.Missing))
		for _, m := range sum.Missing {
			why := b.tr("در دسترس نیست", "unreachable")
			if m.Reason == service.NodeNeedsUpdate {
				why = b.tr("نیاز به به‌روزرسانی", "needs an update")
			}
			parts = append(parts, fmt.Sprintf("%s (%s)", esc(m.Name), why))
		}
		lines = append(lines, "", "⚠️ "+b.tr("ترافیک این نودها در آمار نیست: ", "Not included: ")+strings.Join(parts, b.tr("، ", ", ")))
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
