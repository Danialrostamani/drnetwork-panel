package service

import (
	"encoding/json"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/core"
	"github.com/Danialrostamani/drnetwork-panel/database/model"

	"gorm.io/gorm"
)

// ---- helpers ----

// limited gives an administrator a volume limit.
func limited(t *testing.T, db *gorm.DB, tg int64) {
	t.Helper()
	if err := db.Create(&model.BotQuota{TgId: tg, Total: 1000}).Error; err != nil {
		t.Fatal(err)
	}
}

// usage is what BotQuotaUsage says.
func usage(t *testing.T, db *gorm.DB, tg int64) (used int64, live int) {
	t.Helper()
	used, live, err := BotQuotaUsage(db, tg)
	if err != nil {
		t.Fatal(err)
	}
	return used, live
}

func wantUsage(t *testing.T, db *gorm.DB, tg, wantUsed int64, wantLive int) {
	t.Helper()
	if used, live := usage(t, db, tg); used != wantUsed || live != wantLive {
		t.Fatalf("usage of %d = %d over %d clients, want %d over %d", tg, used, live, wantUsed, wantLive)
	}
}

// traffic sets what a client has used so far in its current cycle.
func traffic(t *testing.T, db *gorm.DB, id uint, up, down int64) {
	t.Helper()
	if err := db.Model(&model.Client{}).Where("id = ?", id).Updates(map[string]interface{}{"up": up, "down": down}).Error; err != nil {
		t.Fatal(err)
	}
}

func banked(t *testing.T, db *gorm.DB, tg int64) int64 {
	t.Helper()
	var q model.BotQuota
	if err := db.Where("tg_id = ?", tg).First(&q).Error; err != nil {
		t.Fatal(err)
	}
	return q.Banked
}

func mapped(t *testing.T, db *gorm.DB, id uint) (m model.BotQuotaClient, ok bool) {
	t.Helper()
	res := db.Where("client_id = ?", id).Limit(1).Find(&m)
	if res.Error != nil {
		t.Fatal(res.Error)
	}
	return m, res.RowsAffected == 1
}

func newClient(t *testing.T, db *gorm.DB, name string) *model.Client {
	t.Helper()
	return createClient(t, db, &model.Client{Name: name, Enable: true})
}

func ids(list ...*model.Client) json.RawMessage {
	out := make([]uint, 0, len(list))
	for _, c := range list {
		out = append(out, c.Id)
	}
	raw, _ := json.Marshal(out)
	return raw
}

// ---- what counts ----

func TestUsageIsWhatCountedClientsUse(t *testing.T) {
	db := clientTestDB(t)
	limited(t, db, 42)
	a, b, other := newClient(t, db, "a"), newClient(t, db, "b"), newClient(t, db, "other")
	wantUsage(t, db, 42, 0, 0)

	// Clients that have not been attached cost nothing, whatever they use.
	traffic(t, db, a.Id, 100, 50)
	wantUsage(t, db, 42, 0, 0)

	// A client created for the administrator counts from its first byte.
	if err := BotQuotaAttach(db, 42, []string{"a", "b"}); err != nil {
		t.Fatal(err)
	}
	wantUsage(t, db, 42, 150, 2)
	traffic(t, db, b.Id, 7, 3)
	wantUsage(t, db, 42, 160, 2)

	// A client of somebody else, or of nobody, does not count.
	traffic(t, db, other.Id, 5000, 5000)
	wantUsage(t, db, 42, 160, 2)

	// A traffic reset folds the cycle into the lifetime totals and gives
	// nothing back: the panel's edit with no traffic, and the periodic job.
	edit, _ := json.Marshal(model.Client{Id: a.Id, Name: "a", Enable: true, Config: json.RawMessage(`{}`), Inbounds: json.RawMessage(`[]`), Links: json.RawMessage(`[]`)})
	if _, err := (&ClientService{}).Save(db, "edit", edit, "example.com"); err != nil {
		t.Fatal(err)
	}
	if got := reload(t, db, a.Id); got.Up != 0 || got.Down != 0 || got.TotalUp != 100 || got.TotalDown != 50 {
		t.Fatalf("the reset did not fold the traffic: %+v", got)
	}
	wantUsage(t, db, 42, 160, 2)
	traffic(t, db, a.Id, 10, 0)
	wantUsage(t, db, 42, 170, 2)

	if err := db.Model(&model.Client{}).Where("id = ?", b.Id).
		Updates(map[string]interface{}{"auto_reset": true, "reset_days": 30, "next_reset": 50}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := (&ClientService{}).ResetClients(db, 100); err != nil {
		t.Fatal(err)
	}
	if got := reload(t, db, b.Id); got.Up != 0 || got.TotalUp != 7 || got.TotalDown != 3 {
		t.Fatalf("the periodic reset did not fold the traffic: %+v", got)
	}
	wantUsage(t, db, 42, 170, 2)

	// Other administrators have their own count.
	limited(t, db, 43)
	wantUsage(t, db, 43, 0, 0)
}

func TestUsageIgnoresOddCounters(t *testing.T) {
	db := clientTestDB(t)
	limited(t, db, 42)
	a := newClient(t, db, "a")
	if err := BotQuotaAttach(db, 42, []string{"a"}); err != nil {
		t.Fatal(err)
	}
	// Counters that went below where the client started never give back.
	if err := db.Model(&model.BotQuotaClient{}).Where("client_id = ?", a.Id).Update("base", 500).Error; err != nil {
		t.Fatal(err)
	}
	traffic(t, db, a.Id, 100, 0)
	wantUsage(t, db, 42, 0, 1)
	traffic(t, db, a.Id, 600, 0)
	wantUsage(t, db, 42, 100, 1)
	// A counter below zero never counts for less than nothing.
	if err := db.Model(&model.BotQuotaClient{}).Where("client_id = ?", a.Id).Update("base", 0).Error; err != nil {
		t.Fatal(err)
	}
	traffic(t, db, a.Id, -900, 0)
	wantUsage(t, db, 42, 0, 1)

	// A row for a client that is gone counts for nothing and is not alive.
	b := newClient(t, db, "b")
	if err := BotQuotaAttach(db, 42, []string{"b"}); err != nil {
		t.Fatal(err)
	}
	traffic(t, db, b.Id, 40, 0)
	wantUsage(t, db, 42, 40, 2)
	if err := db.Delete(&model.Client{}, b.Id).Error; err != nil {
		t.Fatal(err)
	}
	wantUsage(t, db, 42, 0, 1)

	// Somebody without a limit has no usage.
	wantUsage(t, db, 99, 0, 0)
}

func TestAttachKeepsClientsOfTheirFirstAdministrator(t *testing.T) {
	db := clientTestDB(t)
	limited(t, db, 42)
	limited(t, db, 43)
	a := newClient(t, db, "a")
	traffic(t, db, a.Id, 30, 0)
	if err := BotQuotaAttach(db, 42, []string{"a"}); err != nil {
		t.Fatal(err)
	}
	// Attaching again, for somebody else or for the same one, changes nothing.
	for _, tg := range []int64{43, 42} {
		if err := BotQuotaAttach(db, tg, []string{"a", " a "}); err != nil {
			t.Fatal(err)
		}
	}
	if m, ok := mapped(t, db, a.Id); !ok || m.TgId != 42 || m.Base != 0 {
		t.Fatalf("mapping = %+v (%v)", m, ok)
	}
	wantUsage(t, db, 42, 30, 1)
	wantUsage(t, db, 43, 0, 0)

	// Blank and unknown names are nothing, and a long list is not a problem.
	if err := BotQuotaAttach(db, 42, []string{"", "  ", "nobody"}); err != nil {
		t.Fatal(err)
	}
	var names []string
	for i := 0; i < 1000; i++ {
		name := fmt.Sprintf("bulk%d", i)
		newClient(t, db, name)
		names = append(names, name)
	}
	if err := BotQuotaAttach(db, 43, names); err != nil {
		t.Fatal(err)
	}
	wantUsage(t, db, 43, 0, 1000)
}

// ---- deleting ----

func TestDeletingAClientKeepsWhatItUsed(t *testing.T) {
	db := clientTestDB(t)
	limited(t, db, 42)
	svc := &ClientService{}
	a, keep := newClient(t, db, "a"), newClient(t, db, "keep")
	if err := BotQuotaAttach(db, 42, []string{"a", "keep"}); err != nil {
		t.Fatal(err)
	}
	traffic(t, db, a.Id, 15, 5)
	traffic(t, db, keep.Id, 3, 0)
	wantUsage(t, db, 42, 23, 2)

	// "50 GB for 20 used, deleted to issue a new link": the 20 stay counted.
	if _, err := svc.Save(db, "del", json.RawMessage(strconv.Itoa(int(a.Id))), "example.com"); err != nil {
		t.Fatal(err)
	}
	if _, ok := mapped(t, db, a.Id); ok {
		t.Fatal("a deleted client still has a mapping")
	}
	if banked(t, db, 42) != 20 {
		t.Fatalf("banked = %d, want 20", banked(t, db, 42))
	}
	wantUsage(t, db, 42, 23, 1)

	// The new link counts only what it uses itself.
	fresh := newClient(t, db, "a")
	if err := BotQuotaAttach(db, 42, []string{"a"}); err != nil {
		t.Fatal(err)
	}
	wantUsage(t, db, 42, 23, 2)
	traffic(t, db, fresh.Id, 4, 0)
	wantUsage(t, db, 42, 27, 2)

	// And so on: deleting that one banks its own 4.
	if _, err := svc.Save(db, "del", json.RawMessage(strconv.Itoa(int(fresh.Id))), "example.com"); err != nil {
		t.Fatal(err)
	}
	wantUsage(t, db, 42, 27, 1)
	if banked(t, db, 42) != 24 {
		t.Fatalf("banked = %d, want 24", banked(t, db, 42))
	}
}

func TestDeletingSeveralClientsKeepsWhatEachUsed(t *testing.T) {
	db := clientTestDB(t)
	limited(t, db, 42)
	limited(t, db, 43)
	svc := &ClientService{}
	a, b, c, free := newClient(t, db, "a"), newClient(t, db, "b"), newClient(t, db, "c"), newClient(t, db, "free")
	if err := BotQuotaAttach(db, 42, []string{"a", "b"}); err != nil {
		t.Fatal(err)
	}
	if err := BotQuotaAttach(db, 43, []string{"c"}); err != nil {
		t.Fatal(err)
	}
	traffic(t, db, a.Id, 10, 0)
	traffic(t, db, b.Id, 20, 0)
	traffic(t, db, c.Id, 400, 0)
	traffic(t, db, free.Id, 9000, 0)
	// What a client used before it counted is not billed when it is deleted
	// either.
	if err := db.Model(&model.BotQuotaClient{}).Where("client_id = ?", b.Id).Update("base", 15).Error; err != nil {
		t.Fatal(err)
	}
	// And a client whose counters went below where it started banks nothing,
	// instead of taking away what the others used.
	low := newClient(t, db, "low")
	if err := BotQuotaAttach(db, 42, []string{"low"}); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.BotQuotaClient{}).Where("client_id = ?", low.Id).Update("base", 500).Error; err != nil {
		t.Fatal(err)
	}
	traffic(t, db, low.Id, 100, 0)

	if _, err := svc.Save(db, "delbulk", ids(a, b, c, free, low), "example.com"); err != nil {
		t.Fatal(err)
	}
	if got := banked(t, db, 42); got != 15 {
		t.Fatalf("banked of 42 = %d, want 10 + 5 + 0", got)
	}
	if got := banked(t, db, 43); got != 400 {
		t.Fatalf("banked of 43 = %d, want 400", got)
	}
	wantUsage(t, db, 42, 15, 0)
	wantUsage(t, db, 43, 400, 0)
	var left int64
	db.Model(&model.BotQuotaClient{}).Count(&left)
	if left != 0 {
		t.Fatalf("%d mappings of deleted clients are left", left)
	}
}

func TestDeletingWorksWithoutAnyLimit(t *testing.T) {
	db := clientTestDB(t)
	svc := &ClientService{}
	a, b := newClient(t, db, "a"), newClient(t, db, "b")
	traffic(t, db, a.Id, 10, 0)
	if _, err := svc.Save(db, "del", json.RawMessage(strconv.Itoa(int(a.Id))), "example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Save(db, "delbulk", ids(b), "example.com"); err != nil {
		t.Fatal(err)
	}
	// A client still mapped to somebody whose limit was removed goes quietly.
	c := newClient(t, db, "c")
	if err := db.Create(&model.BotQuotaClient{ClientId: c.Id, TgId: 77, Base: 0}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Save(db, "del", json.RawMessage(strconv.Itoa(int(c.Id))), "example.com"); err != nil {
		t.Fatal(err)
	}
	if _, ok := mapped(t, db, c.Id); ok {
		t.Fatal("the mapping of a deleted client was left")
	}
}

// A failed deletion must not bank anything: the two statements are one
// transaction.
func TestAFailedDeletionBanksNothing(t *testing.T) {
	db := clientTestDB(t)
	limited(t, db, 42)
	a := newClient(t, db, "a")
	if err := BotQuotaAttach(db, 42, []string{"a"}); err != nil {
		t.Fatal(err)
	}
	traffic(t, db, a.Id, 50, 0)
	tx := db.Begin()
	if _, err := (&ClientService{}).Save(tx, "delbulk", json.RawMessage(fmt.Sprintf("[%d, 9999]", a.Id)), "example.com"); err == nil {
		t.Fatal("deleting a client that does not exist worked")
	}
	tx.Rollback()
	if banked(t, db, 42) != 0 {
		t.Fatalf("banked = %d after a failed deletion", banked(t, db, 42))
	}
	wantUsage(t, db, 42, 50, 1)
}

// ---- starting over ----

func TestRestartCountsFromNow(t *testing.T) {
	db := clientTestDB(t)
	limited(t, db, 42)
	limited(t, db, 43)
	a, b, c := newClient(t, db, "a"), newClient(t, db, "b"), newClient(t, db, "c")
	if err := BotQuotaAttach(db, 42, []string{"a", "b"}); err != nil {
		t.Fatal(err)
	}
	if err := BotQuotaAttach(db, 43, []string{"c"}); err != nil {
		t.Fatal(err)
	}
	traffic(t, db, a.Id, 100, 0)
	traffic(t, db, b.Id, 50, 0)
	traffic(t, db, c.Id, 70, 0)
	if _, err := (&ClientService{}).Save(db, "del", json.RawMessage(strconv.Itoa(int(b.Id))), "example.com"); err != nil {
		t.Fatal(err)
	}
	wantUsage(t, db, 42, 150, 1)

	if err := BotQuotaRestart(db, 42); err != nil {
		t.Fatal(err)
	}
	wantUsage(t, db, 42, 0, 1)
	if banked(t, db, 42) != 0 {
		t.Fatal("what deleted clients used was not forgotten")
	}
	// The clients keep counting, from where they are now, after a reset too.
	traffic(t, db, a.Id, 130, 0)
	wantUsage(t, db, 42, 30, 1)
	// Nobody else is touched.
	wantUsage(t, db, 43, 70, 1)

	// A row of a client that is gone is dropped.
	if err := db.Create(&model.BotQuotaClient{ClientId: 9999, TgId: 42}).Error; err != nil {
		t.Fatal(err)
	}
	if err := BotQuotaRestart(db, 42); err != nil {
		t.Fatal(err)
	}
	if _, ok := mapped(t, db, 9999); ok {
		t.Fatal("the row of a client that does not exist was kept")
	}
	wantUsage(t, db, 42, 0, 1)

	// Forgetting takes the clients away altogether.
	if err := BotQuotaForget(db, 42); err != nil {
		t.Fatal(err)
	}
	wantUsage(t, db, 42, 0, 0)
	wantUsage(t, db, 43, 70, 1)
}

// ---- finding the clients of an administrator ----

func TestAdoptGroupTakesTheClientsOfTheGroup(t *testing.T) {
	db := clientTestDB(t)
	limited(t, db, 77)
	limited(t, db, 78)
	mk := func(name, group string, used int64) *model.Client {
		c := createClient(t, db, &model.Client{Name: name, Enable: true, Group: group, Up: used})
		return c
	}
	s1, s2, s3 := mk("s1", "Sales", 100), mk("s2", " sales ", 0), mk("s3", "SALES", 5)
	o1, none, cluster := mk("o1", "Other", 900), mk("n1", "", 900), mk("cl", ClusterGroup, 900)
	taken := mk("taken", "Sales", 40)
	if err := BotQuotaAttach(db, 78, []string{"taken"}); err != nil {
		t.Fatal(err)
	}

	if err := BotQuotaAdoptGroup(db, 77, "Sales"); err != nil {
		t.Fatal(err)
	}
	for _, c := range []*model.Client{s1, s2, s3} {
		if m, ok := mapped(t, db, c.Id); !ok || m.TgId != 77 {
			t.Errorf("%s was not adopted: %+v %v", c.Name, m, ok)
		}
	}
	for _, c := range []*model.Client{o1, none, cluster} {
		if _, ok := mapped(t, db, c.Id); ok {
			t.Errorf("%s was adopted", c.Name)
		}
	}
	if m, _ := mapped(t, db, taken.Id); m.TgId != 78 {
		t.Errorf("a client of another administrator was taken: %+v", m)
	}
	// What the clients used before is not charged...
	wantUsage(t, db, 77, 0, 3)
	// ...what they use from now on is, and adopting again does not forget it.
	traffic(t, db, s1.Id, 160, 0)
	if err := BotQuotaAdoptGroup(db, 77, "sales"); err != nil {
		t.Fatal(err)
	}
	wantUsage(t, db, 77, 60, 3)

	// A group that is nothing, or the cluster group, adopts nothing.
	for _, g := range []string{"", "   ", ClusterGroup, "cluster-GROUP"} {
		if err := BotQuotaAdoptGroup(db, 79, g); err != nil {
			t.Fatal(err)
		}
	}
	wantUsage(t, db, 79, 0, 0)
}

func TestAdoptCreatedFindsTheClientsInTheHistory(t *testing.T) {
	db := clientTestDB(t)
	limited(t, db, 42)
	const now = int64(1_700_000_000)
	history := func(actor, key, act, obj string, at int64) {
		t.Helper()
		if err := db.Create(&model.Changes{DateTime: at, Actor: actor, Key: key, Action: act, Obj: json.RawMessage(obj)}).Error; err != nil {
			t.Fatal(err)
		}
	}
	mk := func(name string, created, used int64) *model.Client {
		return createClient(t, db, &model.Client{Name: name, Enable: true, CreatedAt: created, Up: used})
	}
	one := mk("one", now+2, 100)
	bulk1, bulk2 := mk("bulk1", now+1000, 50), mk("bulk2", now+1000, 0)
	reused := mk("reused", now+90000, 800)  // the name came back much later, from somebody else
	early := mk("early", now-90000, 700)    // the client is much older than the entry that names it
	old := mk("old", 0, 30)                 // from before creation times were kept
	theirs := mk("theirs", now+5, 10)       // created by another administrator
	renamed := mk("renamed-later", now, 10) // the history knows it by another name
	edited := mk("edited", now+5, 10)       // an edit is not a creation
	panel := mk("panel", now+5, 10)         // created in the panel

	history("telegram:42", "clients", "new", `{"name":"one","volume":5}`, now)
	history("telegram:42", "clients", "addbulk", `[{"name":"bulk1"},{"name":"bulk2"},{"name":"gone"}]`, now+1001)
	history("telegram:42", "clients", "new", `{"name":"reused"}`, now)
	history("telegram:42", "clients", "new", `{"name":"early"}`, now)
	history("telegram:42", "clients", "new", `{"name":"old"}`, now)
	history("telegram:42", "clients", "new", `{"name":"before-rename"}`, now)
	history("telegram:42", "clients", "edit", `{"name":"edited"}`, now+5)
	history("telegram:42", "inbounds", "new", `{"name":"panel"}`, now+5)
	history("telegram:43", "clients", "new", `{"name":"theirs"}`, now+5)
	history("telegram", "clients", "new", `{"name":"panel"}`, now+5)
	history("admin", "clients", "new", `{"name":"panel"}`, now+5)
	history("telegram:42", "clients", "del", `5`, now+6)
	history("telegram:42", "clients", "new", `"not a client"`, now)
	history("telegram:42", "clients", "new", `null`, now)
	history("telegram:42", "clients", "new", `{broken`, now)

	if err := BotQuotaAdoptCreated(db, 42, "telegram:42"); err != nil {
		t.Fatal(err)
	}
	for _, c := range []*model.Client{one, bulk1, bulk2, old} {
		if m, ok := mapped(t, db, c.Id); !ok || m.TgId != 42 {
			t.Errorf("%s was not found: %+v %v", c.Name, m, ok)
		}
	}
	for _, c := range []*model.Client{reused, early, theirs, renamed, edited, panel} {
		if _, ok := mapped(t, db, c.Id); ok {
			t.Errorf("%s was taken", c.Name)
		}
	}
	// Their earlier usage is not charged; the new usage is.
	wantUsage(t, db, 42, 0, 4)
	traffic(t, db, one.Id, 130, 0)
	wantUsage(t, db, 42, 30, 4)
	// Running it again does not rebase what already counts.
	if err := BotQuotaAdoptCreated(db, 42, "telegram:42"); err != nil {
		t.Fatal(err)
	}
	wantUsage(t, db, 42, 30, 4)
}

// ---- counting what an administrator creates ----

func TestClientsCreatedByALimitedAdministratorCountFromTheirFirstByte(t *testing.T) {
	db := clientTestDB(t)
	limited(t, db, 42)
	one := newClient(t, db, "one")
	bulk1, bulk2 := newClient(t, db, "bulk1"), newClient(t, db, "bulk2")
	made := newClient(t, db, "made-by-someone-else")
	spaced := newClient(t, db, "spaced") // the panel stores the trimmed name

	if err := botQuotaOnClientSave(db, "telegram:42", "new", json.RawMessage(`{"name":"one"}`)); err != nil {
		t.Fatal(err)
	}
	if err := botQuotaOnClientSave(db, "telegram:42", "new", json.RawMessage(`{"name":"  spaced\t"}`)); err != nil {
		t.Fatal(err)
	}
	if err := botQuotaOnClientSave(db, "telegram:42", "addbulk", json.RawMessage(`[{"name":"bulk1"},{"name":"bulk2"}]`)); err != nil {
		t.Fatal(err)
	}
	for _, c := range []*model.Client{one, spaced, bulk1, bulk2} {
		if m, ok := mapped(t, db, c.Id); !ok || m.TgId != 42 || m.Base != 0 {
			t.Errorf("%s: %+v %v", c.Name, m, ok)
		}
	}

	// Not for an edit, another kind of actor, an administrator without a limit,
	// the bot's own copy, or an ID that is not one.
	for _, tc := range []struct{ actor, act string }{
		{"telegram:42", "edit"},
		{"telegram:42", "del"},
		{"telegram:42", "editbulk"},
		{"telegram:43", "new"},
		{"telegram", "new"},
		{"admin", "new"},
		{"telegram:", "new"},
		{"telegram:abc", "new"},
		{"telegram:-5", "new"},
		{"telegram:0", "new"},
	} {
		if err := botQuotaOnClientSave(db, tc.actor, tc.act, json.RawMessage(`{"name":"made-by-someone-else"}`)); err != nil {
			t.Fatalf("%+v: %v", tc, err)
		}
		if _, ok := mapped(t, db, made.Id); ok {
			t.Fatalf("%+v: the client was taken", tc)
		}
	}
	// Data that is not a client is nothing, not an error that stops a save.
	for _, raw := range []string{`null`, `"x"`, `5`, `[]`, `{}`} {
		if err := botQuotaOnClientSave(db, "telegram:42", "new", json.RawMessage(raw)); err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
	}
}

// The same hook runs inside ConfigService.Save, in the creating transaction.
func TestConfigSaveCountsTheClientsAnAdministratorCreates(t *testing.T) {
	db := clientTestDB(t)
	limited(t, db, 42)
	// A real but idle core, as the lifecycle tests have: a save would try to
	// start one.
	previous := corePtr
	t.Cleanup(func() {
		time.Sleep(300 * time.Millisecond)
		corePtr = previous
	})
	svc := NewConfigService(core.NewCore())
	if err := (&SettingService{}).SetMaintenance(true); err != nil {
		t.Fatal(err)
	}
	if _, err := (&SettingService{}).GetAllSetting(); err != nil {
		t.Fatal(err)
	}
	raw := func(name string) json.RawMessage {
		c := model.Client{Name: name, Enable: true, Config: json.RawMessage(`{}`), Inbounds: json.RawMessage(`[]`), Links: json.RawMessage(`[]`)}
		out, _ := json.Marshal(c)
		return out
	}
	if _, err := svc.Save("clients", "new", raw("viaBot"), "", "telegram:42", "example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Save("clients", "new", raw("  viaBotSpaced "), "", "telegram:42", "example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Save("clients", "new", raw("viaPanel"), "", "admin", "example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Save("clients", "new", raw("viaOtherBotAdmin"), "", "telegram:43", "example.com"); err != nil {
		t.Fatal(err)
	}
	var viaBot, viaSpaced, viaPanel, viaOther model.Client
	db.Where("name = ?", "viaBot").First(&viaBot)
	db.Where("name = ?", "viaBotSpaced").First(&viaSpaced)
	db.Where("name = ?", "viaPanel").First(&viaPanel)
	db.Where("name = ?", "viaOtherBotAdmin").First(&viaOther)
	for _, c := range []model.Client{viaBot, viaSpaced} {
		if m, ok := mapped(t, db, c.Id); !ok || m.TgId != 42 || m.Base != 0 {
			t.Fatalf("a client the administrator made does not count (%q): %+v %v", c.Name, m, ok)
		}
	}
	for _, c := range []model.Client{viaPanel, viaOther} {
		if _, ok := mapped(t, db, c.Id); ok {
			t.Fatalf("%s counts for somebody", c.Name)
		}
	}

	// A creation that fails leaves nothing behind.
	if _, err := svc.Save("clients", "new", raw("viaBot"), "", "telegram:42", "example.com"); err == nil {
		t.Fatal("a duplicate name was saved")
	}
	var rows int64
	db.Model(&model.BotQuotaClient{}).Count(&rows)
	if rows != 2 {
		t.Fatalf("%d clients count, want 2", rows)
	}
}
