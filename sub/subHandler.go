package sub

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/Danialrostamani/drnetwork-panel/logger"
	"github.com/Danialrostamani/drnetwork-panel/service"

	"github.com/gin-gonic/gin"
)

type SubHandler struct {
	service.SettingService
	SubService
	JsonService
	ClashService
}

func NewSubHandler(g *gin.RouterGroup) {
	a := &SubHandler{}
	a.initRouter(g)
}

func (s *SubHandler) initRouter(g *gin.RouterGroup) {
	g.GET("/:subid", s.subs)
	g.HEAD("/:subid", s.subHeaders)
}

func (s *SubHandler) subs(c *gin.Context) {
	var headers []string
	var result *string
	var err error
	subId := c.Param("subid")
	if !s.hostServes(c, subId) {
		return
	}
	if wantsPage(c) && s.SettingService.GetSubPage() && s.page(c, subId) {
		return
	}
	format, isFormat := c.GetQuery("format")
	if isFormat {
		switch format {
		case "json":
			result, headers, err = s.JsonService.GetJson(subId, format)
		case "clash":
			result, headers, err = s.ClashService.GetClash(subId)
		}
		if err != nil || result == nil {
			logger.Error(err)
			c.String(400, "Error!")
			return
		}
	} else {
		result, headers, err = s.SubService.GetSubs(subId)
		if err != nil || result == nil {
			logger.Error(err)
			c.String(400, "Error!")
			return
		}
	}

	s.addHeaders(c, headers)

	c.String(200, *result)
}

// hostServes refuses a link opened on the host of another client: under a
// wildcard domain every client has its own label, and the link answers on
// that one only. The refusal looks like a client that does not exist.
func (s *SubHandler) hostServes(c *gin.Context, subId string) bool {
	if s.SettingService.SubLabelOK(c.Request.Host, subId) {
		return true
	}
	c.String(400, "Error!")
	return false
}

func (s *SubHandler) subHeaders(c *gin.Context) {
	subId := c.Param("subid")
	if !s.hostServes(c, subId) {
		return
	}
	client, err := s.SubService.getClientBySubId(subId)
	if err != nil {
		logger.Error(err)
		c.String(400, "Error!")
		return
	}

	headers := s.SubService.getClientHeaders(client)
	s.addHeaders(c, headers)

	c.Status(200)
}

func (s *SubHandler) addHeaders(c *gin.Context, headers []string) {
	c.Writer.Header().Set("Subscription-Userinfo", headers[0])
	c.Writer.Header().Set("Profile-Update-Interval", headers[1])
	c.Writer.Header().Set("Profile-Title", headerText(headers[2]))
	c.Writer.Header().Set("Content-Disposition", contentDispositionHeader(headers[2]))
	// What Happ and v2RayTun show with the subscription; nothing is sent for
	// what is not set.
	if announce := strings.TrimSpace(s.SettingService.GetSubAnnounce()); announce != "" {
		c.Writer.Header().Set("Announce", headerText(announce))
	}
	if support := strings.TrimSpace(s.SettingService.GetSubSupportUrl()); support != "" {
		c.Writer.Header().Set("Support-Url", headerURL(support))
	}
	if s.SettingService.GetSubWebPage() && s.SettingService.GetSubPage() {
		c.Writer.Header().Set("Profile-Web-Page-Url", requestURL(c))
	}
}

// headerText puts text into a header the way the apps read it: as it is when
// it is plain printable ASCII, otherwise base64 behind a "base64:" prefix --
// a header cannot carry Persian or a line break.
func headerText(text string) string {
	plain := true
	for i := 0; i < len(text); i++ {
		if b := text[i]; b < 0x20 || b > 0x7e {
			plain = false
			break
		}
	}
	if plain && !strings.HasPrefix(text, "base64:") {
		return text
	}
	return "base64:" + base64.StdEncoding.EncodeToString([]byte(text))
}

// headerURL escapes what a link may hold but a header may not.
func headerURL(link string) string {
	var b strings.Builder
	const hex = "0123456789ABCDEF"
	for i := 0; i < len(link); i++ {
		c := link[i]
		if c <= 0x20 || c >= 0x7f {
			b.WriteByte('%')
			b.WriteByte(hex[c>>4])
			b.WriteByte(hex[c&0x0f])
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

func contentDispositionHeader(name string) string {
	filename := strings.TrimSpace(name)
	if filename == "" {
		filename = "subscription"
	}

	return fmt.Sprintf("attachment; filename=\"%s\"; filename*=UTF-8''%s", asciiSafeFilename(filename), rfc5987Encode(filename))
}

func asciiSafeFilename(filename string) string {
	var builder strings.Builder
	for _, r := range filename {
		switch {
		case r == '"' || r == '\\':
			builder.WriteByte('_')
		case r >= 0x20 && r <= 0x7e:
			builder.WriteRune(r)
		}
	}

	fallback := strings.TrimSpace(builder.String())
	if fallback == "" {
		return "subscription"
	}

	return fallback
}

func rfc5987Encode(filename string) string {
	const hex = "0123456789ABCDEF"

	var builder strings.Builder
	for _, b := range []byte(filename) {
		if isRFC5987AttrChar(b) {
			builder.WriteByte(b)
			continue
		}

		builder.WriteByte('%')
		builder.WriteByte(hex[b>>4])
		builder.WriteByte(hex[b&0x0f])
	}

	return builder.String()
}

func isRFC5987AttrChar(b byte) bool {
	switch {
	case b >= 'a' && b <= 'z':
		return true
	case b >= 'A' && b <= 'Z':
		return true
	case b >= '0' && b <= '9':
		return true
	}

	switch b {
	case '!', '#', '$', '&', '+', '-', '.', '^', '_', '`', '|', '~':
		return true
	default:
		return false
	}
}
