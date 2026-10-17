package sub

import (
	_ "embed"
	"strings"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"

	"github.com/gin-gonic/gin"
)

//go:embed subPage.html
var subPageHTML string

// wantsPage tells a person's browser from a VPN app fetching the links: apps
// do not ask for HTML first and do not call themselves Mozilla.
func wantsPage(c *gin.Context) bool {
	if _, isFormat := c.GetQuery("format"); isFormat {
		return false
	}
	accept := c.GetHeader("Accept")
	ua := c.GetHeader("User-Agent")
	return strings.Contains(accept, "text/html") && strings.HasPrefix(ua, "Mozilla/")
}

// page serves the status page of a client; false if there is none.
func (s *SubHandler) page(c *gin.Context, subID string) bool {
	var client model.Client
	if err := database.GetDB().Where("name = ?", subID).First(&client).Error; err != nil {
		return false
	}
	c.Header("Cache-Control", "no-store")
	c.Header("X-Robots-Tag", "noindex")
	c.Data(200, "text/html; charset=utf-8", []byte(subPageHTML))
	return true
}
