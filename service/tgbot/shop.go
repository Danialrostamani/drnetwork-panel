package tgbot

import (
	"context"
	"encoding/json"
	"errors"
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
	// SalesGroup owns this desk's sales; it never limits administrative access.
	SalesGroup string       `json:"salesGroup,omitempty"`
	PayCard    string       `json:"payCard,omitempty"`
	PayWallet  string       `json:"payWallet,omitempty"`
	PayHint    string       `json:"payHint,omitempty"`
	Plans      []shopPlan   `json:"plans,omitempty"`
	Wallets    []shopWallet `json:"wallets,omitempty"`
	Trial      *bool        `json:"trial,omitempty"`
	TrialGB    int          `json:"trialGB,omitempty"`
	TrialHours int          `json:"trialHours,omitempty"`
	TrialGroup string       `json:"trialGroup,omitempty"`
	// Support and Channel are the administrator's own, shown on the
	// subscription page of their customers.
	Support string `json:"support,omitempty"`
	Channel string `json:"channel,omitempty"`
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
	"p:buy", "p:renew", "p:my", "p:link", "p:wallet", "p:trial", "p:support", "p:guide",
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
	// A seller the owner suspended sells nothing: their customers go to the
	// owner meanwhile.
	usable := func(id int64) bool { return a.isMember(id) && (a.isOwner(id) || !adminMeta(id).Suspended) }
	if c, ok := customerRow(customerID); ok && c.AdminId != 0 && usable(c.AdminId) {
		return c.AdminId
	}
	stored := loadBotRef(customerID)
	if stored != nil {
		s := b.loadShopRaw()
		if r := s.findRef(stored.Word); r != nil && r.Admin != 0 && usable(r.Admin) {
			return r.Admin
		}
		g := strings.TrimSpace(stored.Group)
		if g != "" {
			for _, id := range a.order {
				if strings.EqualFold(b.sellerGroup(id), g) && usable(id) {
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
	if (b.globalShop && b.canManageAdmins()) || b.self <= 0 {
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
	if b.globalShop && !b.canManageAdmins() {
		return errNotOwner
	}
	cur := b.loadShopRaw()
	if b.self > 0 && b.isAdmin(b.self) && !b.owner && !b.globalShop {
		if cur.Desks == nil {
			cur.Desks = map[string]shopDesk{}
		}
		key := strconv.FormatInt(b.self, 10)
		d := deskFrom(s)
		// The bot does not edit these; the panel does.
		d.Support, d.Channel, d.SalesGroup = cur.Desks[key].Support, cur.Desks[key].Channel, cur.Desks[key].SalesGroup
		cur.Desks[key] = d
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

// setSalesGroup changes business ownership only. It does not write the access
// scopes or permissions. A full partner can set their own group from Telegram.
func (b *bot) setSalesGroup(group string) error {
	if b.owner || b.globalShop || !b.canManageAdmins() {
		return errNotOwner
	}
	group = strings.TrimSpace(group)
	if group == "-" {
		group = ""
	}
	if group != "" && !scopeGroupUsable(group) {
		return errBadGroup
	}
	if b.configService == nil {
		return errors.New("the panel is not ready to save settings")
	}
	s := b.loadShopRaw()
	if s.Desks == nil {
		s.Desks = map[string]shopDesk{}
	}
	key := strconv.FormatInt(b.self, 10)
	d := s.Desks[key]
	d.SalesGroup = group
	s.Desks[key] = d
	raw, err := json.Marshal(s)
	if err != nil {
		return err
	}
	values, err := json.Marshal(map[string]string{"tgBotShop": string(raw)})
	if err != nil {
		return err
	}
	actor := b.actor()
	if b.deskBy > 0 {
		actor = service.TelegramActor + ":" + strconv.FormatInt(b.deskBy, 10)
	}
	_, err = b.configService.Save("settings", "", values, "", actor, b.host())
	return err
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
	list := s.walletList()
	for i := range list {
		if list[i].ID == id {
			return &list[i]
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
			"🔐 هنوز پلنی در فروشگاه نیست. می‌توانید درخواستتان را برای پشتیبانی بنویسید.",
			"🔐 No plan is in the shop yet. You can write your request to support."),
			[][]button{{{Text: b.tr("📩 پیام به پشتیبانی", "📩 Message support"), Data: "u:ask:buy"}}})
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
	var rows [][]button
	for _, w := range s.walletList() {
		label := w.Label
		if label == "" {
			label = w.Amount
		}
		rows = append(rows, []button{{Text: label, Data: "u:wal:" + w.ID}})
	}
	bal := esc(formatMoney(walletBalance(from.ID)))
	b.sendKeyboard(ctx, chatID, b.tr(
		"🏦 <b>کیف پول</b>\nموجودی: <b>"+bal+"</b>\nمبلغ شارژ را انتخاب کنید. بعد از تأیید رسید، به موجودی اضافه می‌شود و می‌توانید پلن را از کیف پول پرداخت کنید.",
		"🏦 <b>Wallet</b>\nBalance: <b>"+bal+"</b>\nPick the amount to add. It is added once the receipt is approved, and plans can then be paid from the wallet."), rows)
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
		// Old wallet screens offered a request without a receipt; a top-up
		// needs one, so the button now asks for it.
		if len(parts) < 3 {
			return
		}
		id, _ := strconv.ParseUint(parts[2], 10, 32)
		b.askReceipt(ctx, chatID, fu, uint(id))
	case "wp":
		if len(parts) < 3 {
			return
		}
		id, _ := strconv.ParseUint(parts[2], 10, 32)
		b.payFromWallet(ctx, chatID, fu, uint(id))
	case "cx":
		if len(parts) < 3 {
			return
		}
		id, _ := strconv.ParseUint(parts[2], 10, 32)
		b.customerCancel(ctx, chatID, fu, uint(id))
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
		// A request without a plan is a message to support, not an order:
		// approving an order without a plan would have nothing to deliver.
		topic := "buy"
		if len(parts) > 2 {
			topic = parts[2]
		}
		b.customerAsk(ctx, chatID, fu, topic, b.tr(
			"📩 درخواستتان را بنویسید (مثلاً حجم و مدت دلخواه)؛ برای پشتیبانی ارسال می‌شود.",
			"📩 Write your request (for example the data and duration you want); it is sent to support."))
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
