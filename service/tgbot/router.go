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
	if b.scope != "" {
		return b.scopedMenu()
	}
	t := b.tr
	return [][]button{
		{{Text: t("🏠 خانه", "🏠 Home"), Data: "h:home"}, {Text: t("👥 کلاینت‌ها", "👥 Clients"), Data: "c:ls:a:0"}},
		{{Text: t("📡 اینباندها", "📡 Inbounds"), Data: "o:in:ls:0"}, {Text: t("📤 اوت‌باندها", "📤 Outbounds"), Data: "o:out:ls:0"}},
		{{Text: t("🔌 اندپوینت‌ها", "🔌 Endpoints"), Data: "o:ep:ls:0"}, {Text: t("🛠 سرویس‌ها", "🛠 Services"), Data: "o:sv:ls:0"}},
		{{Text: "🔐 TLS", Data: "o:tl:ls:0"}, {Text: t("🖥 نودها", "🖥 Nodes"), Data: "o:nd:ls:0"}},
		{{Text: t("📏 قوانین", "📏 Rules"), Data: "g:rl:ls:0"}, {Text: "🌐 DNS", Data: "g:dn:ls:0"}},
		{{Text: t("⚙️ پایه", "⚙️ Basics"), Data: "g:basics:ls:0"}, {Text: t("🔧 تنظیمات", "🔧 Settings"), Data: "s:ls"}},
		{{Text: t("📊 آمار", "📊 Stats"), Data: "t:d1"}, {Text: t("🧾 تغییرات", "🧾 Changes"), Data: "x:ls"}},
		{{Text: t("📜 لاگ‌ها", "📜 Logs"), Data: "m:logs:info"}, {Text: t("👮 مدیران", "👮 Admins"), Data: "a:ls"}},
		{{Text: t("💾 پشتیبان", "💾 Backup"), Data: "m:backup"}, {Text: t("🔁 همگام‌سازی", "🔁 Sync"), Data: "m:sync"}},
		{{Text: t("🚧 نگهداری", "🚧 Maintenance"), Data: "m:maint"}, {Text: t("♻️ ریستارت هسته", "♻️ Restart core"), Data: "m:restart"}},
	}
}

func (b *bot) menuRow() []button { return []button{b.btn("btnMenu", "m:menu")} }

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
	{"home", "وضعیت سرور", "Server status"},
	{"stats", "آمار ترافیک", "Traffic statistics"},
	{"settings", "تنظیمات پنل", "Panel settings"},
	{"changes", "آخرین تغییرات", "Recent changes"},
	{"addbulk", "ساخت گروهی کلاینت", "Bulk create clients"},
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

func (b *bot) handle(ctx context.Context, u update) {
	if u.Callback != nil {
		b.handleCallback(ctx, u)
		return
	}
	m := u.Message
	if m == nil || m.From == nil {
		return
	}
	// Only private chats are served: a group message must never leak panel
	// data to the other members.
	if m.Chat.Type != "private" {
		return
	}
	chatID, from := m.Chat.ID, m.From.ID
	b = b.as(from)
	text := m.Text
	if text == "" && m.Document != nil && b.isAdmin(from) {
		if p := b.pend.get(chatID); p != nil {
			data, err := b.download(ctx, m.Document.FileID)
			if err != nil {
				b.send(ctx, chatID, b.t("failed", esc(err.Error())))
				return
			}
			b.handlePending(ctx, chatID, m.MessageID, string(data), p)
		}
		return
	}
	cmd, arg := parseCommand(text)
	if cmd == "" {
		if p := b.pend.get(chatID); p != nil && b.isAdmin(from) && text != "" {
			b.handlePending(ctx, chatID, m.MessageID, text, p)
		}
		return
	}
	if b.isAdmin(from) {
		b.pend.clear(chatID)
	}
	if cmd == "id" {
		b.send(ctx, chatID, b.t("yourId", from))
		return
	}
	if b.isAdmin(from) {
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
	for _, c := range allClients() {
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
	for _, n := range allOnlineUsers() {
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
			b.sendSub(ctx, chatID, c)
		}
	default:
		b.send(ctx, chatID, b.t("userHelp"))
	}
}

// sendSub sends the subscription link and its QR code. The link comes under a
// summary of the client's account (see subSummary), so that this message alone
// tells them what they have used, what is left and until when.
func (b *bot) sendSub(ctx context.Context, chatID int64, c model.Client) {
	link, err := b.subLink(c.Name)
	if err != nil {
		b.send(ctx, chatID, b.t("failed", esc(err.Error())))
		return
	}
	online := false
	for _, n := range allOnlineUsers() {
		if n == c.Name {
			online = true
		}
	}
	b.send(ctx, chatID, b.subSummary(c, online, time.Now())+"\n\n"+b.t("subTitle", esc(c.Name))+"\n<code>"+esc(link)+"</code>")
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
	if full, err := b.fullClient(c.Id); err == nil {
		c = *full
	}
	online := false
	for _, n := range b.onlineUsers() {
		if n == c.Name {
			online = true
		}
	}
	return b.clientDetail(c, online, time.Now()), b.clientKeyboard(c)
}

func (b *bot) sendCard(ctx context.Context, chatID int64, prefix string, id uint) {
	for _, c := range b.loadClients() {
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

func (b *bot) logsText(level string) string {
	logs := (&service.ServerService{}).GetLogs("40", level)
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
	if !b.can("backup") {
		// The database holds every group's clients and the panel's secrets.
		b.send(ctx, chatID, b.denied())
		return
	}
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
	if text := b.accessErrText(err); text != "" {
		return text
	}
	return err.Error()
}

func (b *bot) adminCommand(ctx context.Context, chatID int64, cmd, arg string) {
	if !b.allowedCommand(cmd) {
		b.send(ctx, chatID, b.denied())
		return
	}
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
		c := b.findClientByName(nameArg)
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
		text, kb := b.menuScreen()
		b.sendKeyboard(ctx, chatID, text, kb)
	case "help":
		switch {
		case b.scope != "":
			b.send(ctx, chatID, b.t("helpScoped", esc(b.scopeGroup())))
		case b.sections != nil:
			b.send(ctx, chatID, b.restrictedHelp())
		default:
			b.send(ctx, chatID, b.t("help")+b.t("helpAdmin2"))
		}
	case "status", "home":
		b.sendKeyboard(ctx, chatID, b.homeText(), b.homeKeyboard())
	case "nodes":
		text, kb := b.objListScreen(kindByCode("nd"), 0)
		b.sendKeyboard(ctx, chatID, text, kb)
	case "online":
		b.send(ctx, chatID, b.onlineText())
	case "clients", "client":
		if arg == "" {
			text, kb := b.clientsScreen("a", 0)
			b.sendKeyboard(ctx, chatID, text, kb)
			break
		}
		text, kb := b.clientsView(arg)
		b.sendKeyboard(ctx, chatID, text, kb)
	case "ips":
		b.send(ctx, chatID, b.ipsText(arg))
	case "inbounds":
		text, kb := b.objListScreen(kindByCode("in"), 0)
		b.sendKeyboard(ctx, chatID, text, kb)
	case "traffic", "stats":
		text, kb := b.statsScreen("d1")
		b.sendKeyboard(ctx, chatID, text, kb)
	case "settings":
		text, kb := b.settingsHome()
		b.sendKeyboard(ctx, chatID, text, kb)
	case "changes":
		text, kb := b.changesScreen()
		b.sendKeyboard(ctx, chatID, text, kb)
	case "addbulk":
		b.cmdAddBulk(ctx, chatID, fields)
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
			b.sendSub(ctx, chatID, *c)
		}
	case "backup":
		b.sendBackup(ctx, chatID)
	case "logs":
		level := strings.ToLower(nameArg)
		if level == "" {
			level = "info"
		}
		text, kb := b.logsScreen(level)
		b.sendKeyboard(ctx, chatID, text, kb)
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
	if err := b.createClient(f[0], "", int64(gb*float64(gib)), days, limit); err != nil {
		b.fail(ctx, chatID, err)
		return
	}
	if c := b.findClientByName(f[0]); c != nil {
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
	if !b.isAdmin(cb.From.ID) {
		b.answer(ctx, cb.ID, b.t("denied"))
		return
	}
	b = b.as(cb.From.ID)
	if !b.allowed(parts) {
		b.answer(ctx, cb.ID, b.denied())
		return
	}
	show := func(text string, kb [][]button) {
		b.answer(ctx, cb.ID, "")
		b.edit(ctx, chatID, msgID, text, kb)
	}
	// Any navigation abandons a half-finished prompt.
	if parts[0] != "w" {
		if p := b.pend.get(chatID); p != nil && !(parts[0] == "x" && parts[1] == "cancel") {
			b.pend.clear(chatID)
		}
	}
	switch parts[0] {
	case "m":
		b.menuCallback(ctx, cb.ID, chatID, msgID, parts)
	case "c":
		b.clientCallback(ctx, cb.ID, chatID, msgID, parts)
	case "o":
		b.objCallback(ctx, cb.ID, chatID, msgID, parts)
	case "g":
		b.cfgCallback(ctx, cb.ID, chatID, msgID, parts)
	case "s":
		b.settingsCallback(ctx, cb.ID, chatID, msgID, parts)
	case "w":
		b.wizardCallback(ctx, cb.ID, chatID, parts)
	case "h":
		show(b.homeText(), b.homeKeyboard())
	case "a":
		b.adminsCallback(ctx, cb.ID, chatID, msgID, parts)
	case "t":
		text, kb := b.statsScreen(parts[1])
		show(text, kb)
	case "x":
		if parts[1] == "cancel" {
			p := b.pend.get(chatID)
			b.pend.clear(chatID)
			back := "m:menu"
			if p != nil && p.back != "" {
				back = p.back
			}
			b.answer(ctx, cb.ID, "")
			cb.Data = back
			u.Callback.Data = back
			b.handleCallback(ctx, u)
			return
		}
		text, kb := b.changesScreen()
		show(text, kb)
	default:
		b.answer(ctx, cb.ID, "")
	}
}

func (b *bot) menuCallback(ctx context.Context, cbID string, chatID, msgID int64, parts []string) {
	action := parts[1]
	back := [][]button{b.menuRow()}
	show := func(text string, kb [][]button) {
		b.answer(ctx, cbID, "")
		b.edit(ctx, chatID, msgID, text, kb)
	}
	switch action {
	case "menu":
		text, kb := b.menuScreen()
		show(text, kb)
	case "status":
		show(b.homeText(), b.homeKeyboard())
	case "nodes":
		text, kb := b.objListScreen(kindByCode("nd"), 0)
		show(text, kb)
	case "online":
		text, kb := b.clientsScreen("o", 0)
		show(text, kb)
	case "inbounds":
		text, kb := b.objListScreen(kindByCode("in"), 0)
		show(text, kb)
	case "traffic":
		text, kb := b.statsScreen("d1")
		show(text, kb)
	case "logs":
		level := "info"
		if len(parts) > 2 {
			level = parts[2]
		}
		text, kb := b.logsScreen(level)
		show(text, kb)
	case "clients":
		text, kb := b.clientsScreen("a", 0)
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
	case "maint":
		if b.configService == nil {
			b.answer(ctx, cbID, "")
			return
		}
		on, _ := (&service.SettingService{}).GetMaintenance()
		if err := b.configService.SetMaintenance(!on); err != nil {
			b.answer(ctx, cbID, b.t("failed", b.errText(err)))
			return
		}
		b.answer(ctx, cbID, b.t(map[bool]string{true: "maintOff", false: "maintOn"}[on]))
		b.edit(ctx, chatID, msgID, b.homeText(), b.homeKeyboard())
	case "prest":
		show(b.tr("⚠️ پنل ریستارت شود؟ چند ثانیه در دسترس نیست.", "⚠️ Restart the panel? It is unavailable for a few seconds."), [][]button{{b.btn("btnConfirm", "m:prestY"), b.btn("btnCancel", "m:menu")}})
	case "prestY":
		show(b.tr("♻️ پنل در حال ریستارت است…", "♻️ Restarting the panel…"), back)
		_ = (&service.PanelService{}).RestartPanel(3 * time.Second)
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
		b.sendSub(ctx, chatID, *client)
	}
}

func (b *bot) cmdAddBulk(ctx context.Context, chatID int64, f []string) {
	if len(f) < 4 {
		if b.scope != "" {
			b.send(ctx, chatID, b.tr("/addbulk <code>پیشوند تعداد GB روز [IP]</code>\nمثال: <code>/addbulk user 10 20 30</code> → user1 تا user10", "/addbulk <code>prefix count GB days [IP]</code>\nExample: <code>/addbulk user 10 20 30</code> → user1 … user10"))
			return
		}
		b.send(ctx, chatID, b.tr("/addbulk <code>پیشوند تعداد GB روز [IP] [گروه]</code>\nمثال: <code>/addbulk user 10 20 30</code> → user1 تا user10\nبا گروه: <code>/addbulk user 10 20 30 0 vip</code>", "/addbulk <code>prefix count GB days [IP] [group]</code>\nExample: <code>/addbulk user 10 20 30</code> → user1 … user10\nWith a group: <code>/addbulk user 10 20 30 0 vip</code>"))
		return
	}
	count, ok1 := parseIntArg(f[1])
	gb, ok2 := parseFloatArg(f[2])
	days, ok3 := parseIntArg(f[3])
	limit, ok4 := 0, true
	if len(f) >= 5 {
		limit, ok4 = parseIntArg(f[4])
	}
	if !ok1 || !ok2 || !ok3 || !ok4 || count < 1 || count > 200 {
		b.send(ctx, chatID, b.t("badNumber"))
		return
	}
	group := ""
	if len(f) >= 6 {
		group = strings.Join(f[5:], " ")
	}
	if err := b.createBulk(f[0], group, count, int64(gb*float64(gib)), days, limit); err != nil {
		b.fail(ctx, chatID, err)
		return
	}
	text, kb := b.clientsScreen("a", 0)
	b.sendKeyboard(ctx, chatID, b.t("done")+"\n\n"+text, kb)
}
