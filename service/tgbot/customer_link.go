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
	raw := strings.TrimSpace(strings.Split(strings.TrimSpace(text), "\n")[0])
	name := clientNameFromSubLink(raw)
	if name == "" || !ownSubLink(raw) {
		b.sendMenu(ctx, chatID, b.tr(
			"❌ این لینک اشتراک این پنل نیست.\nلینک کامل را بفرستید؛ معمولاً با http شروع می‌شود و اسم اشتراک ته مسیر /sub/ است.",
			"❌ That is not this panel's subscription link.\nSend the full link; it usually starts with http and ends with the name after /sub/."),
			b.customerMenu(from.ID))
		return
	}
	var c model.Client
	if err := database.GetDB().Where("name = ?", name).First(&c).Error; err != nil {
		b.sendMenu(ctx, chatID, b.tr(
			"❌ این لینک پیدا نشد. همان لینک اشتراک را کامل بفرستید.",
			"❌ That link was not found. Send the full subscription link."),
			b.customerMenu(from.ID))
		return
	}
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

func ownSubLink(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return false
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
	default:
		return false
	}
	subPath := "/sub/"
	if database.GetDB() != nil {
		if p, err := (&service.SettingService{}).GetSubPath(); err == nil && strings.TrimSpace(p) != "" {
			subPath = p
		}
	}
	prefix := "/" + strings.Trim(subPath, "/")
	path := "/" + strings.Trim(u.Path, "/")
	return path != prefix && strings.HasPrefix(path, prefix+"/")
}
