package tgbot

import (
	"context"
	"net/url"
	"strconv"
	"strings"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/service"
)

func (b *bot) customerAskLink(ctx context.Context, chatID int64, from *fromUser) {
	b.pend.set(chatID, &pending{kind: "cu.link", chatID: chatID})
	b.sendMenu(ctx, chatID, b.tr(
		"🔗 <b>اتصال لینک</b>\nلینک اشتراک کامل را بفرستید (همان آدرسی که در برنامه وارد کرده‌اید).\nفقط اسم اشتراک قبول نیست؛ ادمین باید اتصال را تأیید کند.",
		"🔗 <b>Connect link</b>\nSend the full subscription link (the address you added in the app).\nThe name alone is not enough; an admin must approve the connection."),
		b.customerMenu(from.ID))
}

func (b *bot) customerBindLink(ctx context.Context, chatID int64, from *fromUser, text string) {
	raw := findSubURL(text)
	id := clientNameFromSubLink(raw)
	if id == "" || !ownSubLink(raw, b.host()) {
		b.sendMenu(ctx, chatID, b.tr(
			"❌ این لینک اشتراک این پنل نیست.\nلینک کامل را بفرستید؛ معمولاً با http شروع می‌شود و اسم اشتراک ته مسیر /sub/ است.",
			"❌ That is not this panel's subscription link.\nSend the full link; it usually starts with http and ends with the name after /sub/."),
			b.customerMenu(from.ID))
		return
	}
	c, ok := clientBySubID(id)
	if !ok {
		b.sendMenu(ctx, chatID, b.tr(
			"❌ این لینک پیدا نشد. همان لینک اشتراک را کامل بفرستید.",
			"❌ That link was not found. Send the full subscription link."),
			b.customerMenu(from.ID))
		return
	}
	b.bindClientRequest(ctx, chatID, from, c)
}

// bindClientRequest asks the admins to connect a client to the customer's
// chat, unless it is connected already.
func (b *bot) bindClientRequest(ctx context.Context, chatID int64, from *fromUser, c model.Client) {
	if c.TgId == from.ID {
		b.sendMenu(ctx, chatID, b.tr(
			"✅ این لینک از قبل به همین حساب وصل است.",
			"✅ This link is already connected to this account."),
			b.customerMenu(from.ID))
		b.customerServices(ctx, chatID, from.ID)
		return
	}
	if c.TgId != 0 {
		b.sendMenu(ctx, chatID, b.tr(
			"❌ این اشتراک به حساب دیگری وصل است.",
			"❌ That subscription is already connected to another account."),
			b.customerMenu(from.ID))
		return
	}
	b.placeOrder(ctx, chatID, from, "bind", "", strconv.FormatUint(uint64(c.Id), 10))
}

func clientNameFromSubLink(text string) string {
	line := strings.TrimSpace(strings.Split(strings.TrimSpace(text), "\n")[0])
	if line == "" {
		return ""
	}
	u, err := url.Parse(line)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
	default:
		return ""
	}
	path := strings.Trim(u.Path, "/")
	if path == "" {
		return ""
	}
	parts := strings.Split(path, "/")
	name, err := url.PathUnescape(parts[len(parts)-1])
	if err != nil {
		name = parts[len(parts)-1]
	}
	return strings.TrimSpace(name)
}

// findSubURL is the first http(s) address in a message: an app's share text
// or a copied note can put words, a remark or line breaks around the link.
func findSubURL(text string) string {
	for _, f := range strings.Fields(text) {
		f = strings.Trim(f, "<>()[]\"'`,،")
		l := strings.ToLower(f)
		if strings.HasPrefix(l, "http://") || strings.HasPrefix(l, "https://") {
			return f
		}
	}
	return strings.TrimSpace(strings.Split(strings.TrimSpace(text), "\n")[0])
}

// clientBySubID finds the client a link's last part names, the same way the
// subscription server does: by its token for a client that uses one, by its
// name for one that does not.
func clientBySubID(id string) (model.Client, bool) {
	var c model.Client
	db := database.GetDB()
	if len(id) == model.SubTokenLength {
		if err := db.Where("sub_token = ? AND use_sub_token = ?", id, true).First(&c).Error; err == nil {
			return c, true
		}
	}
	if err := db.Where("name = ? AND use_sub_token = ?", id, false).First(&c).Error; err == nil {
		return c, true
	}
	return model.Client{}, false
}

// ownSubLink tells whether a link is under this panel's subscription path:
// the sub server's own path, or the path of the public subscription address
// (subURI) when a reverse proxy serves the links somewhere else.
func ownSubLink(raw, host string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return false
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
	default:
		return false
	}
	prefixes := []string{"/sub/"}
	if database.GetDB() != nil {
		st := &service.SettingService{}
		if p, err := st.GetSubPath(); err == nil && strings.TrimSpace(p) != "" {
			prefixes[0] = p
		}
		if base, err := st.GetFinalSubURI(host); err == nil {
			if bu, err := url.Parse(strings.TrimSpace(base)); err == nil && bu.Host != "" {
				prefixes = append(prefixes, bu.Path)
			}
		}
	}
	path := "/" + strings.Trim(u.Path, "/")
	for _, p := range prefixes {
		prefix := strings.TrimRight("/"+strings.Trim(p, "/"), "/")
		if path != prefix && path != "/" && strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}
