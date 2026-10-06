package tgbot

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/logger"
	"github.com/Danialrostamani/drnetwork-panel/service"
)

// The owner's side of the volume limits (see quota.go): the 📦 Volume limit
// screen of an administrator, reached from the Admins screen.

// Quick buttons, in GB: the size of a first limit, and the size of a top-up.
var (
	quotaStarts = []int64{50, 100, 500, 1000}
	quotaTopUps = []int64{10, 50, 100, 500}
)

// quotaBrief is how a list shows an administrator's limit.
func (b *bot) quotaBrief(q quotaState) string {
	left, total := humanBytes(q.Left()), humanBytes(q.Total)
	return "📦 " + b.tr(fmt.Sprintf("باقی‌مانده %s از %s", left, total), fmt.Sprintf("%s left of %s", left, total))
}

// quotaStateLine is the limit of an administrator as one line of their card.
func (b *bot) quotaStateLine(id int64) string {
	q, ok, err := b.quotaOf(id)
	switch {
	case err != nil:
		return ""
	case !ok:
		return "📦 " + b.tr("محدودیت حجم: ندارد", "Volume limit: none")
	}
	return b.quotaBrief(q)
}

// checkQuotaEditor is the rule for every change of a limit: only the owner
// edits them, and only those of administrators other than themselves.
func (b *bot) checkQuotaEditor(id int64) error {
	a := b.access()
	switch {
	case !b.owner:
		return errNotOwner
	case a.isOwner(id):
		return errIsOwner
	case !a.isMember(id):
		return errNoSuchAdmin
	}
	return nil
}

// recordQuota writes an edit of a limit into the change history, as the panel
// does for everything else the bot changes. used is what had been used of it,
// or negative when that is not worth noting.
func (b *bot) recordQuota(id int64, action string, total, used int64) {
	fields := map[string]interface{}{
		"name":  fmt.Sprintf("%d %s", id, humanBytes(total)),
		"tgId":  id,
		"total": total,
	}
	if used >= 0 {
		fields["used"] = used
	}
	obj, _ := json.Marshal(fields)
	err := database.GetDB().Create(&model.Changes{DateTime: time.Now().Unix(), Actor: b.actor(), Key: "quota", Action: action, Obj: obj}).Error
	if err != nil {
		logger.Warning("telegram bot: record the change of a volume limit: ", err)
	}
}

// quotaAdd raises the total of an administrator by delta bytes, or gives them a
// limit of that size when they have none.
func (b *bot) quotaAdd(id, delta int64) error {
	if err := b.checkQuotaEditor(id); err != nil {
		return err
	}
	q, found, err := shiftQuotaTotal(id, delta)
	action := "add"
	if err == nil && !found {
		q, err = setQuotaTotal(id, delta)
		action = "set"
	}
	if err != nil {
		return err
	}
	b.recordQuota(id, action, q.Total, -1)
	return nil
}

// quotaReset counts what an administrator's clients use from zero again.
func (b *bot) quotaReset(id int64) error {
	if err := b.checkQuotaEditor(id); err != nil {
		return err
	}
	q, ok, err := b.quotaOf(id)
	switch {
	case err != nil:
		return err
	case !ok:
		return b.errNoQuotaYet()
	}
	if err := service.BotQuotaRestart(database.GetDB(), id); err != nil {
		return err
	}
	b.recordQuota(id, "reset", q.Total, q.Used)
	return nil
}

// quotaRemove lifts the limit of an administrator.
func (b *bot) quotaRemove(id int64) error {
	if err := b.checkQuotaEditor(id); err != nil {
		return err
	}
	return b.dropQuota(id)
}

// dropQuota deletes an administrator's limit, if they have one, and records it.
// Taking an administrator off the list does the same.
func (b *bot) dropQuota(id int64) error {
	q, ok, err := loadQuota(id)
	if err != nil || !ok {
		return err
	}
	if err := deleteQuota(id); err != nil {
		return err
	}
	b.recordQuota(id, "del", q.Total, -1)
	return nil
}

func (b *bot) errNoQuotaYet() error {
	return &quotaError{b.tr("هنوز محدودیتی تعیین نشده؛ یک عدد ساده بفرستید، مثلاً 500.", "No limit is set yet; send a plain number, e.g. 500.")}
}

// latinDigits turns the Persian and Arabic-Indic digits, and the decimal
// separator, of a typed number into the ASCII ones.
func latinDigits(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= '۰' && r <= '۹':
			return '0' + (r - '۰')
		case r >= '٠' && r <= '٩':
			return '0' + (r - '٠')
		case r == '٫':
			return '.'
		case r == '٬':
			return -1
		case r == '−':
			return '-'
		}
		return r
	}, s)
}

// applyQuotaInput applies what the owner typed on the limit prompt: a plain
// number in GB sets the total, +N or -N changes it.
func (b *bot) applyQuotaInput(id int64, text string) error {
	if err := b.checkQuotaEditor(id); err != nil {
		return err
	}
	t := strings.TrimSpace(latinDigits(text))
	if strings.HasPrefix(t, "+") || strings.HasPrefix(t, "-") {
		gb, ok := parseSignedFloat(t)
		if !ok {
			return fmt.Errorf("%s", b.t("badNumber"))
		}
		q, found, err := shiftQuotaTotal(id, int64(gb*float64(gib)))
		if err != nil {
			return err
		}
		if !found {
			return b.errNoQuotaYet()
		}
		b.recordQuota(id, "add", q.Total, -1)
		return nil
	}
	gb, ok := parseFloatArg(t)
	if !ok {
		return fmt.Errorf("%s", b.t("badNumber"))
	}
	q, err := setQuotaTotal(id, int64(gb*float64(gib)))
	if err != nil {
		return err
	}
	b.recordQuota(id, "set", q.Total, -1)
	return nil
}

// adminQuotaScreen is where the owner sets how much traffic one administrator's
// clients may use.
func (b *bot) adminQuotaScreen(ctx context.Context, id int64) (string, [][]button) {
	a := b.access()
	if !a.isMember(id) || a.isOwner(id) {
		return b.adminsScreen(ctx)
	}
	sid := strconv.FormatInt(id, 10)
	q, ok, err := b.quotaOf(id)
	if err != nil {
		return b.t("failed", esc(err.Error())), [][]button{b.navRow("a:e:" + sid)}
	}
	lines := []string{
		b.header("📦", b.tr("محدودیت حجم", "Volume limit")),
		b.adminLine(a, id, b.adminNames(ctx, []int64{id})[id]),
		"",
	}
	var kb [][]button
	if ok {
		lines = append(lines,
			"📦 "+b.tr("کل: ", "Total: ")+"<b>"+humanBytes(q.Total)+"</b>",
			"➖ "+b.tr("مصرف‌شده: ", "Used: ")+humanBytes(q.Used),
			"✅ "+b.tr("باقی‌مانده: ", "Left: ")+"<b>"+humanBytes(q.Left())+"</b>",
			"👥 "+b.tr("کلاینت‌های حساب‌شده: ", "Clients counted: ")+strconv.Itoa(q.Clients))
		var top []button
		for _, gb := range quotaTopUps {
			top = append(top, button{Text: fmt.Sprintf("➕ %d GB", gb), Data: fmt.Sprintf("a:qa:%s:%d", sid, gb)})
		}
		kb = append(kb, top,
			[]button{{Text: b.tr("✏️ ویرایش کل", "✏️ Edit total"), Data: "a:qt:" + sid}},
			[]button{{Text: b.tr("🔄 صفر کردن مصرف", "🔄 Reset used"), Data: "a:qr:" + sid}, {Text: b.tr("♾ حذف محدودیت", "♾ Remove limit"), Data: "a:qc:" + sid}})
	} else {
		lines = append(lines, "♾ "+b.tr("بدون محدودیت: مصرف کلاینت‌های این ادمین محدود نمی‌شود. برای محدود کردن، سقف کل را تعیین کنید.", "No limit: what this admin's clients use is not limited. Set a total to limit it."))
		var start []button
		for _, gb := range quotaStarts {
			start = append(start, button{Text: b.tr("تعیین ", "Set ") + fmt.Sprintf("%d GB", gb), Data: fmt.Sprintf("a:qa:%s:%d", sid, gb)})
		}
		kb = append(kb, start, []button{{Text: b.tr("✏️ مقدار دلخواه", "✏️ Custom"), Data: "a:qt:" + sid}})
	}
	lines = append(lines, "", b.tr(
		"ℹ️ هنگام ساخت کلاینت یا تعیین حجم چیزی کم نمی‌شود؛ «کل» به اندازهٔ ترافیکی که کلاینت‌های این ادمین واقعاً مصرف می‌کنند کم می‌شود. ریست ترافیک یا حذف کلاینت چیزی برنمی‌گرداند و کلاینتِ جایگزین از مصرف خودش حساب می‌شود. کلاینتی حساب می‌شود که او با ربات و پس از تعیین محدودیت ساخته باشد؛ برای ادمینِ گروهی همهٔ کلاینت‌های گروهش. مصرفِ پیش از آن هرگز محاسبه نمی‌شود. وقتی چیزی نماند نمی‌تواند کلاینت جدید بسازد یا حجم بیشتری بدهد؛ کلاینت‌های موجود قطع نمی‌شوند.",
		"ℹ️ Nothing is deducted when this admin creates a client or sets its volume; the total goes down as the clients that count for them really use traffic. A traffic reset or deleting a client gives nothing back, and a replacement client counts from its own usage. A client counts when they created it through the bot after the limit was set; for a group-limited admin, every client of the group. What a client used before that is never charged. When nothing is left they cannot create clients or give more volume; existing clients are not cut off."))
	kb = append(kb, b.navRow("a:e:"+sid))
	return strings.Join(lines, "\n"), kb
}

// quotaCallback handles the buttons of the limit screen: "a:q:<id>" shows it,
// "a:qa:<id>:<GB>" adds to the total (or starts a limit), "a:qt:<id>" asks for a
// typed value, "a:qr:<id>" and "a:qc:<id>" ask before resetting what has been
// used or removing the limit, and their "y" forms do it.
func (b *bot) quotaCallback(ctx context.Context, cbID string, chatID, msgID int64, verb string, id int64, parts []string) {
	sid := strconv.FormatInt(id, 10)
	show := func(text string, kb [][]button) { b.edit(ctx, chatID, msgID, text, kb) }
	screen := func() {
		text, kb := b.adminQuotaScreen(ctx, id)
		show(text, kb)
	}
	// apply runs an edit of the limit and shows the result.
	apply := func(err error) {
		if err != nil {
			b.answer(ctx, cbID, b.t("failed", b.errText(err)))
			return
		}
		b.answer(ctx, cbID, "")
		screen()
	}
	switch verb {
	case "q":
		b.answer(ctx, cbID, "")
		screen()
	case "qa":
		gb := int64(0)
		if len(parts) > 3 {
			gb, _ = strconv.ParseInt(parts[3], 10, 64)
		}
		if gb < 1 || gb > 1e6 {
			b.answer(ctx, cbID, "")
			return
		}
		apply(b.quotaAdd(id, gb*gib))
	case "qt":
		if err := b.checkQuotaEditor(id); err != nil {
			b.answer(ctx, cbID, b.t("failed", b.errText(err)))
			return
		}
		b.answer(ctx, cbID, "")
		b.pend.set(chatID, &pending{kind: "ad.quota", msgID: msgID, back: "a:q:" + sid, data: map[string]string{"id": sid}})
		show(b.header("📦", b.tr("محدودیت حجم", "Volume limit"))+"\n"+b.tr(
			"سقف کل ترافیک کلاینت‌های این ادمین را به گیگابایت بفرستید؛ مثلاً 500 یا 2.5. با + یا - کل فعلی را کم و زیاد می‌کنید؛ مثلاً +100. عدد 0 یعنی چیزی نمانده و نمی‌تواند کلاینت جدید بسازد؛ برای برداشتن محدودیت از دکمهٔ «♾ حذف محدودیت» استفاده کنید.",
			"Send the total traffic this admin's clients may use, in GB, e.g. 500 or 2.5. A leading + or - changes the current total instead, e.g. +100. 0 means nothing is left, so they cannot create clients; to lift the limit use “♾ Remove limit”."),
			[][]button{b.navRow("a:q:" + sid), b.cancelRow()})
	case "qr", "qc":
		if err := b.checkQuotaEditor(id); err != nil {
			b.answer(ctx, cbID, b.t("failed", b.errText(err)))
			return
		}
		b.answer(ctx, cbID, "")
		ask := b.tr("⚠️ «مصرف‌شده» این ادمین صفر شود؟ شمارش از همین حالا شروع می‌شود و باقی‌مانده برابر کل می‌شود:", "⚠️ Reset this admin's “used” to zero? Counting starts again from now and what is left becomes the whole total:")
		if verb == "qc" {
			ask = b.tr("⚠️ محدودیت حجم این ادمین برداشته شود؟ مصرف کلاینت‌هایش دیگر محدود نیست:", "⚠️ Lift this admin's volume limit? What their clients use is no longer limited:")
		}
		show(ask+"\n"+b.quotaStateLine(id), [][]button{{{Text: b.t("btnConfirm"), Data: "a:" + verb + "y:" + sid}, {Text: b.t("btnCancel"), Data: "a:q:" + sid}}})
	case "qry":
		apply(b.quotaReset(id))
	case "qcy":
		apply(b.quotaRemove(id))
	}
}
