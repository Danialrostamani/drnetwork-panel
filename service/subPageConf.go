package service

import (
	"encoding/json"
	"net/url"
	"regexp"
	"strings"
	"time"
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
	// The checks: through the VPN, world services, server states, report.
	HideVpn     bool `json:"hideVpn,omitempty"`
	HideSites   bool `json:"hideSites,omitempty"`
	HideServers bool `json:"hideServers,omitempty"`
	HideReport  bool `json:"hideReport,omitempty"`
	// Notices are shown at the top until their day ends; Sites are the
	// services the page checks (empty: its own list).
	Notices []SubPageNotice `json:"notices,omitempty"`
	Sites   []SubPageSite   `json:"sites,omitempty"`
	// NoBot keeps the renew and connect buttons off the Telegram bot even
	// while it runs.
	NoBot bool `json:"noBot,omitempty"`
}

// SubPageNotice is one notice of the page: kind "info", "warn" or "promo",
// shown until the end of the day Until (YYYY-MM-DD, panel time); no Until
// is for good.
type SubPageNotice struct {
	Text   string `json:"text,omitempty"`
	TextEn string `json:"textEn,omitempty"`
	Kind   string `json:"kind,omitempty"`
	Until  string `json:"until,omitempty"`
}

// SubPageSite is a service the page checks it can reach.
type SubPageSite struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

const (
	subPageNoticesMax = 10
	subPageSitesMax   = 24
)

var untilRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// ActiveNotices are the notices still to show at now, the single notice of
// older settings first.
func (c SubPageConf) ActiveNotices(now time.Time, loc *time.Location) []SubPageNotice {
	var out []SubPageNotice
	if c.Notice != "" || c.NoticeEn != "" {
		out = append(out, SubPageNotice{Text: c.Notice, TextEn: c.NoticeEn, Kind: "info"})
	}
	if loc == nil {
		loc = time.Local
	}
	for _, n := range c.Notices {
		if n.Until != "" {
			day, err := time.ParseInLocation("2006-01-02", n.Until, loc)
			if err == nil && !now.Before(day.AddDate(0, 0, 1)) {
				continue
			}
		}
		out = append(out, n)
	}
	return out
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

// SubPageFlags is the page setting read for its switches.
func SubPageFlags() SubPageConf { return (&SettingService{}).GetSubPageConf() }

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
	if len(conf.Notices) > subPageNoticesMax {
		return "", common.NewError("subscription page: at most ", subPageNoticesMax, " notices")
	}
	notices := conf.Notices[:0]
	for _, n := range conf.Notices {
		n.Text, n.TextEn, n.Until = strings.TrimSpace(n.Text), strings.TrimSpace(n.TextEn), strings.TrimSpace(n.Until)
		if n.Text == "" && n.TextEn == "" {
			continue
		}
		if utf8.RuneCountInString(n.Text) > subPageNoticeMax || utf8.RuneCountInString(n.TextEn) > subPageNoticeMax {
			return "", common.NewError("subscription page: text is too long")
		}
		switch n.Kind {
		case "info", "warn", "promo":
		case "":
			n.Kind = "info"
		default:
			return "", common.NewError("subscription page: unknown notice kind ", n.Kind)
		}
		if n.Until != "" && !untilRe.MatchString(n.Until) {
			return "", common.NewError("subscription page: a notice date must look like 2025-12-31")
		}
		notices = append(notices, n)
	}
	conf.Notices = notices
	if len(conf.Sites) > subPageSitesMax {
		return "", common.NewError("subscription page: at most ", subPageSitesMax, " services to check")
	}
	sites := conf.Sites[:0]
	for _, x := range conf.Sites {
		x.Name, x.URL = strings.TrimSpace(x.Name), strings.TrimSpace(x.URL)
		if x.Name == "" && x.URL == "" {
			continue
		}
		u, err := url.Parse(x.URL)
		if x.Name == "" || utf8.RuneCountInString(x.Name) > subPageNameMax || err != nil || !strings.EqualFold(u.Scheme, "https") || u.Host == "" || len(x.URL) > subPageLinkMax {
			return "", common.NewError("subscription page: each service needs a name and an https:// address")
		}
		sites = append(sites, x)
	}
	conf.Sites = sites
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
