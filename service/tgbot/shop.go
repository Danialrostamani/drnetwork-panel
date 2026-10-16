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
	WelcomeFa  string         `json:"welcomeFa,omitempty"`
	WelcomeEn  string         `json:"welcomeEn,omitempty"`
	GuideFa    string         `json:"guideFa,omitempty"`
	GuideEn    string         `json:"guideEn,omitempty"`
	Trial      *bool          `json:"trial,omitempty"`
	TrialGB    int            `json:"trialGB,omitempty"`
	TrialHours int            `json:"trialHours,omitempty"`
	TrialGroup string         `json:"trialGroup,omitempty"`
	PayCard    string         `json:"payCard,omitempty"`
	PayWallet  string         `json:"payWallet,omitempty"`
	PayHint    string         `json:"payHint,omitempty"`
	Plans      []shopPlan     `json:"plans,omitempty"`
	Wallets    []shopWallet   `json:"wallets,omitempty"`
	Buttons    []shopMenuBtn  `json:"buttons,omitempty"`
	Refs       []shopReferrer `json:"refs,omitempty"`
	// Desks is each administrator's own plans and payment details, keyed by
	// their Telegram ID. The owner still edits the fields above as the default.
	Desks map[string]shopDesk `json:"desks,omitempty"`
}

// shopDesk is one administrator's plans and payment details.
type shopDesk struct {
	PayCard    string       `json:"payCard,omitempty"`
	PayWallet  string       `json:"payWallet,omitempty"`
	PayHint    string       `json:"payHint,omitempty"`
	Plans      []shopPlan   `json:"plans,omitempty"`
	Wallets    []shopWallet `json:"wallets,omitempty"`
	Trial      *bool        `json:"trial,omitempty"`
	TrialGB    int          `json:"trialGB,omitempty"`
	TrialHours int          `json:"trialHours,omitempty"`
	TrialGroup string       `json:"trialGroup,omitempty"`
}

func (s shopConfig) hasPayment() bool {
	return strings.TrimSpace(s.PayCard) != "" || strings.TrimSpace(s.PayWallet) != "" || strings.TrimSpace(s.PayHint) != ""
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
	Group string `json:"group,omitempty"`
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
	"p:buy", "p:renew", "p:my", "p:link", "p:wallet", "p:trial", "p:tariff", "p:support", "p:guide",
}

func (b *bot) loadShopRaw() shopConfig {
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

func (s shopConfig) forAdmin(id int64) shopConfig {
	out := s
	out.Desks = nil
	if id == 0 {
		return out
	}
	d, ok := s.Desks[strconv.FormatInt(id, 10)]
	if !ok {
		return out
	}
	if strings.TrimSpace(d.PayCard) != "" {
		out.PayCard = d.PayCard
	}
	if strings.TrimSpace(d.PayWallet) != "" {
		out.PayWallet = d.PayWallet
	}
	if strings.TrimSpace(d.PayHint) != "" {
		out.PayHint = d.PayHint
	}
	if len(d.Plans) > 0 {
		out.Plans = d.Plans
	}
	if len(d.Wallets) > 0 {
		out.Wallets = d.Wallets
	}
	if d.Trial != nil {
		out.Trial = d.Trial
	}
	if d.TrialGB != 0 {
		out.TrialGB = d.TrialGB
	}
	if d.TrialHours != 0 {
		out.TrialHours = d.TrialHours
	}
	if strings.TrimSpace(d.TrialGroup) != "" {
		out.TrialGroup = d.TrialGroup
	}
	return out
}

func deskFrom(s shopConfig) shopDesk {
	return shopDesk{
		PayCard: s.PayCard, PayWallet: s.PayWallet, PayHint: s.PayHint,
		Plans: s.Plans, Wallets: s.Wallets,
		Trial: s.Trial, TrialGB: s.TrialGB, TrialHours: s.TrialHours, TrialGroup: s.TrialGroup,
	}
}

func (b *bot) customerAdminID(customerID int64) int64 {
	a := b.access()
	stored := loadBotRef(customerID)
	if stored != nil {
		s := b.loadShopRaw()
		if r := s.findRef(stored.Word); r != nil && r.Admin != 0 {
			return r.Admin
		}
		g := strings.TrimSpace(stored.Group)
		if g != "" {
			for _, id := range a.order {
				if strings.EqualFold(strings.TrimSpace(a.roleOf(id).group), g) {
					return id
				}
			}
		}
	}
	if a.owner != 0 {
		return a.owner
	}
	return 0
}

func (b *bot) loadShop() shopConfig {
	s := b.loadShopRaw()
	if b.self <= 0 {
		return s
	}
	if b.isAdmin(b.self) && !b.owner {
		return s.forAdmin(b.self)
	}
	if !b.isAdmin(b.self) {
		return s.forAdmin(b.customerAdminID(b.self))
	}
	return s
}

func (b *bot) saveShop(s shopConfig) error {
	cur := b.loadShopRaw()
	if b.self > 0 && b.isAdmin(b.self) && !b.owner {
		if cur.Desks == nil {
			cur.Desks = map[string]shopDesk{}
		}
		cur.Desks[strconv.FormatInt(b.self, 10)] = deskFrom(s)
		s = cur
	} else {
		s.Desks = cur.Desks
	}
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
	if renewID == "" && b.customerNeedsRef(from.ID) {
		b.customerAskRef(ctx, chatID, from, "buy")
		return
	}
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
		b.placeOrder(ctx, chatID, fu, "buy", parts[2], "")
	case "rb":
		if len(parts) < 4 {
			return
		}
		b.placeOrder(ctx, chatID, fu, "renew", parts[3], parts[2])
	case "wal":
		if len(parts) < 3 {
			return
		}
		b.placeOrder(ctx, chatID, fu, "wallet", parts[2], "")
	case "pay":
		if len(parts) < 3 {
			return
		}
		id, _ := strconv.ParseUint(parts[2], 10, 32)
		b.askReceipt(ctx, chatID, fu, uint(id))
	case "skip":
		if len(parts) < 3 {
			return
		}
		id, _ := strconv.ParseUint(parts[2], 10, 32)
		b.skipReceipt(ctx, chatID, fu, uint(id))
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
		b.placeOrder(ctx, chatID, fu, topic, planID, extra)
	case "ask":
		topic := "buy"
		if len(parts) > 2 {
			topic = parts[2]
		}
		b.placeOrder(ctx, chatID, fu, topic, "", "")
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
