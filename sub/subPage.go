package sub

import (
	_ "embed"
	"encoding/json"
	"strings"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/service"
	"github.com/Danialrostamani/drnetwork-panel/service/tgbot"

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
	html := withPageConfig(pageFor(&client, time.Now()), s.pageConfig(&client))
	c.Data(200, "text/html; charset=utf-8", []byte(html))
	return true
}

// subConfigMark is where the page reads the panel's settings for it.
const subConfigMark = "/*SUB_CONFIG*/null"

// withPageConfig puts the panel's settings into a page. json.Marshal escapes
// <, > and &, so no text in them can close the script.
func withPageConfig(html string, cfg map[string]any) string {
	if len(cfg) == 0 {
		return html
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return html
	}
	return strings.Replace(html, subConfigMark, string(raw), 1)
}

// pageConfig is what the page of a client changes in its built-in settings:
// the panel's page settings, and from the bot the seller's support, channel
// and plans and the bot to renew through.
func (s *SubHandler) pageConfig(client *model.Client) map[string]any {
	conf := s.SettingService.GetSubPageConf()
	extra := tgbot.SubPageExtras(client)
	out := map[string]any{}
	set := func(key string, vals ...string) {
		for _, v := range vals {
			if v != "" {
				out[key] = v
				return
			}
		}
	}
	set("panelName", conf.Name)
	set("panelNameEn", conf.NameEn)
	set("supportLink", extra.Support, conf.Support)
	set("channelLink", extra.Channel, conf.Channel)
	if _, own := out["supportLink"]; own {
		// The renewal request goes to this support, not the built-in one.
		out["renewTarget"] = ""
	}
	if !conf.NoBot && extra.Bot != "" {
		out["renewBot"] = extra.Bot
	}
	if !conf.HidePlans && len(extra.Plans) > 0 {
		out["plans"] = extra.Plans
	}
	out["statusSelf"] = !conf.HideLive
	if len(conf.Sites) > 0 {
		out["sites"] = conf.Sites
	}
	if p := tgbot.LastPlanOf(client); p != nil {
		out["lastPlan"] = p
	}
	loc, _ := s.SettingService.GetTimeLocation()
	notices := conf.ActiveNotices(time.Now(), loc)
	if notices == nil {
		notices = []service.SubPageNotice{}
	}
	out["ui"] = map[string]any{
		"logo": conf.Logo, "accent": conf.Accent,
		"notices": notices,
		"hideNet": conf.HideNet, "hideApps": conf.HideApps, "hideQr": conf.HideQr,
		"hideLive": conf.HideLive, "hidePlans": conf.HidePlans, "hideTips": conf.HideTips,
		"hideVpn": conf.HideVpn, "hideSites": conf.HideSites,
		"hideServers": conf.HideServers || conf.HideLive,
		"report":      !conf.HideReport && extra.Bot != "",
		"connectBot":  !conf.NoBot && extra.Bot != "",
	}
	return out
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
