package tgbot

import (
	"context"
	"fmt"
	"strings"
	"time"

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

// announce tells the administrators the bot is up.
func (b *bot) announce(ctx context.Context) {
	b.broadcast(ctx, b.t("started"))
}

type nodeWatch struct {
	failures int
	downAt   time.Time
	alerted  bool
}

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
		if b.cfg.Notify {
			b.checkNodes(ctx, nodes)
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
	b.broadcast(ctx, b.t("reportTitle")+"\n\n"+b.statusText()+"\n\n"+b.trafficText())
	if b.cfg.ReportBackup {
		for _, id := range b.cfg.Admins {
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
			b.broadcast(ctx, b.t("coreUp"))
		}
		*w = coreWatch{}
		return
	}
	w.failures++
	if w.failures >= nodeDownAfterChecks && !w.alerted {
		w.alerted = true
		b.broadcast(ctx, b.t("coreDown"))
	}
}

func (b *bot) checkNodes(ctx context.Context, watched map[uint]*nodeWatch) {
	var nodes []model.Node
	if err := database.GetDB().Where("enable = ?", true).Find(&nodes).Error; err != nil {
		return
	}
	statuses := (&service.NodeService{}).GetStatuses()
	alive := map[uint]bool{}
	for _, n := range nodes {
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
		if st.State == "online" {
			if w.alerted {
				b.broadcast(ctx, b.t("nodeUp", esc(n.Name), b.humanDuration(time.Since(w.downAt))))
			}
			*w = nodeWatch{}
			continue
		}
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
			b.broadcast(ctx, b.t("nodeDown", esc(n.Name), esc(reason)))
		}
	}
	for id := range watched {
		if !alive[id] {
			delete(watched, id)
		}
	}
}

func (b *bot) checkClients(ctx context.Context, reported map[string]bool) {
	now := time.Now()
	current := map[string]bool{}
	var fresh []string
	var owner model.Client
	add := func(key, reason string) {
		current[key] = true
		if reported[key] {
			return
		}
		fresh = append(fresh, fmt.Sprintf("• <b>%s</b> — %s", esc(owner.Name), reason))
		// A client bound to Telegram hears about its own limits directly.
		if owner.TgId != 0 && !b.cfg.isAdmin(owner.TgId) {
			b.send(ctx, owner.TgId, b.t("userAlertTitle", esc(owner.Name))+"\n"+reason)
		}
	}
	for _, c := range loadClients() {
		if !c.Enable {
			continue
		}
		owner = c
		if p := usagePercent(c); p >= 100 {
			add(c.Name+"|vol", b.t("alertDepleted"))
		} else if p >= volumeAlertPercent {
			add(c.Name+"|vol", b.t("alertVolume", p))
		}
		if c.Expiry > 0 {
			left := time.Unix(c.Expiry, 0).Sub(now)
			if left <= 0 {
				add(c.Name+"|exp", b.t("alertExpired"))
			} else if left <= expiryAlertDays*24*time.Hour {
				add(c.Name+"|exp", b.t("alertExpiry", expiryAlertDays))
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
	const maxLines = 40
	if len(fresh) > maxLines {
		fresh = append(fresh[:maxLines], b.t("andMore", len(fresh)-maxLines))
	}
	b.broadcast(ctx, b.t("alertTitle")+"\n"+strings.Join(fresh, "\n"))
}
