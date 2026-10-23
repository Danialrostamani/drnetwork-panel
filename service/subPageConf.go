package service

import (
	"encoding/json"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/Danialrostamani/drnetwork-panel/util/common"
)

// SubPageConf is how the subscription page looks and what it offers, set in
// the panel. Empty fields keep the page's own defaults.
type SubPageConf struct {
	Name     string `json:"name,omitempty"`
	NameEn   string `json:"nameEn,omitempty"`
	Logo     string `json:"logo,omitempty"`
	Accent   string `json:"accent,omitempty"`
	Notice   string `json:"notice,omitempty"`
	NoticeEn string `json:"noticeEn,omitempty"`
	Support  string `json:"support,omitempty"`
	Channel  string `json:"channel,omitempty"`
	// The sections a panel can leave out of the page.
	HideNet   bool `json:"hideNet,omitempty"`
	HideApps  bool `json:"hideApps,omitempty"`
	HideQr    bool `json:"hideQr,omitempty"`
	HideLive  bool `json:"hideLive,omitempty"`
	HidePlans bool `json:"hidePlans,omitempty"`
	HideTips  bool `json:"hideTips,omitempty"`
	// NoBot keeps the renew and connect buttons off the Telegram bot even
	// while it runs.
	NoBot bool `json:"noBot,omitempty"`
}

const (
	subPageNameMax   = 60
	subPageNoticeMax = 600
	subPageLinkMax   = 2048
)

var accentRe = regexp.MustCompile(`^#(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)

// GetSubPageConf is the page setting; a broken value reads as empty.
func (s *SettingService) GetSubPageConf() SubPageConf {
	var conf SubPageConf
	raw, err := s.getString("subPageConf")
	if err != nil || strings.TrimSpace(raw) == "" {
		return conf
	}
	_ = json.Unmarshal([]byte(raw), &conf)
	return conf
}

// PageLink is a link the page can put in an href: an http(s) or tg address,
// with a bare @handle taken as a Telegram one. Anything else is "".
func PageLink(raw string) string {
	v := strings.TrimSpace(raw)
	if v == "" || len(v) > subPageLinkMax {
		return ""
	}
	if strings.HasPrefix(v, "@") && len(v) > 1 && !strings.ContainsAny(v, " /?#") {
		return "https://t.me/" + v[1:]
	}
	u, err := url.Parse(v)
	if err != nil {
		return ""
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		if u.Host == "" {
			return ""
		}
	case "tg":
	default:
		return ""
	}
	return v
}

// normalizeSubPageConf checks a posted page setting and writes it back in a
// fixed form: links that are not links and colours that are not colours
// would only break the page.
func normalizeSubPageConf(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "{}", nil
	}
	var conf SubPageConf
	if err := json.Unmarshal([]byte(raw), &conf); err != nil {
		return "", common.NewError("invalid subscription page settings")
	}
	trim := func(v *string, max int) error {
		*v = strings.TrimSpace(*v)
		if utf8.RuneCountInString(*v) > max {
			return common.NewError("subscription page: text is too long")
		}
		return nil
	}
	for _, f := range []struct {
		v   *string
		max int
	}{{&conf.Name, subPageNameMax}, {&conf.NameEn, subPageNameMax}, {&conf.Notice, subPageNoticeMax}, {&conf.NoticeEn, subPageNoticeMax}} {
		if err := trim(f.v, f.max); err != nil {
			return "", err
		}
	}
	for _, f := range []struct {
		v    *string
		name string
	}{{&conf.Support, "support"}, {&conf.Channel, "channel"}} {
		if strings.TrimSpace(*f.v) == "" {
			*f.v = ""
			continue
		}
		link := PageLink(*f.v)
		if link == "" {
			return "", common.NewError("subscription page: the ", f.name, " link must start with https:// or @")
		}
		*f.v = link
	}
	conf.Logo = strings.TrimSpace(conf.Logo)
	if conf.Logo != "" {
		if u, err := url.Parse(conf.Logo); err != nil || len(conf.Logo) > subPageLinkMax || u.Host == "" ||
			(!strings.EqualFold(u.Scheme, "https") && !strings.EqualFold(u.Scheme, "http")) {
			return "", common.NewError("subscription page: the logo must be an http(s) image address")
		}
	}
	conf.Accent = strings.TrimSpace(conf.Accent)
	if conf.Accent != "" && !accentRe.MatchString(conf.Accent) {
		return "", common.NewError("subscription page: the colour must look like #0C96B2")
	}
	out, err := json.Marshal(conf)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
