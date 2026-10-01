package tgbot

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/service"
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

// watch sends node down/up alerts and client volume/expiry alerts. Each
// condition is reported once and again only after it clears.
func (b *bot) watch(ctx context.Context) {
	nodes := map[uint]*nodeWatch{}
	reported := map[string]bool{}
	var lastClientCheck time.Time
	ticker := time.NewTicker(watchInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		b.checkNodes(ctx, nodes)
		if time.Since(lastClientCheck) >= clientCheckEvery {
			lastClientCheck = time.Now()
			b.checkClients(ctx, reported)
		}
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
	add := func(key, line string) {
		current[key] = true
		if !reported[key] {
			fresh = append(fresh, line)
		}
	}
	for _, c := range loadClients() {
		if !c.Enable {
			continue
		}
		name := "<b>" + esc(c.Name) + "</b>"
		if p := usagePercent(c); p >= 100 {
			add(c.Name+"|vol", fmt.Sprintf("• %s — %s", name, b.t("alertDepleted")))
		} else if p >= volumeAlertPercent {
			add(c.Name+"|vol", fmt.Sprintf("• %s — %s", name, b.t("alertVolume", p)))
		}
		if c.Expiry > 0 {
			left := time.Unix(c.Expiry, 0).Sub(now)
			if left <= 0 {
				add(c.Name+"|exp", fmt.Sprintf("• %s — %s", name, b.t("alertExpired")))
			} else if left <= expiryAlertDays*24*time.Hour {
				add(c.Name+"|exp", fmt.Sprintf("• %s — %s", name, b.t("alertExpiry", expiryAlertDays)))
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
