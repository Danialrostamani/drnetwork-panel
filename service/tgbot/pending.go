package tgbot

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// reply refreshes the panel message the prompt came from, or sends a new one
// when that message is gone.
func (b *bot) reply(ctx context.Context, chatID, msgID int64, text string, kb [][]button) {
	if msgID != 0 {
		b.edit(ctx, chatID, msgID, text, kb)
		return
	}
	b.sendKeyboard(ctx, chatID, text, kb)
}

// handlePending consumes an administrator's typed answer. On a bad answer the
// prompt stays open so it can be retried; on success the screen is refreshed.
func (b *bot) handlePending(ctx context.Context, chatID, userMsgID int64, text string, p *pending) {
	b.deleteMessage(ctx, chatID, userMsgID)
	if !b.pendingAllowed(p) {
		// The prompt is not (or no longer) the administrator's to answer: a
		// group-limited one only answers client prompts, and rights can be
		// taken away between the question and the answer.
		b.pend.clear(chatID)
		return
	}
	retry := func(err error) {
		b.pend.set(chatID, p)
		b.reply(ctx, chatID, p.msgID, b.t("failed", esc(b.errText(err)))+"\n\n"+b.tr("دوباره بفرستید یا انصراف دهید.", "Send it again or cancel."), [][]button{b.cancelRow()})
	}
	finish := func(text string, kb [][]button) {
		b.pend.clear(chatID)
		b.reply(ctx, chatID, p.msgID, text, kb)
	}
	showClient := func(id uint) {
		if c := b.clientByID(id); c != nil {
			t, kb := b.card(*c)
			finish(b.t("done")+"\n\n"+t, kb)
			return
		}
		t, kb := b.clientsScreen("a", 0)
		finish(t, kb)
	}
	switch {
	case p.kind == "cl.newj":
		name, err := b.createClientFromJSON(text)
		if err != nil {
			retry(err)
			return
		}
		if c := b.findClientByName(name); c != nil {
			t, kb := b.card(*c)
			finish(b.t("created", esc(c.Name))+"\n\n"+t, kb)
			return
		}
		t, kb := b.clientsScreen("a", 0)
		finish(t, kb)
	case strings.HasPrefix(p.kind, "bk."):
		b.bulkPending(ctx, chatID, p, text, retry, finish)
	case strings.HasPrefix(p.kind, "cl."):
		field := strings.TrimPrefix(p.kind, "cl.")
		if field == "search" {
			t, kb := b.clientsView(strings.TrimSpace(text))
			finish(t, append(kb, []button{{Text: b.tr("⬅️ کلاینت‌ها", "⬅️ Clients"), Data: "c:ls:a:0"}}))
			return
		}
		if err := b.applyClientAnswer(field, p.id, text); err != nil {
			retry(err)
			return
		}
		showClient(p.id)
	case p.kind == "wiz":
		b.wizardStep(ctx, chatID, p, strings.TrimSpace(text), false)
	case p.kind == "in.port":
		if err := b.objPortEdit(p.id, text); err != nil {
			retry(err)
			return
		}
		t, kb := b.objCard(kindByCode("in"), p.id)
		finish(b.t("done")+"\n\n"+t, kb)
	case p.kind == "obj.edit" || p.kind == "obj.new":
		k := kindByCode(p.key)
		m, err := parseObjectInput(k, text)
		if err != nil {
			retry(err)
			return
		}
		act := "new"
		if p.kind == "obj.edit" {
			act = "edit"
			m["id"] = json.Number(strconv.FormatUint(uint64(p.id), 10))
		} else {
			delete(m, "id")
		}
		if err := b.saveObj(k, act, m); err != nil {
			retry(err)
			return
		}
		if act == "edit" {
			t, kb := b.objCard(k, p.id)
			finish(b.t("done")+"\n\n"+t, kb)
			return
		}
		if items, err := b.objItems(k); err == nil {
			name := scalar(m["tag"])
			if name == "" {
				name = scalar(m["name"])
			}
			for _, it := range items {
				if it.Tag == name {
					t, kb := b.objCard(k, it.ID)
					finish(b.t("done")+"\n\n"+t, kb)
					return
				}
			}
		}
		t, kb := b.objListScreen(k, 0)
		finish(b.t("done")+"\n\n"+t, kb)
	case strings.HasPrefix(p.kind, "cfg."):
		b.cfgPending(ctx, chatID, p, text, retry, finish)
	case strings.HasPrefix(p.kind, "ad."):
		b.adminPending(ctx, chatID, p, text, retry, finish)
	case p.kind == "set":
		v := strings.TrimSpace(text)
		if v == "-" {
			v = ""
		}
		if err := b.saveSetting(p.key, v); err != nil {
			retry(err)
			return
		}
		if g := groupOfKey(p.key); g != nil {
			t, kb := b.settingGroupScreen(g)
			finish(b.t("done")+"\n\n"+t, kb)
		}
	default:
		b.pend.clear(chatID)
	}
}

func (b *bot) cfgPending(ctx context.Context, chatID int64, p *pending, text string, retry func(error), finish func(string, [][]button)) {
	switch p.kind {
	case "cfg.opt":
		v, err := decodeJSONValue(text)
		m, ok := v.(map[string]interface{})
		if err == nil && !ok {
			err = fmt.Errorf("JSON object expected")
		}
		if err == nil {
			err = b.saveOption(p.key, m)
		}
		if err != nil {
			retry(err)
			return
		}
		if p.key == "route" || p.key == "dns" {
			code := "rl"
			if p.key == "dns" {
				code = "dn"
			}
			t, kb := b.cfgListScreen(cfgSecByCode(code), 0)
			finish(b.t("done")+"\n\n"+t, kb)
			return
		}
		t, kb := b.basicsScreen()
		finish(b.t("done")+"\n\n"+t, kb)
	case "cfg.new", "cfg.edit":
		sec := cfgSecByCode(p.key)
		v, err := decodeJSONValue(text)
		if err != nil {
			retry(err)
			return
		}
		cfg, err := b.loadConfigMap()
		if err != nil {
			retry(err)
			return
		}
		items := getPath(cfg, sec.path)
		idx := int(p.id)
		if p.kind == "cfg.new" {
			items = append(items, v)
			idx = len(items) - 1
		} else if idx < len(items) {
			items[idx] = v
		}
		setPath(cfg, sec.path, items)
		if err := b.saveConfigMap(cfg); err != nil {
			retry(err)
			return
		}
		t, kb := b.cfgItemScreen(sec, idx)
		finish(b.t("done")+"\n\n"+t, kb)
	}
}

// ---- new-client wizard ----

// stepNo numbers a step of the new-client wizard. A limited administrator has
// no group step (every client goes to their group), so theirs has one step less.
func (b *bot) stepNo(n int) string {
	total := 5
	if b.scope != "" {
		total = 4
		if n > 2 {
			n--
		}
	}
	return fmt.Sprintf("%d/%d", n, total)
}

// scopedPendingKind tells which prompts a group-limited administrator may
// answer: the client ones. Settings, objects and the sing-box config are not
// theirs.
func scopedPendingKind(kind string) bool {
	return strings.HasPrefix(kind, "cl.") || strings.HasPrefix(kind, "bk.") || kind == "wiz"
}

func (b *bot) wizardPrompt(p *pending) (string, [][]button) {
	btn := func(label, step, val string) button { return button{Text: label, Data: "w:" + step + ":" + val} }
	switch p.key {
	case "vol":
		return b.header("➕", b.stepNo(3)) + "\n" + b.tr("حجم کل (GB) را انتخاب کنید یا عدد بفرستید.", "Pick the total volume (GB) or send a number."), [][]button{
			{btn("10", "vol", "10"), btn("20", "vol", "20"), btn("30", "vol", "30")},
			{btn("50", "vol", "50"), btn("100", "vol", "100"), btn("200", "vol", "200")},
			{btn("300", "vol", "300"), btn("∞", "vol", "0"), btn(b.tr("✏️ دلخواه", "✏️ Custom"), "vol", "#c")},
			b.cancelRow()}
	case "days":
		return b.header("➕", b.stepNo(4)) + "\n" + b.tr("مدت اعتبار (روز) را انتخاب کنید یا عدد بفرستید.", "Pick the validity (days) or send a number."), [][]button{
			{btn("7", "days", "7"), btn("30", "days", "30"), btn("60", "days", "60")},
			{btn("90", "days", "90"), btn("180", "days", "180"), btn("∞", "days", "0")},
			b.cancelRow()}
	case "grp":
		text := b.header("➕", b.tr("گروه — ", "Group — ")+b.stepNo(2)) + "\n" + b.tr("گروه کلاینت را از دکمه‌ها انتخاب کنید، یا نام یک گروه جدید را بفرستید تا ساخته شود.", "Pick a group below, or send a new group name to create it.")
		if len(b.clientGroups()) == 0 {
			text += "\n" + b.tr("(هنوز گروهی وجود ندارد.)", "(There are no groups yet.)")
		}
		return text, b.groupPicker("w:grp:")
	case "name":
		if p.data["rnd"] == "" {
			p.data["rnd"] = b.randomClientName()
		}
		return b.header("➕", b.tr("کلاینت جدید — ", "New client — ")+b.stepNo(1)) + "\n" +
				b.tr("نام پیشنهادی (روی آن بزنید تا کپی شود):", "Suggested name (tap to copy):") + " <code>" + esc(p.data["rnd"]) + "</code>\n" +
				b.tr("• «استفاده از این نام» را بزنید\n• یا نام دلخواه را بفرستید\n• یا با + شروع کنید تا به انتهای نام پیشنهادی اضافه شود؛ مثلاً <code>+_ali</code>", "• Tap “Use this name”\n• or send your own name\n• or start with + to append to the suggestion, e.g. <code>+_ali</code>"), [][]button{
				{btn(b.tr("✅ استفاده از این نام", "✅ Use this name"), "name", "#ok"), btn(b.tr("🎲 نام دیگر", "🎲 Another"), "name", "#new")},
				b.cancelRow()}
	default:
		return b.header("➕", b.stepNo(5)) + "\n" + b.tr("محدودیت تعداد IP همزمان؟", "Concurrent IP limit?"), [][]button{
			{btn("∞", "ip", "0"), btn("1", "ip", "1"), btn("2", "ip", "2"), btn("3", "ip", "3"), btn("5", "ip", "5")},
			b.cancelRow()}
	}
}

// wizardStep consumes one answer of the new-client wizard. fromButton tells a
// pressed button from typed text (a typed group name may be any text).
func (b *bot) wizardStep(ctx context.Context, chatID int64, p *pending, value string, fromButton bool) {
	fail := func(err error) {
		b.pend.set(chatID, p)
		b.reply(ctx, chatID, p.msgID, b.t("failed", esc(b.errText(err)))+"\n\n"+b.tr("دوباره بفرستید یا انصراف دهید.", "Send it again or cancel."), [][]button{b.cancelRow()})
	}
	// render shows a prompt of the current step under the client's name (and
	// group, once chosen) and keeps the wizard pending.
	render := func(t string, kb [][]button) {
		b.pend.set(chatID, p)
		head := "👤 <b>" + esc(p.data["name"]) + "</b>"
		if g := p.data["grp"]; g != "" {
			head += "  🏷 " + esc(g)
		}
		b.reply(ctx, chatID, p.msgID, head+"\n"+t, kb)
	}
	advance := func(next string) {
		p.key = next
		t, kb := b.wizardPrompt(p)
		render(t, kb)
	}
	switch p.key {
	case "name":
		switch {
		case value == "#new":
			p.data["rnd"] = b.randomClientName()
			b.pend.set(chatID, p)
			t, kb := b.wizardPrompt(p)
			b.reply(ctx, chatID, p.msgID, t, kb)
			return
		case value == "#ok":
			value = p.data["rnd"]
		case strings.HasPrefix(value, "+"):
			value = p.data["rnd"] + strings.TrimPrefix(value, "+")
		}
		if !clientNameRe.MatchString(value) {
			fail(fmt.Errorf("bad name"))
			return
		}
		if rawFindClientByName(value) != nil {
			fail(fmt.Errorf("%s", b.tr("این نام قبلاً استفاده شده است.", "That name is already in use.")))
			return
		}
		p.data["name"] = value
		if b.scope != "" {
			// Every client of a limited administrator goes to their group.
			p.data["grp"] = b.scopeGroup()
			advance("vol")
			return
		}
		advance("grp")
	case "grp":
		if fromButton && value == "#new" {
			// The "New group" button: ask for the name; the typed text is
			// handled below like any typed group.
			t, kb := b.newGroupPrompt("w:grp:#b")
			render(t, kb)
			return
		}
		if fromButton && value == "#b" {
			t, kb := b.wizardPrompt(p)
			render(t, kb)
			return
		}
		group := ""
		switch {
		case fromButton && value == "-":
		case fromButton:
			groups := b.clientGroups()
			idx, err := strconv.Atoi(value)
			if err != nil || idx < 0 || idx >= len(groups) {
				fail(fmt.Errorf("%s", b.t("notFound")))
				return
			}
			group = groups[idx]
		default:
			g, err := b.normalizeGroup(value)
			if err != nil {
				fail(err)
				return
			}
			group = g
		}
		p.data["grp"] = group
		advance("vol")
	case "vol":
		if fromButton && value == "#c" {
			// The "Custom" button: ask for the number; typed text is handled below.
			render(b.header("➕", b.stepNo(3))+"\n✏️ "+b.tr("حجم را به گیگابایت بفرستید؛ مثلاً 75 یا 2.5 (۰ = نامحدود).", "Send the volume in GB, e.g. 75 or 2.5 (0 = unlimited)."),
				[][]button{{{Text: b.tr("⬅️ حجم‌های آماده", "⬅️ Presets"), Data: "w:vol:#b"}}, b.cancelRow()})
			return
		}
		if fromButton && value == "#b" {
			t, kb := b.wizardPrompt(p)
			render(t, kb)
			return
		}
		if _, ok := parseFloatArg(value); !ok {
			fail(fmt.Errorf("%s", b.t("badNumber")))
			return
		}
		p.data["vol"] = value
		advance("days")
	case "days":
		if _, ok := parseIntArg(value); !ok {
			fail(fmt.Errorf("%s", b.t("badNumber")))
			return
		}
		p.data["days"] = value
		advance("ip")
	case "ip":
		limit, ok := parseIntArg(value)
		if !ok {
			fail(fmt.Errorf("%s", b.t("badNumber")))
			return
		}
		gb, _ := parseFloatArg(p.data["vol"])
		days, _ := parseIntArg(p.data["days"])
		if err := b.createClient(p.data["name"], p.data["grp"], int64(gb*float64(gib)), days, limit); err != nil {
			fail(err)
			return
		}
		b.pend.clear(chatID)
		if c := b.findClientByName(p.data["name"]); c != nil {
			t, kb := b.card(*c)
			b.reply(ctx, chatID, p.msgID, b.t("created", esc(c.Name))+"\n\n"+t, kb)
		}
	}
}

func (b *bot) wizardCallback(ctx context.Context, cbID string, chatID int64, parts []string) {
	p := b.pend.get(chatID)
	if p == nil || p.kind != "wiz" || len(parts) < 3 || p.key != parts[1] {
		b.answer(ctx, cbID, b.tr("این مرحله منقضی شده است.", "This step has expired."))
		return
	}
	b.answer(ctx, cbID, "")
	b.wizardStep(ctx, chatID, p, parts[2], true)
}
