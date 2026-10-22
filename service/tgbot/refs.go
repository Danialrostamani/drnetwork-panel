package tgbot

import (
	"context"
	"strings"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
)

type shopReferrer struct {
	ID    string `json:"id"`
	Word  string `json:"word"`
	Group string `json:"group"`
	Admin int64  `json:"admin,omitempty"`
}

func normalizeRefWord(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func (s shopConfig) findRef(word string) *shopReferrer {
	w := normalizeRefWord(word)
	if w == "" {
		return nil
	}
	for i := range s.Refs {
		if normalizeRefWord(s.Refs[i].Word) == w {
			return &s.Refs[i]
		}
	}
	return nil
}

func loadBotRef(tgID int64) *model.BotRef {
	var r model.BotRef
	if err := database.GetDB().First(&r, tgID).Error; err != nil {
		return nil
	}
	return &r
}

func saveBotRef(tgID int64, word, group string) error {
	return database.GetDB().Save(&model.BotRef{TgId: tgID, Word: word, Group: group}).Error
}

func (b *bot) trySaveRef(tgID int64, word string) bool {
	r := b.loadShop().findRef(word)
	if r == nil {
		return false
	}
	return saveBotRef(tgID, r.Word, r.Group) == nil
}

func (s shopConfig) hasRefs() bool {
	for _, r := range s.Refs {
		if normalizeRefWord(r.Word) != "" {
			return true
		}
	}
	return false
}

func (b *bot) customerNeedsRef(tgID int64) bool {
	s := b.loadShop()
	if !s.hasRefs() {
		return false
	}
	if len(boundClients(tgID)) > 0 {
		return false
	}
	stored := loadBotRef(tgID)
	return stored == nil || s.findRef(stored.Word) == nil
}

func (b *bot) customerAskRef(ctx context.Context, chatID int64, from *fromUser, after string) {
	b.pend.set(chatID, &pending{kind: "cu.ref", chatID: chatID, data: map[string]string{"after": after}})
	b.sendMenu(ctx, chatID, b.tr(
		"👋 خوش آمدید\n\n🪪 <b>معرف</b>\nقبل از هر کاری، کلمه معرف را بفرستید.\nهمان کلمه‌ای که معرف‌تان گفته است.\n\nاگر کلمه معرف را ندارید، به «پشتیبانی» پیام دهید.",
		"👋 Welcome\n\n🪪 <b>Introducer</b>\nBefore anything else, send the introducer word.\nThe same word your introducer gave you.\n\nIf you don't have the word, message Support."),
		b.customerMenu(from.ID))
}

func (b *bot) customerAcceptRef(ctx context.Context, chatID int64, from *fromUser, text string) {
	if !b.trySaveRef(from.ID, text) {
		b.sendMenu(ctx, chatID, b.tr(
			"❌ این معرف معتبر نیست. کلمه را درست بفرستید.",
			"❌ That introducer is not valid. Send the word exactly."),
			b.customerMenu(from.ID))
		return
	}
	after := "buy"
	if p := b.pend.get(chatID); p != nil && p.data != nil && p.data["after"] != "" {
		after = p.data["after"]
	}
	b.pend.clear(chatID)
	b.send(ctx, chatID, b.tr("✅ معرف ثبت شد.", "✅ Introducer saved."))
	switch {
	case after == "trial" || after == "test":
		b.autoTrial(ctx, chatID, from)
	case after == "home":
		b.customerStart(ctx, chatID, from)
	case strings.HasPrefix(after, "p:"):
		b.customerPress(ctx, chatID, from, after)
	default:
		b.customerBuy(ctx, chatID, from, "")
	}
}

func (s shopConfig) refGroup(word string) string {
	if r := s.findRef(word); r != nil {
		return strings.TrimSpace(r.Group)
	}
	return ""
}
