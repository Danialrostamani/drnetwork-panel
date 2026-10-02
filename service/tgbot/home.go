package tgbot

import (
	"fmt"
	"strings"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/config"
	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/service"
)

func asMap(v interface{}) map[string]interface{} {
	switch m := v.(type) {
	case map[string]interface{}:
		if m != nil {
			return m
		}
	case map[string]int64:
		out := make(map[string]interface{}, len(m))
		for k, n := range m {
			out[k] = n
		}
		return out
	}
	return map[string]interface{}{}
}

func stripMask(addrs []string) []string {
	out := make([]string, 0, len(addrs))
	for _, a := range addrs {
		if i := strings.IndexByte(a, '/'); i >= 0 {
			a = a[:i]
		}
		out = append(out, a)
	}
	return out
}

func asStrings(v interface{}) []string {
	var out []string
	switch list := v.(type) {
	case []string:
		out = list
	case []interface{}:
		for _, e := range list {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
	}
	return out
}

func (b *bot) meter(icon, label string, cur, total uint64) string {
	p := pctOf(cur, total)
	return fmt.Sprintf("%s <b>%s</b> %s %d%%\n      %s / %s", icon, label, bar(p, 10), p, humanBytes(int64(cur)), humanBytes(int64(total)))
}

// homeText is the panel's Home page: server resources, the core, and totals.
func (b *bot) homeText() string {
	st := *(&service.ServerService{}).GetStatus("cpu,mem,dsk,swp,net,sys,sbd,db")
	sys := asMap(st["sys"])
	mem, dsk, swp := asMap(st["mem"]), asMap(st["dsk"]), asMap(st["swp"])
	netw, sbd, dbi := asMap(st["net"]), asMap(st["sbd"]), asMap(st["db"])

	cpu := toFloat(st["cpu"])
	lines := []string{b.header("🏠", "DrNetwork "+esc(config.GetVersion()))}
	if h, _ := sys["hostName"].(string); h != "" {
		lines = append(lines, "🖥 <b>"+esc(h)+"</b>")
	}
	lines = append(lines, fmt.Sprintf("🧠 <b>CPU</b> %s %.0f%%\n      %v %s", bar(int(cpu), 10), cpu, sys["cpuCount"], b.tr("هسته", "cores")))
	if t := toFloat(mem["total"]); t > 0 {
		lines = append(lines, b.meter("💾", "RAM", uint64(toFloat(mem["current"])), uint64(t)))
	}
	if t := toFloat(dsk["total"]); t > 0 {
		lines = append(lines, b.meter("💽", b.tr("دیسک", "Disk"), uint64(toFloat(dsk["current"])), uint64(t)))
	}
	if t := toFloat(swp["total"]); t > 0 {
		lines = append(lines, b.meter("🔁", "Swap", uint64(toFloat(swp["current"])), uint64(t)))
	}
	if boot := int64(toFloat(sys["bootTime"])); boot > 0 {
		lines = append(lines, "⏱ "+b.tr("روشن بودن سرور", "Server uptime")+": "+b.humanDuration(time.Since(time.Unix(boot, 0))))
	}
	if v4 := stripMask(asStrings(sys["ipv4"])); len(v4) > 0 {
		lines = append(lines, "🌐 IPv4: <code>"+esc(strings.Join(v4, ", "))+"</code>")
	}
	if v6 := stripMask(asStrings(sys["ipv6"])); len(v6) > 0 {
		lines = append(lines, "🌐 IPv6: <code>"+esc(v6[0])+"</code>")
	}
	if netw["sent"] != nil {
		lines = append(lines, fmt.Sprintf("📶 %s: ↑ %s ↓ %s", b.tr("شبکه", "Network"), humanBytes(int64(toFloat(netw["sent"]))), humanBytes(int64(toFloat(netw["recv"])))))
	}

	lines = append(lines, "", b.header("⚙️", "sing-box"))
	running, _ := sbd["running"].(bool)
	core := "🔴 " + b.t("stopped")
	if running {
		stats := asMap(sbd["stats"])
		core = fmt.Sprintf("🟢 %s · %s · %s · %v goroutines", b.t("running"), b.humanDuration(time.Duration(toFloat(stats["Uptime"]))*time.Second), humanBytes(int64(toFloat(stats["Alloc"]))), stats["NumGoroutine"])
	}
	lines = append(lines, core)
	if m, _ := (&service.SettingService{}).GetMaintenance(); m {
		lines = append(lines, "🚧 "+b.tr("حالت نگهداری روشن است", "Maintenance mode is ON"))
	}

	lines = append(lines, "", b.header("📋", b.tr("خلاصه", "Summary")))
	lines = append(lines, fmt.Sprintf("📡 %v · 📤 %v · 🔌 %v · 🛠 %v", dbi["inbounds"], dbi["outbounds"], dbi["endpoints"], dbi["services"]))
	clients := loadClients()
	enabled := 0
	for _, c := range clients {
		if c.Enable {
			enabled++
		}
	}
	lines = append(lines, fmt.Sprintf("👥 %s: %d (%s: %d) · 🟢 %s: %d", b.tr("کلاینت", "Clients"), len(clients), b.tr("فعال", "active"), enabled, b.tr("آنلاین", "online"), len(onlineUsers())))
	lines = append(lines, fmt.Sprintf("📈 %s: ↑ %s ↓ %s", b.tr("مصرف کل", "Total"), humanBytes(int64(toFloat(dbi["clientUp"]))), humanBytes(int64(toFloat(dbi["clientDown"])))))
	var nodes []model.Node
	_ = database.GetDB().Where("enable = ?", true).Find(&nodes).Error
	if len(nodes) > 0 {
		statuses := (&service.NodeService{}).GetStatuses()
		up := 0
		for _, n := range nodes {
			if statuses[n.Id].State == "online" {
				up++
			}
		}
		lines = append(lines, fmt.Sprintf("🖥 %s: %d / %d %s", b.tr("نودها", "Nodes"), up, len(nodes), b.tr("آنلاین", "online")))
	}
	return strings.Join(lines, "\n")
}

func (b *bot) homeKeyboard() [][]button {
	return [][]button{
		{{Text: b.tr("🔄 به‌روزرسانی", "🔄 Refresh"), Data: "h:home"}, {Text: b.tr("🏠 منو", "🏠 Menu"), Data: "m:menu"}},
		{{Text: b.tr("♻️ ریستارت هسته", "♻️ Restart core"), Data: "m:restart"}, {Text: b.tr("🚧 نگهداری", "🚧 Maintenance"), Data: "m:maint"}},
		{{Text: b.tr("📜 لاگ‌ها", "📜 Logs"), Data: "m:logs:info"}, {Text: b.tr("💾 پشتیبان", "💾 Backup"), Data: "m:backup"}},
	}
}
