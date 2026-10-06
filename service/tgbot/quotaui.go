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
)

// The owner's side of the volume limits (see quota.go): the 📦 Volume limit
// screen of an administrator, reached from the Admins screen.

// Quick buttons, in GB: the size of a first limit, and the size of a top-up.
var (
	quotaStarts = []int64{50, 100, 500, 1000}
	quotaTopUps = []int64{10, 50, 100, 500}
)

// quotaBrief is how a list shows an administrator's limit.
func (b *bot) quotaBrief(q model.BotQuota) string {
	left, total := humanBytes(quotaLeft(q)), humanBytes(q.Total)
	return "📦 " + b.tr(fmt.Sprintf("باقی‌مانده %s از %s", left, total), fmt.Sprintf("%s left of %s", left, total))
}

// quotaStateLine is the limit of an administrator as one line of their card.
func (b *bot) quotaStateLine(id int64) string {
	q, ok, err := loadQuota(id)
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
// does for everything else the bot changes.
func (b *bot) recordQuota(id int64, action string, q model.BotQuota) {
	obj, _ := json.Marshal(map[string]interface{}{
		"name":    fmt.Sprintf("%d %s", id, humanBytes(q.Total)),
		"tgId":    id,
		"total":   q.Total,
		"granted": q.Granted,
	})
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
	b.recordQuota(id, action, q)
	return nil
}

// quotaReset counts what an administrator has handed out from zero again.
func (b *bot) quotaReset(id int64) error {
	if err := b.checkQuotaEditor(id); err != nil {
		return err
	}
	q, found, err := resetQuotaGranted(id)
	if err != nil {
		return err
	}
	if !found {
		return b.errNoQuotaYet()
	}
	b.recordQuota(id, "reset", q)
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
	b.recordQuota(id, "del", q)
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
		b.recordQuota(id, "add", q)
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
	b.recordQuota(id, "set", q)
	return nil
}

// adminQuotaScreen is where the owner sets what one administrator may hand out.
func (b *bot) adminQuotaScreen(ctx context.Context, id int64) (string, [][]button) {
	a := b.access()
	if !a.isMember(id) || a.isOwner(id) {
		return b.adminsScreen(ctx)
	}
	sid := strconv.FormatInt(id, 10)
	q, ok, err := loadQuota(id)
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
			"➖ "+b.tr("واگذار شده: ", "Handed out: ")+humanBytes(q.Granted),
			"✅ "+b.tr("باقی‌مانده: ", "Left: ")+"<b>"+humanBytes(quotaLeft(q))+"</b>")
		var top []button
		for _, gb := range quotaTopUps {
			top = append(top, button{Text: fmt.Sprintf("➕ %d GB", gb), Data: fmt.Sprintf("a:qa:%s:%d", sid, gb)})
		}
		kb = append(kb, top,
			[]button{{Text: b.tr("✏️ ویرایش کل", "✏️ Edit total"), Data: "a:qt:" + sid}},
			[]button{{Text: b.tr("🔄 صفر کردن واگذار شده", "🔄 Reset handed out"), Data: "a:qr:" + sid}, {Text: b.tr("♾ حذف محدودیت", "♾ Remove limit"), Data: "a:qc:" + sid}})
	} else {
		lines = append(lines, "♾ "+b.tr("بدون محدودیت: این ادمین می‌تواند هر حجمی را به کلاینت‌ها بدهد. برای محدود کردن، سقف کل را تعیین کنید.", "No limit: this admin can give clients any volume. Set a total to limit them."))
		var start []button
		for _, gb := range quotaStarts {
			start = append(start, button{Text: b.tr("تعیین ", "Set ") + fmt.Sprintf("%d GB", gb), Data: fmt.Sprintf("a:qa:%s:%d", sid, gb)})
		}
		kb = append(kb, start, []button{{Text: b.tr("✏️ مقدار دلخواه", "✏️ Custom"), Data: "a:qt:" + sid}})
	}
	lines = append(lines, "", b.tr(
		"ℹ️ هر حجمی که این ادمین از ربات به کلاینت‌ها می‌دهد از «کل» کم می‌شود: حجم کلاینت جدید، افزودن یا بالا بردن حجم، و ریست ترافیک (چون دوباره حجم می‌دهد). حذف کلاینت یا کم کردن حجم چیزی برنمی‌گرداند؛ هر وقت لازم بود کل را بالا ببرید یا «واگذار شده» را صفر کنید. وقتی حجم تمام شود فقط نمی‌تواند حجم بیشتری بدهد. با محدودیت حجم، کلاینت نامحدود و ریست خودکار برای او بسته است.",
		"ℹ️ Whatever volume this admin gives to clients through the bot is deducted from the total: a new client's volume, adding or raising volume, and a traffic reset (it gives the volume again). Deleting a client or lowering its volume gives nothing back; raise the total or zero “handed out” when needed. When it runs out they just cannot give more volume. With a limit, unlimited clients and auto reset are blocked for them."))
	kb = append(kb, b.navRow("a:e:"+sid))
	return strings.Join(lines, "\n"), kb
}

// quotaCallback handles the buttons of the limit screen: "a:q:<id>" shows it,
// "a:qa:<id>:<GB>" adds to the total (or starts a limit), "a:qt:<id>" asks for a
// typed value, "a:qr:<id>" and "a:qc:<id>" ask before resetting what has been
// handed out or removing the limit, and their "y" forms do it.
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
			"سقف کل حجم این ادمین را به گیگابایت بفرستید؛ مثلاً 500 یا 2.5. با + یا - کل فعلی را کم و زیاد می‌کنید؛ مثلاً +100. عدد 0 یعنی هیچ حجمی نمی‌تواند بدهد؛ برای برداشتن محدودیت از دکمهٔ «♾ حذف محدودیت» استفاده کنید.",
			"Send this admin's total volume in GB, e.g. 500 or 2.5. A leading + or - changes the current total instead, e.g. +100. 0 means they cannot give any volume; to lift the limit use “♾ Remove limit”."),
			[][]button{b.navRow("a:q:" + sid), b.cancelRow()})
	case "qr", "qc":
		if err := b.checkQuotaEditor(id); err != nil {
			b.answer(ctx, cbID, b.t("failed", b.errText(err)))
			return
		}
		b.answer(ctx, cbID, "")
		ask := b.tr("⚠️ «واگذار شده» این ادمین صفر شود؟ باقی‌ماندهٔ او برابر کل می‌شود:", "⚠️ Reset this admin's “handed out” to zero? Their remaining volume becomes the whole total:")
		if verb == "qc" {
			ask = b.tr("⚠️ محدودیت حجم این ادمین برداشته شود؟ دوباره می‌تواند هر حجمی بدهد:", "⚠️ Lift this admin's volume limit? They can give any volume again:")
		}
		show(ask+"\n"+b.quotaStateLine(id), [][]button{{{Text: b.t("btnConfirm"), Data: "a:" + verb + "y:" + sid}, {Text: b.t("btnCancel"), Data: "a:q:" + sid}}})
	case "qry":
		apply(b.quotaReset(id))
	case "qcy":
		apply(b.quotaRemove(id))
	}
}
