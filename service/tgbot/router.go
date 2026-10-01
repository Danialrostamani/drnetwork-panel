package tgbot

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/logger"
	"github.com/Danialrostamani/drnetwork-panel/service"
)

type button struct {
	Text string `json:"text"`
	Data string `json:"callback_data,omitempty"`
}

func (b *bot) btn(key, data string, args ...interface{}) button {
	return button{Text: b.t(key, args...), Data: data}
}

func (b *bot) mainMenu() [][]button {
	return [][]button{
		{b.btn("btnStatus", "m:status"), b.btn("btnNodes", "m:nodes")},
		{b.btn("btnOnline", "m:online"), b.btn("btnClients", "m:clients")},
		{b.btn("btnInbounds", "m:inbounds"), b.btn("btnTraffic", "m:traffic")},
		{b.btn("btnBackup", "m:backup"), b.btn("btnLogs", "m:logs")},
		{b.btn("btnSync", "m:sync"), b.btn("btnRestart", "m:restart")},
	}
}

func (b *bot) menuRow() []button { return []button{b.btn("btnMenu", "m:menu")} }

func (b *bot) clientKeyboard(c model.Client) [][]button {
	id := strconv.FormatUint(uint64(c.Id), 10)
	toggle := b.btn("btnDisable", "c:tog:"+id)
	if !c.Enable {
		toggle = b.btn("btnEnable", "c:tog:"+id)
	}
	return [][]button{
		{toggle, b.btn("btnReset", "c:rst:"+id)},
		{b.btn("btnAddVol", "c:gb10:"+id, 10), b.btn("btnAddVol", "c:gb50:"+id, 50), b.btn("btnAddDays", "c:d30:"+id, 30)},
		{b.btn("btnSub", "c:sub:"+id), b.btn("btnIps", "c:ips:"+id)},
		{b.btn("btnDelete", "c:del:"+id), b.btn("btnRefresh", "c:view:"+id)},
		b.menuRow(),
	}
}

// clientButtons lists clients as buttons, two per row, capped to keep the
// keyboard readable.
func (b *bot) clientButtons(clients []model.Client) [][]button {
	rows := [][]button{}
	var row []button
	for i, c := range clients {
		if i == 20 {
			break
		}
		row = append(row, button{Text: c.Name, Data: "c:view:" + strconv.FormatUint(uint64(c.Id), 10)})
		if len(row) == 2 {
			rows = append(rows, row)
			row = nil
		}
	}
	if len(row) > 0 {
		rows = append(rows, row)
	}
	return append(rows, b.menuRow())
}

type commandInfo struct{ name, fa, en string }

var adminCommands = []commandInfo{
	{"menu", "منوی دکمه‌ای", "Button menu"},
	{"status", "وضعیت پنل و کلاستر", "Panel and cluster status"},
	{"nodes", "وضعیت نودها", "Node status"},
	{"online", "کاربران آنلاین", "Online users"},
	{"clients", "کلاینت‌ها / جستجو", "Clients / search"},
	{"add", "ساخت کلاینت", "Create a client"},
	{"enable", "فعال‌سازی کلاینت", "Enable a client"},
	{"disable", "غیرفعال‌سازی کلاینت", "Disable a client"},
	{"reset", "ریست ترافیک کلاینت", "Reset client traffic"},
	{"volume", "تعیین حجم", "Set volume"},
	{"expiry", "تعیین انقضا", "Set expiry"},
	{"limitip", "محدودیت IP", "Set IP limit"},
	{"del", "حذف کلاینت", "Delete a client"},
	{"sub", "لینک اشتراک و QR", "Subscription link and QR"},
	{"ips", "IP های آنلاین کلاینت", "Online IPs of a client"},
	{"bind", "اتصال کلاینت به تلگرام", "Bind a client to Telegram"},
	{"unbind", "قطع اتصال تلگرام", "Unbind Telegram"},
	{"inbounds", "اینباندها", "Inbounds"},
	{"traffic", "مصرف کل", "Total traffic"},
	{"backup", "پشتیبان دیتابیس", "Database backup"},
	{"logs", "لاگ‌ها", "Logs"},
	{"sync", "همگام‌سازی نودها", "Sync nodes"},
	{"restart", "ریستارت هسته", "Restart core"},
	{"maintenance", "حالت نگهداری", "Maintenance mode"},
	{"help", "راهنما", "Help"},
}

var userCommands = []commandInfo{
	{"usage", "وضعیت اشتراک من", "My subscription status"},
	{"sub", "لینک اشتراک و QR", "Subscription link and QR"},
	{"id", "شناسه تلگرام من", "My Telegram ID"},
}

func (b *bot) registerCommands(ctx context.Context) {
	build := func(list []commandInfo) []map[string]string {
		out := make([]map[string]string, 0, len(list))
		for _, c := range list {
			desc := c.en
			if b.cfg.Lang == "fa" {
				desc = c.fa
			}
			out = append(out, map[string]string{"command": c.name, "description": desc})
		}
		return out
	}
	if err := b.call(ctx, "setMyCommands", map[string]any{"commands": build(userCommands)}, nil); err != nil && ctx.Err() == nil {
		logger.Warning("telegram bot: set commands: ", err)
	}
	for _, id := range b.cfg.Admins {
		_ = b.call(ctx, "setMyCommands", map[string]any{
			"commands": build(append(append([]commandInfo{}, adminCommands...), userCommands[2])),
			"scope":    map[string]any{"type": "chat", "chat_id": id},
		}, nil)
	}
}

func (b *bot) handle(ctx context.Context, u update) {
	if u.Callback != nil {
		b.handleCallback(ctx, u)
		return
	}
	m := u.Message
	if m == nil || m.Text == "" || m.From == nil {
		return
	}
	cmd, arg := parseCommand(m.Text)
	if cmd == "" {
		return
	}
	// Only private chats are served: a group message must never leak panel
	// data to the other members.
	if m.Chat.Type != "private" {
		return
	}
	chatID, from := m.Chat.ID, m.From.ID
	if cmd == "id" {
		b.send(ctx, chatID, b.t("yourId", from))
		return
	}
	if b.cfg.isAdmin(from) {
		b.adminCommand(ctx, chatID, cmd, arg)
		return
	}
	bound := boundClients(from)
	if len(bound) == 0 {
		if cmd == "start" {
			b.send(ctx, chatID, b.t("yourId", from))
		} else {
			b.send(ctx, chatID, b.t("denied"))
		}
		return
	}
	b.userCommand(ctx, chatID, bound, cmd)
}

func boundClients(tgID int64) []model.Client {
	var out []model.Client
	for _, c := range loadClients() {
		if c.TgId == tgID {
			out = append(out, c)
		}
	}
	return out
}

// ---- self-service for bound clients ----

func (b *bot) userView(c model.Client) (string, [][]button) {
	c.Desc, c.Group = "", ""
	c.TgId = 0
	online := false
	for _, n := range onlineUsers() {
		if n == c.Name {
			online = true
		}
	}
	id := strconv.FormatUint(uint64(c.Id), 10)
	return b.clientDetail(c, online, time.Now()), [][]button{{b.btn("btnSub", "u:sub:"+id), b.btn("btnRefresh", "u:view:"+id)}}
}

func (b *bot) userCommand(ctx context.Context, chatID int64, bound []model.Client, cmd string) {
	switch cmd {
	case "usage", "start", "help":
		if cmd != "usage" {
			b.send(ctx, chatID, b.t("userHelp"))
		}
		for _, c := range bound {
			text, kb := b.userView(c)
			b.sendKeyboard(ctx, chatID, text, kb)
		}
	case "sub":
		for _, c := range bound {
			b.sendSub(ctx, chatID, c.Name)
		}
	default:
		b.send(ctx, chatID, b.t("userHelp"))
	}
}

// sendSub sends the subscription link and its QR code.
func (b *bot) sendSub(ctx context.Context, chatID int64, name string) {
	link, err := b.subLink(name)
	if err != nil {
		b.send(ctx, chatID, b.t("failed", esc(err.Error())))
		return
	}
	b.send(ctx, chatID, b.t("subTitle", esc(name))+"\n<code>"+esc(link)+"</code>")
	png, err := qrPNG(link)
	if err != nil {
		b.send(ctx, chatID, b.t("subLinkOnly"))
		return
	}
	if err := b.upload(ctx, "sendPhoto", "photo", chatID, "sub.png", "", png); err != nil {
		logger.Warning("telegram bot: send QR: ", err)
	}
}

// ---- admin commands ----

func parseFloatArg(s string) (float64, bool) {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return v, err == nil && v >= 0 && v < 1e9
}

func parseIntArg(s string) (int, bool) {
	v, err := strconv.Atoi(strings.TrimSpace(s))
	return v, err == nil && v >= 0 && v < 1e7
}

func (b *bot) card(c model.Client) (string, [][]button) {
	online := false
	for _, n := range onlineUsers() {
		if n == c.Name {
			online = true
		}
	}
	return b.clientDetail(c, online, time.Now()), b.clientKeyboard(c)
}

func (b *bot) sendCard(ctx context.Context, chatID int64, prefix string, id uint) {
	for _, c := range loadClients() {
		if c.Id == id {
			text, kb := b.card(c)
			if prefix != "" {
				text = prefix + "\n\n" + text
			}
			b.sendKeyboard(ctx, chatID, text, kb)
			return
		}
	}
}

func (b *bot) logsText() string {
	logs := (&service.ServerService{}).GetLogs("40", "info")
	if len(logs) == 0 {
		return b.t("noLogs")
	}
	var kept []string
	total := 0
	for i := len(logs) - 1; i >= 0; i-- {
		line := strings.TrimSpace(logs[i])
		if r := []rune(line); len(r) > 300 {
			line = string(r[:300]) + "…"
		}
		if total+len([]rune(line))+1 > 3300 {
			break
		}
		total += len([]rune(line)) + 1
		kept = append([]string{esc(line)}, kept...)
	}
	return b.t("logsTitle") + "\n<pre>" + strings.Join(kept, "\n") + "</pre>"
}

func (b *bot) sendBackup(ctx context.Context, chatID int64) {
	data, err := database.GetDb("")
	if err != nil {
		b.send(ctx, chatID, b.t("failed", esc(err.Error())))
		return
	}
	stamp := time.Now().In(b.loc).Format("20060102_150405")
	if err := b.upload(ctx, "sendDocument", "document", chatID, "s-ui_"+stamp+".db", b.t("backupCaption", stamp), data); err != nil {
		b.send(ctx, chatID, b.t("failed", esc(err.Error())))
	}
}

func (b *bot) fail(ctx context.Context, chatID int64, err error) {
	b.send(ctx, chatID, b.t("failed", esc(b.errText(err))))
}

func (b *bot) errText(err error) string {
	switch err.Error() {
	case "unlimited":
		return b.t("unlimitedNo")
	case "delay start":
		return b.t("useWeb")
	case "bad name":
		return b.t("badName")
	}
	return err.Error()
}

func (b *bot) adminCommand(ctx context.Context, chatID int64, cmd, arg string) {
	fields := strings.Fields(arg)
	nameArg := ""
	if len(fields) > 0 {
		nameArg = fields[0]
	}
	// withClient resolves the client named by the first argument.
	withClient := func(usageKey string) *model.Client {
		if nameArg == "" {
			if usageKey != "" {
				b.send(ctx, chatID, b.t(usageKey))
			} else {
				b.send(ctx, chatID, b.t("nameUsage", cmd))
			}
			return nil
		}
		c := findClientByName(nameArg)
		if c == nil {
			b.send(ctx, chatID, b.t("notFound"))
		}
		return c
	}
	finish := func(c *model.Client, err error) {
		if err != nil {
			b.fail(ctx, chatID, err)
			return
		}
		b.sendCard(ctx, chatID, b.t("done"), c.Id)
	}
	switch cmd {
	case "start", "menu":
		b.sendKeyboard(ctx, chatID, b.t("menuTitle"), b.mainMenu())
	case "help":
		b.send(ctx, chatID, b.t("help")+b.t("helpAdmin2"))
	case "status":
		b.sendKeyboard(ctx, chatID, b.statusText(), [][]button{b.menuRow()})
	case "nodes":
		b.send(ctx, chatID, b.nodesText())
	case "online":
		b.send(ctx, chatID, b.onlineText())
	case "clients", "client":
		text, kb := b.clientsView(arg)
		b.sendKeyboard(ctx, chatID, text, kb)
	case "ips":
		b.send(ctx, chatID, b.ipsText(arg))
	case "inbounds":
		b.send(ctx, chatID, b.inboundsText())
	case "traffic":
		b.send(ctx, chatID, b.trafficText())
	case "add":
		b.cmdAdd(ctx, chatID, fields)
	case "enable", "disable":
		if c := withClient(""); c != nil {
			finish(c, b.setEnabled(c.Id, cmd == "enable"))
		}
	case "reset":
		if c := withClient(""); c != nil {
			b.sendKeyboard(ctx, chatID, b.t("confirmReset", esc(c.Name)), b.confirmKeyboard("c:rsty:", c.Id))
		}
	case "del", "delete":
		if c := withClient(""); c != nil {
			b.sendKeyboard(ctx, chatID, b.t("confirmDelete", esc(c.Name)), b.confirmKeyboard("c:dely:", c.Id))
		}
	case "volume":
		c := withClient("volumeUsage")
		if c == nil {
			return
		}
		gb, ok := parseFloatArg(strings.Join(fields[min(1, len(fields)):], ""))
		if len(fields) < 2 || !ok {
			b.send(ctx, chatID, b.t("volumeUsage"))
			return
		}
		finish(c, b.setVolume(c.Id, int64(gb*float64(gib))))
	case "expiry":
		c := withClient("expiryUsage")
		if c == nil {
			return
		}
		days, ok := parseIntArg(strings.Join(fields[min(1, len(fields)):], ""))
		if len(fields) < 2 || !ok {
			b.send(ctx, chatID, b.t("expiryUsage"))
			return
		}
		finish(c, b.setExpiryDays(c.Id, days))
	case "limitip":
		c := withClient("limitUsage")
		if c == nil {
			return
		}
		n, ok := parseIntArg(strings.Join(fields[min(1, len(fields)):], ""))
		if len(fields) < 2 || !ok {
			b.send(ctx, chatID, b.t("limitUsage"))
			return
		}
		finish(c, b.setLimitIP(c.Id, n))
	case "bind":
		c := withClient("bindUsage")
		if c == nil {
			return
		}
		var tgID int64
		if len(fields) >= 2 {
			tgID, _ = strconv.ParseInt(fields[1], 10, 64)
		}
		if tgID <= 0 {
			b.send(ctx, chatID, b.t("bindUsage"))
			return
		}
		if err := b.bindClient(c.Id, tgID); err != nil {
			b.fail(ctx, chatID, err)
			return
		}
		b.send(ctx, chatID, b.t("bound", esc(c.Name), tgID))
	case "unbind":
		if c := withClient("bindUsage"); c != nil {
			if err := b.bindClient(c.Id, 0); err != nil {
				b.fail(ctx, chatID, err)
				return
			}
			b.send(ctx, chatID, b.t("unbound", esc(c.Name)))
		}
	case "sub":
		if c := withClient(""); c != nil {
			b.sendSub(ctx, chatID, c.Name)
		}
	case "backup":
		b.sendBackup(ctx, chatID)
	case "logs":
		b.send(ctx, chatID, b.logsText())
	case "sync":
		b.fanOut()
		b.send(ctx, chatID, b.t("syncStarted"))
	case "restart":
		b.sendKeyboard(ctx, chatID, b.t("confirmRestart"), [][]button{{b.btn("btnConfirm", "m:restarty"), b.btn("btnCancel", "m:menu")}})
	case "maintenance":
		b.cmdMaintenance(ctx, chatID, strings.ToLower(nameArg))
	default:
		b.send(ctx, chatID, b.t("unknown"))
	}
}

func (b *bot) confirmKeyboard(prefix string, id uint) [][]button {
	sid := strconv.FormatUint(uint64(id), 10)
	return [][]button{{b.btn("btnConfirm", prefix+sid), b.btn("btnCancel", "c:view:"+sid)}}
}

func (b *bot) cmdAdd(ctx context.Context, chatID int64, f []string) {
	if len(f) < 3 {
		b.send(ctx, chatID, b.t("addUsage"))
		return
	}
	if !clientNameRe.MatchString(f[0]) {
		b.send(ctx, chatID, b.t("badName"))
		return
	}
	gb, ok1 := parseFloatArg(f[1])
	days, ok2 := parseIntArg(f[2])
	limit, ok3 := 0, true
	if len(f) >= 4 {
		limit, ok3 = parseIntArg(f[3])
	}
	if !ok1 || !ok2 || !ok3 {
		b.send(ctx, chatID, b.t("badNumber"))
		return
	}
	if err := b.createClient(f[0], int64(gb*float64(gib)), days, limit); err != nil {
		b.fail(ctx, chatID, err)
		return
	}
	if c := findClientByName(f[0]); c != nil {
		b.sendCard(ctx, chatID, b.t("created", esc(c.Name)), c.Id)
	}
}

func (b *bot) cmdMaintenance(ctx context.Context, chatID int64, arg string) {
	if b.configService == nil || (arg != "on" && arg != "off") {
		b.send(ctx, chatID, b.t("maintUsage"))
		return
	}
	if err := b.configService.SetMaintenance(arg == "on"); err != nil {
		b.fail(ctx, chatID, err)
		return
	}
	if arg == "on" {
		b.send(ctx, chatID, b.t("maintOn"))
	} else {
		b.send(ctx, chatID, b.t("maintOff"))
	}
}

// ---- callbacks ----

func (b *bot) handleCallback(ctx context.Context, u update) {
	cb := u.Callback
	if cb.Message == nil || cb.Message.Chat.Type != "private" {
		b.answer(ctx, cb.ID, "")
		return
	}
	chatID, msgID := cb.Message.Chat.ID, cb.Message.MessageID
	parts := strings.Split(cb.Data, ":")
	if len(parts) < 2 {
		b.answer(ctx, cb.ID, "")
		return
	}
	if parts[0] == "u" {
		b.userCallback(ctx, cb.ID, chatID, msgID, cb.From.ID, parts)
		return
	}
	if !b.cfg.isAdmin(cb.From.ID) {
		b.answer(ctx, cb.ID, b.t("denied"))
		return
	}
	switch parts[0] {
	case "m":
		b.menuCallback(ctx, cb.ID, chatID, msgID, parts[1])
	case "c":
		if len(parts) < 3 {
			b.answer(ctx, cb.ID, "")
			return
		}
		id64, err := strconv.ParseUint(parts[2], 10, 32)
		if err != nil {
			b.answer(ctx, cb.ID, "")
			return
		}
		b.clientCallback(ctx, cb.ID, chatID, msgID, parts[1], uint(id64))
	default:
		b.answer(ctx, cb.ID, "")
	}
}

func (b *bot) menuCallback(ctx context.Context, cbID string, chatID, msgID int64, action string) {
	back := [][]button{b.menuRow()}
	show := func(text string, kb [][]button) {
		b.answer(ctx, cbID, "")
		b.edit(ctx, chatID, msgID, text, kb)
	}
	switch action {
	case "menu":
		show(b.t("menuTitle"), b.mainMenu())
	case "status":
		show(b.statusText(), back)
	case "nodes":
		show(b.nodesText(), back)
	case "online":
		show(b.onlineText(), back)
	case "inbounds":
		show(b.inboundsText(), back)
	case "traffic":
		show(b.trafficText(), back)
	case "logs":
		show(b.logsText(), back)
	case "clients":
		text, kb := b.clientsView("")
		show(text, kb)
	case "backup":
		b.answer(ctx, cbID, "")
		b.sendBackup(ctx, chatID)
	case "sync":
		b.fanOut()
		b.answer(ctx, cbID, b.t("syncStarted"))
	case "restart":
		show(b.t("confirmRestart"), [][]button{{b.btn("btnConfirm", "m:restarty"), b.btn("btnCancel", "m:menu")}})
	case "restarty":
		b.answer(ctx, cbID, "")
		if b.configService == nil {
			return
		}
		if err := b.configService.RestartCore(); err != nil {
			b.edit(ctx, chatID, msgID, b.t("failed", esc(err.Error())), back)
			return
		}
		b.edit(ctx, chatID, msgID, b.t("restarted"), back)
	default:
		b.answer(ctx, cbID, "")
	}
}

func (b *bot) clientCallback(ctx context.Context, cbID string, chatID, msgID int64, action string, id uint) {
	var client *model.Client
	for _, c := range loadClients() {
		if c.Id == id {
			c := c
			client = &c
			break
		}
	}
	if client == nil {
		b.answer(ctx, cbID, b.t("notFound"))
		return
	}
	showCard := func(note string) {
		for _, c := range loadClients() {
			if c.Id == id {
				text, kb := b.card(c)
				if note != "" {
					text = note + "\n\n" + text
				}
				b.edit(ctx, chatID, msgID, text, kb)
				return
			}
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
	switch action {
	case "view":
		b.answer(ctx, cbID, "")
		showCard("")
	case "tog":
		err := b.setEnabled(id, !client.Enable)
		mutate(err, func() {
			note := ""
			if !client.Enable {
				c, _ := fullClient(id)
				if c != nil && ((c.Volume > 0 && c.Up+c.Down >= c.Volume) || (c.Expiry > 0 && c.Expiry <= time.Now().Unix())) {
					note = strings.TrimSpace(b.t("stillDepleted"))
				}
			}
			showCard(note)
		})
	case "rst":
		b.answer(ctx, cbID, "")
		b.edit(ctx, chatID, msgID, b.t("confirmReset", esc(client.Name)), b.confirmKeyboard("c:rsty:", id))
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
		b.sendSub(ctx, chatID, client.Name)
	case "ips":
		b.answer(ctx, cbID, "")
		b.edit(ctx, chatID, msgID, b.ipsText(client.Name), [][]button{{b.btn("btnBack", "c:view:"+strconv.FormatUint(uint64(id), 10))}, b.menuRow()})
	case "del":
		b.answer(ctx, cbID, "")
		b.edit(ctx, chatID, msgID, b.t("confirmDelete", esc(client.Name)), b.confirmKeyboard("c:dely:", id))
	case "dely":
		mutate(b.deleteClient(id), func() {
			b.edit(ctx, chatID, msgID, b.t("deleted", esc(client.Name)), [][]button{b.menuRow()})
		})
	default:
		b.answer(ctx, cbID, "")
	}
}

func (b *bot) userCallback(ctx context.Context, cbID string, chatID, msgID, from int64, parts []string) {
	if len(parts) < 3 {
		b.answer(ctx, cbID, "")
		return
	}
	id64, err := strconv.ParseUint(parts[2], 10, 32)
	if err != nil {
		b.answer(ctx, cbID, "")
		return
	}
	var client *model.Client
	for _, c := range boundClients(from) {
		if uint64(c.Id) == id64 {
			c := c
			client = &c
		}
	}
	if client == nil {
		b.answer(ctx, cbID, b.t("denied"))
		return
	}
	b.answer(ctx, cbID, "")
	switch parts[1] {
	case "view":
		text, kb := b.userView(*client)
		b.edit(ctx, chatID, msgID, text, kb)
	case "sub":
		b.sendSub(ctx, chatID, client.Name)
	}
}
