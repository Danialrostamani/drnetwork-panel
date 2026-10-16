package tgbot

import (
	"context"
	"net/url"
	"strings"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
)

func (b *bot) customerAskLink(ctx context.Context, chatID int64, from *fromUser) {
	b.pend.set(chatID, &pending{kind: "cu.link", chatID: chatID})
	b.sendMenu(ctx, chatID, b.tr(
		"🔗 <b>اتصال لینک</b>\nلینک اشتراک فعلی‌تان را بفرستید (همان آدرسی که در برنامه وارد کرده‌اید).\nاسم اشتراک هم کافی است.",
		"🔗 <b>Connect link</b>\nSend your current subscription link (the address you added in the app).\nThe subscription name alone also works."),
		b.customerMenu(from.ID))
}

func (b *bot) customerBindLink(ctx context.Context, chatID int64, from *fromUser, text string) {
	name := clientNameFromSubLink(text)
	if name == "" {
		b.sendMenu(ctx, chatID, b.tr(
			"❌ این یک لینک اشتراک نیست.\nلینک پنل را بفرستید؛ معمولاً با http شروع می‌شود و اسم اشتراک ته آن است.",
			"❌ That is not a subscription link.\nSend the panel link; it usually starts with http and ends with the subscription name."),
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
	if err := b.bindClient(c.Id, from.ID); err != nil {
		b.sendMenu(ctx, chatID, b.tr("❌ اتصال انجام نشد.", "❌ Could not connect the link."), b.customerMenu(from.ID))
		return
	}
	b.sendMenu(ctx, chatID, b.tr(
		"✅ لینک به این ربات وصل شد. از «سرویس های من» ببینیدش.",
		"✅ The link is connected to this bot. See it under My services."),
		b.customerMenu(from.ID))
	b.customerServices(ctx, chatID, from.ID, false)
}

func clientNameFromSubLink(text string) string {
	line := strings.TrimSpace(strings.Split(strings.TrimSpace(text), "\n")[0])
	if line == "" {
		return ""
	}
	if u, err := url.Parse(line); err == nil && u.Scheme != "" && u.Host != "" {
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
	if strings.Contains(line, "://") {
		return ""
	}
	return line
}
