package tgbot

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/config"
	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/logger"
	"github.com/Danialrostamani/drnetwork-panel/service"

	"github.com/robfig/cron/v3"
)

const (
	watchInterval       = 10 * time.Second
	nodeDownAfterChecks = 3 // consecutive failed checks before alerting
	clientCheckEvery    = 5 * time.Minute
	volumeAlertPercent  = 90
	expiryAlertDays     = 3
)

// announce tells every administrator the bot is up.
func (b *bot) announce(ctx context.Context) {
	b.broadcastAll(ctx, b.t("started"))
}

type nodeWatch struct {
	failures int
	downAt   time.Time
	alerted  bool
	// For each threshold warning: the checks in a row it was seen, the ones
	// announced, and for those the checks in a row it was gone.
	over   map[string]int
	warned map[string]bool
	under  map[string]int
}

const (
	// A busy CPU is normal for a moment: it has to stay over the limit longer.
	cpuWarnAfterChecks = 6
	// An announced warning clears only after this many checks without it, so
	// a value going back and forth over the limit does not flood the chat.
	warnClearChecks = 18
)

// watch sends node, core and client alerts and the scheduled report. Each
// alert condition is reported once and again only after it clears.
func (b *bot) watch(ctx context.Context) {
	nodes := map[uint]*nodeWatch{}
	reported := map[string]bool{}
	core := &coreWatch{}
	var lastClientCheck time.Time
	var report *reportSchedule
	if b.cfg.Report != "" {
		sched, err := service.CronParser.Parse(b.cfg.Report)
		if err != nil {
			logger.Warning("telegram bot: invalid report schedule: ", err)
		} else {
			report = &reportSchedule{sched: sched, next: sched.Next(time.Now().In(b.loc))}
		}
	}
	ticker := time.NewTicker(watchInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		// Taken every round, so events from while alerts were off are not
		// announced later.
		events := service.DrainNodeEvents()
		if b.cfg.Notify {
			b.checkNodes(ctx, nodes)
			b.announceNodeEvents(ctx, events)
			b.checkCore(ctx, core)
			if time.Since(lastClientCheck) >= clientCheckEvery {
				lastClientCheck = time.Now()
				b.checkClients(ctx, reported)
			}
		}
		if report != nil && !time.Now().Before(report.next) {
			report.next = report.sched.Next(time.Now().In(b.loc))
			b.sendReport(ctx)
		}
	}
}

type reportSchedule struct {
	sched cron.Schedule
	next  time.Time
}

// sendReport posts the status and traffic summary, and the database when asked.
func (b *bot) sendReport(ctx context.Context) {
	status, traffic := b.statusText(), b.trafficText()
	a := b.access()
	for _, id := range a.order {
		// Everybody gets the parts of the report their sections cover.
		var parts []string
		if a.allows(id, "home") {
			parts = append(parts, status)
		}
		if a.allows(id, "stats") {
			parts = append(parts, traffic)
		}
		if len(parts) > 0 {
			b.send(ctx, id, b.t("reportTitle")+"\n\n"+strings.Join(parts, "\n\n"))
		}
	}
	if b.cfg.ReportBackup {
		// The database holds every group's clients: only for administrators
		// with the Backup section.
		for _, id := range a.with("backup") {
			b.sendBackup(ctx, id)
		}
	}
}

type coreWatch struct {
	failures int
	alerted  bool
}

// checkCore reports the master's own sing-box core stopping. Maintenance mode
// stops it on purpose, so that is not an alert.
func (b *bot) checkCore(ctx context.Context, w *coreWatch) {
	if maintenance, _ := (&service.SettingService{}).GetMaintenance(); maintenance {
		*w = coreWatch{}
		return
	}
	status := (&service.ServerService{}).GetStatus("sbd")
	running := false
	if sbd, ok := (*status)["sbd"].(map[string]interface{}); ok {
		running, _ = sbd["running"].(bool)
	}
	if running {
		if w.alerted {
			b.broadcast(ctx, b.t("coreUp"), "home", "core")
		}
		*w = coreWatch{}
		return
	}
	w.failures++
	if w.failures >= nodeDownAfterChecks && !w.alerted {
		w.alerted = true
		b.broadcast(ctx, b.t("coreDown"), "home", "core")
	}
}

func (b *bot) checkNodes(ctx context.Context, watched map[uint]*nodeWatch) {
	var nodes []model.Node
	if err := database.GetDB().Where("enable = ?", true).Find(&nodes).Error; err != nil {
		return
	}
	statuses := (&service.NodeService{}).GetStatuses()
	alive := map[uint]bool{}
	for i := range nodes {
		n := &nodes[i]
		alive[n.Id] = true
		st, probed := statuses[n.Id]
		if !probed {
			continue
		}
		w := watched[n.Id]
		if w == nil {
			w = &nodeWatch{}
			watched[n.Id] = w
		}
		reachable := st.State == "online" || st.State == "core-stopped"
		// A node held in maintenance stops its core on purpose.
		if st.State == "online" || (st.State == "core-stopped" && st.Maintenance) {
			if w.alerted {
				b.toOwner(ctx, b.t("nodeUp", esc(n.Name), b.humanDuration(time.Since(w.downAt))), "nodes")
			}
			w.failures, w.alerted, w.downAt = 0, false, time.Time{}
		} else {
			if w.failures == 0 {
				w.downAt = time.Now()
			}
			w.failures++
			if w.failures >= nodeDownAfterChecks && !w.alerted {
				w.alerted = true
				reason := st.State
				if st.Error != "" {
					reason = st.Error
				}
				b.toOwner(ctx, b.t("nodeDown", esc(n.Name), esc(reason)), "nodes")
			}
		}
		if reachable {
			b.checkNodeWarnings(ctx, n, st.Warnings, w)
		} else {
			// Nothing is measured while the node cannot be reached: the
			// warnings already announced stay as they are.
			w.over = nil
			w.under = nil
		}
	}
	for id := range watched {
		if !alive[id] {
			delete(watched, id)
		}
	}
}

// checkNodeWarnings announces a threshold (CPU, RAM, disk, ping, certificate,
// version) a node stays over, and again when it is back under it.
func (b *bot) checkNodeWarnings(ctx context.Context, n *model.Node, warnings []service.NodeWarning, w *nodeWatch) {
	if w.over == nil {
		w.over = map[string]int{}
	}
	if w.warned == nil {
		w.warned = map[string]bool{}
	}
	if w.under == nil {
		w.under = map[string]int{}
	}
	seen := map[string]bool{}
	for _, warn := range warnings {
		if seen[warn.Key] {
			continue
		}
		seen[warn.Key] = true
		delete(w.under, warn.Key)
		w.over[warn.Key]++
		need := nodeDownAfterChecks
		if warn.Key == "cpu" {
			need = cpuWarnAfterChecks
		}
		if w.over[warn.Key] >= need && !w.warned[warn.Key] {
			w.warned[warn.Key] = true
			b.broadcast(ctx, b.nodeWarningText(n.Name, warn), "nodes")
		}
	}
	for key := range w.over {
		if !seen[key] {
			delete(w.over, key)
		}
	}
	for key := range w.warned {
		if seen[key] {
			continue
		}
		w.under[key]++
		if w.under[key] >= warnClearChecks {
			delete(w.warned, key)
			delete(w.under, key)
			b.broadcast(ctx, b.nodeClearedText(n.Name, key), "nodes")
		}
	}
}

func (b *bot) nodeWarningText(name string, w service.NodeWarning) string {
	head := "⚠️ <b>" + b.tr("نود ", "Node ") + esc(name) + "</b>\n"
	switch w.Key {
	case "cpu":
		return head + fmt.Sprintf(b.tr("مصرف CPU بالاست: %.0f%% (حد %.0f%%)", "High CPU: %.0f%% (limit %.0f%%)"), w.Value, w.Limit)
	case "mem":
		return head + fmt.Sprintf(b.tr("مصرف RAM بالاست: %.0f%% (حد %.0f%%)", "High RAM: %.0f%% (limit %.0f%%)"), w.Value, w.Limit)
	case "disk":
		return head + fmt.Sprintf(b.tr("دیسک در حال پر شدن است: %.0f%% (حد %.0f%%)", "Disk filling up: %.0f%% (limit %.0f%%)"), w.Value, w.Limit)
	case "ping":
		return head + fmt.Sprintf(b.tr("پینگ بالاست: %.0f ms (حد %.0f ms)", "High ping: %.0f ms (limit %.0f ms)"), w.Value, w.Limit)
	case "cert":
		if w.Value <= 0 {
			return head + b.tr("گواهی TLS پنل نود منقضی شده است.", "The node panel's TLS certificate has expired.")
		}
		if w.Value < 1 {
			return head + b.tr("گواهی TLS پنل نود کمتر از یک روز دیگر منقضی می‌شود.", "The node panel's TLS certificate expires within a day.")
		}
		return head + fmt.Sprintf(b.tr("گواهی TLS پنل نود تا %d روز دیگر منقضی می‌شود.", "The node panel's TLS certificate expires in %d days."), int(w.Value))
	case "version":
		return head + fmt.Sprintf(b.tr("نسخه نود (%s) از نسخه پنل اصلی (%s) قدیمی‌تر است؛ نود را به‌روز کنید.", "The node runs %s, older than the master's %s; update the node."), esc(w.Info), esc(config.GetFullVersion()))
	}
	return head + esc(w.Key)
}

func (b *bot) nodeClearedText(name, key string) string {
	head := "✅ <b>" + b.tr("نود ", "Node ") + esc(name) + "</b>\n"
	switch key {
	case "cpu":
		return head + b.tr("مصرف CPU به حالت عادی برگشت.", "CPU is back to normal.")
	case "mem":
		return head + b.tr("مصرف RAM به حالت عادی برگشت.", "RAM is back to normal.")
	case "disk":
		return head + b.tr("فضای دیسک به حالت عادی برگشت.", "Disk usage is back to normal.")
	case "ping":
		return head + b.tr("پینگ به حالت عادی برگشت.", "Ping is back to normal.")
	case "cert":
		return head + b.tr("گواهی TLS پنل نود دیگر نزدیک انقضا نیست.", "The node panel's TLS certificate is no longer close to expiring.")
	case "version":
		return head + b.tr("نسخه نود به‌روز است.", "The node is up to date.")
	}
	return head + esc(key)
}

// announceNodeEvents tells about the monthly cap levels a node crossed and
// its links leaving or coming back to the subscriptions.
func (b *bot) announceNodeEvents(ctx context.Context, events []service.NodeEvent) {
	for _, e := range events {
		text := b.nodeEventText(e)
		switch {
		case text == "":
		case nodeEventForOwner[e.Kind]:
			b.toOwner(ctx, text, "nodes")
		default:
			b.broadcast(ctx, text, "nodes")
		}
	}
}

// nodeEventForOwner are the node events that, like a node going down or
// coming back, only the owner hears about: the node's links leaving the
// subscriptions or coming back, and the node becoming unreachable from Iran or
// reachable again. The monthly cap levels go to every administrator of the
// nodes.
var nodeEventForOwner = map[string]bool{"hidden": true, "shown": true, "filtered": true, "unfiltered": true}

func (b *bot) nodeEventText(e service.NodeEvent) string {
	name := esc(e.Name)
	switch e.Kind {
	case "cap":
		used := fmt.Sprintf("%s / %s", humanBytes(e.Used), humanBytes(e.Limit))
		if e.Level >= 100 {
			return "⛔️ <b>" + b.tr("نود ", "Node ") + name + "</b>\n" + b.tr("سقف ترافیک ماهانه تمام شد: ", "Monthly traffic cap reached: ") + used
		}
		return "📊 <b>" + b.tr("نود ", "Node ") + name + "</b>\n" + fmt.Sprintf(b.tr("%d%% سقف ترافیک ماهانه مصرف شد: %s", "%d%% of the monthly traffic cap used: %s"), e.Level, used)
	case "hidden":
		why := b.tr("چون مدتی قطع است", "because it is down")
		if e.Reason == "cap" {
			why = b.tr("چون سقف ترافیک ماهانه‌اش پر شده", "because its monthly cap is reached")
		} else if e.Reason == "filtered" {
			why = b.tr("چون از ایران در دسترس نیست", "because it is not reachable from Iran")
		}
		return "🙈 <b>" + b.tr("نود ", "Node ") + name + "</b>\n" + fmt.Sprintf(b.tr("لینک‌های این نود %s از سابسکریپشن‌ها برداشته شد.", "Its links were taken out of the subscriptions %s."), why)
	case "filtered":
		return "🚫 <b>" + b.tr("نود ", "Node ") + name + "</b>\n" + b.tr("به نظر فیلتر شده: سرورهای داخل ایران به آن وصل نمی‌شوند، ولی خود نود روشن است.", "Looks filtered: the servers inside Iran cannot connect to it, while the node itself is up.")
	case "unfiltered":
		return "✅ <b>" + b.tr("نود ", "Node ") + name + "</b>\n" + b.tr("دوباره از ایران در دسترس است.", "Reachable from Iran again.")
	case "shown":
		return "👁 <b>" + b.tr("نود ", "Node ") + name + "</b>\n" + b.tr("لینک‌های این نود دوباره به سابسکریپشن‌ها برگشت.", "Its links are back in the subscriptions.")
	}
	return ""
}

// firstCheckKey marks in the reported set that the clients were checked once.
const firstCheckKey = "\x00checked"

// clientAlert is one line of the client alert; group tells which group-limited
// administrators it is for.
type clientAlert struct {
	group, line string
}

func (b *bot) checkClients(ctx context.Context, reported map[string]bool) {
	now := time.Now()
	current := map[string]bool{}
	var fresh []clientAlert
	var owner model.Client
	add := func(key, reason string) {
		current[key] = true
		if reported[key] {
			return
		}
		fresh = append(fresh, clientAlert{owner.Group, fmt.Sprintf("• <b>%s</b> — %s", esc(owner.Name), reason)})
		// A client bound to Telegram hears about its own limits directly.
		if owner.TgId != 0 && !b.isAdmin(owner.TgId) {
			b.send(ctx, owner.TgId, b.t("userAlertTitle", esc(owner.Name))+"\n"+reason)
		}
	}
	// The first check after the start only notes what is already over: those
	// clients were told before, and the chat should not fill up with them.
	first := !reported[firstCheckKey]
	current[firstCheckKey] = true
	for _, c := range b.loadClients() {
		owner = c
		// The panel turns a client off a few seconds after its volume or time
		// runs out, so a client that is off still gets those two alerts; the
		// early warnings are for the clients still on.
		over := func(key, reason string) {
			if first && !c.Enable {
				current[key] = true
				reported[key] = true
				return
			}
			add(key, reason)
		}
		if p := usagePercent(c); p >= 100 {
			over(c.Name+"|vol100", b.t("alertDepleted"))
		} else if p >= volumeAlertPercent && c.Enable {
			add(c.Name+"|vol90", b.t("alertVolume", p))
		}
		if c.Expiry > 0 {
			left := time.Unix(c.Expiry, 0).Sub(now)
			if left <= 0 {
				over(c.Name+"|expired", b.t("alertExpired"))
			} else if left <= expiryAlertDays*24*time.Hour && c.Enable {
				add(c.Name+"|exp3d", b.t("alertExpiry", expiryAlertDays))
			}
		}
	}
	for key := range reported {
		if !current[key] {
			delete(reported, key)
		}
	}
	for key := range current {
		reported[key] = true
	}
	if len(fresh) == 0 {
		return
	}
	// Administrators with the Clients section get every line; a group-limited
	// one only the lines of their own group; the others none.
	const maxLines = 40
	acc := b.access()
	for _, id := range acc.order {
		r := acc.roleOf(id)
		if r.group == "" && !acc.allows(id, "clients") {
			continue
		}
		var lines []string
		for _, a := range fresh {
			if r.group == "" || sameGroup(a.group, r.group) {
				lines = append(lines, a.line)
			}
		}
		if len(lines) == 0 {
			continue
		}
		if len(lines) > maxLines {
			lines = append(lines[:maxLines], b.t("andMore", len(lines)-maxLines))
		}
		b.send(ctx, id, b.t("alertTitle")+"\n"+strings.Join(lines, "\n"))
	}
}
