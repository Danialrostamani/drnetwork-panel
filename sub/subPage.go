package sub

import (
	_ "embed"
	"encoding/json"
	"strings"
	"time"

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
	c.Data(200, "text/html; charset=utf-8", []byte(pageFor(&client, time.Now())))
	return true
}

// subStateMark is where the page reads what the panel already knows.
const subStateMark = "/*SUB_STATE*/null"

// pageFor is the page with the state of the client put in: a subscription
// that has ended -- time or data used up, or switched off -- tells the page so,
// since its link no longer answers and the page would have nothing to show.
func pageFor(client *model.Client, now time.Time) string {
	reason := endedReason(client, now)
	if reason == "" {
		return subPageHTML
	}
	state, _ := json.Marshal(map[string]any{"ended": true, "reason": reason})
	return strings.Replace(subPageHTML, subStateMark, string(state), 1)
}

// endedReason is why a client cannot use its subscription any more: "volume",
// "expired", "both", "disabled", or "" while it still can.
func endedReason(c *model.Client, now time.Time) string {
	vol := c.Volume > 0 && c.Up+c.Down >= c.Volume
	exp := c.Expiry > 0 && c.Expiry <= now.Unix()
	switch {
	case vol && exp:
		return "both"
	case vol:
		return "volume"
	case exp:
		return "expired"
	case !c.Enable:
		return "disabled"
	}
	return ""
}
