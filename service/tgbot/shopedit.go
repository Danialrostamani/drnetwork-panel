package tgbot

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

func (b *bot) shopEditHome() (string, [][]button) {
	s := b.loadShop()
	title := b.tr("فروشگاه مشتری", "Customer shop")
	hint := b.tr("هر چیزی که مشتری می‌بیند از اینجا عوض می‌شود. مراحل مشتری دکمه‌ای است.",
		"Everything the customer sees is edited here. Their steps are buttons.")
	if !b.owner && b.self > 0 {
		title = b.tr("فروشگاه شما", "Your shop")
		hint = b.tr("پلن‌ها، قیمت و شماره کارت همین ادمین. مشتری‌های گروه شما همین را می‌بینند.",
			"Plans, price and card number for this admin. Customers in your group see this.")
	}
	lines := []string{"🛍 <b>" + title + "</b>", hint}
	if b.deskBy != 0 {
		lines = append(lines, "👑 "+b.tr("ویرایش فروشگاه این ادمین توسط مالک: ", "The owner is editing this admin's shop: ")+fmt.Sprintf("<code>%d</code>", b.self))
	}
	lines = append(lines, fmt.Sprintf("\n%s: %d · %s: %d · %s: %d · %s: %s",
		b.tr("پلن", "Plans"), len(s.Plans),
		b.tr("مبالغ کیف پول", "Wallet amounts"), len(s.Wallets),
		b.tr("معرف", "Introducers"), len(s.Refs),
		b.tr("اکانت تست", "Trial"), onOff(s.trialOn())))
	kb := [][]button{
		{{Text: b.tr("📦 پلن‌ها", "📦 Plans"), Data: "k:plans"}, {Text: b.tr("🏦 کیف پول", "🏦 Wallet"), Data: "k:wals"}},
	}
	if b.owner || b.self == 0 {
		kb = append(kb,
			[]button{{Text: b.tr("📝 متن‌ها", "📝 Texts"), Data: "k:txts"}, {Text: b.tr("🔘 دکمه‌ها", "🔘 Buttons"), Data: "k:btns"}},
			[]button{{Text: b.tr("👥 معرف‌ها", "👥 Introducers"), Data: "k:refs"}},
		)
	}
	kb = append(kb,
		[]button{{Text: b.tr("🔑 اکانت تست", "🔑 Trial") + " " + onOff(s.trialOn()), Data: "k:trial"}},
		[]button{{Text: b.tr("💳 پرداخت", "💳 Payment"), Data: "k:pay"}},
		[]button{{Text: b.tr("📋 سفارش‌ها", "📋 Orders"), Data: "k:ordsP"}, {Text: b.tr("📊 گزارش فروش", "📊 Sales report"), Data: "k:sales"}},
		[]button{{Text: b.tr("👁 نمای مشتری", "👁 Customer view"), Data: "k:shop"}},
	)
	if b.deskBy != 0 {
		kb = append(kb, []button{{Text: b.tr("🚪 خروج از فروشگاه این ادمین", "🚪 Leave this admin's shop"), Data: "q:dx"}})
	}
	return strings.Join(lines, "\n"), kb
}

func (b *bot) shopPayScreen() (string, [][]button) {
	s := b.loadShop()
	card, wal, hint := s.PayCard, s.PayWallet, s.PayHint
	if card == "" {
		card = "—"
	}
	if wal == "" {
		wal = "—"
	}
	if hint == "" {
		hint = "—"
	}
	text := "💳 <b>" + b.tr("پرداخت", "Payment") + "</b>\n" +
		b.tr("کارت", "Card") + ": " + esc(card) + "\n" +
		b.tr("کیف پول", "Wallet") + ": " + esc(wal) + "\n" +
		b.tr("راهنما", "Hint") + ": " + esc(hint) + "\n" +
		b.tr("تست", "Trial") + ": " + strconv.Itoa(s.TrialGB) + "GB / " + strconv.Itoa(s.TrialHours) + "h"
	kb := [][]button{
		{{Text: b.tr("✏️ کارت", "✏️ Card"), Data: "k:pcard"}, {Text: b.tr("✏️ کیف پول", "✏️ Wallet"), Data: "k:pwal"}},
		{{Text: b.tr("✏️ راهنما", "✏️ Hint"), Data: "k:phint"}},
		{{Text: b.tr("📦 حجم تست", "📦 Trial GB"), Data: "k:tgb"}, {Text: b.tr("⏱ ساعت تست", "⏱ Trial hours"), Data: "k:thrs"}},
		{{Text: b.tr("⬅️ فروشگاه", "⬅️ Shop"), Data: "k:edit"}},
	}
	return text, kb
}

func (b *bot) shopPlansScreen() (string, [][]button) {
	s := b.loadShop()
	lines := []string{"📦 <b>" + b.tr("پلن‌ها", "Plans") + "</b>"}
	var kb [][]button
	if len(s.Plans) == 0 {
		lines = append(lines, b.tr("هنوز پلنی نیست. با دکمه زیر یکی بسازید.", "No plans yet. Add one with the button below."))
	}
	for _, p := range s.Plans {
		lines = append(lines, "• "+esc(p.label()))
		kb = append(kb, []button{
			{Text: "✏️ " + truncate(p.Name, 18), Data: "k:plan:" + p.ID},
			{Text: "🗑", Data: "k:pdel:" + p.ID},
		})
	}
	kb = append(kb, []button{{Text: b.tr("➕ پلن جدید", "➕ New plan"), Data: "k:pnew"}})
	kb = append(kb, []button{{Text: b.tr("⬅️ فروشگاه", "⬅️ Shop"), Data: "k:edit"}})
	return strings.Join(lines, "\n"), kb
}

func (b *bot) shopPlanScreen(id string) (string, [][]button) {
	s := b.loadShop()
	p := s.plan(id)
	if p == nil {
		return b.shopPlansScreen()
	}
	vol := b.tr("نامحدود", "Unlimited")
	if p.GB > 0 {
		vol = strconv.Itoa(p.GB) + " GB"
	}
	days := b.tr("بدون انقضا", "No expiry")
	if p.Days > 0 {
		days = strconv.Itoa(p.Days) + " " + b.tr("روز", "days")
	}
	text := fmt.Sprintf("📦 <b>%s</b>\n%s: %s\n%s: %s\n%s: %s",
		esc(p.Name),
		b.tr("مدت", "Duration"), days,
		b.tr("حجم", "Volume"), vol,
		b.tr("قیمت", "Price"), nonempty(p.Price, "—"))
	if !b.owner && b.self > 0 {
		if min := b.sellerMinPrice(b.self, p.GB); min > 0 {
			text += "\n" + b.tr("حداقل قیمت مجاز: ", "Lowest allowed price: ") + formatMoney(min)
			if n, ok := parseMoney(p.Price); ok && n < min {
				text += "\n⚠️ " + b.tr("قیمت این پلن کمتر از حداقل است؛ مشتری نمی‌تواند آن را بخرد.", "This plan is priced under the minimum; customers cannot buy it.")
			}
		}
	}
	kb := [][]button{
		{{Text: b.tr("✏️ نام", "✏️ Name"), Data: "k:pn:" + id}, {Text: b.tr("📅 مدت", "📅 Days"), Data: "k:pd:" + id}},
		{{Text: b.tr("📦 حجم", "📦 Volume"), Data: "k:pg:" + id}, {Text: b.tr("💵 قیمت", "💵 Price"), Data: "k:pp:" + id}},
		{{Text: b.tr("⬅️ پلن‌ها", "⬅️ Plans"), Data: "k:plans"}},
	}
	return text, kb
}

func (b *bot) shopWalletsScreen() (string, [][]button) {
	s := b.loadShop()
	lines := []string{"🏦 <b>" + b.tr("مبالغ شارژ", "Top-up amounts") + "</b>"}
	var kb [][]button
	if len(s.Wallets) == 0 {
		lines = append(lines, b.tr("اگر خالی باشد مشتری ۴ مبلغ پیش‌فرض می‌بیند.", "If this is empty the customer sees 4 default amounts."))
	}
	for _, w := range s.Wallets {
		kb = append(kb, []button{
			{Text: nonempty(w.Label, w.Amount), Data: "k:wal:" + w.ID},
			{Text: "🗑", Data: "k:wdel:" + w.ID},
		})
	}
	kb = append(kb, []button{{Text: b.tr("➕ مبلغ", "➕ Amount"), Data: "k:wnew"}})
	kb = append(kb, []button{{Text: b.tr("⬅️ فروشگاه", "⬅️ Shop"), Data: "k:edit"}})
	return strings.Join(lines, "\n"), kb
}

func (b *bot) shopTextsScreen() (string, [][]button) {
	s := b.loadShop()
	welcome := b.tr("پیش‌فرض", "Default")
	if s.WelcomeFa != "" || s.WelcomeEn != "" {
		welcome = b.tr("سفارشی", "Custom")
	}
	guide := b.tr("پیش‌فرض", "Default")
	if s.GuideFa != "" || s.GuideEn != "" {
		guide = b.tr("سفارشی", "Custom")
	}
	text := "📝 <b>" + b.tr("متن‌ها", "Texts") + "</b>\n" +
		b.tr("خوش‌آمد", "Welcome") + ": " + welcome + "\n" +
		b.tr("آموزش", "Guide") + ": " + guide
	kb := [][]button{
		{{Text: b.tr("✏️ خوش‌آمد", "✏️ Welcome"), Data: "k:tw"}, {Text: b.tr("♻️ پیش‌فرض خوش‌آمد", "♻️ Default welcome"), Data: "k:twx"}},
		{{Text: b.tr("✏️ آموزش", "✏️ Guide"), Data: "k:tg"}, {Text: b.tr("♻️ پیش‌فرض آموزش", "♻️ Default guide"), Data: "k:tgx"}},
		{{Text: b.tr("⬅️ فروشگاه", "⬅️ Shop"), Data: "k:edit"}},
	}
	return text, kb
}

func (b *bot) shopRefsScreen() (string, [][]button) {
	s := b.loadShop()
	lines := []string{"👥 <b>" + b.tr("معرف‌ها", "Introducers") + "</b>",
		b.tr("مشتری جدید تا این کلمه را نفرستد نمی‌تواند بخرد. هر معرف به یک گروه وصل می‌شود.",
			"A new customer cannot buy until they send this word. Each introducer is tied to a group.")}
	var kb [][]button
	if len(s.Refs) == 0 {
		lines = append(lines, b.tr("هنوز معرفی نیست. اگر خالی بماند خرید بدون معرف باز است.",
			"No introducers yet. If this stays empty, buying does not require one."))
	}
	for _, r := range s.Refs {
		g := r.Group
		if g == "" {
			g = "—"
		}
		lines = append(lines, "• "+esc(r.Word)+" → "+esc(g))
		kb = append(kb, []button{
			{Text: "✏️ " + truncate(r.Word, 18), Data: "k:rgrp:" + r.ID},
			{Text: "🗑", Data: "k:rdel:" + r.ID},
		})
	}
	kb = append(kb, []button{{Text: b.tr("➕ معرف جدید", "➕ New introducer"), Data: "k:rnew"}})
	kb = append(kb, []button{{Text: b.tr("⬅️ فروشگاه", "⬅️ Shop"), Data: "k:edit"}})
	return strings.Join(lines, "\n"), kb
}

func (b *bot) shopButtonsScreen() (string, [][]button) {
	s := b.loadShop()
	lines := []string{"🔘 <b>" + b.tr("دکمه‌های مشتری", "Customer buttons") + "</b>",
		b.tr("روشن/خاموش کنید یا اسم دکمه را عوض کنید.", "Toggle them or rename the label.")}
	var kb [][]button
	for _, k := range shopMenuKeys {
		it := s.btn(k)
		label := b.menuLabel(k)
		if it.Fa != "" {
			label = b.tr(it.Fa, nonempty(it.En, it.Fa))
		}
		state := onOff(!it.Off)
		if k == "p:trial" && !s.trialOn() {
			state = onOff(false)
		}
		kb = append(kb, []button{
			{Text: state + " " + label, Data: "k:bt:" + k},
			{Text: "✏️", Data: "k:bn:" + k},
		})
	}
	kb = append(kb, []button{{Text: b.tr("⬅️ فروشگاه", "⬅️ Shop"), Data: "k:edit"}})
	return strings.Join(lines, "\n"), kb
}

func shopDaysButtons(id string) [][]button {
	var rows [][]button
	var row []button
	for _, d := range []int{7, 15, 30, 60, 90, 180, 365} {
		row = append(row, button{Text: strconv.Itoa(d), Data: fmt.Sprintf("k:pds:%s:%d", id, d)})
		if len(row) == 4 {
			rows = append(rows, row)
			row = nil
		}
	}
	if len(row) > 0 {
		rows = append(rows, row)
	}
	rows = append(rows, []button{{Text: "∞", Data: "k:pds:" + id + ":0"}})
	return rows
}

func shopGBButtons(id string) [][]button {
	var rows [][]button
	var row []button
	for _, g := range []int{5, 10, 20, 30, 50, 100, 200, 500} {
		row = append(row, button{Text: strconv.Itoa(g) + "G", Data: fmt.Sprintf("k:pgs:%s:%d", id, g)})
		if len(row) == 4 {
			rows = append(rows, row)
			row = nil
		}
	}
	if len(row) > 0 {
		rows = append(rows, row)
	}
	rows = append(rows, []button{{Text: "∞", Data: "k:pgs:" + id + ":0"}})
	return rows
}

func shopPriceButtons(id string) [][]button {
	var rows [][]button
	var row []button
	for _, p := range []string{"50000", "100000", "150000", "200000", "250000", "350000", "500000", "1000000"} {
		row = append(row, button{Text: p, Data: "k:pps:" + id + ":" + p})
		if len(row) == 2 {
			rows = append(rows, row)
			row = nil
		}
	}
	if len(row) > 0 {
		rows = append(rows, row)
	}
	rows = append(rows, []button{{Text: "✏️", Data: "k:ppa:" + id}})
	return rows
}

func (b *bot) shopEditCallback(ctx context.Context, cbID string, chatID, msgID int64, parts []string) {
	b = b.deskView(chatID)
	show := func(text string, kb [][]button) {
		b.answer(ctx, cbID, "")
		b.edit(ctx, chatID, msgID, text, kb)
	}
	if len(parts) < 2 {
		b.answer(ctx, cbID, "")
		return
	}
	if !b.owner && b.self > 0 {
		switch parts[1] {
		case "txts", "btns", "refs", "rnew", "rgrp", "rdel", "rdelY":
			b.answer(ctx, cbID, b.denied())
			return
		}
	}
	switch parts[1] {
	case "edit":
		show(b.shopEditHome())
	case "ords":
		show(b.orderQueueScreen(false))
	case "ordsP":
		show(b.orderQueueScreen(true))
	case "sales":
		show(b.salesReportScreen())
	case "plans":
		show(b.shopPlansScreen())
	case "wals":
		show(b.shopWalletsScreen())
	case "txts":
		show(b.shopTextsScreen())
	case "btns":
		show(b.shopButtonsScreen())
	case "refs":
		show(b.shopRefsScreen())
	case "rnew":
		b.answer(ctx, cbID, "")
		b.pend.set(chatID, &pending{kind: "shop.rword", msgID: msgID, back: "k:refs", data: map[string]string{}})
		b.edit(ctx, chatID, msgID, b.tr("کلمه معرف را بفرستید. مشتری همین را باید تایپ کند.",
			"Send the introducer word. The customer must type this."), [][]button{b.cancelRow()})
	case "rgrp":
		if len(parts) < 3 {
			return
		}
		b.answer(ctx, cbID, "")
		b.pend.set(chatID, &pending{kind: "shop.rgroup", msgID: msgID, back: "k:refs", data: map[string]string{"id": parts[2]}})
		b.edit(ctx, chatID, msgID, b.tr("گروه این معرف را بفرستید.", "Send this introducer's group."), [][]button{b.cancelRow()})
	case "rdel":
		if len(parts) < 3 {
			return
		}
		show(b.tr("این معرف حذف شود؟", "Delete this introducer?"), [][]button{
			{{Text: b.tr("✅ حذف", "✅ Delete"), Data: "k:rdelY:" + parts[2]}, {Text: b.tr("✖️", "✖️"), Data: "k:refs"}},
		})
	case "rdelY":
		if len(parts) < 3 {
			return
		}
		s := b.loadShop()
		var next []shopReferrer
		for _, r := range s.Refs {
			if r.ID != parts[2] {
				next = append(next, r)
			}
		}
		s.Refs = next
		_ = b.saveShop(s)
		show(b.shopRefsScreen())
	case "shop":
		b.answer(ctx, cbID, "")
		b.sendMenu(ctx, chatID, b.shopHomeText(), b.customerMenu(chatID))
	case "trial":
		s := b.loadShop()
		on := !s.trialOn()
		s.Trial = &on
		_ = b.saveShop(s)
		show(b.shopEditHome())
	case "pay":
		show(b.shopPayScreen())
	case "pcard":
		b.answer(ctx, cbID, "")
		b.pend.set(chatID, &pending{kind: "shop.pcard", msgID: msgID, back: "k:pay", data: map[string]string{}})
		b.edit(ctx, chatID, msgID, b.tr("شماره کارت را بفرستید.", "Send the card number."), [][]button{b.cancelRow()})
	case "pwal":
		b.answer(ctx, cbID, "")
		b.pend.set(chatID, &pending{kind: "shop.pwal", msgID: msgID, back: "k:pay", data: map[string]string{}})
		b.edit(ctx, chatID, msgID, b.tr("آدرس کیف پول را بفرستید.", "Send the wallet address."), [][]button{b.cancelRow()})
	case "phint":
		b.answer(ctx, cbID, "")
		b.pend.set(chatID, &pending{kind: "shop.phint", msgID: msgID, back: "k:pay", data: map[string]string{}})
		b.edit(ctx, chatID, msgID, b.tr("متن راهنمای پرداخت را بفرستید.", "Send the payment instructions."), [][]button{b.cancelRow()})
	case "tgb":
		b.answer(ctx, cbID, "")
		b.pend.set(chatID, &pending{kind: "shop.tgb", msgID: msgID, back: "k:pay", data: map[string]string{}})
		b.edit(ctx, chatID, msgID, b.tr("حجم اکانت تست به گیگابایت را بفرستید (مثلا ۱).", "Send the test account volume in GB (for example 1)."), [][]button{b.cancelRow()})
	case "thrs":
		b.answer(ctx, cbID, "")
		b.pend.set(chatID, &pending{kind: "shop.thrs", msgID: msgID, back: "k:pay", data: map[string]string{}})
		b.edit(ctx, chatID, msgID, b.tr("مدت اکانت تست به ساعت را بفرستید (مثلا ۲۴).", "Send the test account duration in hours (for example 24)."), [][]button{b.cancelRow()})
	case "plan":
		if len(parts) > 2 {
			show(b.shopPlanScreen(parts[2]))
		}
	case "pnew":
		b.answer(ctx, cbID, "")
		b.pend.set(chatID, &pending{kind: "shop.pname", msgID: msgID, back: "k:plans", data: map[string]string{}})
		b.edit(ctx, chatID, msgID, b.tr("نام پلن را بفرستید.", "Send the plan name."), [][]button{b.cancelRow()})
	case "pn":
		if len(parts) < 3 {
			return
		}
		b.answer(ctx, cbID, "")
		b.pend.set(chatID, &pending{kind: "shop.pname", msgID: msgID, back: "k:plan:" + parts[2], data: map[string]string{"id": parts[2]}})
		b.edit(ctx, chatID, msgID, b.tr("نام جدید پلن را بفرستید.", "Send the new plan name."), [][]button{b.cancelRow()})
	case "pd":
		if len(parts) < 3 {
			return
		}
		show(b.tr("مدت پلن (روز) را انتخاب کنید.", "Pick the plan duration in days."),
			append(shopDaysButtons(parts[2]), []button{{Text: b.tr("⬅️", "⬅️"), Data: "k:plan:" + parts[2]}}))
	case "pds":
		if len(parts) < 4 {
			return
		}
		days, _ := strconv.Atoi(parts[3])
		b.patchPlan(parts[2], func(p *shopPlan) { p.Days = days })
		show(b.shopPlanScreen(parts[2]))
	case "pg":
		if len(parts) < 3 {
			return
		}
		show(b.tr("حجم پلن را انتخاب کنید.", "Pick the plan volume."),
			append(shopGBButtons(parts[2]), []button{{Text: b.tr("⬅️", "⬅️"), Data: "k:plan:" + parts[2]}}))
	case "pgs":
		if len(parts) < 4 {
			return
		}
		gb, _ := strconv.Atoi(parts[3])
		b.patchPlan(parts[2], func(p *shopPlan) { p.GB = gb })
		show(b.shopPlanScreen(parts[2]))
	case "pp":
		if len(parts) < 3 {
			return
		}
		show(b.tr("قیمت را انتخاب کنید یا بنویسید.", "Pick a price or type one."),
			append(shopPriceButtons(parts[2]), []button{{Text: b.tr("⬅️", "⬅️"), Data: "k:plan:" + parts[2]}}))
	case "pps":
		if len(parts) < 4 {
			return
		}
		b.patchPlan(parts[2], func(p *shopPlan) { p.Price = parts[3] })
		show(b.shopPlanScreen(parts[2]))
	case "ppa":
		if len(parts) < 3 {
			return
		}
		b.answer(ctx, cbID, "")
		b.pend.set(chatID, &pending{kind: "shop.price", msgID: msgID, back: "k:plan:" + parts[2], data: map[string]string{"id": parts[2]}})
		b.edit(ctx, chatID, msgID, b.tr("قیمت را بفرستید (مثلا ۱۵۰ هزار).", "Send the price (for example 150 thousand)."), [][]button{b.cancelRow()})
	case "pdel":
		if len(parts) < 3 {
			return
		}
		show(b.tr("این پلن حذف شود؟", "Delete this plan?"), [][]button{
			{{Text: b.tr("✅ حذف", "✅ Delete"), Data: "k:pdelY:" + parts[2]}, {Text: b.tr("✖️", "✖️"), Data: "k:plans"}},
		})
	case "pdelY":
		if len(parts) < 3 {
			return
		}
		s := b.loadShop()
		var next []shopPlan
		for _, p := range s.Plans {
			if p.ID != parts[2] {
				next = append(next, p)
			}
		}
		s.Plans = next
		_ = b.saveShop(s)
		show(b.shopPlansScreen())
	case "wnew":
		b.answer(ctx, cbID, "")
		b.pend.set(chatID, &pending{kind: "shop.wal", msgID: msgID, back: "k:wals", data: map[string]string{}})
		b.edit(ctx, chatID, msgID, b.tr("برچسب مبلغ را بفرستید (مثلا ۱۰۰ هزار).", "Send the amount label (for example 100 thousand)."), [][]button{b.cancelRow()})
	case "wdel":
		if len(parts) < 3 {
			return
		}
		s := b.loadShop()
		var next []shopWallet
		for _, w := range s.Wallets {
			if w.ID != parts[2] {
				next = append(next, w)
			}
		}
		s.Wallets = next
		_ = b.saveShop(s)
		show(b.shopWalletsScreen())
	case "tw":
		b.answer(ctx, cbID, "")
		b.pend.set(chatID, &pending{kind: "shop.welcome", msgID: msgID, back: "k:txts", data: map[string]string{}})
		b.edit(ctx, chatID, msgID, b.tr("متن خوش‌آمد مشتری را بفرستید.", "Send the customer welcome text."), [][]button{b.cancelRow()})
	case "twx":
		s := b.loadShop()
		s.WelcomeFa, s.WelcomeEn = "", ""
		_ = b.saveShop(s)
		show(b.shopTextsScreen())
	case "tg":
		b.answer(ctx, cbID, "")
		b.pend.set(chatID, &pending{kind: "shop.guide", msgID: msgID, back: "k:txts", data: map[string]string{}})
		b.edit(ctx, chatID, msgID, b.tr("متن آموزش اتصال را بفرستید.", "Send the connection guide text."), [][]button{b.cancelRow()})
	case "tgx":
		s := b.loadShop()
		s.GuideFa, s.GuideEn = "", ""
		_ = b.saveShop(s)
		show(b.shopTextsScreen())
	case "bt":
		if len(parts) < 3 {
			return
		}
		key := strings.Join(parts[2:], ":")
		s := b.loadShop()
		found := false
		for i := range s.Buttons {
			if s.Buttons[i].Data == key {
				s.Buttons[i].Off = !s.Buttons[i].Off
				found = true
			}
		}
		if !found {
			s.Buttons = append(s.Buttons, shopMenuBtn{Data: key, Off: true})
		}
		_ = b.saveShop(s)
		show(b.shopButtonsScreen())
	case "bn":
		if len(parts) < 3 {
			return
		}
		key := strings.Join(parts[2:], ":")
		b.answer(ctx, cbID, "")
		b.pend.set(chatID, &pending{kind: "shop.btn", msgID: msgID, back: "k:btns", data: map[string]string{"key": key}})
		b.edit(ctx, chatID, msgID, b.tr("اسم دکمه را بفرستید.", "Send the button label."), [][]button{b.cancelRow()})
	default:
		b.answer(ctx, cbID, "")
	}
}

func (b *bot) patchPlan(id string, fn func(*shopPlan)) {
	s := b.loadShop()
	for i := range s.Plans {
		if s.Plans[i].ID == id {
			fn(&s.Plans[i])
			_ = b.saveShop(s)
			return
		}
	}
}

func (b *bot) shopPending(ctx context.Context, chatID int64, p *pending, text string, retry func(error), finish func(string, [][]button)) {
	b = b.deskView(chatID)
	text = strings.TrimSpace(text)
	if text == "" {
		retry(fmt.Errorf("%s", b.tr("خالی بود", "it was empty")))
		return
	}
	s := b.loadShop()
	switch p.kind {
	case "shop.pname":
		id := ""
		if p.data != nil {
			id = p.data["id"]
		}
		if id == "" {
			ids := make([]string, 0, len(s.Plans))
			for _, x := range s.Plans {
				ids = append(ids, x.ID)
			}
			id = nextShopID(ids)
			s.Plans = append(s.Plans, shopPlan{ID: id, Name: text, Days: 30, GB: 50})
			_ = b.saveShop(s)
			t, kb := b.shopPlanScreen(id)
			finish(b.t("done")+"\n\n"+t, kb)
			return
		}
		b.patchPlan(id, func(pl *shopPlan) { pl.Name = text })
		t, kb := b.shopPlanScreen(id)
		finish(b.t("done")+"\n\n"+t, kb)
	case "shop.price":
		id := p.data["id"]
		b.patchPlan(id, func(pl *shopPlan) { pl.Price = text })
		t, kb := b.shopPlanScreen(id)
		finish(b.t("done")+"\n\n"+t, kb)
	case "shop.wal":
		ids := make([]string, 0, len(s.Wallets))
		for _, w := range s.Wallets {
			ids = append(ids, w.ID)
		}
		s.Wallets = append(s.Wallets, shopWallet{ID: nextShopID(ids), Label: text, Amount: text})
		_ = b.saveShop(s)
		t, kb := b.shopWalletsScreen()
		finish(b.t("done")+"\n\n"+t, kb)
	case "shop.welcome":
		s.WelcomeFa, s.WelcomeEn = text, text
		_ = b.saveShop(s)
		t, kb := b.shopTextsScreen()
		finish(b.t("done")+"\n\n"+t, kb)
	case "shop.guide":
		s.GuideFa, s.GuideEn = text, text
		_ = b.saveShop(s)
		t, kb := b.shopTextsScreen()
		finish(b.t("done")+"\n\n"+t, kb)
	case "shop.btn":
		key := p.data["key"]
		found := false
		for i := range s.Buttons {
			if s.Buttons[i].Data == key {
				s.Buttons[i].Fa, s.Buttons[i].En = text, text
				found = true
			}
		}
		if !found {
			s.Buttons = append(s.Buttons, shopMenuBtn{Data: key, Fa: text, En: text})
		}
		_ = b.saveShop(s)
		t, kb := b.shopButtonsScreen()
		finish(b.t("done")+"\n\n"+t, kb)
	case "shop.pcard":
		s.PayCard = text
		_ = b.saveShop(s)
		t, kb := b.shopPayScreen()
		finish(b.t("done")+"\n\n"+t, kb)
	case "shop.pwal":
		s.PayWallet = text
		_ = b.saveShop(s)
		t, kb := b.shopPayScreen()
		finish(b.t("done")+"\n\n"+t, kb)
	case "shop.phint":
		s.PayHint = text
		_ = b.saveShop(s)
		t, kb := b.shopPayScreen()
		finish(b.t("done")+"\n\n"+t, kb)
	case "shop.tgb":
		n, _ := strconv.Atoi(strings.TrimSpace(text))
		if n < 0 {
			n = 0
		}
		s.TrialGB = n
		_ = b.saveShop(s)
		t, kb := b.shopPayScreen()
		finish(b.t("done")+"\n\n"+t, kb)
	case "shop.thrs":
		n, _ := strconv.Atoi(strings.TrimSpace(text))
		if n < 0 {
			n = 0
		}
		s.TrialHours = n
		_ = b.saveShop(s)
		t, kb := b.shopPayScreen()
		finish(b.t("done")+"\n\n"+t, kb)
	case "shop.rword":
		word := strings.TrimSpace(text)
		if normalizeRefWord(word) == "" {
			retry(fmt.Errorf("%s", b.tr("خالی بود", "it was empty")))
			return
		}
		if s.findRef(word) != nil {
			retry(fmt.Errorf("%s", b.tr("این کلمه تکراری است", "that word is already used")))
			return
		}
		ids := make([]string, 0, len(s.Refs))
		for _, r := range s.Refs {
			ids = append(ids, r.ID)
		}
		b.pend.set(chatID, &pending{kind: "shop.rgroup", msgID: p.msgID, back: "k:refs", data: map[string]string{
			"word": word, "newid": nextShopID(ids),
		}})
		b.reply(ctx, chatID, p.msgID, b.tr(
			"گروه این معرف را بفرستید. کلاینت‌های جدید این معرف در همان گروه ساخته می‌شوند.",
			"Send this introducer's group. New clients from them are created in that group."),
			[][]button{b.cancelRow()})
	case "shop.rgroup":
		group := strings.TrimSpace(text)
		id := ""
		word := ""
		if p.data != nil {
			id = p.data["id"]
			word = p.data["word"]
		}
		if id != "" {
			for i := range s.Refs {
				if s.Refs[i].ID == id {
					s.Refs[i].Group = group
				}
			}
			_ = b.saveShop(s)
			t, kb := b.shopRefsScreen()
			finish(b.t("done")+"\n\n"+t, kb)
			return
		}
		if normalizeRefWord(word) == "" {
			retry(fmt.Errorf("%s", b.tr("کلمه معرف نبود", "the introducer word was missing")))
			return
		}
		s.Refs = append(s.Refs, shopReferrer{ID: nonempty(p.data["newid"], nextShopID(nil)), Word: word, Group: group})
		_ = b.saveShop(s)
		t, kb := b.shopRefsScreen()
		finish(b.t("done")+"\n\n"+t, kb)
	}
}
