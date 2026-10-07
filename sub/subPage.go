package sub

import (
	"bytes"
	_ "embed"
	"encoding/base64"
	"fmt"
	"html/template"
	"net/url"
	"strings"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/service"

	"github.com/gin-gonic/gin"
	"github.com/skip2/go-qrcode"
)

//go:embed subPage.html
var subPageHTML string

var subPageTmpl = template.Must(template.New("sub").Parse(subPageHTML))

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

type subApp struct {
	Name, Platform, Download string
	// Import is a deep link into the app; html/template would refuse its
	// custom scheme as a plain string.
	Import template.URL
}

type subPageData struct {
	RTL               bool
	Lang              string
	T                 map[string]string
	Name              string
	State, StateClass string
	Used, Total, Left string
	Percent           int
	Expiry, DaysLeft  string
	Link              string
	QR                template.URL
	Apps              []subApp
	Bot               string
	Online            bool
	Unlimited         bool
}

var subPageText = map[string]map[string]string{
	"fa": {
		"title": "وضعیت اشتراک", "used": "مصرف شده", "total": "حجم کل", "left": "باقی‌مانده", "expiry": "تاریخ انقضا", "days": "روز مانده",
		"link": "لینک اشتراک", "copy": "کپی لینک", "copied": "کپی شد", "apps": "افزودن به برنامه", "download": "دانلود",
		"renew": "تمدید یا خرید از ربات تلگرام", "active": "فعال", "off": "غیرفعال", "expired": "منقضی شده", "depleted": "حجم تمام شده",
		"unlimited": "نامحدود", "never": "بدون انقضا", "online": "آنلاین", "import": "افزودن",
		"hint": "لینک را کپی کنید و در برنامه با گزینه «افزودن از کلیپ‌بورد» وارد کنید، یا QR را اسکن کنید.",
	},
	"en": {
		"title": "Subscription status", "used": "Used", "total": "Total", "left": "Left", "expiry": "Expires", "days": "days left",
		"link": "Subscription link", "copy": "Copy link", "copied": "Copied", "apps": "Add to an app", "download": "Download",
		"renew": "Renew or buy in the Telegram bot", "active": "Active", "off": "Disabled", "expired": "Expired", "depleted": "Out of volume",
		"unlimited": "Unlimited", "never": "Never", "online": "Online", "import": "Import",
		"hint": "Copy the link and add it in your app with “import from clipboard”, or scan the QR code.",
	},
}

func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

// requestURL is the subscription link as the browser opened it.
func requestURL(c *gin.Context) string {
	scheme := "http"
	if c.Request.TLS != nil || strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	return scheme + "://" + c.Request.Host + c.Request.URL.EscapedPath()
}

// page renders the account page of a client; false if there is none.
func (s *SubHandler) page(c *gin.Context, subID string) bool {
	var client model.Client
	if err := database.GetDB().Where("name = ?", subID).First(&client).Error; err != nil {
		return false
	}
	lang := "en"
	if al := strings.ToLower(c.GetHeader("Accept-Language")); strings.HasPrefix(al, "fa") || strings.Contains(al, ",fa") {
		lang = "fa"
	}
	t := subPageText[lang]
	now := time.Now()
	used := client.Up + client.Down
	d := subPageData{RTL: lang == "fa", Lang: lang, T: t, Name: client.Name, Used: humanSize(used), Link: requestURL(c)}
	switch {
	case client.Volume > 0 && used >= client.Volume:
		d.State, d.StateClass = t["depleted"], "bad"
	case client.Expiry > 0 && client.Expiry <= now.Unix():
		d.State, d.StateClass = t["expired"], "bad"
	case !client.Enable:
		d.State, d.StateClass = t["off"], "bad"
	default:
		d.State, d.StateClass = t["active"], "ok"
	}
	if client.Volume > 0 {
		d.Total = humanSize(client.Volume)
		d.Left = humanSize(max(client.Volume-used, 0))
		d.Percent = int(min(used*100/client.Volume, 100))
	} else {
		d.Total, d.Left, d.Unlimited = t["unlimited"], t["unlimited"], true
	}
	switch {
	case client.DelayStart:
		d.Expiry = fmt.Sprintf("%d %s", client.ResetDays, t["days"])
	case client.Expiry > 0:
		loc, _ := s.SettingService.GetTimeLocation()
		if loc == nil {
			loc = time.Local
		}
		d.Expiry = time.Unix(client.Expiry, 0).In(loc).Format("2006-01-02 15:04")
		if left := time.Until(time.Unix(client.Expiry, 0)); left > 0 {
			d.DaysLeft = fmt.Sprintf("%d %s", int(left.Hours()/24), t["days"])
		}
	default:
		d.Expiry = t["never"]
	}
	d.Online = client.OnlineAt > now.Unix()-120
	if png, err := qrcode.Encode(d.Link, qrcode.Medium, 320); err == nil {
		d.QR = template.URL("data:image/png;base64," + base64.StdEncoding.EncodeToString(png))
	}
	q := url.QueryEscape(d.Link)
	name := url.PathEscape(client.Name)
	d.Apps = []subApp{
		{"Hiddify", "Android · iOS · Windows · macOS · Linux", "https://github.com/hiddify/hiddify-app/releases/latest", template.URL("hiddify://import/" + d.Link + "#" + name)},
		{"v2rayNG", "Android", "https://github.com/2dust/v2rayNG/releases/latest", template.URL("v2rayng://install-config?url=" + q)},
		{"sing-box", "Android · iOS · macOS", "https://sing-box.sagernet.org/clients/", template.URL("sing-box://import-remote-profile?url=" + q + "#" + name)},
		{"Streisand", "iOS · macOS", "https://apps.apple.com/app/streisand/id6450534064", template.URL("streisand://import/" + d.Link + "#" + name)},
		{"v2rayN", "Windows · Linux · macOS", "https://github.com/2dust/v2rayN/releases/latest", ""},
		{"NekoBox", "Android", "https://github.com/MatsuriDayo/NekoBoxForAndroid/releases/latest", template.URL("sn://subscription?url=" + q + "&name=" + name)},
	}
	if (&service.ShopService{}).Settings().Enable {
		if bot, _ := service.BotUsername.Load().(string); bot != "" {
			d.Bot = "https://t.me/" + bot + "?start=sub"
		}
	}
	var buf bytes.Buffer
	if err := subPageTmpl.Execute(&buf, d); err != nil {
		return false
	}
	c.Header("Cache-Control", "no-store")
	c.Header("X-Robots-Tag", "noindex")
	c.Data(200, "text/html; charset=utf-8", buf.Bytes())
	return true
}
