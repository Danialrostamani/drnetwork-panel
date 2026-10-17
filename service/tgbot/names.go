package tgbot

import (
	"strconv"
	"strings"
	"unicode"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
)

func sanitizeClientName(s string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(s) {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '_' || r == '.' || r == '-' || r == '@':
			b.WriteRune(r)
		case unicode.IsSpace(r):
			b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), "._-")
	for strings.Contains(out, "__") {
		out = strings.ReplaceAll(out, "__", "_")
	}
	if len(out) > 32 {
		out = out[:32]
	}
	return out
}

func clientNameHint(from *fromUser) string {
	if from == nil {
		return ""
	}
	if u := sanitizeClientName(from.Username); clientNameRe.MatchString(u) {
		return u
	}
	first := sanitizeClientName(from.FirstName)
	last := sanitizeClientName(from.LastName)
	joined := strings.Trim(first+"_"+last, "_")
	if clientNameRe.MatchString(joined) {
		return joined
	}
	if clientNameRe.MatchString(first) {
		return first
	}
	return ""
}

func (b *bot) uniqueClientName(tgID int64, hint string) string {
	base := sanitizeClientName(hint)
	if !clientNameRe.MatchString(base) {
		base = "u" + strconv.FormatInt(tgID, 10)
	}
	name := base
	for i := 2; ; i++ {
		var n int64
		database.GetDB().Model(&model.Client{}).Where("name = ?", name).Count(&n)
		if n == 0 {
			return name
		}
		name = base + "_" + strconv.Itoa(i)
	}
}
