package tgbot

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"sort"
	"strings"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/config"
	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/logger"
	"github.com/Danialrostamani/drnetwork-panel/service"
)

func esc(s string) string { return html.EscapeString(s) }

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit && exp < 4; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %ciB", float64(n)/float64(div), "KMGTP"[exp])
}

func (b *bot) humanDuration(d time.Duration) string {
	switch {
	case d >= 24*time.Hour:
		return fmt.Sprintf("%d %s %d %s", int(d.Hours())/24, b.t("day"), int(d.Hours())%24, b.t("hour"))
	case d >= time.Hour:
		return fmt.Sprintf("%d %s %d %s", int(d.Hours()), b.t("hour"), int(d.Minutes())%60, b.t("min"))
	case d >= time.Minute:
		return fmt.Sprintf("%d %s", int(d.Minutes()), b.t("min"))
	}
	return fmt.Sprintf("%d %s", int(d.Seconds()), b.t("sec"))
}

func (b *bot) stamp(unix int64) string {
	if unix <= 0 {
		return b.t("never")
	}
	return time.Unix(unix, 0).In(b.loc).Format("2006-01-02 15:04")
}

// parseCommand splits "/cmd@botname arg text" into the lower-cased command and
// the trimmed argument.
func parseCommand(text string) (string, string) {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "/") {
		return "", ""
	}
	head, arg, _ := strings.Cut(text, " ")
	if i := strings.IndexByte(head, '@'); i >= 0 {
		head = head[:i]
	}
	return strings.ToLower(strings.TrimPrefix(head, "/")), strings.TrimSpace(arg)
}

func (b *bot) handle(ctx context.Context, u update) {
	m := u.Message
	if m == nil || m.Text == "" || m.From == nil {
		return
	}
	cmd, arg := parseCommand(m.Text)
	if cmd == "" {
		return
	}
	// Only private chats are served, and only for the listed administrators:
	// a group message must never leak panel data to the other members.
	if m.Chat.Type != "private" {
		return
	}
	if cmd == "id" || (cmd == "start" && !b.cfg.isAdmin(m.From.ID)) {
		b.send(ctx, m.Chat.ID, b.t("yourId", m.From.ID))
		return
	}
	if !b.cfg.isAdmin(m.From.ID) {
		b.send(ctx, m.Chat.ID, b.t("denied"))
		return
	}
	var reply string
	switch cmd {
	case "start", "help":
		reply = b.t("help")
	case "status":
		reply = b.statusText()
	case "nodes":
		reply = b.nodesText()
	case "online":
		reply = b.onlineText()
	case "clients", "client":
		reply = b.clientsText(arg)
	case "ips":
		reply = b.ipsText(arg)
	case "inbounds":
		reply = b.inboundsText()
	case "traffic":
		reply = b.trafficText()
	default:
		reply = b.t("unknown")
	}
	b.send(ctx, m.Chat.ID, reply)
}

func toFloat(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case uint64:
		return float64(n)
	case int64:
		return float64(n)
	case int:
		return float64(n)
	case uint32:
		return float64(n)
	}
	return 0
}

func loadClients() []model.Client {
	var clients []model.Client
	err := database.GetDB().Model(model.Client{}).
		Select("`id`, `enable`, `name`, `desc`, `group`, `up`, `down`, `volume`, `expiry`, `created_at`, `online_at`, `limit_ip`, `delay_start`, `reset_days`").
		Scan(&clients).Error
	if err != nil {
		logger.Warning("telegram bot: load clients: ", err)
		return nil
	}
	return clients
}

func onlineUsers() []string {
	o, err := (&service.StatsService{}).GetClusterOnlines()
	if err != nil {
		return nil
	}
	users := append([]string(nil), o.User...)
	sort.Strings(users)
	return users
}

func (b *bot) statusText() string {
	status := (&service.ServerService{}).GetStatus("cpu,mem,sbd")
	var lines []string
	lines = append(lines, b.t("status", esc(config.GetVersion())))
	mem, _ := (*status)["mem"].(map[string]interface{})
	lines = append(lines, b.t("cpuMem", toFloat((*status)["cpu"]), humanBytes(int64(toFloat(mem["current"]))), humanBytes(int64(toFloat(mem["total"])))))
	coreState, uptime := b.t("stopped"), int64(0)
	if sbd, ok := (*status)["sbd"].(map[string]interface{}); ok {
		if running, _ := sbd["running"].(bool); running {
			coreState = b.t("running")
			if st, ok := sbd["stats"].(map[string]interface{}); ok {
				uptime = int64(toFloat(st["Uptime"]))
			}
		}
	}
	lines = append(lines, b.t("core", coreState, b.humanDuration(time.Duration(uptime)*time.Second)))

	var nodes []model.Node
	_ = database.GetDB().Where("enable = ?", true).Find(&nodes).Error
	statuses := (&service.NodeService{}).GetStatuses()
	up := 0
	for _, n := range nodes {
		if statuses[n.Id].State == "online" {
			up++
		}
	}
	if len(nodes) > 0 {
		lines = append(lines, b.t("nodesLine", up, len(nodes)))
	}
	clients := loadClients()
	enabled := 0
	var upBytes, downBytes int64
	for _, c := range clients {
		if c.Enable {
			enabled++
		}
		upBytes += c.Up
		downBytes += c.Down
	}
	lines = append(lines, b.t("clientsLine", len(clients), enabled, len(onlineUsers())))
	lines = append(lines, b.t("trafficLine", humanBytes(upBytes), humanBytes(downBytes)))
	return strings.Join(lines, "\n")
}

func (b *bot) nodesText() string {
	var nodes []model.Node
	if err := database.GetDB().Order("id").Find(&nodes).Error; err != nil || len(nodes) == 0 {
		return b.t("noNodes")
	}
	statuses := (&service.NodeService{}).GetStatuses()
	var lines []string
	for _, n := range nodes {
		st, probed := statuses[n.Id]
		icon, state := "🔴", b.t("nodeOffline")
		switch {
		case !n.Enable:
			icon, state = "⚪️", b.t("disabled")
		case st.State == "online":
			icon, state = "🟢", b.t("nodeOnline")
		case st.State == "core-stopped":
			icon, state = "🟠", b.t("nodeCore")
		}
		lines = append(lines, fmt.Sprintf("%s <b>%s</b> — %s", icon, esc(n.Name), state))
		if n.Enable && probed && st.State != "" && st.State != "offline" {
			memPct := 0.0
			if st.Mem.Total > 0 {
				memPct = float64(st.Mem.Current) * 100 / float64(st.Mem.Total)
			}
			lines = append(lines, fmt.Sprintf("   CPU %.0f%% · RAM %.0f%% · %d ms · v%s", st.Cpu, memPct, st.Latency, esc(st.AppVersion)))
		} else if st.Error != "" {
			lines = append(lines, "   "+esc(st.Error))
		}
		seen := n.LastSeen
		if st.LastOnline > seen {
			seen = st.LastOnline
		}
		if st.State != "online" {
			lines = append(lines, fmt.Sprintf("   %s: %s", b.t("lastSeen"), b.stamp(seen)))
		}
	}
	return strings.Join(lines, "\n")
}

func (b *bot) onlineText() string {
	users := onlineUsers()
	if len(users) == 0 {
		return b.t("noOnline")
	}
	const shown = 60
	lines := []string{b.t("onlineTitle", len(users))}
	for i, name := range users {
		if i == shown {
			lines = append(lines, b.t("andMore", len(users)-shown))
			break
		}
		lines = append(lines, "• "+esc(name))
	}
	return strings.Join(lines, "\n")
}

// usagePercent is -1 for an unlimited client.
func usagePercent(c model.Client) int {
	if c.Volume <= 0 {
		return -1
	}
	return int((c.Up + c.Down) * 100 / c.Volume)
}

func (b *bot) expiryText(c model.Client, now time.Time) string {
	switch {
	case c.DelayStart:
		return b.t("delayStart", c.ResetDays)
	case c.Expiry <= 0:
		return b.t("unlimited")
	}
	left := time.Unix(c.Expiry, 0).Sub(now)
	switch {
	case left <= 0:
		return b.t("expired")
	case left < 48*time.Hour:
		return b.t("hoursLeft", int(left.Hours()))
	}
	return fmt.Sprintf("%s (%s)", b.stamp(c.Expiry), b.t("daysLeft", int(left.Hours()/24)))
}

func (b *bot) clientLine(c model.Client, now time.Time) string {
	icon := "🟢"
	if !c.Enable {
		icon = "🔴"
	}
	usage := humanBytes(c.Up + c.Down)
	if c.Volume > 0 {
		usage = fmt.Sprintf("%s / %s (%d%%)", usage, humanBytes(c.Volume), usagePercent(c))
	} else {
		usage += " / " + b.t("unlimited")
	}
	return fmt.Sprintf("%s <b>%s</b> — %s — %s", icon, esc(c.Name), usage, b.expiryText(c, now))
}

func (b *bot) clientDetail(c model.Client, online bool, now time.Time) string {
	state := b.t("enabled")
	if !c.Enable {
		state = b.t("disabled")
	}
	dot := ""
	if online {
		dot = " 🟢"
	}
	lines := []string{
		fmt.Sprintf("👤 <b>%s</b> — %s%s", esc(c.Name), state, dot),
	}
	if c.Desc != "" {
		lines = append(lines, esc(c.Desc))
	}
	usage := fmt.Sprintf("%s (↑ %s ↓ %s)", humanBytes(c.Up+c.Down), humanBytes(c.Up), humanBytes(c.Down))
	if c.Volume > 0 {
		usage += fmt.Sprintf(" / %s · %d%%", humanBytes(c.Volume), usagePercent(c))
	} else {
		usage += " / " + b.t("unlimited")
	}
	lines = append(lines,
		fmt.Sprintf("%s: %s", b.t("usage"), usage),
		fmt.Sprintf("%s: %s", b.t("expiry"), b.expiryText(c, now)),
	)
	if c.Group != "" {
		lines = append(lines, fmt.Sprintf("%s: %s", b.t("group"), esc(c.Group)))
	}
	if c.LimitIp > 0 {
		lines = append(lines, fmt.Sprintf("%s: %d", b.t("ipLimit"), c.LimitIp))
	}
	lines = append(lines,
		fmt.Sprintf("%s: %s", b.t("lastOnline"), b.stamp(c.OnlineAt)),
		fmt.Sprintf("%s: %s", b.t("createdAt"), b.stamp(c.CreatedAt)),
	)
	return strings.Join(lines, "\n")
}

func (b *bot) clientsText(query string) string {
	clients := loadClients()
	now := time.Now()
	if query == "" {
		type scored struct {
			c     model.Client
			score int
		}
		var near []scored
		for _, c := range clients {
			score := 0
			if p := usagePercent(c); p >= 80 {
				score = p
			}
			if c.Expiry > 0 {
				if days := int(time.Unix(c.Expiry, 0).Sub(now).Hours() / 24); days <= 7 && score < 100-days {
					score = 100 - days
				}
			}
			if score > 0 {
				near = append(near, scored{c, score})
			}
		}
		if len(near) == 0 {
			return b.t("noNear")
		}
		sort.Slice(near, func(i, j int) bool { return near[i].score > near[j].score })
		lines := []string{b.t("nearTitle")}
		for i, n := range near {
			if i == 25 {
				lines = append(lines, b.t("andMore", len(near)-25))
				break
			}
			lines = append(lines, b.clientLine(n.c, now))
		}
		return strings.Join(lines, "\n")
	}
	needle := strings.ToLower(query)
	var found []model.Client
	for _, c := range clients {
		if strings.ToLower(c.Name) == needle {
			found = []model.Client{c}
			break
		}
		if strings.Contains(strings.ToLower(c.Name), needle) || strings.Contains(strings.ToLower(c.Desc), needle) {
			found = append(found, c)
		}
	}
	switch len(found) {
	case 0:
		return b.t("noClients")
	case 1:
		online := false
		for _, u := range onlineUsers() {
			if u == found[0].Name {
				online = true
			}
		}
		return b.clientDetail(found[0], online, now)
	}
	lines := []string{b.t("foundTitle", len(found))}
	for i, c := range found {
		if i == 25 {
			lines = append(lines, b.t("andMore", len(found)-25))
			break
		}
		lines = append(lines, b.clientLine(c, now))
	}
	return strings.Join(lines, "\n")
}

func (b *bot) ipsText(name string) string {
	if name == "" {
		return b.t("ipsUsage")
	}
	var client *model.Client
	for _, c := range loadClients() {
		if strings.EqualFold(c.Name, name) {
			c := c
			client = &c
			break
		}
	}
	if client == nil {
		return b.t("noClients")
	}
	ips, ok := service.ClusterOnlineIPsOf(client.Name)
	if !ok {
		ips = service.OnlineIPsOf(client.Name)
	}
	if len(ips) == 0 {
		text := b.t("noIps")
		if client.LimitIp <= 0 {
			text += "\n" + b.t("ipsNoLimit")
		}
		return text
	}
	lines := []string{b.t("ipsTitle", esc(client.Name), len(ips))}
	for _, ip := range ips {
		lines = append(lines, "• <code>"+esc(ip.IP)+"</code>")
	}
	return strings.Join(lines, "\n")
}

func (b *bot) inboundsText() string {
	var inbounds []model.Inbound
	if err := database.GetDB().Order("id").Find(&inbounds).Error; err != nil || len(inbounds) == 0 {
		return b.t("noInbounds")
	}
	online := map[string]bool{}
	if o, err := (&service.StatsService{}).GetClusterOnlines(); err == nil {
		for _, tag := range o.Inbound {
			online[tag] = true
		}
	}
	lines := []string{b.t("inboundsHead", len(inbounds))}
	for _, in := range inbounds {
		icon := "⚪️"
		if online[in.Tag] {
			icon = "🟢"
		}
		var opts struct {
			Port int `json:"listen_port"`
		}
		_ = json.Unmarshal(in.Options, &opts)
		line := fmt.Sprintf("%s <b>%s</b> — %s", icon, esc(in.Tag), esc(in.Type))
		if opts.Port > 0 {
			line += fmt.Sprintf(" :%d", opts.Port)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func (b *bot) trafficText() string {
	clients := loadClients()
	var up, down int64
	for _, c := range clients {
		up += c.Up
		down += c.Down
	}
	sort.Slice(clients, func(i, j int) bool { return clients[i].Up+clients[i].Down > clients[j].Up+clients[j].Down })
	lines := []string{
		b.t("trafficHead"),
		b.t("trafficLine", humanBytes(up), humanBytes(down)),
	}
	if len(clients) > 0 {
		lines = append(lines, "", b.t("topUsers"))
	}
	for i, c := range clients {
		if i == 10 || c.Up+c.Down == 0 {
			break
		}
		lines = append(lines, fmt.Sprintf("%d. <b>%s</b> — %s", i+1, esc(c.Name), humanBytes(c.Up+c.Down)))
	}
	return strings.Join(lines, "\n")
}
