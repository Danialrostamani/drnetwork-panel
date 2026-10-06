package tgbot

import (
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"unicode/utf8"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
)

// ---- helpers ----

// giveQuota makes the administrator's total volume gb GB, nothing handed out.
func giveQuota(t *testing.T, id int64, gb float64) {
	t.Helper()
	if _, err := setQuotaTotal(id, int64(gb*float64(gib))); err != nil {
		t.Fatal(err)
	}
}

func quotaOf(t *testing.T, id int64) model.BotQuota {
	t.Helper()
	q, ok, err := loadQuota(id)
	if err != nil || !ok {
		t.Fatalf("no volume limit for %d (%v)", id, err)
	}
	return q
}

func noQuota(t *testing.T, id int64) bool {
	t.Helper()
	_, ok, err := loadQuota(id)
	if err != nil {
		t.Fatal(err)
	}
	return !ok
}

// allText is everything the bot said since n messages ago: texts, edits and
// the pop-up answers of buttons.
func (e *scopeEnv) allText(n int) string {
	var parts []string
	for _, m := range e.since(n) {
		parts = append(parts, m.Text)
	}
	return strings.Join(parts, "\n---\n")
}

// run says a command or presses a button and returns what the bot answered.
func (e *scopeEnv) runSay(from int64, text string) string {
	e.t.Helper()
	n := len(e.got())
	e.say(from, text)
	return e.allText(n)
}

func (e *scopeEnv) runPress(from int64, data string) string {
	e.t.Helper()
	n := len(e.got())
	e.press(from, data)
	return e.allText(n)
}

func cl(volume, up, down int64) model.Client {
	return model.Client{Id: 1, Name: "x", Volume: volume, Up: up, Down: down}
}

const notEnough = "Not enough volume left"

// ---- what a write costs ----

func TestClientCost(t *testing.T) {
	b := newBot(botConfig{Lang: "en"})
	auto := func(c model.Client) model.Client { c.AutoReset = true; return c }
	ptr := func(c model.Client) *model.Client { return &c }
	cases := []struct {
		name    string
		before  *model.Client
		after   model.Client
		want    int64
		wantErr string
	}{
		{"new client costs its volume", nil, cl(50*gib, 0, 0), 50 * gib, ""},
		{"new unlimited client is refused", nil, cl(0, 0, 0), 0, "cannot be unlimited"},
		{"new client with auto reset is refused", nil, auto(cl(10*gib, 0, 0)), 0, "auto reset"},
		{"raising a volume costs the difference", ptr(cl(50*gib, 10*gib, 0)), cl(80*gib, 10*gib, 0), 30 * gib, ""},
		{"lowering a volume costs nothing", ptr(cl(80*gib, 10*gib, 0)), cl(50*gib, 10*gib, 0), 0, ""},
		{"saving without a change costs nothing", ptr(cl(50*gib, 10*gib, 0)), cl(50*gib, 10*gib, 0), 0, ""},
		{"a reset costs what it gives back", ptr(cl(50*gib, 30*gib, 10*gib)), cl(50*gib, 0, 0), 40 * gib, ""},
		{"a reset of a used-up client costs its whole volume", ptr(cl(50*gib, 60*gib, 0)), cl(50*gib, 0, 0), 50 * gib, ""},
		{"a reset of an unused client costs nothing", ptr(cl(50*gib, 0, 0)), cl(50*gib, 0, 0), 0, ""},
		{"raising an overused client costs only what it can use", ptr(cl(50*gib, 60*gib, 0)), cl(80*gib, 60*gib, 0), 20 * gib, ""},
		{"making a client unlimited is refused", ptr(cl(50*gib, 0, 0)), cl(0, 0, 0), 0, "cannot be unlimited"},
		{"a limit on an unlimited client is free", ptr(cl(0, 5*gib, 0)), cl(50*gib, 5*gib, 0), 0, ""},
		{"an unlimited client stays unlimited for free", ptr(cl(0, 5*gib, 0)), cl(0, 0, 0), 0, ""},
		{"switching auto reset on is refused", ptr(cl(50*gib, 0, 0)), auto(cl(50*gib, 0, 0)), 0, "auto reset"},
		{"an auto reset client may be raised", ptr(auto(cl(50*gib, 0, 0))), auto(cl(60*gib, 0, 0)), 10 * gib, ""},
		{"traffic that grew meanwhile is not a reset or a cost", ptr(cl(50*gib, 10*gib, 0)), cl(50*gib, 5*gib, 0), 0, ""},
		{"the cost cannot overflow", nil, cl(math.MaxInt64, 0, 0), math.MaxInt64, ""},
	}
	for _, tc := range cases {
		got, err := b.clientCost(tc.before, tc.after)
		switch {
		case tc.wantErr != "":
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("%s: err = %v, want %q", tc.name, err, tc.wantErr)
			}
		case err != nil:
			t.Errorf("%s: %v", tc.name, err)
		case got != tc.want:
			t.Errorf("%s: cost = %d, want %d", tc.name, got, tc.want)
		}
	}

	// Prices add up without wrapping around, and a payload the bot does not know
	// how to price is refused rather than let through free.
	if got, err := b.volumeCost("addbulk", []model.Client{cl(math.MaxInt64, 0, 0), cl(math.MaxInt64, 0, 0)}); err != nil || got != math.MaxInt64 {
		t.Errorf("a huge bulk costs %d (%v)", got, err)
	}
	if _, err := b.volumeCost("new", "nonsense"); err == nil {
		t.Error("a payload of the wrong type was priced")
	}
	if _, err := b.volumeCost("somethingnew", nil); err == nil {
		t.Error("an unknown write was priced")
	}
	for _, act := range []string{"del", "delbulk", "attachall"} {
		if got, err := b.volumeCost(act, nil); got != 0 || err != nil {
			t.Errorf("%s costs %d (%v)", act, got, err)
		}
	}
}

// ---- the store ----

func TestQuotaStore(t *testing.T) {
	testBot(t)
	if !noQuota(t, 42) {
		t.Fatal("a fresh database has a limit")
	}
	q, err := setQuotaTotal(42, 100*gib)
	if err != nil || q.Total != 100*gib || q.Granted != 0 {
		t.Fatalf("set: %+v %v", q, err)
	}
	if reserved, _, err := reserveQuota(42, 30*gib); !reserved || err != nil {
		t.Fatalf("reserve: %v %v", reserved, err)
	}
	// Setting the total again keeps what was handed out.
	if q, _ = setQuotaTotal(42, 200*gib); q.Total != 200*gib || q.Granted != 30*gib {
		t.Fatalf("the total changed what was handed out: %+v", q)
	}
	// Relative changes clamp at both ends.
	if q, found, err := shiftQuotaTotal(42, -500*gib); !found || err != nil || q.Total != 0 || q.Granted != 30*gib {
		t.Fatalf("shift below zero: %+v %v %v", q, found, err)
	}
	if q, _, _ := shiftQuotaTotal(42, math.MaxInt64/4); q.Total != quotaCap {
		t.Fatalf("shift above the cap: %+v", q)
	}
	if _, found, err := shiftQuotaTotal(43, gib); found || err != nil {
		t.Fatalf("shift of nobody: %v %v", found, err)
	}
	if q, _ := setQuotaTotal(42, 10*gib); quotaLeft(q) != 0 {
		t.Fatalf("a total under what was handed out leaves %d", quotaLeft(q))
	}
	if q, found, _ := resetQuotaGranted(42); !found || q.Granted != 0 || q.Total != 10*gib {
		t.Fatalf("reset: %+v", q)
	}
	if _, found, _ := resetQuotaGranted(43); found {
		t.Fatal("reset of nobody")
	}
	// A negative or huge total is clamped.
	if q, _ := setQuotaTotal(44, -5); q.Total != 0 {
		t.Fatalf("negative total: %+v", q)
	}
	if q, _ := setQuotaTotal(44, math.MaxInt64); q.Total != quotaCap {
		t.Fatalf("huge total: %+v", q)
	}
	if got := allQuotas(); len(got) != 2 || got[42].Total != 10*gib {
		t.Fatalf("allQuotas = %+v", got)
	}
	if err := deleteQuota(42); err != nil || !noQuota(t, 42) {
		t.Fatalf("delete: %v", err)
	}
}

func TestReservationsNeverOverdraw(t *testing.T) {
	testBot(t)
	giveQuota(t, fullAdmin, 100)
	var won, short int32
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			reserved, left, err := reserveQuota(fullAdmin, 10*gib)
			switch {
			case err == nil && reserved:
				atomic.AddInt32(&won, 1)
			case err == errQuotaShort && left < 10*gib:
				atomic.AddInt32(&short, 1)
			}
		}()
	}
	wg.Wait()
	if won != 10 || short != 30 {
		t.Fatalf("%d reservations won and %d were refused, want 10 and 30", won, short)
	}
	if q := quotaOf(t, fullAdmin); q.Granted != 100*gib {
		t.Fatalf("granted = %d", q.Granted)
	}
	// Giving back more than was taken never goes below zero.
	releaseQuota(fullAdmin, 500*gib)
	if q := quotaOf(t, fullAdmin); q.Granted != 0 {
		t.Fatalf("granted after an over-release = %d", q.Granted)
	}
	// An administrator without a limit reserves nothing and is not refused.
	if reserved, _, err := reserveQuota(999, gib); reserved || err != nil {
		t.Fatalf("reserve without a limit: %v %v", reserved, err)
	}
}

// ---- the limit at work ----

func TestLimitedAdminPaysForNewClients(t *testing.T) {
	e := newAccessEnv(t)
	giveQuota(t, fullAdmin, 100)

	if out := e.runSay(fullAdmin, "/add ali 30 30"); strings.Contains(out, "Failed") || !e.exists("ali") {
		t.Fatalf("a client within the limit was refused:\n%s", out)
	}
	if q := quotaOf(t, fullAdmin); q.Granted != 30*gib || quotaLeft(q) != 70*gib {
		t.Fatalf("after 30 GB: %+v", q)
	}

	out := e.runSay(fullAdmin, "/add bob 80 30")
	mustContain(t, out, "Failed", notEnough, "needs 80.00 GiB", "you have 70.00 GiB", "owner")
	if e.exists("bob") || quotaOf(t, fullAdmin).Granted != 30*gib {
		t.Fatal("a refused client was created or charged")
	}

	if out := e.runSay(fullAdmin, "/add bob 70 30"); strings.Contains(out, "Failed") || !e.exists("bob") {
		t.Fatalf("a client that fits exactly was refused:\n%s", out)
	}
	if q := quotaOf(t, fullAdmin); quotaLeft(q) != 0 || q.Granted != 100*gib {
		t.Fatalf("after using it all: %+v", q)
	}
	if e.client("bob").Volume != 70*gib {
		t.Fatalf("bob's volume = %d", e.client("bob").Volume)
	}

	out = e.runSay(fullAdmin, "/add carl 1 30")
	mustContain(t, out, notEnough, "you have 0 B")
	if e.exists("carl") {
		t.Fatal("a client was created with nothing left")
	}
	out = e.runSay(fullAdmin, "/add dave 0 30")
	mustContain(t, out, "cannot be unlimited")
	if e.exists("dave") {
		t.Fatal("an unlimited client was created")
	}

	// Without volume left they still manage what they have.
	id := itoa(int64(e.client("ali").Id))
	if out := e.runPress(fullAdmin, "c:tog:"+id); strings.Contains(out, "Failed") {
		t.Fatalf("disabling a client was refused:\n%s", out)
	}
	if out := e.runSay(fullAdmin, "/expiry ali 60"); strings.Contains(out, "Failed") {
		t.Fatalf("changing the expiry was refused:\n%s", out)
	}
	if q := quotaOf(t, fullAdmin); q.Granted != 100*gib {
		t.Fatalf("a change that gives no volume was charged: %+v", q)
	}
}

func TestLimitedAdminPaysForVolumeChanges(t *testing.T) {
	e := newAccessEnv(t)
	giveQuota(t, fullAdmin, 100)
	v := seedClient(t, "v1", "", 50*gib, 10*gib)
	id := itoa(int64(v.Id))
	volume := func() int64 { return e.client("v1").Volume }
	granted := func() int64 { return quotaOf(t, fullAdmin).Granted }

	e.runPress(fullAdmin, "c:gb10:"+id)
	if volume() != 60*gib || granted() != 10*gib {
		t.Fatalf("+10 GB: volume %d, granted %d", volume(), granted())
	}
	e.runPress(fullAdmin, "c:gb50:"+id)
	if volume() != 110*gib || granted() != 60*gib {
		t.Fatalf("+50 GB: volume %d, granted %d", volume(), granted())
	}
	e.runSay(fullAdmin, "/volume v1 150")
	if volume() != 150*gib || granted() != 100*gib {
		t.Fatalf("/volume 150: volume %d, granted %d", volume(), granted())
	}

	out := e.runPress(fullAdmin, "c:gb10:"+id)
	mustContain(t, out, notEnough)
	if volume() != 150*gib || granted() != 100*gib {
		t.Fatalf("a refused +10 GB changed something: volume %d, granted %d", volume(), granted())
	}

	// Lowering is free and gives nothing back.
	e.runSay(fullAdmin, "/volume v1 20")
	if volume() != 20*gib || granted() != 100*gib {
		t.Fatalf("/volume 20: volume %d, granted %d", volume(), granted())
	}
	// Unlimited is refused, and so is the typed edit with 0.
	mustContain(t, e.runSay(fullAdmin, "/volume v1 0"), "cannot be unlimited")
	e.runPress(fullAdmin, "c:ask:vol:"+id)
	mustContain(t, e.runSay(fullAdmin, "0"), "cannot be unlimited")
	if volume() != 20*gib {
		t.Fatalf("an unlimited volume was saved: %d", volume())
	}
	if e.b.pend.get(fullAdmin) == nil {
		t.Fatal("the prompt closed after a refusal; it should stay open for another try")
	}
	e.runPress(fullAdmin, "x:cancel")

	// A reset gives back what the client used (10 of 20 GB), which costs that.
	out = e.runPress(fullAdmin, "c:rsty:"+id)
	mustContain(t, out, notEnough)
	if e.client("v1").Up != 10*gib {
		t.Fatalf("a refused reset still reset the traffic: up = %d", e.client("v1").Up)
	}

	// The owner tops the total up, and the reset goes through.
	e.runPress(ownerID, "a:qa:42:50")
	if out := e.runPress(fullAdmin, "c:rsty:"+id); strings.Contains(out, "Failed") {
		t.Fatalf("the reset was refused after a top-up:\n%s", out)
	}
	if c := e.client("v1"); c.Up != 0 || c.Down != 0 || granted() != 110*gib {
		t.Fatalf("after the reset: up %d down %d granted %d", c.Up, c.Down, granted())
	}
}

func TestBulkWritesAreMetered(t *testing.T) {
	e := newAccessEnv(t)
	database.GetDB().Where("id > 0").Delete(&model.Client{})
	for _, n := range []string{"b1", "b2", "b3"} {
		seedClient(t, n, "", 50*gib, 10*gib)
	}
	giveQuota(t, fullAdmin, 40)
	ab := e.b.as(fullAdmin)
	granted := func() int64 { return quotaOf(t, fullAdmin).Granted }

	// +5 GB each is 15 GB.
	if n, err := ab.bulkApply("fa", "vol", "5"); err != nil || n != 3 {
		t.Fatalf("bulk +5: %d %v", n, err)
	}
	if granted() != 15*gib || e.client("b2").Volume != 55*gib {
		t.Fatalf("bulk +5: granted %d, b2 %d", granted(), e.client("b2").Volume)
	}
	// Subtracting costs nothing.
	if _, err := ab.bulkApply("fa", "vol", "-5"); err != nil || granted() != 15*gib {
		t.Fatalf("bulk -5: %v granted %d", err, granted())
	}
	// A reset of the three would give back 30 GB, and only 25 are left: the
	// whole batch is refused.
	_, err := ab.bulkApply("fa", "rst", "")
	if err == nil || !strings.Contains(err.Error(), notEnough) {
		t.Fatalf("bulk reset: %v", err)
	}
	for _, n := range []string{"b1", "b2", "b3"} {
		if e.client(n).Up != 10*gib {
			t.Fatalf("%s was reset by a refused batch", n)
		}
	}
	if granted() != 15*gib {
		t.Fatalf("a refused batch was charged: %d", granted())
	}
	// 30 GB more of room, and it goes through for 30 GB.
	giveQuota(t, fullAdmin, 50)
	if n, err := ab.bulkApply("fa", "rst", ""); err != nil || n != 3 {
		t.Fatalf("bulk reset: %d %v", n, err)
	}
	if granted() != 45*gib || e.client("b1").Up != 0 {
		t.Fatalf("after the bulk reset: granted %d, up %d", granted(), e.client("b1").Up)
	}
	// Disabling, deleting and the like are free.
	if _, err := ab.bulkApply("fa", "dis", ""); err != nil || granted() != 45*gib {
		t.Fatalf("bulk disable: %v granted %d", err, granted())
	}

	// /addbulk: 3 x 10 GB is 30 GB and only 5 are left.
	out := e.runSay(fullAdmin, "/addbulk pre 3 10 30")
	mustContain(t, out, notEnough, "needs 30.00 GiB", "you have 5.00 GiB")
	if e.exists("pre1") || granted() != 45*gib {
		t.Fatal("a refused bulk create made clients or charged")
	}
	giveQuota(t, fullAdmin, 100)
	e.runSay(fullAdmin, "/addbulk pre 3 10 30")
	if !e.exists("pre3") || granted() != 75*gib {
		t.Fatalf("bulk create: pre3 exists %v, granted %d", e.exists("pre3"), granted())
	}
	// Deleting gives nothing back.
	e.runSay(fullAdmin, "/del pre1")
	e.runPress(fullAdmin, "c:dely:"+itoa(int64(e.client("pre1").Id)))
	if granted() != 75*gib {
		t.Fatalf("deleting changed what was handed out: %d", granted())
	}
}

func TestUnlimitedAndAutoResetAreClosedToLimitedAdmins(t *testing.T) {
	e := newAccessEnv(t)
	giveQuota(t, fullAdmin, 100)
	c := seedClient(t, "ar1", "", 10*gib, 0)
	id := itoa(int64(c.Id))

	mustContain(t, e.runPress(fullAdmin, "c:ar:"+id), "auto reset")
	if e.client("ar1").AutoReset {
		t.Fatal("a limited admin switched auto reset on")
	}
	// The owner is not limited.
	e.runPress(ownerID, "c:ar:"+id)
	if !e.client("ar1").AutoReset {
		t.Fatal("the owner could not switch auto reset on")
	}
	e.runPress(ownerID, "c:ar:"+id)

	ab := e.b.as(fullAdmin)
	for text, want := range map[string]string{
		`{"name":"j1","volumeGB":5,"autoReset":true}`: "auto reset",
		`{"name":"j2"}`:                "cannot be unlimited",
		`{"name":"j3","volumeGB":500}`: notEnough,
	} {
		if _, err := ab.createClientFromJSON(text); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("new client %s: err = %v, want %q", text, err, want)
		}
	}
	if e.exists("j1") || e.exists("j2") || e.exists("j3") || quotaOf(t, fullAdmin).Granted != 0 {
		t.Fatal("a refused JSON client was created or charged")
	}
	if name, err := ab.createClientFromJSON(`{"name":"j4","volumeGB":5}`); err != nil || name != "j4" || quotaOf(t, fullAdmin).Granted != 5*gib {
		t.Fatalf("a JSON client within the limit: %q %v granted %d", name, err, quotaOf(t, fullAdmin).Granted)
	}

	// The same doors on an existing client, by JSON.
	if err := ab.applyClientJSON(c.Id, `{"volume":0}`); err == nil || !strings.Contains(err.Error(), "cannot be unlimited") {
		t.Errorf("JSON edit to unlimited: %v", err)
	}
	if err := ab.applyClientJSON(c.Id, `{"autoReset":true,"resetDays":3}`); err == nil || !strings.Contains(err.Error(), "auto reset") {
		t.Errorf("JSON edit to auto reset: %v", err)
	}
	if got := e.client("ar1"); got.Volume != 10*gib || got.AutoReset {
		t.Fatalf("a refused JSON edit changed the client: %+v", got)
	}
	if err := ab.applyClientJSON(c.Id, `{"volume":`+itoa(30*gib)+`}`); err != nil || quotaOf(t, fullAdmin).Granted != 25*gib {
		t.Fatalf("JSON edit within the limit: %v granted %d", err, quotaOf(t, fullAdmin).Granted)
	}

	// An unlimited client of the owner's can be managed, and limited, but not
	// the other way round.
	u := seedClient(t, "unl", "", 0, 0)
	if err := ab.setEnabled(u.Id, false); err != nil {
		t.Fatalf("disabling an unlimited client: %v", err)
	}
	if err := ab.setVolume(u.Id, 5*gib); err != nil || quotaOf(t, fullAdmin).Granted != 25*gib {
		t.Fatalf("limiting an unlimited client: %v granted %d", err, quotaOf(t, fullAdmin).Granted)
	}
	if err := ab.setVolume(u.Id, 0); err == nil {
		t.Fatal("an unlimited client was made again")
	}
}

func TestQuotaIsGivenBackWhenTheSaveFails(t *testing.T) {
	e := newAccessEnv(t)
	giveQuota(t, fullAdmin, 100)
	seedClient(t, "dup", "", 5*gib, 0)

	out := e.runSay(fullAdmin, "/add dup 10 30")
	mustContain(t, out, "Failed")
	if q := quotaOf(t, fullAdmin); q.Granted != 0 {
		t.Fatalf("a save that failed still cost %d", q.Granted)
	}
	// The same through a bulk create whose second name is taken.
	seedClient(t, "pk2", "", 5*gib, 0)
	mustContain(t, e.runSay(fullAdmin, "/addbulk pk 3 10 30"), "Failed")
	if q := quotaOf(t, fullAdmin); q.Granted != 0 || e.exists("pk1") || e.exists("pk3") {
		t.Fatalf("a failed bulk create left traces: %+v", q)
	}
	// A charge that is refunded never goes below zero.
	charge := &quotaCharge{id: fullAdmin, n: 40 * gib}
	charge.refund()
	var none *quotaCharge
	none.refund()
	if q := quotaOf(t, fullAdmin); q.Granted != 0 {
		t.Fatalf("granted = %d", q.Granted)
	}
}

func TestOwnerAndUnlimitedAdminsAreNotMetered(t *testing.T) {
	e := newAccessEnv(t)
	// A row for the owner changes nothing: the owner is never limited.
	if _, err := setQuotaTotal(ownerID, 1); err != nil {
		t.Fatal(err)
	}
	if out := e.runSay(ownerID, "/add big 5000 30"); strings.Contains(out, "Failed") || !e.exists("big") {
		t.Fatalf("the owner was limited:\n%s", out)
	}
	if q := quotaOf(t, ownerID); q.Granted != 0 {
		t.Fatalf("the owner was charged: %+v", q)
	}
	// An administrator without a row is unlimited, and gets no row.
	if out := e.runSay(fullAdmin, "/add free 5000 0"); strings.Contains(out, "Failed") || !e.exists("free") {
		t.Fatalf("an admin without a limit was refused:\n%s", out)
	}
	if !noQuota(t, fullAdmin) {
		t.Fatal("a limit appeared from nowhere")
	}
	// The bot's own copy (supervisor, alerts) writes freely.
	if ch, err := e.b.chargeVolume("new", cl(5000*gib, 0, 0)); ch != nil || err != nil {
		t.Fatalf("the supervisor's copy was charged: %+v %v", ch, err)
	}
}

func TestGroupLimitedAdminHasALimitToo(t *testing.T) {
	e := newAccessEnv(t)
	giveQuota(t, salesAdmin, 10)

	e.runSay(salesAdmin, "/add sa1 4 30")
	if c := e.client("sa1"); c.Group != salesGroup || c.Volume != 4*gib {
		t.Fatalf("sa1 = %+v", c)
	}
	mustContain(t, e.runSay(salesAdmin, "/add sa2 7 30"), notEnough, "you have 6.00 GiB")
	if e.exists("sa2") {
		t.Fatal("sa2 was created")
	}
	// s1 has 1 GiB; making it 5 GB costs 4.
	e.runSay(salesAdmin, "/volume s1 5")
	if q := quotaOf(t, salesAdmin); q.Granted != 8*gib {
		t.Fatalf("granted = %d", q.Granted)
	}
	// A client of another group is out of reach, and costs nothing.
	mustContain(t, e.runSay(salesAdmin, "/volume o1 500"), "No client")
	if q := quotaOf(t, salesAdmin); q.Granted != 8*gib || e.client("o1").Volume != 1*gib {
		t.Fatalf("an out-of-scope edit changed something: %+v", q)
	}
	// The wizard of a limited admin has no group step and no ∞.
	e.runPress(salesAdmin, "c:new")
	e.runPress(salesAdmin, "w:name:#ok")
	text := lastText(e.got(), "editMessageText")
	kb := keyboardOf(e.got())
	mustContain(t, text, "Volume you can still give: <b>2.00 GiB</b> of 10.00 GiB")
	if hasButtonData(kb, "w:vol:0") || !hasButtonData(kb, "w:vol:10") {
		t.Fatalf("the volume step offers the wrong buttons: %v", callbackData(kb))
	}
}

// ---- what the administrator sees ----

func TestLimitedAdminSeesTheirLimit(t *testing.T) {
	e := newAccessEnv(t)
	giveQuota(t, fullAdmin, 100)
	e.runSay(fullAdmin, "/add ali 30 30")
	line := "📦 Volume you can still give: <b>70.00 GiB</b> of 100.00 GiB"

	mustContain(t, e.runSay(fullAdmin, "/menu"), line)
	mustContain(t, e.runPress(fullAdmin, "m:menu"), line)
	mustContain(t, e.runPress(fullAdmin, "c:ls:a:0"), line)
	mustContain(t, e.runPress(fullAdmin, "c:ask:vol:"+itoa(int64(e.client("ali").Id))), line)
	e.runPress(fullAdmin, "x:cancel")
	mustContain(t, e.runPress(fullAdmin, "c:bk:fa:vol"), line)
	e.runPress(fullAdmin, "x:cancel")

	// So does a full administrator's Home page.
	mustContain(t, e.runPress(fullAdmin, "h:home"), line)

	// A group-limited admin sees it on their home page.
	giveQuota(t, salesAdmin, 10)
	mustContain(t, e.runPress(salesAdmin, "h:home"), "Volume you can still give: <b>10.00 GiB</b> of 10.00 GiB")

	// The owner and an unlimited admin see nothing about it, even when a row
	// of the owner's was left over from before they became the owner.
	giveQuota(t, ownerID, 5)
	for _, who := range []int64{ownerID, clientsOnly} {
		out := e.runSay(who, "/menu") + e.runPress(who, "c:ls:a:0") + e.runPress(who, "h:home")
		mustNotContain(t, out, "Volume you can still give")
	}
	if line := e.b.as(ownerID).quotaLine(); line != "" {
		t.Fatalf("the owner is shown a limit: %q", line)
	}
	// It follows the spending: after the last byte it says so.
	e.runSay(fullAdmin, "/add rest 70 30")
	mustContain(t, e.runSay(fullAdmin, "/menu"), "<b>0 B</b> of 100.00 GiB")
}

func TestWizardRefusesAVolumeOverTheLimitAtOnce(t *testing.T) {
	e := newAccessEnv(t)
	giveQuota(t, fullAdmin, 100)
	e.runSay(fullAdmin, "/add ali 30 30")

	e.runPress(fullAdmin, "c:new")
	e.runPress(fullAdmin, "w:name:#ok")
	e.runPress(fullAdmin, "w:grp:-")
	prompt := lastText(e.got(), "editMessageText")
	mustContain(t, prompt, "Pick the total volume", "Volume you can still give: <b>70.00 GiB</b>")
	if kb := keyboardOf(e.got()); hasButtonData(kb, "w:vol:0") || !hasButtonData(kb, "w:vol:50") {
		t.Fatalf("buttons: %v", callbackData(kb))
	}

	// 100 GB does not fit: the volume step stays, with its buttons, and says why.
	out := e.runPress(fullAdmin, "w:vol:100")
	mustContain(t, out, notEnough, "needs 100.00 GiB", "you have 70.00 GiB", "Pick the total volume")
	if kb := keyboardOf(e.got()); !hasButtonData(kb, "w:vol:50") {
		t.Fatalf("the presets are gone after a refusal: %v", callbackData(kb))
	}
	// So does a typed one, and ∞ (reached by a stale button).
	mustContain(t, e.runSay(fullAdmin, "70.5"), notEnough)
	mustContain(t, e.runPress(fullAdmin, "w:vol:0"), "cannot be unlimited")
	// The custom prompt does not promise unlimited either.
	custom := e.runPress(fullAdmin, "w:vol:#c")
	mustContain(t, custom, "Volume you can still give")
	mustNotContain(t, custom, "0 = unlimited")

	// A volume that fits goes on to the next question and, at the end, makes
	// the client.
	mustContain(t, e.runPress(fullAdmin, "w:vol:50"), "validity")
	e.runPress(fullAdmin, "w:days:30")
	e.runPress(fullAdmin, "w:ip:0")
	var made int64
	database.GetDB().Model(&model.Client{}).Where("volume = ?", 50*gib).Count(&made)
	if made != 1 || quotaOf(t, fullAdmin).Granted != 80*gib {
		t.Fatalf("the wizard's client was not made or charged: %d made, granted %d", made, quotaOf(t, fullAdmin).Granted)
	}
}

func TestQuotaMessagesFitAButtonPopUp(t *testing.T) {
	huge := int64(1023) * 1024 * 1024 * gib
	for _, lang := range []string{"en", "fa"} {
		b := newBot(botConfig{Lang: lang})
		for name, err := range map[string]error{
			"low":       b.errQuotaLow(huge, huge-gib),
			"unlimited": b.errQuotaUnlimited(),
			"autoReset": b.errQuotaAutoReset(),
		} {
			text := b.t("failed", esc(b.errText(err)))
			if n := utf8.RuneCountInString(text); n > 200 {
				t.Errorf("%s/%s is %d characters; Telegram drops a pop-up over 200", lang, name, n)
			}
		}
	}
	// In Persian the refusal is in Persian.
	e := newAccessEnv(t)
	e.b.cfg.Lang = "fa"
	giveQuota(t, fullAdmin, 1)
	mustContain(t, e.runSay(fullAdmin, "/add ali 5 30"), "حجم باقی‌مانده کافی نیست")
	mustContain(t, e.runSay(fullAdmin, "/menu"), "حجم قابل واگذاری")
}

// A limit that cannot be read is not "no limit": the write waits, it does not
// go through for free.
func TestAnUnreadableLimitStopsTheWrite(t *testing.T) {
	e := newAccessEnv(t)
	giveQuota(t, fullAdmin, 100)
	m := database.GetDB().Migrator()
	if err := m.RenameTable(&model.BotQuota{}, "bot_quotas_away"); err != nil {
		t.Fatal(err)
	}
	restored := false
	restore := func() {
		if !restored {
			restored = true
			if err := m.RenameTable("bot_quotas_away", &model.BotQuota{}); err != nil {
				t.Fatal(err)
			}
		}
	}
	defer restore()

	out := e.runSay(fullAdmin, "/add sneaky 50 30")
	mustContain(t, out, "Failed")
	if e.exists("sneaky") {
		t.Fatal("a client was made while the limit could not be read")
	}
	if ch, err := e.b.as(fullAdmin).chargeVolume("new", cl(1*gib, 0, 0)); ch != nil || err == nil {
		t.Fatalf("charge = %+v, %v", ch, err)
	}

	// Once it can be read again, the limit holds as before.
	restore()
	if out := e.runSay(fullAdmin, "/add fine 5 30"); strings.Contains(out, "Failed") || !e.exists("fine") {
		t.Fatalf("after the table came back:\n%s", out)
	}
	if q := quotaOf(t, fullAdmin); q.Granted != 5*gib {
		t.Fatalf("granted = %d", q.Granted)
	}
}
