package tgbot

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/config"
	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/service"
	"github.com/Danialrostamani/drnetwork-panel/util"
)

// ---- node: connection test and outage history ----

// nodeTestText connects to a node right now and says what it found, without
// waiting for the next probe or changing the status the panel keeps.
func (b *bot) nodeTestText(id uint) string {
	var n model.Node
	if err := database.GetDB().Where("id = ?", id).First(&n).Error; err != nil {
		return b.t("failed", esc(err.Error()))
	}
	raw, err := json.Marshal(n)
	if err != nil {
		return b.t("failed", esc(err.Error()))
	}
	start := time.Now()
	st, err := (&service.NodeService{}).TestNode(raw)
	took := time.Since(start)
	lines := []string{b.header("🧪", b.tr("تست اتصال نود ", "Connection test — ")+esc(n.Name)),
		"🔗 <code>" + esc(n.BaseUrl+n.WebPath) + "</code>"}
	if err != nil {
		return strings.Join(append(lines, "❌ "+esc(err.Error())), "\n")
	}
	reached := st.State == "online" || st.State == "core-stopped"
	switch st.State {
	case "online":
		lines = append(lines, "🟢 "+b.tr("وصل است؛ پنل نود و هسته کار می‌کنند.", "Reachable; the node panel and the core are running."))
	case "core-stopped":
		lines = append(lines, "🟠 "+b.tr("وصل است، ولی هستهٔ نود متوقف است.", "Reachable, but the node's core is stopped."))
	default:
		lines = append(lines, "🔴 "+b.tr("به نود وصل نشد.", "Could not reach the node."))
	}
	if st.Error != "" {
		lines = append(lines, "⚠️ "+esc(truncate(st.Error, 300)))
	}
	if reached {
		version := st.AppFull
		if version == "" {
			version = st.AppVersion
		}
		lines = append(lines,
			fmt.Sprintf("⏱ %s: %d ms", b.tr("پاسخ", "Response"), st.Latency),
			fmt.Sprintf("🧩 %s · core %s", esc(config.VersionLabel(version)), esc(st.CoreVersion)),
			fmt.Sprintf("🧠 CPU %.0f%% · 💾 RAM %d%% · 🗄 %s %d%%", st.Cpu,
				pctOf(uint64(max(st.Mem.Current, 0)), uint64(max(st.Mem.Total, 0))),
				b.tr("دیسک", "Disk"), pctOf(uint64(max(st.Disk.Current, 0)), uint64(max(st.Disk.Total, 0)))))
		if st.CertExpiry > 0 {
			left := time.Until(time.Unix(st.CertExpiry, 0))
			icon := "🔐"
			if left < 7*24*time.Hour {
				icon = "⚠️"
			}
			lines = append(lines, fmt.Sprintf("%s %s: %s (%s)", icon, b.tr("گواهی HTTPS تا", "HTTPS certificate until"), b.stamp(st.CertExpiry), b.humanDuration(max(left, 0))))
		}
	}
	lines = append(lines, "⌛ "+b.tr("مدت تست", "Test took")+": "+strconv.FormatInt(took.Milliseconds(), 10)+" ms")
	return strings.Join(lines, "\n")
}

// nodeOutagesListedInBot is how many outages the history screen lists.
const nodeOutagesListedInBot = 15

// nodeOutagesScreen is the outage history of a node.
func (b *bot) nodeOutagesScreen(id uint) (string, [][]button) {
	sid := strconv.FormatUint(uint64(id), 10)
	back := "o:nd:v:" + sid
	var n model.Node
	if err := database.GetDB().Where("id = ?", id).First(&n).Error; err != nil {
		return b.t("failed", esc(err.Error())), [][]button{b.navRow("o:nd:ls:0")}
	}
	rep, err := (&service.NodeService{}).GetNodeOutages(id)
	if err != nil {
		return b.t("failed", esc(err.Error())), [][]button{b.navRow(back)}
	}
	now := time.Now()
	lines := []string{b.header("📉", b.tr("قطعی‌های نود ", "Outages — ")+esc(n.Name)),
		fmt.Sprintf("📈 %s: 24h %s · 7d %s", b.tr("آپتایم", "Uptime"), uptimeText(rep.Uptime24), uptimeText(rep.Uptime7d))}
	window := func(label string, count int, secs int64) string {
		if count == 0 {
			return "• " + label + ": ✅ " + b.tr("بدون قطعی", "no outage")
		}
		return fmt.Sprintf("• %s: %d %s · %s", label, count, b.tr("قطعی", "outages"), b.humanDuration(time.Duration(secs)*time.Second))
	}
	lines = append(lines,
		window(b.tr("۲۴ ساعت", "24 hours"), rep.Count24, rep.Down24),
		window(b.tr("۷ روز", "7 days"), rep.Count7d, rep.Down7d),
		window(b.tr("۳۰ روز", "30 days"), rep.Count30d, rep.Down30d),
		rule)
	if len(rep.Outages) == 0 {
		lines = append(lines, "✅ "+b.tr("هیچ قطعی‌ای ثبت نشده است.", "No outage recorded."))
	}
	for i, o := range rep.Outages {
		if i == nodeOutagesListedInBot {
			lines = append(lines, b.t("andMore", len(rep.Outages)-nodeOutagesListedInBot))
			break
		}
		icon := "🔴"
		if o.State == "core-stopped" {
			icon = "🟠"
		}
		end, last := b.tr("⏳ ادامه دارد", "⏳ ongoing"), now.Unix()
		if o.End > 0 {
			end, last = b.stamp(o.End), o.End
		}
		line := fmt.Sprintf("%s %s → %s · <b>%s</b>", icon, b.stamp(o.Start), end, b.humanDuration(time.Duration(max(last-o.Start, 0))*time.Second))
		if o.Reason != "" {
			line += "\n    <i>" + esc(truncate(o.Reason, 80)) + "</i>"
		}
		lines = append(lines, line)
	}
	kb := [][]button{
		{{Text: b.tr("🔄 تازه‌سازی", "🔄 Refresh"), Data: "o:nd:otg:" + sid}, {Text: b.tr("🧪 تست اتصال", "🧪 Connection test"), Data: "o:nd:ntest:" + sid}},
		b.navRow(back),
	}
	return strings.Join(lines, "\n"), kb
}

// ---- outbounds from share links and subscriptions ----

// outboundImportMax caps one import.
const outboundImportMax = 100

type outboundImport struct {
	added  []string
	failed []string
}

// importOutbounds adds the outbounds of the share links in text, one per
// line, and of the subscriptions behind the http(s) addresses among them.
// Tags are kept unique: a taken one gets a number.
func (b *bot) importOutbounds(text string) outboundImport {
	var res outboundImport
	var found []map[string]interface{}
	for _, line := range strings.Fields(stripFence(text)) {
		lower := strings.ToLower(line)
		switch {
		case strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://"):
			outs, err := util.GetExternalSub(line)
			if err != nil {
				res.failed = append(res.failed, esc(truncate(line, 40))+": "+esc(err.Error()))
				continue
			}
			found = append(found, outs...)
		case strings.Contains(line, "://"):
			m, _, err := util.GetOutbound(line, 0)
			if err != nil || m == nil {
				msg := "?"
				if err != nil {
					msg = err.Error()
				}
				res.failed = append(res.failed, esc(truncate(line, 40))+": "+esc(msg))
				continue
			}
			found = append(found, *m)
		default:
			res.failed = append(res.failed, esc(truncate(line, 40))+": "+b.tr("لینک نیست", "not a link"))
		}
	}
	if len(found) > outboundImportMax {
		res.failed = append(res.failed, b.tr(fmt.Sprintf("فقط %d مورد اول اضافه شد", outboundImportMax), fmt.Sprintf("only the first %d were added", outboundImportMax)))
		found = found[:outboundImportMax]
	}
	if len(found) == 0 {
		return res
	}
	taken := map[string]bool{}
	for _, code := range []string{"out", "ep"} {
		if items, err := b.objItems(kindByCode(code)); err == nil {
			for _, it := range items {
				taken[it.Tag] = true
			}
		}
	}
	k := kindByCode("out")
	for _, raw := range found {
		m := normalize(raw)
		if m == nil {
			continue
		}
		delete(m, "id")
		tag := strings.TrimSpace(scalar(m["tag"]))
		if tag == "" {
			tag = scalar(m["type"]) + "-" + scalar(m["server"])
		}
		m["tag"] = uniqueTag(tag, taken)
		if err := b.saveObj(k, "new", m); err != nil {
			res.failed = append(res.failed, esc(scalar(m["tag"]))+": "+esc(b.errText(err)))
			continue
		}
		taken[scalar(m["tag"])] = true
		res.added = append(res.added, scalar(m["tag"]))
	}
	return res
}

// uniqueTag is tag, or tag-2, tag-3, ... when it is taken.
func uniqueTag(tag string, taken map[string]bool) string {
	if !taken[tag] {
		return tag
	}
	for i := 2; ; i++ {
		if t := tag + "-" + strconv.Itoa(i); !taken[t] {
			return t
		}
	}
}

// importReport tells what an import did.
func (b *bot) importReport(r outboundImport) string {
	var lines []string
	if len(r.added) > 0 {
		lines = append(lines, "✅ "+b.tr(fmt.Sprintf("%d اوت‌باند اضافه شد:", len(r.added)), fmt.Sprintf("%d outbounds added:", len(r.added))))
		for i, t := range r.added {
			if i == 30 {
				lines = append(lines, b.t("andMore", len(r.added)-30))
				break
			}
			lines = append(lines, "• <code>"+esc(t)+"</code>")
		}
	}
	if len(r.failed) > 0 {
		lines = append(lines, "⚠️ "+b.tr("این‌ها اضافه نشدند:", "These were not added:"))
		for i, f := range r.failed {
			if i == 15 {
				lines = append(lines, b.t("andMore", len(r.failed)-15))
				break
			}
			lines = append(lines, "• "+f)
		}
	}
	return strings.Join(lines, "\n")
}

func (b *bot) outboundLinkPrompt() string {
	return "🔗 " + b.tr(
		"لینک‌های کانفیگ (vless://، vmess://، trojan://، ss://، hy2://، tuic://، ...) را هر کدام در یک خط بفرستید، یا لینک سابسکریپشن (http/https) را. همه به‌عنوان اوت‌باند اضافه می‌شوند.",
		"Send config links (vless://, vmess://, trojan://, ss://, hy2://, tuic://, ...), one per line, or a subscription address (http/https). Each becomes an outbound.")
}
