package util

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/util/hosts"
)

// WildcardLabel gives the per-client label of a wildcard address
// ("*.cdn.example.com"). The service sets the keyed version at start.
var WildcardLabel = func(base, name string) string { return hosts.Label(nil, base, name) }

// NameTemplateVars are the variables a link name template can use. {REMARK}
// and {USER_REMARK} are the same thing, the client's remark; {ROUTE} is the
// remark of the inbound address the link goes through.
var NameTemplateVars = []string{
	"USER", "REMARK", "USER_REMARK", "GROUP", "INBOUND", "PROTOCOL", "NETWORK", "NODE", "ROUTE",
	"SERVER", "PORT", "USED", "REMAINING", "TOTAL", "DAYS_LEFT", "EXPIRE", "EXPIRE_JALALI",
}

var nameTemplateVar = func() map[string]bool {
	m := map[string]bool{}
	for _, v := range NameTemplateVars {
		m[v] = true
	}
	return m
}()

// MaxLinkName caps a rendered name; longer ones are cut.
const MaxLinkName = 128

type tplPart struct {
	text  string
	name  string // the variable, "" for literal text
	empty bool
}

// parseNameTemplate splits a template into text and known variables. An
// unknown {NAME} stays text, exactly as written.
func parseNameTemplate(tpl string) []tplPart {
	var parts []tplPart
	var lit strings.Builder
	flush := func() {
		if lit.Len() > 0 {
			parts = append(parts, tplPart{text: lit.String()})
			lit.Reset()
		}
	}
	for i := 0; i < len(tpl); {
		if tpl[i] == '{' {
			if end := strings.IndexByte(tpl[i+1:], '}'); end > 0 {
				name := tpl[i+1 : i+1+end]
				if nameTemplateVar[name] {
					flush()
					parts = append(parts, tplPart{name: name})
					i += end + 2
					continue
				}
			}
		}
		lit.WriteByte(tpl[i])
		i++
	}
	flush()
	return parts
}

// CheckNameTemplate refuses a template that would not name anything: one
// without any known variable, an overlong one, or one with control
// characters.
func CheckNameTemplate(tpl string) error {
	if tpl == "" {
		return nil
	}
	if utf8.RuneCountInString(tpl) > 256 {
		return errors.New("the link name template is longer than 256 characters")
	}
	for _, r := range tpl {
		if unicode.IsControl(r) {
			return errors.New("the link name template cannot have line breaks or control characters")
		}
	}
	for _, p := range parseNameTemplate(tpl) {
		if p.name != "" {
			return nil
		}
	}
	return errors.New("the link name template uses no variable; use some of {" + strings.Join(NameTemplateVars, "} {") + "}")
}

func isNameSep(r rune) bool {
	return unicode.IsSpace(r) || strings.ContainsRune("-_|/\\·•,:;–—~+", r)
}

// RenderName fills a template in. A variable without a value takes the
// separator next to it along, so "{REMARK} | {NODE} | {INBOUND}" without a
// node reads "Ali | vless" rather than "Ali |  | vless". The result is cut to
// MaxLinkName characters; "" means the template gave nothing and the caller
// keeps its default name.
func RenderName(tpl string, vars map[string]string) string {
	parts := parseNameTemplate(tpl)
	for i := range parts {
		if parts[i].name != "" {
			parts[i].text = strings.TrimSpace(vars[parts[i].name])
			parts[i].empty = parts[i].text == ""
		}
	}
	for i := range parts {
		if !parts[i].empty {
			continue
		}
		if i > 0 && parts[i-1].name == "" {
			if t := strings.TrimRightFunc(parts[i-1].text, isNameSep); t != parts[i-1].text {
				parts[i-1].text = t
				continue
			}
		}
		if i+1 < len(parts) && parts[i+1].name == "" {
			parts[i+1].text = strings.TrimLeftFunc(parts[i+1].text, isNameSep)
		}
	}
	var sb strings.Builder
	for _, p := range parts {
		sb.WriteString(p.text)
	}
	out := strings.TrimSpace(sb.String())
	if strings.TrimFunc(out, isNameSep) == "" {
		return ""
	}
	if utf8.RuneCountInString(out) > MaxLinkName {
		out = strings.TrimSpace(string([]rune(out)[:MaxLinkName]))
	}
	return out
}

// ShortBytes writes a volume the way a link name has room for: 512MB, 1.5GB.
func ShortBytes(n int64) string {
	if n < 0 {
		n = 0
	}
	const mb, gb, tb = int64(1) << 20, int64(1) << 30, int64(1) << 40
	trim := func(f float64) string {
		return strings.TrimSuffix(strings.TrimRight(strconv.FormatFloat(f, 'f', 2, 64), "0"), ".")
	}
	switch {
	case n >= tb:
		return trim(float64(n)/float64(tb)) + "TB"
	case n >= gb:
		return trim(float64(n)/float64(gb)) + "GB"
	default:
		return trim(float64(n)/float64(mb)) + "MB"
	}
}

// ClientNameVars are the variables that come from the client itself.
func ClientNameVars(c *model.Client, now time.Time, loc *time.Location) map[string]string {
	if loc == nil {
		loc = time.Local
	}
	used := c.Up + c.Down
	v := map[string]string{
		"USER":        c.Name,
		"REMARK":      c.Remark,
		"USER_REMARK": c.Remark,
		"GROUP":       c.Group,
		"USED":        ShortBytes(used),
		"TOTAL":       "∞",
		"REMAINING":   "∞",
		"DAYS_LEFT":   "∞",
	}
	if c.Volume > 0 {
		v["TOTAL"] = ShortBytes(c.Volume)
		v["REMAINING"] = ShortBytes(max(c.Volume-used, 0))
	}
	switch {
	case c.DelayStart && c.ResetDays > 0:
		v["DAYS_LEFT"] = strconv.Itoa(c.ResetDays)
	case c.Expiry > 0:
		left := (c.Expiry - now.Unix()) / 86400
		v["DAYS_LEFT"] = strconv.FormatInt(max(left, 0), 10)
		t := time.Unix(c.Expiry, 0).In(loc)
		v["EXPIRE"] = t.Format("2006-01-02")
		jy, jm, jd := GregorianToJalali(t.Year(), int(t.Month()), t.Day())
		v["EXPIRE_JALALI"] = fmt.Sprintf("%04d/%02d/%02d", jy, jm, jd)
	}
	return v
}

// GregorianToJalali converts a date of the Gregorian calendar to the Solar
// Hijri (Jalali) one, by the 33-year arithmetic rule that matches the
// official calendar for the years in use.
func GregorianToJalali(gy, gm, gd int) (jy, jm, jd int) {
	gDaysBeforeMonth := [...]int{0, 31, 59, 90, 120, 151, 181, 212, 243, 273, 304, 334}
	if gm < 1 || gm > 12 {
		return 0, 0, 0
	}
	gy2 := gy
	if gm > 2 {
		gy2 = gy + 1
	}
	days := 355666 + 365*gy + (gy2+3)/4 - (gy2+99)/100 + (gy2+399)/400 + gd + gDaysBeforeMonth[gm-1]
	jy = -1595 + 33*(days/12053)
	days %= 12053
	jy += 4 * (days / 1461)
	days %= 1461
	if days > 365 {
		jy += (days - 1) / 365
		days = (days - 1) % 365
	}
	if days < 186 {
		jm = 1 + days/31
		jd = 1 + days%31
	} else {
		jm = 7 + (days-186)/30
		jd = 1 + (days-186)%30
	}
	return jy, jm, jd
}
