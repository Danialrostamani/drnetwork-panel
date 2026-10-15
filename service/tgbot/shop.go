package tgbot

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/service"
)

// shopConfig is what the owner edits: the plans, wallet amounts and texts
// the customer sees. Empty fields keep the built-in wording.
type shopConfig struct {
	WelcomeFa string        `json:"welcomeFa,omitempty"`
	WelcomeEn string        `json:"welcomeEn,omitempty"`
	GuideFa   string        `json:"guideFa,omitempty"`
	GuideEn   string        `json:"guideEn,omitempty"`
	Trial     *bool         `json:"trial,omitempty"`
	Plans     []shopPlan    `json:"plans,omitempty"`
	Wallets   []shopWallet  `json:"wallets,omitempty"`
	Buttons   []shopMenuBtn `json:"buttons,omitempty"`
}

func (s shopConfig) trialOn() bool {
	return s.Trial == nil || *s.Trial
}

type shopPlan struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Days  int    `json:"days"`
	GB    int    `json:"gb"` // 0 = unlimited
	Price string `json:"price"`
}

type shopWallet struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Amount string `json:"amount"`
}

type shopMenuBtn struct {
	Data string `json:"data"`
	Fa   string `json:"fa,omitempty"`
	En   string `json:"en,omitempty"`
	Off  bool   `json:"off,omitempty"`
}

var shopMenuKeys = []string{
	"p:buy", "p:renew", "p:my", "p:wallet", "p:trial", "p:tariff", "p:support", "p:guide",
}

func (b *bot) loadShop() shopConfig {
	raw, err := (&service.SettingService{}).GetTgBotShop()
	if err != nil || strings.TrimSpace(raw) == "" {
		return shopConfig{}
	}
	var s shopConfig
	if json.Unmarshal([]byte(raw), &s) != nil {
		return shopConfig{}
	}
	return s
}

func (b *bot) saveShop(s shopConfig) error {
	raw, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return (&service.SettingService{}).SetTgBotShop(string(raw))
}

func (s shopConfig) plan(id string) *shopPlan {
	for i := range s.Plans {
		if s.Plans[i].ID == id {
			return &s.Plans[i]
		}
	}
	return nil
}

func (s shopConfig) wallet(id string) *shopWallet {
	for i := range s.Wallets {
		if s.Wallets[i].ID == id {
			return &s.Wallets[i]
		}
	}
	return nil
}

func (s shopConfig) btn(data string) shopMenuBtn {
	for _, it := range s.Buttons {
		if it.Data == data {
			return it
		}
	}
	return shopMenuBtn{Data: data}
}

func (p shopPlan) label() string {
	vol := "∞"
	if p.GB > 0 {
		vol = strconv.Itoa(p.GB) + "GB"
	}
	days := ""
	if p.Days > 0 {
		days = strconv.Itoa(p.Days) + "d"
	}
	bits := []string{p.Name}
	if days != "" || vol != "∞" || p.GB > 0 {
		bits = append(bits, strings.TrimSpace(days+" "+vol))
	}
	if p.Price != "" {
		bits = append(bits, p.Price)
	}
	return strings.Join(bits, " · ")
}

func nextShopID(existing []string) string {
	max := 0
	for _, id := range existing {
		n, err := strconv.Atoi(id)
		if err == nil && n > max {
			max = n
		}
	}
	return strconv.Itoa(max + 1)
}

func (b *bot) shopHomeText() string {
	s := b.loadShop()
	if s.WelcomeFa != "" || s.WelcomeEn != "" {
		return b.tr(s.WelcomeFa, nonempty(s.WelcomeEn, s.WelcomeFa))
	}
	return b.customerHome()
}

func (b *bot) shopGuideText() string {
	s := b.loadShop()
	if s.GuideFa != "" || s.GuideEn != "" {
		return b.tr(s.GuideFa, nonempty(s.GuideEn, s.GuideFa))
	}
	return b.customerGuide()
}

func nonempty(v, fallback string) string {
	if strings.TrimSpace(v) != "" {
		return v
	}
	return fallback
}

func (b *bot) customerMenu(id int64) [][]string {
	s := b.loadShop()
	var keys []string
	for _, k := range shopMenuKeys {
		if k == "p:trial" && !s.trialOn() {
			continue
		}
		if s.btn(k).Off {
			continue
		}
		keys = append(keys, k)
	}
	var rows [][]string
	for i := 0; i < len(keys); i += 2 {
		if i+1 < len(keys) {
			rows = append(rows, []string{keys[i], keys[i+1]})
		} else {
			rows = append(rows, []string{keys[i]})
		}
	}
	if b.isAdmin(id) {
		rows = append(rows, []string{"k:admin"})
	}
	return rows
}

func (b *bot) menuLookup(text string) string {
	if d := menuData(text); d != "" {
		return d
	}
	s := b.loadShop()
	for _, k := range shopMenuKeys {
		it := s.btn(k)
		if it.Fa != "" && text == it.Fa {
			return k
		}
		if it.En != "" && text == it.En {
			return k
		}
	}
	return ""
}

func (b *bot) shopPlanButtons(prefix string) [][]button {
	s := b.loadShop()
	var rows [][]button
	var row []button
	for _, p := range s.Plans {
		row = append(row, button{Text: p.label(), Data: prefix + p.ID})
		if len(row) == 1 {
			rows = append(rows, row)
			row = nil
		}
	}
	if len(row) > 0 {
		rows = append(rows, row)
	}
	return rows
}

func (b *bot) customerBuy(ctx context.Context, chatID int64, from *fromUser, renewID string) {
	s := b.loadShop()
	if len(s.Plans) == 0 {
		b.sendKeyboard(ctx, chatID, b.tr(
			"🔐 هنوز پلنی در فروشگاه نیست. درخواست برای پشتیبانی فرستاده می‌شود.",
			"🔐 No plan is in the shop yet. A request is sent to support."),
			[][]button{{{Text: b.tr("📩 ارسال درخواست خرید", "📩 Send buy request"), Data: "u:ask:buy"}}})
		return
	}
	prefix := "u:buy:"
	title := b.tr("🔐 <b>خرید اشتراک</b>\nپلن را انتخاب کنید.", "🔐 <b>Buy a plan</b>\nPick a plan.")
	if renewID != "" {
		prefix = "u:rb:" + renewID + ":"
		title = b.tr("♻️ <b>تمدید</b>\nپلن تمدید را انتخاب کنید.", "♻️ <b>Renew</b>\nPick the plan to renew with.")
	}
	b.sendKeyboard(ctx, chatID, title, b.shopPlanButtons(prefix))
}

func (b *bot) customerWallet(ctx context.Context, chatID int64, from *fromUser) {
	s := b.loadShop()
	wallets := s.Wallets
	if len(wallets) == 0 {
		wallets = []shopWallet{
			{ID: "50", Label: "50,000", Amount: "50000"},
			{ID: "100", Label: "100,000", Amount: "100000"},
			{ID: "200", Label: "200,000", Amount: "200000"},
			{ID: "500", Label: "500,000", Amount: "500000"},
		}
	}
	var rows [][]button
	for _, w := range wallets {
		label := w.Label
		if label == "" {
			label = w.Amount
		}
		rows = append(rows, []button{{Text: label, Data: "u:wal:" + w.ID}})
	}
	b.sendKeyboard(ctx, chatID, b.tr(
		"🏦 <b>کیف پول</b>\nمبلغ شارژ را انتخاب کنید.",
		"🏦 <b>Wallet</b>\nPick the amount to add."), rows)
}

func (b *bot) customerTariff(ctx context.Context, chatID int64, from *fromUser) {
	s := b.loadShop()
	if len(s.Plans) == 0 {
		b.sendMenu(ctx, chatID, b.tr(
			"💵 هنوز تعرفه‌ای ثبت نشده. از پشتیبانی بپرسید.",
			"💵 No plans are listed yet. Ask support."), b.customerMenu(from.ID))
		return
	}
	var lines []string
	lines = append(lines, b.tr("💵 <b>تعرفه اشتراک‌ها</b>", "💵 <b>Plans</b>"))
	for _, p := range s.Plans {
		lines = append(lines, "• "+esc(p.label()))
	}
	b.sendKeyboard(ctx, chatID, strings.Join(lines, "\n"), b.shopPlanButtons("u:buy:"))
}

func (b *bot) askCustomName(ctx context.Context, chatID int64, from *fromUser, topic, planID, extra string) {
	b.pend.set(chatID, &pending{kind: "cu.name", chatID: chatID, data: map[string]string{
		"topic": topic, "plan": planID, "extra": extra,
	}})
	b.sendKeyboard(ctx, chatID, b.tr(
		"✏️ اگر اسم دلخواه می‌خواهید همان را بفرستید؛ وگرنه رد شوید.",
		"✏️ Send a custom name if you want one; otherwise skip."),
		[][]button{{{Text: b.tr("⏭ بدون اسم دلخواه", "⏭ No custom name"), Data: "u:nm:" + topic + ":" + planID + ":" + extra}}})
}

func (b *bot) finishCustomerOrder(ctx context.Context, chatID int64, from *fromUser, topic, planID, extra, name string) {
	s := b.loadShop()
	detail := ""
	switch topic {
	case "buy", "renew":
		if p := s.plan(planID); p != nil {
			detail = p.label()
		} else if planID != "" {
			detail = planID
		}
		if extra != "" {
			if c := clientByIDForUser(from.ID, extra); c != nil {
				detail = strings.TrimSpace(c.Name + " → " + detail)
			}
		}
	case "wallet":
		if w := s.wallet(planID); w != nil {
			detail = nonempty(w.Label, w.Amount)
		} else {
			detail = planID
		}
	}
	if name != "" {
		detail = strings.TrimSpace(detail + "\n" + b.tr("اسم دلخواه: ", "Custom name: ") + name)
	}
	b.notifyAdmins(ctx, b.customerNotice(from, topic, detail))
	b.sendMenu(ctx, chatID, b.tr(
		"✅ درخواست شما به پشتیبانی رسید.",
		"✅ Your request was sent to support."), b.customerMenu(from.ID))
}

func clientByIDForUser(from int64, id string) *model.Client {
	want, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		return nil
	}
	for _, c := range boundClients(from) {
		if uint64(c.Id) == want {
			c := c
			return &c
		}
	}
	return nil
}

func (b *bot) customerShopCallback(ctx context.Context, cbID string, chatID, msgID int64, from *fromUser, parts []string) {
	fu := from
	if fu == nil {
		fu = &fromUser{}
	}
	b.answer(ctx, cbID, "")
	switch parts[1] {
	case "buy":
		if len(parts) < 3 {
			return
		}
		b.askCustomName(ctx, chatID, fu, "buy", parts[2], "")
	case "rb":
		if len(parts) < 4 {
			return
		}
		b.askCustomName(ctx, chatID, fu, "renew", parts[3], parts[2])
	case "wal":
		if len(parts) < 3 {
			return
		}
		b.finishCustomerOrder(ctx, chatID, fu, "wallet", parts[2], "", "")
	case "nm":
		if len(parts) < 3 {
			return
		}
		topic, planID, extra := parts[2], "", ""
		if len(parts) > 3 {
			planID = parts[3]
		}
		if len(parts) > 4 {
			extra = parts[4]
		}
		b.pend.clear(chatID)
		b.finishCustomerOrder(ctx, chatID, fu, topic, planID, extra, "")
	case "ask":
		topic := "buy"
		if len(parts) > 2 {
			topic = parts[2]
		}
		b.finishCustomerOrder(ctx, chatID, fu, topic, "", "", "")
	case "ren":
		if len(parts) < 3 {
			return
		}
		b.customerBuy(ctx, chatID, fu, parts[2])
	}
}

func (b *bot) customerRenewPick(ctx context.Context, chatID int64, from *fromUser) {
	bound := boundClients(from.ID)
	if len(bound) == 0 {
		b.customerBuy(ctx, chatID, from, "")
		return
	}
	var rows [][]button
	for _, c := range bound {
		rows = append(rows, []button{{Text: c.Name, Data: "u:ren:" + strconv.FormatUint(uint64(c.Id), 10)}})
	}
	b.sendKeyboard(ctx, chatID, b.tr(
		"♻️ سرویسی که تمدید می‌شود را انتخاب کنید.",
		"♻️ Pick the service to renew."), rows)
}
