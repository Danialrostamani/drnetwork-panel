package sub

import (
	"github.com/Danialrostamani/drnetwork-panel/service"
	"github.com/gin-gonic/gin"
	"time"
)

// live is a small account-only snapshot, not the cached historical/status
// report. The subscription token and host checks run before this handler.
func (s *SubHandler) live(c *gin.Context, subID string) {
	client, err := service.LiveClient(subID)
	if err != nil {
		c.String(400, "Error!")
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Header("X-Robots-Tag", "noindex")
	c.JSON(200, gin.H{
		"ok": true, "now": time.Now().Unix(), "enabled": client.Enable,
		"upload": client.Up, "download": client.Down, "total": client.Volume,
		"expire": client.Expiry,
	})
}
