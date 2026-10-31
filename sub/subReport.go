package sub

import "github.com/gin-gonic/gin"

// Old cached pages must never keep sending web reports to the Telegram bot.
// The current page prepares a report locally and opens the configured direct.
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
		c.Status(404)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(410, gin.H{"ok": false, "error": "web_reports_use_support_direct"})
}
