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

// The bot has two looks, picked by the tgBotSkin setting: "colorful" (the
// default) with coloured emoji bars, a small dashboard above the main menu and
// three buttons a row, and "classic", the plain look of the earlier versions.
const (
	skinColorful = "colorful"
	skinClassic  = "classic"
)

func normSkin(s string) string {
	if strings.ToLower(strings.TrimSpace(s)) == skinClassic {
		return skinClassic
	}
	return skinColorful
}

func (b *bot) colorful() bool { return b.cfg.Skin != skinClassic }

// bar draws a progress bar in the bot's look.
func (b *bot) bar(pct, width int) string {
	if b.colorful() {
		return colorBar(pct, width)
	}
	return bar(pct, width)
}

// colorBar is a bar of coloured squares: green while low, yellow from 60%
// and red from 85%.
func colorBar(pct, width int) string {
	pct = min(max(pct, 0), 100)
	n := (pct*width + 50) / 100
	fill := "🟩"
	switch {
	case pct >= 85:
		fill = "🟥"
	case pct >= 60:
		fill = "🟨"
	}
	return strings.Repeat(fill, n) + strings.Repeat("⬜", width-n)
}

// regrid lays the buttons of a keyboard out again, n to a row.
func regrid(kb [][]button, n int) [][]button {
	var flat []button
	for _, row := range kb {
		flat = append(flat, row...)
	}
	var out [][]button
	for len(flat) > 0 {
		k := min(n, len(flat))
		out = append(out, flat[:k])
		flat = flat[k:]
	}
	return out
}

// expiryBar shows how much of a client's time has gone, from its creation to
// its expiry; empty when the client has no end date.
func (b *bot) expiryBar(c model.Client, now time.Time) string {
	if !b.colorful() || c.DelayStart || c.Expiry <= 0 || c.CreatedAt <= 0 || c.Expiry <= c.CreatedAt {
		return ""
	}
	total := c.Expiry - c.CreatedAt
	gone := min(max(now.Unix()-c.CreatedAt, 0), total)
	p := int(gone * 100 / total)
	return fmt.Sprintf("      %s %d%%", colorBar(p, 10), p)
}

var dashTips = [][2]string{
	{"/client <i>نام</i> — پیدا کردن سریع یک کلاینت", "/client <i>name</i> — find a client quickly"},
	{"/online — کلاینت‌های آنلاین همین الان", "/online — who is online right now"},
	{"/add — ساختن کلاینت تازه", "/add — create a new client"},
	{"/nodes — وضعیت همه نودها", "/nodes — the state of every node"},
	{"/traffic — آمار مصرف", "/traffic — usage statistics"},
}

// dashText is the colourful dashboard: the core, CPU and RAM, the nodes, the
// clients and the traffic, in a few lines.
func (b *bot) dashText(withTip bool) string {
	st := *(&service.ServerService{}).GetStatus("cpu,mem,sbd")
	mem, sbd := asMap(st["mem"]), asMap(st["sbd"])
	lines := []string{"✨ <b>" + b.tr("پنل DrNetwork", "DrNetwork panel") + "</b> ✨  <i>" + esc(config.GetFullVersion()) + "</i>", ""}

	core := "🔴 " + b.tr("هسته خاموش", "Core stopped")
	if running, _ := sbd["running"].(bool); running {
		core = "🟢 " + b.tr("هسته روشن", "Core running")
		if up := toFloat(asMap(sbd["stats"])["Uptime"]); up > 0 {
			core += " ⏱ " + b.humanDuration(time.Duration(up)*time.Second)
		}
	}
	lines = append(lines, core)
	res := fmt.Sprintf("🧠 CPU <b>%.0f%%</b>", toFloat(st["cpu"]))
	if t := toFloat(mem["total"]); t > 0 {
		res += fmt.Sprintf("  |  💾 RAM <b>%d%%</b>", pctOf(uint64(toFloat(mem["current"])), uint64(t)))
	}
	lines = append(lines, res, "")

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
		lines = append(lines, fmt.Sprintf("🖥 %s ➜ 🟢 <b>%d</b>  🔴 <b>%d</b>", b.tr("نودها", "Nodes"), up, len(nodes)-up))
	}
	clients := b.loadClients()
	enabled := 0
	var upBytes, downBytes int64
	for _, c := range clients {
		if c.Enable {
			enabled++
		}
		upBytes += c.Up
		downBytes += c.Down
	}
	lines = append(lines,
		fmt.Sprintf("👥 %s ➜ ✅ <b>%d</b>  ⛔ <b>%d</b>", b.tr("کلاینت‌ها", "Clients"), enabled, len(clients)-enabled),
		fmt.Sprintf("⚡️ %s ➜ <b>%d</b> %s", b.tr("آنلاین الان", "Online now"), len(b.onlineUsers()), b.tr("نفر", "users")),
		fmt.Sprintf("📶 %s ➜ 🔼 %s  🔽 %s", b.tr("مصرف", "Usage"), humanBytes(upBytes), humanBytes(downBytes)))
	if withTip {
		tip := dashTips[time.Now().Minute()%len(dashTips)]
		lines = append(lines, "", "💡 <i>"+b.tr("نکته: ", "Tip: ")+"</i>"+b.tr(tip[0], tip[1]))
	}
	return strings.Join(lines, "\n")
}
