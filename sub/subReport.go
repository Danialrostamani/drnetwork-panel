package sub

import (
	"context"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/service/tgbot"

	"github.com/gin-gonic/gin"
)

// The page's "report a problem" button sends what its checks found to the
// client's seller through the bot. A client can send one now and then, and
// an address a few, so the button cannot flood anybody's chat.

const (
	reportGap      = 10 * time.Minute
	reportPerIP    = 3
	reportTextMax  = 1500
	reportTableMax = 10000
)

var (
	reportMu     sync.Mutex
	reportByName = map[string]time.Time{}
	reportByIP   = map[string][]time.Time{}
)

// reportAllowed books a report of a client from an address, or says it is
// too soon.
func reportAllowed(name, ip string, now time.Time) bool {
	reportMu.Lock()
	defer reportMu.Unlock()
	if len(reportByName) > reportTableMax || len(reportByIP) > reportTableMax {
		reportByName, reportByIP = map[string]time.Time{}, map[string][]time.Time{}
	}
	if last, ok := reportByName[name]; ok && now.Sub(last) < reportGap {
		return false
	}
	recent := reportByIP[ip][:0]
	for _, at := range reportByIP[ip] {
		if now.Sub(at) < reportGap {
			recent = append(recent, at)
		}
	}
	if len(recent) >= reportPerIP {
		reportByIP[ip] = recent
		return false
	}
	reportByName[name] = now
	reportByIP[ip] = append(recent, now)
	return true
}

func (s *SubHandler) report(c *gin.Context) {
	subID, ok := s.resolveSub(c, c.Param("subid"))
	if !ok {
		c.String(400, "Error!")
		return
	}
	if !s.hostServes(c, subID) {
		return
	}
	if c.Query("format") != "report" || s.SettingService.GetSubPageConf().HideReport {
		c.String(404, "")
		return
	}
	// JSON only: a form on another site cannot send it without asking first.
	if !strings.HasPrefix(c.ContentType(), "application/json") {
		c.JSON(415, gin.H{"ok": false})
		return
	}
	var body struct {
		Text string `json:"text"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.Text) == "" {
		c.JSON(400, gin.H{"ok": false})
		return
	}
	text := strings.TrimSpace(body.Text)
	if utf8.RuneCountInString(text) > reportTextMax {
		text = string([]rune(text)[:reportTextMax]) + "…"
	}
	var client model.Client
	if err := database.GetDB().Where("name = ?", subID).First(&client).Error; err != nil {
		c.JSON(400, gin.H{"ok": false})
		return
	}
	if tgbot.RunningName() == "" {
		// No bot to carry it; do not spend the client's turn on nothing.
		c.JSON(503, gin.H{"ok": false})
		return
	}
	ip := subRequestIP(c)
	if !reportAllowed(client.Name, ip, time.Now()) {
		c.JSON(429, gin.H{"ok": false, "wait": true})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	if err := tgbot.SubPageReport(ctx, &client, ip, text); err != nil {
		c.JSON(503, gin.H{"ok": false})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}
