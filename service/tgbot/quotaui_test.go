package tgbot

import (
	"strings"
	"testing"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
)

func quotaHistory(t *testing.T) []string {
	t.Helper()
	var rows []model.Changes
	if err := database.GetDB().Where("`key` = ?", "quota").Order("id").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Action)
	}
	return out
}

func TestOwnerSetsAnAdminsVolumeLimit(t *testing.T) {
	e := newAccessEnv(t)

	// The admin's card says there is no limit, and offers the screen.
	out := e.runPress(ownerID, "a:e:42")
	mustContain(t, out, "📦 Volume limit: none")
	if !hasButtonData(keyboardOf(e.got()), "a:q:42") {
		t.Fatal("the admin's card has no Volume limit button")
	}

	// Without a limit the screen offers to start one.
	out = e.runPress(ownerID, "a:q:42")
	mustContain(t, out, "Volume limit", "No limit")
	kb := keyboardOf(e.got())
	for _, data := range []string{"a:qa:42:50", "a:qa:42:100", "a:qa:42:500", "a:qa:42:1000", "a:qt:42", "a:e:42"} {
		if !hasButtonData(kb, data) {
			t.Errorf("the screen lacks %s: %v", data, callbackData(kb))
		}
	}
	for _, data := range []string{"a:qr:42", "a:qc:42"} {
		if hasButtonData(kb, data) {
			t.Errorf("a screen with no limit offers %s", data)
		}
	}
	if !noQuota(t, 42) {
		t.Fatal("looking at the screen made a limit")
	}

	// A preset starts a limit; a top-up adds to it.
	out = e.runPress(ownerID, "a:qa:42:100")
	mustContain(t, out, "Total: <b>100.00 GiB</b>", "Handed out: 0 B", "Left: <b>100.00 GiB</b>")
	e.runPress(ownerID, "a:qa:42:50")
	if q := quotaOf(t, 42); q.Total != 150*gib || q.Granted != 0 {
		t.Fatalf("after the preset and a top-up: %+v", q)
	}
	kb = keyboardOf(e.got())
	for _, data := range []string{"a:qa:42:10", "a:qa:42:50", "a:qa:42:100", "a:qa:42:500", "a:qt:42", "a:qr:42", "a:qc:42"} {
		if !hasButtonData(kb, data) {
			t.Errorf("the screen of a limit lacks %s: %v", data, callbackData(kb))
		}
	}
	// The admin's card and the list show it.
	mustContain(t, e.runPress(ownerID, "a:e:42"), "📦 150.00 GiB left of 150.00 GiB")
	mustContain(t, e.runPress(ownerID, "a:ls"), "📦 150.00 GiB left of 150.00 GiB")

	// A typed number sets the total; the prompt explains itself and cancel
	// goes back to the screen.
	out = e.runPress(ownerID, "a:qt:42")
	mustContain(t, out, "Send this admin's total volume in GB", "+100")
	if p := e.b.pend.get(ownerID); p == nil || p.kind != "ad.quota" {
		t.Fatalf("pending = %+v", p)
	}
	mustContain(t, e.runPress(ownerID, "x:cancel"), "Total: <b>150.00 GiB</b>")
	if e.b.pend.get(ownerID) != nil {
		t.Fatal("cancel left the prompt open")
	}

	type step struct {
		say   string
		total float64 // in GiB; -1 = refused
	}
	for _, s := range []step{
		{"500", 500}, {"+100.5", 600.5}, {"-50", 550.5}, {"2.5", 2.5}, {"۱۲٫۵", 12.5}, {"+٣", 15.5}, {"-1000", 0}, {"0", 0},
		{"abc", -1}, {"   ", -1}, {"-", -1}, {"1e12", -1}, {"+2000000", -1}, {"NaN", -1}, {"1000000000", -1},
	} {
		e.runPress(ownerID, "a:qt:42")
		before := quotaOf(t, 42)
		out := e.runSay(ownerID, s.say)
		after := quotaOf(t, 42)
		if s.total < 0 {
			if !strings.Contains(out, "Failed") {
				t.Errorf("%q: no refusal in:\n%s", s.say, out)
			}
			if after != before {
				t.Errorf("%q changed the limit: %+v -> %+v", s.say, before, after)
			}
			if e.b.pend.get(ownerID) == nil {
				t.Errorf("%q closed the prompt; it should stay open", s.say)
			}
			e.runPress(ownerID, "x:cancel")
			continue
		}
		mustContain(t, out, "Done")
		if want := int64(s.total * float64(gib)); after.Total != want {
			t.Errorf("%q: total = %d, want %d", s.say, after.Total, want)
		}
		if e.b.pend.get(ownerID) != nil {
			t.Errorf("%q left the prompt open", s.say)
		}
	}
	// Zero is a limit of nothing, not the end of the limit.
	if q := quotaOf(t, 42); q.Total != 0 {
		t.Fatalf("total = %d", q.Total)
	}
	// "+N" without a limit has nothing to add to.
	e.runPress(ownerID, "a:qc:42")
	e.runPress(ownerID, "a:qcy:42")
	e.runPress(ownerID, "a:qt:42")
	mustContain(t, e.runSay(ownerID, "+50"), "No limit is set yet")
	if !noQuota(t, 42) {
		t.Fatal("a relative change made a limit")
	}
	e.runPress(ownerID, "x:cancel")

	// Every edit is in the change history, under the owner's name.
	hist := quotaHistory(t)
	if len(hist) < 8 || hist[0] != "set" || hist[1] != "add" || hist[len(hist)-1] != "del" {
		t.Fatalf("history = %v", hist)
	}
	mustContain(t, e.runSay(ownerID, "/changes"), "telegram:1000", "quota.set", "quota.add", "quota.del")
}

func TestOwnerResetsAndRemovesALimit(t *testing.T) {
	e := newAccessEnv(t)
	giveQuota(t, fullAdmin, 100)
	if reserved, _, err := reserveQuota(fullAdmin, 30*gib); !reserved || err != nil {
		t.Fatal(err)
	}

	// Asking first: nothing changes until the confirmation.
	out := e.runPress(ownerID, "a:qr:42")
	mustContain(t, out, "Reset this admin's “handed out” to zero", "70.00 GiB left of 100.00 GiB")
	if !hasButtonData(keyboardOf(e.got()), "a:qry:42") || !hasButtonData(keyboardOf(e.got()), "a:q:42") {
		t.Fatalf("confirm buttons: %v", callbackData(keyboardOf(e.got())))
	}
	if quotaOf(t, fullAdmin).Granted != 30*gib {
		t.Fatal("asking reset the count")
	}
	out = e.runPress(ownerID, "a:qry:42")
	mustContain(t, out, "Handed out: 0 B", "Left: <b>100.00 GiB</b>")
	if q := quotaOf(t, fullAdmin); q.Granted != 0 || q.Total != 100*gib {
		t.Fatalf("after the reset: %+v", q)
	}

	out = e.runPress(ownerID, "a:qc:42")
	mustContain(t, out, "Lift this admin's volume limit")
	if !hasButtonData(keyboardOf(e.got()), "a:qcy:42") || noQuota(t, fullAdmin) {
		t.Fatal("asking removed the limit, or no confirm button")
	}
	out = e.runPress(ownerID, "a:qcy:42")
	mustContain(t, out, "No limit")
	if !noQuota(t, fullAdmin) {
		t.Fatal("the limit is still there")
	}
	// And now they can give any volume again.
	if out := e.runSay(fullAdmin, "/add big 5000 30"); strings.Contains(out, "Failed") {
		t.Fatalf("after lifting the limit:\n%s", out)
	}
	// There is nothing to reset any more.
	mustContain(t, e.runPress(ownerID, "a:qry:42"), "No limit is set yet")
	if got := quotaHistory(t); strings.Join(got, ",") != "reset,del" {
		t.Fatalf("history = %v", got)
	}
}

func TestOnlyTheOwnerCanEditVolumeLimits(t *testing.T) {
	e := newAccessEnv(t)
	giveQuota(t, clientsOnly, 10)
	before := quotaOf(t, clientsOnly)
	buttons := []string{"a:q:55", "a:qa:55:50", "a:qt:55", "a:qr:55", "a:qry:55", "a:qc:55", "a:qcy:55", "a:q:42", "a:qa:42:50", "a:qa:77:50"}
	for _, who := range []int64{fullAdmin, salesAdmin, clientsOnly, noSections, 999} {
		for _, data := range buttons {
			n := len(e.got())
			e.press(who, data)
			msgs := e.since(n)
			if len(msgs) != 1 || msgs[0].Method != "answerCallbackQuery" || msgs[0].Text == "" {
				t.Errorf("%d pressing %s got %+v", who, data, msgs)
			}
		}
		// A prompt only the owner could have opened does nothing for them.
		for _, text := range []string{"999999", "+5", "0"} {
			e.b.pend.set(who, &pending{kind: "ad.quota", data: map[string]string{"id": "55"}})
			e.say(who, text)
			e.b.pend.clear(who)
		}
	}
	if got := quotaOf(t, clientsOnly); got != before {
		t.Fatalf("somebody but the owner changed a limit: %+v -> %+v", before, got)
	}
	if len(allQuotas()) != 1 {
		t.Fatalf("somebody but the owner made a limit: %+v", allQuotas())
	}
	// An admin cannot raise their own limit by any route.
	giveQuota(t, fullAdmin, 1)
	e.runPress(fullAdmin, "a:qa:42:500")
	e.runSay(fullAdmin, "/add x 5 30")
	if q := quotaOf(t, fullAdmin); q.Total != 1*gib {
		t.Fatalf("their own total moved: %+v", q)
	}
}

func TestOwnerCannotBeGivenAVolumeLimit(t *testing.T) {
	e := newAccessEnv(t)
	for _, data := range []string{"a:q:1000", "a:qa:1000:50", "a:qt:1000", "a:qr:1000", "a:qry:1000", "a:qc:1000", "a:qcy:1000"} {
		n := len(e.got())
		e.press(ownerID, data)
		msgs := e.since(n)
		if len(msgs) != 1 || msgs[0].Method != "answerCallbackQuery" || !strings.Contains(msgs[0].Text, "owner") {
			t.Errorf("%s answered %+v", data, msgs)
		}
	}
	// Nor through a prompt left over from somebody else.
	e.b.pend.set(ownerID, &pending{kind: "ad.quota", data: map[string]string{"id": "1000"}})
	mustContain(t, e.runSay(ownerID, "50"), "owner")
	e.b.pend.clear(ownerID)
	if len(allQuotas()) != 0 {
		t.Fatalf("the owner got a limit: %+v", allQuotas())
	}
}

func TestVolumeLimitButtonsIgnoreJunk(t *testing.T) {
	e := newAccessEnv(t)
	for _, data := range []string{
		"a:q", "a:q:abc", "a:qa:55", "a:qa:55:x", "a:qa:55:-5", "a:qa:55:0", "a:qa:55:99999999", "a:qa:55:1.5",
		"a:q:99999", "a:qa:99999:50", "a:qt:99999", "a:qr:99999", "a:qry:99999", "a:qc:99999", "a:qcy:99999",
	} {
		e.press(ownerID, data)
	}
	if len(allQuotas()) != 0 {
		t.Fatalf("junk made limits: %+v", allQuotas())
	}
	// A prompt whose admin is gone, or whose data is broken, closes quietly.
	for _, id := range []string{"99999", "x", ""} {
		e.b.pend.set(ownerID, &pending{kind: "ad.quota", data: map[string]string{"id": id}})
		e.say(ownerID, "50")
	}
	if len(allQuotas()) != 0 {
		t.Fatalf("a stale prompt made a limit: %+v", allQuotas())
	}
}

func TestRemovingAnAdminRemovesTheirVolumeLimit(t *testing.T) {
	e := newAccessEnv(t)
	giveQuota(t, clientsOnly, 10)
	giveQuota(t, fullAdmin, 20)
	e.runPress(ownerID, "a:rm:55")
	if noQuota(t, clientsOnly) {
		t.Fatal("asking to remove an admin already removed the limit")
	}
	e.runPress(ownerID, "a:rmy:55")
	if e.b.access().isMember(clientsOnly) || !noQuota(t, clientsOnly) {
		t.Fatal("the admin or their limit is still there")
	}
	if noQuota(t, fullAdmin) {
		t.Fatal("somebody else's limit went too")
	}
	// Added again later, they start with no limit.
	e.runPress(ownerID, "a:add")
	e.say(ownerID, "55")
	if !noQuota(t, clientsOnly) {
		t.Fatal("the old limit came back")
	}
}

func TestAdminsListShowsVolumeLimits(t *testing.T) {
	e := newAccessEnv(t)
	giveQuota(t, clientsOnly, 10)
	giveQuota(t, salesAdmin, 100)
	if reserved, _, _ := reserveQuota(salesAdmin, 40*gib); !reserved {
		t.Fatal("reserve")
	}
	out := e.runPress(ownerID, "a:ls")
	var line55, line77, line42 string
	for _, l := range strings.Split(out, "\n") {
		switch {
		case strings.Contains(l, "<code>55</code>"):
			line55 = l
		case strings.Contains(l, "<code>77</code>"):
			line77 = l
		case strings.Contains(l, "<code>42</code>"):
			line42 = l
		}
	}
	mustContain(t, line55, "📦 10.00 GiB left of 10.00 GiB")
	mustContain(t, line77, "group: <b>Sales</b>", "📦 60.00 GiB left of 100.00 GiB")
	mustNotContain(t, line42, "📦")
	mustNotContain(t, out, "<code>1000</code> — owner · 📦")
}

func TestVolumeLimitScreenInPersian(t *testing.T) {
	e := newAccessEnv(t)
	e.b.cfg.Lang = "fa"
	out := e.runPress(ownerID, "a:q:42")
	mustContain(t, out, "محدودیت حجم", "بدون محدودیت")
	e.runPress(ownerID, "a:qa:42:50")
	out = e.runPress(ownerID, "a:q:42")
	mustContain(t, out, "کل:", "واگذار شده:", "باقی‌مانده:", "حذف کلاینت یا کم کردن حجم چیزی برنمی‌گرداند")
	mustContain(t, e.runPress(ownerID, "a:e:42"), "📦 باقی‌مانده 50.00 GiB از 50.00 GiB")
}

// The guards in front of the buttons are not the only ones: whatever calls the
// edit of a limit, only the owner's copy of the bot gets it done.
func TestOnlyTheOwnersBotEditsLimitsWhateverCallsIt(t *testing.T) {
	e := newAccessEnv(t)
	giveQuota(t, clientsOnly, 10)
	before := quotaOf(t, clientsOnly)
	for _, who := range []int64{fullAdmin, salesAdmin, clientsOnly, noSections, 999} {
		nb := e.b.as(who)
		for name, err := range map[string]error{
			"add":    nb.quotaAdd(clientsOnly, 5*gib),
			"reset":  nb.quotaReset(clientsOnly),
			"remove": nb.quotaRemove(clientsOnly),
			"set":    nb.applyQuotaInput(clientsOnly, "999"),
			"shift":  nb.applyQuotaInput(clientsOnly, "+5"),
		} {
			if err == nil {
				t.Errorf("%d: %s went through", who, name)
			}
		}
	}
	if after := quotaOf(t, clientsOnly); after != before {
		t.Fatalf("a non-owner changed the limit: %+v -> %+v", before, after)
	}
	if got := quotaHistory(t); len(got) != 0 {
		t.Fatalf("history = %v", got)
	}
	// The owner's own copy does.
	if err := e.b.as(ownerID).quotaAdd(clientsOnly, 5*gib); err != nil {
		t.Fatal(err)
	}
	if after := quotaOf(t, clientsOnly); after.Total != before.Total+5*gib {
		t.Fatalf("the owner's edit: %+v", after)
	}
}
