package tgbot

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"

	"gorm.io/gorm"
)

// ---- helpers ----

// giveQuota gives the administrator a limit of gb GB, nothing used.
func giveQuota(t *testing.T, id int64, gb float64) {
	t.Helper()
	if _, err := setQuotaTotal(id, int64(gb*float64(gib))); err != nil {
		t.Fatal(err)
	}
}

// quotaRow is the stored row of a limit.
func quotaRow(t *testing.T, id int64) model.BotQuota {
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

// quota is a limit with what has been used of it, read the way the bot reads
// it: which also finds the clients that count.
func (e *scopeEnv) quota(id int64) quotaState {
	e.t.Helper()
	q, ok, err := e.b.quotaOf(id)
	if err != nil || !ok {
		e.t.Fatalf("no volume limit for %d (%v)", id, err)
	}
	return q
}

// consume is what the stats job does: the client has used n more bytes.
func (e *scopeEnv) consume(name string, n int64) {
	e.t.Helper()
	err := database.GetDB().Model(&model.Client{}).Where("name = ?", name).Update("up", gorm.Expr("up + ?", n)).Error
	if err != nil {
		e.t.Fatal(err)
	}
}

// counted is how many clients count for an administrator, read from the table
// without the catch-up a read of the limit does.
func counted(t *testing.T, id int64) (n int64) {
	t.Helper()
	if err := database.GetDB().Model(&model.BotQuotaClient{}).Where("tg_id = ?", id).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	return n
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

const usedUp = "volume limit is used up"

// ---- what a limit counts ----

// The complaint that led to this model: a 50 GB client of an administrator
// with 1000 GB took 10 GB, or 50 GB, off at once. Nothing is taken off when a
// client is made or its volume is set; what users really consume is.
func TestCreatingAClientDeductsNothing(t *testing.T) {
	e := newAccessEnv(t)
	giveQuota(t, fullAdmin, 1000)

	out := e.runSay(fullAdmin, "/add ali 50 30")
	if strings.Contains(out, "Failed") || !e.exists("ali") || e.client("ali").Volume != 50*gib {
		t.Fatalf("the client was not made:\n%s", out)
	}
	if q := e.quota(fullAdmin); q.Used != 0 || q.Left() != 1000*gib || q.Clients != 1 {
		t.Fatalf("after creating a 50 GB client: %+v left %d", q, q.Left())
	}
	mustContain(t, e.runSay(fullAdmin, "/menu"), "Remaining volume: <b>1000.00 GiB</b> of 1000.00 GiB")

	// Raising and setting volumes costs nothing either, however high.
	id := itoa(int64(e.client("ali").Id))
	e.runSay(fullAdmin, "/volume ali 200")
	e.runPress(fullAdmin, "c:gb50:"+id)
	e.runSay(fullAdmin, "/add big 900 30")
	if q := e.quota(fullAdmin); q.Used != 0 || e.client("ali").Volume != 250*gib {
		t.Fatalf("volume edits cost something: %+v, volume %d", q, e.client("ali").Volume)
	}

	// It is the traffic the users consume that is deducted.
	e.consume("ali", 20*gib)
	q := e.quota(fullAdmin)
	if q.Used != 20*gib || q.Left() != 980*gib || q.Clients != 2 {
		t.Fatalf("after 20 GB of traffic: used %d left %d clients %d", q.Used, q.Left(), q.Clients)
	}
	mustContain(t, e.runSay(fullAdmin, "/menu"), "Remaining volume: <b>980.00 GiB</b> of 1000.00 GiB")
	e.consume("big", 5*gib)
	if q := e.quota(fullAdmin); q.Used != 25*gib {
		t.Fatalf("used = %d after traffic on two clients", q.Used)
	}

	// The traffic of clients that are nobody's, or somebody else's, is not.
	e.consume("s1", 7*gib)
	e.runSay(ownerID, "/add theirs 10 30")
	e.consume("theirs", 3*gib)
	if q := e.quota(fullAdmin); q.Used != 25*gib || q.Clients != 2 {
		t.Fatalf("clients of others counted: %+v", q)
	}
}

// "A user with 50 GB consumes 20, we delete the client to issue a new link:
// exactly those 20 are deducted from the admin" — and a traffic reset gives
// nothing back either.
func TestDeletingAndResettingKeepWhatWasUsed(t *testing.T) {
	e := newAccessEnv(t)
	giveQuota(t, fullAdmin, 1000)
	e.runSay(fullAdmin, "/add ali 50 30")
	e.consume("ali", 20*gib)
	id := itoa(int64(e.client("ali").Id))

	// A reset starts the client's own counter again, but not the limit's.
	e.runSay(fullAdmin, "/reset ali")
	if out := e.runPress(fullAdmin, "c:rsty:"+id); strings.Contains(out, "Failed") {
		t.Fatalf("the reset failed:\n%s", out)
	}
	if c := e.client("ali"); c.Up != 0 || c.TotalUp != 20*gib {
		t.Fatalf("the reset did not fold the traffic: up %d, total %d", c.Up, c.TotalUp)
	}
	if q := e.quota(fullAdmin); q.Used != 20*gib {
		t.Fatalf("a reset gave back: used %d", q.Used)
	}
	e.consume("ali", 5*gib)
	if q := e.quota(fullAdmin); q.Used != 25*gib {
		t.Fatalf("used = %d after more traffic", q.Used)
	}

	// The client is deleted for a new link: its 25 stay on the admin.
	e.runSay(fullAdmin, "/del ali")
	if out := e.runPress(fullAdmin, "c:dely:"+id); strings.Contains(out, "Failed") || e.exists("ali") {
		t.Fatalf("the client was not deleted:\n%s", out)
	}
	q := e.quota(fullAdmin)
	if q.Used != 25*gib || q.Banked != 25*gib || q.Clients != 0 || q.Left() != 975*gib {
		t.Fatalf("after deleting: %+v left %d", q, q.Left())
	}
	// The new link counts what it uses itself.
	e.runSay(fullAdmin, "/add ali2 50 30")
	if q := e.quota(fullAdmin); q.Used != 25*gib || q.Clients != 1 {
		t.Fatalf("the new client cost something: %+v", q)
	}
	e.consume("ali2", 3*gib)
	if q := e.quota(fullAdmin); q.Used != 28*gib || q.Left() != 972*gib {
		t.Fatalf("used = %d, left = %d", q.Used, q.Left())
	}

	// The same through "delete the depleted clients", which deletes in bulk.
	e.runSay(fullAdmin, "/add dep1 1 30")
	e.consume("dep1", 2*gib)
	e.runSay(fullAdmin, "/add dep2 1 30")
	e.consume("dep2", 1*gib)
	if q := e.quota(fullAdmin); q.Used != 31*gib {
		t.Fatalf("used = %d", q.Used)
	}
	out := e.runPress(fullAdmin, "c:clean")
	mustContain(t, out, "dep1")
	e.runPress(fullAdmin, "c:cleany")
	if e.exists("dep1") || e.exists("dep2") {
		t.Fatal("the depleted clients are still there")
	}
	if q := e.quota(fullAdmin); q.Used != 31*gib || q.Clients != 1 {
		t.Fatalf("after the bulk delete: %+v", q)
	}
}

func TestAnAdminsEarlierClientsAreFoundInTheHistory(t *testing.T) {
	e := newAccessEnv(t)
	// The administrator made clients while they had no limit; so did the owner.
	e.runSay(fullAdmin, "/add early 50 30")
	e.runSay(fullAdmin, "/addbulk pre 2 10 30")
	e.runSay(ownerID, "/add byowner 50 30")
	e.consume("early", 5*gib)
	e.consume("pre1", 2*gib)
	e.consume("byowner", 9*gib)

	// The owner gives them a limit, and opens it.
	e.runPress(ownerID, "a:qa:42:100")
	q := e.quota(fullAdmin)
	if q.Used != 0 || q.Clients != 3 || !q.Adopted {
		t.Fatalf("the limit started with %+v", q)
	}
	if got := countedNames(t, fullAdmin); got != "early,pre1,pre2" {
		t.Fatalf("counted clients: %s", got)
	}
	// What they use from now on counts, the owner's client does not.
	e.consume("early", 1*gib)
	e.consume("pre2", 2*gib)
	e.consume("byowner", 4*gib)
	if q := e.quota(fullAdmin); q.Used != 3*gib {
		t.Fatalf("used = %d, want 3 GiB", q.Used)
	}
	// The history is read once: a client that only looks like theirs later is
	// not taken.
	seedClient(t, "ghost", "", 5*gib, 0)
	if err := database.GetDB().Create(&model.Changes{DateTime: 1, Actor: "telegram:42", Key: "clients", Action: "new", Obj: json.RawMessage(`{"name":"ghost"}`)}).Error; err != nil {
		t.Fatal(err)
	}
	if q := e.quota(fullAdmin); q.Clients != 3 {
		t.Fatalf("the history was read again: %+v", q)
	}
}

// countedNames lists the clients that count for an administrator.
func countedNames(t *testing.T, id int64) string {
	t.Helper()
	var names []string
	err := database.GetDB().Raw(`SELECT c.name FROM bot_quota_clients m JOIN clients c ON c.id = m.client_id
		WHERE m.tg_id = ? ORDER BY c.name`, id).Scan(&names).Error
	if err != nil {
		t.Fatal(err)
	}
	return strings.Join(names, ",")
}

func TestGroupLimitedAdminCountsTheirGroup(t *testing.T) {
	e := newAccessEnv(t)
	giveQuota(t, salesAdmin, 10)

	// The first look finds the clients of the group; what they used before the
	// limit is not charged (sd has used 100 bytes).
	q := e.quota(salesAdmin)
	if q.Used != 0 || q.Clients != 3 {
		t.Fatalf("a new limit of a group: %+v", q)
	}
	// Other groups, no group and the cluster group never count.
	for _, name := range []string{"o1", "od", "n1", "cl1"} {
		e.consume(name, 7*gib)
	}
	if q := e.quota(salesAdmin); q.Used != 0 || q.Clients != 3 {
		t.Fatalf("clients outside the group counted: %+v", q)
	}
	// What the clients of the group use counts, whoever made them.
	e.consume("s1", 2*gib)
	e.consume("s2", 1*gib)
	if q := e.quota(salesAdmin); q.Used != 3*gib {
		t.Fatalf("used = %d, want 3 GiB", q.Used)
	}
	// Their own new clients count from the first byte; so does a client the
	// owner adds to the group, once the group is looked at again.
	e.runSay(salesAdmin, "/add sa1 4 30")
	e.consume("sa1", 1*gib)
	seedClient(t, "s9", "SALES", 0, 0)
	q = e.quota(salesAdmin)
	if q.Used != 4*gib || q.Clients != 5 {
		t.Fatalf("after a new client of theirs and one of the owner's: %+v", q)
	}
	if left := q.Left(); left != 6*gib {
		t.Fatalf("left = %d", left)
	}

	// Used up: no new client, and no more volume; the group is still theirs.
	e.consume("s1", 20*gib)
	mustContain(t, e.runSay(salesAdmin, "/add sa2 4 30"), "Failed", usedUp)
	if e.exists("sa2") {
		t.Fatal("a client was made with nothing left")
	}
	mustContain(t, e.runSay(salesAdmin, "/volume s1 500"), usedUp)
	if out := e.runSay(salesAdmin, "/expiry s1 60"); strings.Contains(out, "Failed") {
		t.Fatalf("managing a client of the group was refused:\n%s", out)
	}
	// A client of another group is out of reach, as before.
	mustContain(t, e.runSay(salesAdmin, "/volume o1 500"), "No client")
	mustContain(t, e.runSay(salesAdmin, "/menu"), "<b>0 B</b> of 10.00 GiB")
}

// ---- when nothing is left ----

func TestUsedUpLimitStopsOnlyNewVolume(t *testing.T) {
	e := newAccessEnv(t)
	giveQuota(t, fullAdmin, 10)
	// Even a 50 GB client is allowed while something is left: it is the
	// traffic that counts.
	if out := e.runSay(fullAdmin, "/add a 50 30"); strings.Contains(out, "Failed") || !e.exists("a") {
		t.Fatalf("creating with volume left was refused:\n%s", out)
	}
	e.consume("a", 10*gib)
	id := itoa(int64(e.client("a").Id))
	if q := e.quota(fullAdmin); q.Left() != 0 {
		t.Fatalf("left = %d", q.Left())
	}
	// Beyond the limit is still "nothing left", never a negative number.
	e.consume("a", 5*gib)
	if q := e.quota(fullAdmin); q.Left() != 0 || q.Used != 15*gib {
		t.Fatalf("used %d, left %d", q.Used, q.Left())
	}
	mustContain(t, e.runSay(fullAdmin, "/menu"), "<b>0 B</b> of 10.00 GiB")

	// No new clients, any way.
	mustContain(t, e.runSay(fullAdmin, "/add b 1 30"), "Failed", usedUp, "10.00 GiB", "owner")
	mustContain(t, e.runSay(fullAdmin, "/addbulk pre 3 1 30"), "Failed", usedUp)
	if e.exists("b") || e.exists("pre1") {
		t.Fatal("a client was made with nothing left")
	}
	// The new-client wizard says so before its questions.
	out := e.runPress(fullAdmin, "c:new")
	mustContain(t, out, usedUp)
	if e.b.pend.get(fullAdmin) != nil {
		t.Fatal("the wizard was opened with nothing left")
	}

	// No more volume for the clients they have.
	ab := e.b.as(fullAdmin)
	for _, tc := range []struct {
		name string
		do   func() string
	}{
		{"/volume", func() string { return e.runSay(fullAdmin, "/volume a 100") }},
		{"+10 GB", func() string { return e.runPress(fullAdmin, "c:gb10:"+id) }},
		{"reset", func() string { return e.runPress(fullAdmin, "c:rsty:"+id) }},
		{"auto reset", func() string { return e.runPress(fullAdmin, "c:ar:"+id) }},
		{"unlimited", func() string { return e.runSay(fullAdmin, "/volume a 0") }},
		{"typed volume", func() string {
			e.runPress(fullAdmin, "c:ask:vol:"+id)
			return e.runSay(fullAdmin, "500")
		}},
	} {
		if out := tc.do(); !strings.Contains(out, usedUp) {
			t.Errorf("%s was not refused:\n%s", tc.name, out)
		}
		if e.b.pend.get(fullAdmin) != nil {
			e.runPress(fullAdmin, "x:cancel")
		}
	}
	if c := e.client("a"); c.Volume != 50*gib || c.Up != 15*gib || c.AutoReset {
		t.Fatalf("a refused change changed the client: %+v", c)
	}
	if _, err := ab.bulkApply("fa", "vol", "5"); err == nil || !strings.Contains(err.Error(), usedUp) {
		t.Fatalf("bulk +5 GB: %v", err)
	}
	if _, err := ab.bulkApply("fa", "rst", ""); err == nil || !strings.Contains(err.Error(), usedUp) {
		t.Fatalf("bulk reset: %v", err)
	}
	if _, err := ab.createClientFromJSON(`{"name":"j1","volumeGB":5}`); err == nil || !strings.Contains(err.Error(), usedUp) {
		t.Fatalf("a new client by JSON: %v", err)
	}
	if err := ab.applyClientJSON(e.client("a").Id, `{"volume":0}`); err == nil || !strings.Contains(err.Error(), usedUp) {
		t.Fatalf("JSON edit to unlimited: %v", err)
	}
	if err := ab.applyClientJSON(e.client("a").Id, `{"autoReset":true,"resetDays":3}`); err == nil || !strings.Contains(err.Error(), usedUp) {
		t.Fatalf("JSON edit to auto reset: %v", err)
	}
	if e.exists("j1") {
		t.Fatal("a refused JSON client was made")
	}

	// Everything else still works, and the clients they have are not cut off.
	if out := e.runPress(fullAdmin, "c:tog:"+id); strings.Contains(out, "Failed") || e.client("a").Enable {
		t.Fatalf("disabling was refused:\n%s", out)
	}
	if out := e.runPress(fullAdmin, "c:tog:"+id); strings.Contains(out, "Failed") || !e.client("a").Enable {
		t.Fatalf("enabling was refused:\n%s", out)
	}
	for _, say := range []string{"/expiry a 60", "/limitip a 2", "/volume a 20"} {
		if out := e.runSay(fullAdmin, say); strings.Contains(out, "Failed") {
			t.Fatalf("%s was refused:\n%s", say, out)
		}
	}
	if e.client("a").Volume != 20*gib {
		t.Fatalf("lowering the volume did not work: %d", e.client("a").Volume)
	}
	if err := ab.applyClientJSON(e.client("a").Id, `{"desc":"still mine"}`); err != nil {
		t.Fatalf("a JSON edit that gives nothing: %v", err)
	}
	if _, err := ab.bulkApply("fa", "vol", "-5"); err != nil {
		t.Fatalf("bulk -5 GB: %v", err)
	}
	if _, err := ab.bulkApply("fa", "dis", ""); err != nil {
		t.Fatalf("bulk disable: %v", err)
	}
	if _, err := ab.bulkApply("fa", "en", ""); err != nil {
		t.Fatalf("bulk enable: %v", err)
	}
	if q := e.quota(fullAdmin); q.Used != 15*gib {
		t.Fatalf("managing clients changed the usage: %d", q.Used)
	}

	// Deleting is fine too, and what the client used stays counted.
	e.runSay(fullAdmin, "/del a")
	if out := e.runPress(fullAdmin, "c:dely:"+id); strings.Contains(out, "Failed") || e.exists("a") {
		t.Fatalf("deleting was refused:\n%s", out)
	}
	if q := e.quota(fullAdmin); q.Used != 15*gib || q.Left() != 0 {
		t.Fatalf("after deleting: %+v", q)
	}

	// The owner raises the total, and they can create again.
	e.runPress(ownerID, "a:qa:42:50")
	if out := e.runSay(fullAdmin, "/add b 1 30"); strings.Contains(out, "Failed") || !e.exists("b") {
		t.Fatalf("after the top-up:\n%s", out)
	}
	mustContain(t, e.runSay(fullAdmin, "/menu"), "Remaining volume: <b>45.00 GiB</b> of 60.00 GiB")
}

// A limit is about what users consume, so unlimited clients and auto reset are
// no longer closed to a limited administrator: they are counted like any other.
func TestUnlimitedAndAutoResetAreAllowedAndCounted(t *testing.T) {
	e := newAccessEnv(t)
	giveQuota(t, fullAdmin, 100)

	if out := e.runSay(fullAdmin, "/add forever 0 30"); strings.Contains(out, "Failed") || e.client("forever").Volume != 0 {
		t.Fatalf("an unlimited client was refused:\n%s", out)
	}
	id := itoa(int64(e.client("forever").Id))
	if out := e.runPress(fullAdmin, "c:ar:"+id); strings.Contains(out, "Failed") || !e.client("forever").AutoReset {
		t.Fatalf("auto reset was refused:\n%s", out)
	}
	e.consume("forever", 30*gib)
	// An auto reset folds the traffic away each period; the limit keeps it.
	e.runPress(fullAdmin, "c:rsty:"+id)
	e.consume("forever", 10*gib)
	if q := e.quota(fullAdmin); q.Used != 40*gib || q.Left() != 60*gib {
		t.Fatalf("used = %d", q.Used)
	}

	// The wizard offers ∞ again, and says "0 = unlimited".
	e.runPress(fullAdmin, "c:new")
	e.runPress(fullAdmin, "w:name:#ok")
	e.runPress(fullAdmin, "w:grp:-")
	if kb := keyboardOf(e.got()); !hasButtonData(kb, "w:vol:0") || !hasButtonData(kb, "w:vol:300") {
		t.Fatalf("the volume step: %v", callbackData(kb))
	}
	mustContain(t, e.runPress(fullAdmin, "w:vol:#c"), "0 = unlimited")
	e.runPress(fullAdmin, "x:cancel")
	// A volume over what is left is no reason to refuse: only traffic counts.
	if out := e.runSay(fullAdmin, "/add huge 5000 30"); strings.Contains(out, "Failed") {
		t.Fatalf("a client bigger than the limit was refused:\n%s", out)
	}
	// The prompts for a volume do not talk about the limit.
	mustNotContain(t, e.runPress(fullAdmin, "c:ask:vol:"+id), "Remaining volume")
	e.runPress(fullAdmin, "x:cancel")
}

func TestOwnerAndAdminsWithoutALimitAreNotGated(t *testing.T) {
	e := newAccessEnv(t)
	// A row of the owner's changes nothing: they are never limited.
	giveQuota(t, ownerID, 0)
	if out := e.runSay(ownerID, "/add big 5000 30"); strings.Contains(out, "Failed") || !e.exists("big") {
		t.Fatalf("the owner was limited:\n%s", out)
	}
	if out := e.runPress(ownerID, "c:new"); strings.Contains(out, usedUp) {
		t.Fatalf("the owner's wizard was refused:\n%s", out)
	}
	e.runPress(ownerID, "x:cancel")
	// An administrator without a row is unlimited, and gets no row.
	if out := e.runSay(fullAdmin, "/add free 5000 0"); strings.Contains(out, "Failed") || !e.exists("free") {
		t.Fatalf("an admin without a limit was refused:\n%s", out)
	}
	if !noQuota(t, fullAdmin) || counted(t, fullAdmin) != 0 {
		t.Fatal("a limit appeared from nowhere")
	}
	// The bot's own copy (supervisor, alerts) writes freely.
	if err := e.b.quotaGate("new", model.Client{}); err != nil {
		t.Fatalf("the supervisor's copy was gated: %v", err)
	}
	// And the owner is shown nothing about a leftover row.
	if line := e.b.as(ownerID).quotaLine(); line != "" {
		t.Fatalf("the owner is shown a limit: %q", line)
	}
}

// ---- what the gate lets through ----

func TestWhatGivesVolume(t *testing.T) {
	c := func(volume, up, down int64) model.Client {
		return model.Client{Id: 1, Name: "x", Volume: volume, Up: up, Down: down}
	}
	auto := func(c model.Client) model.Client { c.AutoReset = true; return c }
	ptr := func(c model.Client) *model.Client { return &c }
	cases := []struct {
		name   string
		before *model.Client
		after  model.Client
		gives  bool
	}{
		{"a new client", nil, c(50*gib, 0, 0), true},
		{"a new unlimited client", nil, c(0, 0, 0), true},
		{"raising a volume", ptr(c(50*gib, 10*gib, 0)), c(80*gib, 10*gib, 0), true},
		{"lowering a volume", ptr(c(80*gib, 10*gib, 0)), c(50*gib, 10*gib, 0), false},
		{"saving without a change", ptr(c(50*gib, 10*gib, 0)), c(50*gib, 10*gib, 0), false},
		{"a reset of a client that used some", ptr(c(50*gib, 30*gib, 10*gib)), c(50*gib, 0, 0), true},
		{"a reset of a used-up client", ptr(c(50*gib, 60*gib, 0)), c(50*gib, 0, 0), true},
		{"a reset of a client that used nothing", ptr(c(50*gib, 0, 0)), c(50*gib, 0, 0), false},
		{"raising an overused client, not enough to use", ptr(c(50*gib, 60*gib, 0)), c(55*gib, 60*gib, 0), false},
		{"raising an overused client, enough to use", ptr(c(50*gib, 60*gib, 0)), c(80*gib, 60*gib, 0), true},
		{"making a client unlimited", ptr(c(50*gib, 0, 0)), c(0, 0, 0), true},
		{"limiting an unlimited client", ptr(c(0, 5*gib, 0)), c(50*gib, 5*gib, 0), false},
		{"an unlimited client stays unlimited", ptr(c(0, 5*gib, 0)), c(0, 0, 0), false},
		{"switching auto reset on", ptr(c(50*gib, 0, 0)), auto(c(50*gib, 0, 0)), true},
		{"an auto reset client, untouched", ptr(auto(c(50*gib, 0, 0))), auto(c(50*gib, 0, 0)), false},
		{"lowering an auto reset client", ptr(auto(c(50*gib, 0, 0))), auto(c(40*gib, 0, 0)), false},
		{"traffic that grew meanwhile is not a reset", ptr(c(50*gib, 10*gib, 0)), c(50*gib, 5*gib, 0), false},
		{"a client that does not exist yet", nil, c(0, 0, 0), true},
		{"a huge volume", ptr(c(50*gib, 0, 0)), c(math.MaxInt64, 0, 0), true},
	}
	for _, tc := range cases {
		if got := givesVolumeTo(tc.before, tc.after); got != tc.gives {
			t.Errorf("%s: gives = %v, want %v", tc.name, got, tc.gives)
		}
	}
}

func TestTheGateRefusesWhatItCannotJudgeOnlyWhenUsedUp(t *testing.T) {
	e := newAccessEnv(t)
	giveQuota(t, fullAdmin, 10)
	ab := e.b.as(fullAdmin)
	odd := []struct {
		act     string
		payload interface{}
	}{
		{"somethingnew", nil},
		{"new", "nonsense"},
		{"new", &model.Client{}},
		{"addbulk", "nonsense"},
		{"edit", model.Client{}},
		{"edit", (*model.Client)(nil)},
		{"editbulk", 5},
	}
	// With something left, whatever it is, it is no business of the limit.
	for _, tc := range odd {
		if err := ab.quotaGate(tc.act, tc.payload); err != nil {
			t.Errorf("%s with volume left: %v", tc.act, err)
		}
	}
	e.runSay(fullAdmin, "/add a 5 30")
	e.consume("a", 10*gib)
	for _, tc := range odd {
		if err := ab.quotaGate(tc.act, tc.payload); err == nil {
			t.Errorf("%s %T went through with nothing left", tc.act, tc.payload)
		}
	}
	for _, act := range []string{"del", "delbulk", "attachall"} {
		if err := ab.quotaGate(act, nil); err != nil {
			t.Errorf("%s was refused: %v", act, err)
		}
	}
	// An empty batch gives nothing.
	if err := ab.quotaGate("addbulk", []model.Client{}); err != nil {
		t.Errorf("an empty bulk create: %v", err)
	}
	if err := ab.quotaGate("editbulk", []model.Client{}); err != nil {
		t.Errorf("an empty bulk edit: %v", err)
	}
	// A client that does not exist cannot be told from a new one: refused.
	if err := ab.quotaGate("edit", &model.Client{Id: 9999, Volume: 5}); err == nil {
		t.Error("an edit of a client that is not there went through")
	}
}

// ---- the stored limits ----

func TestQuotaStore(t *testing.T) {
	testBot(t)
	if !noQuota(t, 42) {
		t.Fatal("a fresh database has a limit")
	}
	q, err := setQuotaTotal(42, 100*gib)
	if err != nil || q.Total != 100*gib || q.Banked != 0 || q.Adopted {
		t.Fatalf("set: %+v %v", q, err)
	}
	// Setting the total again keeps what has been banked.
	if err := database.GetDB().Model(&model.BotQuota{}).Where("tg_id = ?", 42).Update("banked", 30*gib).Error; err != nil {
		t.Fatal(err)
	}
	if q, _ = setQuotaTotal(42, 200*gib); q.Total != 200*gib || q.Banked != 30*gib {
		t.Fatalf("the total changed what was used: %+v", q)
	}
	// Relative changes clamp at both ends.
	if q, found, err := shiftQuotaTotal(42, -500*gib); !found || err != nil || q.Total != 0 || q.Banked != 30*gib {
		t.Fatalf("shift below zero: %+v %v %v", q, found, err)
	}
	if q, _, _ := shiftQuotaTotal(42, math.MaxInt64/4); q.Total != quotaCap {
		t.Fatalf("shift above the cap: %+v", q)
	}
	if _, found, err := shiftQuotaTotal(43, gib); found || err != nil {
		t.Fatalf("shift of nobody: %v %v", found, err)
	}
	// A total under what has been used leaves nothing, not less than nothing.
	q, _ = setQuotaTotal(42, 10*gib)
	for used, want := range map[int64]int64{0: 10 * gib, 4 * gib: 6 * gib, 10 * gib: 0, 30 * gib: 0, math.MaxInt64: 0} {
		if got := (quotaState{BotQuota: q, Used: used}).Left(); got != want {
			t.Fatalf("a total of %d with %d used leaves %d, want %d", q.Total, used, got, want)
		}
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
}

func TestANewLimitStartsFromNothingAndRemovingOneForgetsTheClients(t *testing.T) {
	e := newAccessEnv(t)
	giveQuota(t, fullAdmin, 100)
	e.runSay(fullAdmin, "/add a 5 30")
	e.consume("a", 4*gib)
	if counted(t, fullAdmin) != 1 || e.quota(fullAdmin).Used != 4*gib {
		t.Fatal("the client does not count")
	}

	// Removing the limit takes the clients with it...
	if err := deleteQuota(fullAdmin); err != nil {
		t.Fatal(err)
	}
	if !noQuota(t, fullAdmin) || counted(t, fullAdmin) != 0 {
		t.Fatal("the limit or its clients are still there")
	}
	// ...and a new one counts from now, not from what the old one saw.
	giveQuota(t, fullAdmin, 100)
	e.consume("a", 1*gib)
	q := e.quota(fullAdmin)
	if q.Used != 0 || q.Banked != 0 {
		t.Fatalf("the new limit started with %+v", q)
	}
	// Setting the total of an existing limit keeps its clients.
	e.consume("a", 1*gib)
	giveQuota(t, fullAdmin, 200)
	if q := e.quota(fullAdmin); q.Used != 1*gib {
		t.Fatalf("changing the total changed the usage: %+v", q)
	}
	// Rows a crash left behind do not leak into a new limit.
	if err := deleteQuota(fullAdmin); err != nil {
		t.Fatal(err)
	}
	stray := e.client("a").Id
	if err := database.GetDB().Create(&model.BotQuotaClient{ClientId: stray, TgId: fullAdmin, Base: 0}).Error; err != nil {
		t.Fatal(err)
	}
	giveQuota(t, fullAdmin, 100)
	if counted(t, fullAdmin) != 0 {
		t.Fatal("a stray row came back with the new limit")
	}
}

// ---- what the administrator sees ----

func TestLimitedAdminSeesTheirLimit(t *testing.T) {
	e := newAccessEnv(t)
	giveQuota(t, fullAdmin, 100)
	e.runSay(fullAdmin, "/add ali 30 30")
	e.consume("ali", 30*gib)
	line := "📦 Remaining volume: <b>70.00 GiB</b> of 100.00 GiB"

	mustContain(t, e.runSay(fullAdmin, "/menu"), line)
	mustContain(t, e.runPress(fullAdmin, "m:menu"), line)
	mustContain(t, e.runPress(fullAdmin, "c:ls:a:0"), line)
	// So does a full administrator's Home page.
	mustContain(t, e.runPress(fullAdmin, "h:home"), line)

	// A group-limited admin sees it on their home page.
	giveQuota(t, salesAdmin, 10)
	mustContain(t, e.runPress(salesAdmin, "h:home"), "Remaining volume: <b>10.00 GiB</b> of 10.00 GiB")

	// The owner and an unlimited admin see nothing about it, even when a row
	// of the owner's was left over from before they became the owner.
	giveQuota(t, ownerID, 5)
	for _, who := range []int64{ownerID, clientsOnly} {
		out := e.runSay(who, "/menu") + e.runPress(who, "c:ls:a:0") + e.runPress(who, "h:home")
		mustNotContain(t, out, "Remaining volume")
	}
	// It follows the traffic, and says so after the last byte.
	e.consume("ali", 100*gib)
	mustContain(t, e.runSay(fullAdmin, "/menu"), "<b>0 B</b> of 100.00 GiB")
}

func TestQuotaMessagesFitAButtonPopUp(t *testing.T) {
	huge := quotaState{BotQuota: model.BotQuota{Total: int64(1023) * 1024 * 1024 * gib}, Used: int64(1023) * 1024 * 1024 * gib}
	for _, lang := range []string{"en", "fa"} {
		b := newBot(botConfig{Lang: lang})
		text := b.t("failed", esc(b.errText(b.errQuotaUsedUp(huge))))
		if n := utf8.RuneCountInString(text); n > 200 {
			t.Errorf("%s is %d characters; Telegram drops a pop-up over 200", lang, n)
		}
	}
	// In Persian the refusal and the line are in Persian.
	e := newAccessEnv(t)
	e.b.cfg.Lang = "fa"
	giveQuota(t, fullAdmin, 1)
	mustContain(t, e.runSay(fullAdmin, "/menu"), "حجم باقی‌مانده: <b>1.00 GiB</b> از 1.00 GiB")
	e.runSay(fullAdmin, "/add ali 5 30")
	e.consume("ali", 2*gib)
	mustContain(t, e.runSay(fullAdmin, "/add bob 5 30"), "سقف حجم شما تمام شده است")
}

// A limit that cannot be read is not "no limit": the write waits, it does not
// go through for free.
func TestAnUnreadableLimitStopsTheWrite(t *testing.T) {
	for _, table := range []interface{}{&model.BotQuota{}, &model.BotQuotaClient{}} {
		e := newAccessEnv(t)
		giveQuota(t, fullAdmin, 100)
		e.runSay(fullAdmin, "/add base 5 30")
		m := database.GetDB().Migrator()
		if err := m.RenameTable(table, "table_away"); err != nil {
			t.Fatal(err)
		}
		restored := false
		restore := func() {
			if !restored {
				restored = true
				if err := m.RenameTable("table_away", table); err != nil {
					t.Fatal(err)
				}
			}
		}

		out := e.runSay(fullAdmin, "/add sneaky 50 30")
		mustContain(t, out, "Failed")
		if e.exists("sneaky") {
			restore()
			t.Fatalf("%T: a client was made while the limit could not be read", table)
		}
		// With nothing used the write would be allowed; unreadable is an error.
		if err := e.b.as(fullAdmin).quotaGate("new", model.Client{}); err == nil {
			restore()
			t.Fatalf("%T: the gate let a write through while the limit could not be read", table)
		}

		// Once it can be read again, the limit holds as before.
		restore()
		if out := e.runSay(fullAdmin, "/add fine 5 30"); strings.Contains(out, "Failed") || !e.exists("fine") {
			t.Fatalf("%T: after the table came back:\n%s", table, out)
		}
	}
}

// A JSON edit cannot zero the counters that the limit is worked out from.
func TestJSONCannotWipeWhatWasUsed(t *testing.T) {
	e := newAccessEnv(t)
	giveQuota(t, fullAdmin, 100)
	e.runSay(fullAdmin, "/add ali 30 30")
	e.consume("ali", 12*gib)
	ab := e.b.as(fullAdmin)
	id := e.client("ali").Id
	for _, text := range []string{
		`{"up":0,"down":0,"totalUp":0,"totalDown":0}`,
		`{"up":-5000,"down":-5000,"totalUp":-1,"totalDown":-1}`,
		`{"id":9999,"createdAt":1,"onlineAt":1,"tgId":5}`,
	} {
		if err := ab.applyClientJSON(id, text); err != nil {
			t.Fatalf("%s: %v", text, err)
		}
		if q := e.quota(fullAdmin); q.Used != 12*gib {
			t.Fatalf("%s changed the usage to %d", text, q.Used)
		}
	}
	// New clients by JSON start from nothing whatever the JSON says.
	if _, err := ab.createClientFromJSON(`{"name":"j1","volumeGB":5,"up":999999,"down":999999,"totalUp":999999,"totalDown":999999}`); err != nil {
		t.Fatal(err)
	}
	if c := e.client("j1"); c.Up != 0 || c.Down != 0 || c.TotalUp != 0 || c.TotalDown != 0 {
		t.Fatalf("a client was made with traffic: %+v", c)
	}
	if q := e.quota(fullAdmin); q.Used != 12*gib || q.Clients != 2 {
		t.Fatalf("usage after a new JSON client: %+v", q)
	}
}
