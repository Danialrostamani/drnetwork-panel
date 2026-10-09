package database

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Danialrostamani/drnetwork-panel/database/model"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// A row in every table the schema describes, so a dropped table shows up as a
// missing row rather than as an empty database nobody notices until a restore.
func seedEveryTable(t *testing.T) {
	t.Helper()

	rows := []any{
		// The settings table used to be filled in by the per-migration flags
		// the old InitDB wrote; migrations record the version instead, and
		// that is what a live database carries here.
		&model.Setting{Key: "version", Value: "1.6.2"},
		&model.Tls{Name: "cert", Server: json.RawMessage(`{"enabled":true}`), Client: json.RawMessage(`{}`)},
		&model.Inbound{Type: "vless", Tag: "in-1", Options: json.RawMessage(`{}`), OutJson: json.RawMessage(`{}`)},
		&model.Outbound{Type: "direct", Tag: "out-1", Options: json.RawMessage(`{}`)},
		&model.Service{Type: "derp", Tag: "svc-1", Options: json.RawMessage(`{}`)},
		&model.Endpoint{Type: "wireguard", Tag: "ep-1", Options: json.RawMessage(`{}`)},
		&model.Tokens{UserId: 1, Token: "a-token", Desc: "for the test", Expiry: 0},
		&model.Node{Name: "node-1", Enable: true, BaseUrl: "https://node.example", WebPath: "/app/", Token: "node-token"},
		&model.Stats{DateTime: 1, Resource: "user", Tag: "someone", Direction: true, Traffic: 42},
		&model.Client{Name: "someone", Enable: true, Config: json.RawMessage(`{}`), Inbounds: json.RawMessage(`[]`), Links: json.RawMessage(`[]`)},
		&model.Changes{DateTime: 1, Actor: "admin", Key: "clients", Action: "new", Obj: json.RawMessage(`"someone"`)},
		&model.BotQuota{TgId: 42, Total: 500 << 30, Banked: 120 << 30, Adopted: true},
		&model.BotQuotaClient{ClientId: 1, TgId: 42, Base: 7 << 20},
		&model.NodeMetric{NodeId: 1, DateTime: 60, Probes: 12, Up: 12, Latency: 30, Cpu: 5, Mem: 40, Disk: 20, Online: 3, Sent: 1000, Recv: 2000},
		&model.NodeOutage{NodeId: 1, Start: 100, End: 200, State: "offline", Reason: "timeout", Checked: 190},
		&model.NodeTraffic{NodeId: 1, DateTime: 3600, Up: 1 << 20, Down: 2 << 20},
		&model.ShopPlan{Name: "1 month", Volume: 30 << 30, Days: 30},
		&model.ShopOrder{TgId: 42, Kind: "buy", PlanId: 1, PlanName: "1 month"},
		&model.ShopWallet{TgId: 42, Balance: 1000},
		&model.ShopWalletTx{TgId: 42, Amount: 1000, Balance: 1000, Reason: "topup"},
		&model.ShopDiscount{Code: "OFF10", Percent: 10},
		&model.ShopUser{TgId: 42, Name: "someone"},
		&model.ShopCodeUse{CodeId: 1, Code: "OFF10", TgId: 42},
		&model.ShopSms{Hash: "h1", Text: "deposit 1,000", Status: model.SmsUnmatched},
		&model.ShopAutoRenew{ClientId: 1, TgId: 42, PlanId: 1, Enable: true},
	}
	for _, row := range rows {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("seeding %T: %v", row, err)
		}
	}
}

func openBackup(t *testing.T, contents []byte) *gorm.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "restored.db")
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	restored, err := gorm.Open(sqlite.Open(path), &gorm.Config{Logger: gormlogger.Discard})
	if err != nil {
		t.Fatalf("opening the backup: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, e := restored.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	return restored
}

// The schema drift: InitDB migrated eleven models while GetDb backed up nine,
// so services and API tokens were absent from every backup.
func TestBackupCarriesEveryTable(t *testing.T) {
	if err := InitDB(filepath.Join(t.TempDir(), "test.db")); err != nil {
		t.Fatal(err)
	}
	seedEveryTable(t)

	contents, err := GetDb("")
	if err != nil {
		t.Fatalf("GetDb: %v", err)
	}
	restored := openBackup(t, contents)

	for _, table := range schema() {
		var live, backed int64
		if err := db.Table(table.name).Count(&live).Error; err != nil {
			t.Fatalf("counting live %s: %v", table.name, err)
		}
		if err := restored.Table(table.name).Count(&backed).Error; err != nil {
			t.Errorf("table %q is missing from the backup: %v", table.name, err)
			continue
		}
		if live == 0 {
			t.Errorf("table %q had no rows to back up; the fixture needs one", table.name)
		}
		if backed != live {
			t.Errorf("table %q: backup has %d rows, live database has %d", table.name, backed, live)
		}
	}
}

// exclude must skip exactly the named tables and nothing else.
func TestBackupExcludesOnlyWhatWasAsked(t *testing.T) {
	if err := InitDB(filepath.Join(t.TempDir(), "test.db")); err != nil {
		t.Fatal(err)
	}
	seedEveryTable(t)

	contents, err := GetDb("stats,changes")
	if err != nil {
		t.Fatalf("GetDb: %v", err)
	}
	restored := openBackup(t, contents)

	for _, name := range []string{"stats", "changes"} {
		var count int64
		if err := restored.Table(name).Count(&count).Error; err != nil {
			t.Errorf("excluded table %q should still exist but be empty: %v", name, err)
			continue
		}
		if count != 0 {
			t.Errorf("excluded table %q still holds %d rows", name, count)
		}
	}

	// The per-minute node history goes with the traffic history.
	var metrics int64
	if err := restored.Table("node_metrics").Count(&metrics).Error; err != nil || metrics != 0 {
		t.Errorf("node_metrics with stats excluded: %d rows, %v", metrics, err)
	}

	// Everything not named must survive.
	for _, name := range []string{"clients", "inbounds", "services", "tokens", "node_outages", "node_traffic"} {
		var count int64
		if err := restored.Table(name).Count(&count).Error; err != nil {
			t.Errorf("counting %s: %v", name, err)
			continue
		}
		if count == 0 {
			t.Errorf("table %q was dropped even though it was not excluded", name)
		}
	}
}

// The temp path used the layout "20060102-200203", a typo for "150405", so
// every call in a period rendered the same name and two downloads collided.
func TestBackupsDoNotShareATempFile(t *testing.T) {
	if err := InitDB(filepath.Join(t.TempDir(), "test.db")); err != nil {
		t.Fatal(err)
	}
	seedEveryTable(t)

	first, err := GetDb("")
	if err != nil {
		t.Fatalf("first GetDb: %v", err)
	}
	second, err := GetDb("")
	if err != nil {
		t.Fatalf("second GetDb: %v", err)
	}
	if len(first) == 0 || len(second) == 0 {
		t.Fatalf("a backup came back empty: %d and %d bytes", len(first), len(second))
	}

	// A shared temp file leaves one of them truncated or gone.
	for i, contents := range [][]byte{first, second} {
		restored := openBackup(t, contents)
		var count int64
		if err := restored.Table("clients").Count(&count).Error; err != nil {
			t.Errorf("backup %d is not a usable database: %v", i+1, err)
			continue
		}
		if count == 0 {
			t.Errorf("backup %d lost its clients", i+1)
		}
	}
}

// GetDb leaves no scratch files next to the binary.
func TestBackupCleansUpAfterItself(t *testing.T) {
	if err := InitDB(filepath.Join(t.TempDir(), "test.db")); err != nil {
		t.Fatal(err)
	}
	dir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		t.Fatal(err)
	}
	before := backupTempFiles(t, dir)

	if _, err := GetDb(""); err != nil {
		t.Fatalf("GetDb: %v", err)
	}

	if after := backupTempFiles(t, dir); len(after) != len(before) {
		t.Errorf("GetDb left scratch files behind: %v", after)
	}
}

func backupTempFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var found []string
	for _, e := range entries {
		if strings.Contains(e.Name(), "-backup-") {
			found = append(found, e.Name())
		}
	}
	return found
}

// A bot administrator's volume limit is made of numbers a backup must keep:
// the total, what deleted clients used, and, for each client that counts, where
// it started. A backup that kept the rows but lost the numbers would hand
// everybody a fresh allowance on restore.
func TestBackupKeepsVolumeLimits(t *testing.T) {
	if err := InitDB(filepath.Join(t.TempDir(), "test.db")); err != nil {
		t.Fatal(err)
	}
	want := []model.BotQuota{
		{TgId: 42, Total: 500 << 30, Banked: 120 << 30, Adopted: true},
		{TgId: 7_000_000_000, Total: 3, Banked: 3}, // Telegram IDs outgrow 32 bits
		{TgId: 9, Total: 0, Banked: 0},
	}
	if err := db.Create(&want).Error; err != nil {
		t.Fatal(err)
	}
	counted := []model.BotQuotaClient{
		{ClientId: 1, TgId: 42, Base: 7 << 30},
		{ClientId: 2, TgId: 7_000_000_000, Base: 0},
		{ClientId: 3, TgId: 42, Base: 1},
	}
	if err := db.Create(&counted).Error; err != nil {
		t.Fatal(err)
	}

	contents, err := GetDb("")
	if err != nil {
		t.Fatalf("GetDb: %v", err)
	}
	restored := openBackup(t, contents)
	var got []model.BotQuota
	if err := restored.Order("tg_id").Find(&got).Error; err != nil {
		t.Fatalf("reading the limits back: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("the backup holds %d limits, want %d: %+v", len(got), len(want), got)
	}
	byID := map[int64]model.BotQuota{}
	for _, q := range got {
		byID[q.TgId] = q
	}
	for _, q := range want {
		if byID[q.TgId] != q {
			t.Errorf("limit of %d came back as %+v, want %+v", q.TgId, byID[q.TgId], q)
		}
	}
	var gotCounted []model.BotQuotaClient
	if err := restored.Order("client_id").Find(&gotCounted).Error; err != nil {
		t.Fatalf("reading the counted clients back: %v", err)
	}
	if len(gotCounted) != len(counted) {
		t.Fatalf("the backup holds %d counted clients, want %d: %+v", len(gotCounted), len(counted), gotCounted)
	}
	for i := range counted {
		if gotCounted[i] != counted[i] {
			t.Errorf("counted client %d came back as %+v, want %+v", counted[i].ClientId, gotCounted[i], counted[i])
		}
	}
}

// A database from before the limits existed gets their tables on the next
// start, empty: nobody is limited until the owner says so.
func TestOlderDatabaseGainsTheVolumeLimitTable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	for _, m := range []any{&model.BotQuota{}, &model.BotQuotaClient{}} {
		if err := db.Migrator().DropTable(m); err != nil {
			t.Fatal(err)
		}
		if db.Migrator().HasTable(m) {
			t.Fatalf("the table of %T is still there", m)
		}
	}
	if err := InitDB(path); err != nil {
		t.Fatalf("starting on the older database: %v", err)
	}
	for _, m := range []any{&model.BotQuota{}, &model.BotQuotaClient{}} {
		if !db.Migrator().HasTable(m) {
			t.Fatalf("the table of %T was not created", m)
		}
		var n int64
		if err := db.Model(m).Count(&n).Error; err != nil || n != 0 {
			t.Fatalf("rows on a fresh %T table: %d, %v", m, n, err)
		}
	}
}

// The first release of the limits kept a "granted" count and nothing else. A
// database from then keeps its limits on the next start, with nothing used:
// the old count was of volume handed out, which means something else now, and
// the column it lived in stays out of the way.
func TestFirstVolumeLimitsUpgrade(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	if err := db.Migrator().DropTable(&model.BotQuota{}); err != nil {
		t.Fatal(err)
	}
	err := db.Exec("CREATE TABLE `bot_quotas` (`tg_id` integer,`total` integer NOT NULL DEFAULT 0,`granted` integer NOT NULL DEFAULT 0,PRIMARY KEY (`tg_id`))").Error
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO bot_quotas (tg_id, total, granted) VALUES (42, ?, ?)", 500<<30, 120<<30).Error; err != nil {
		t.Fatal(err)
	}
	if err := InitDB(path); err != nil {
		t.Fatalf("starting on the older database: %v", err)
	}

	var q model.BotQuota
	if err := db.Where("tg_id = ?", 42).First(&q).Error; err != nil {
		t.Fatal(err)
	}
	if q != (model.BotQuota{TgId: 42, Total: 500 << 30}) {
		t.Fatalf("the limit came through as %+v", q)
	}
	// New limits can still be written with the old column in the table, and so
	// can the rows of a restored backup.
	if err := db.Create(&model.BotQuota{TgId: 43, Total: 1 << 30, Banked: 5}).Error; err != nil {
		t.Fatalf("a new limit cannot be written: %v", err)
	}
	if err := db.Save([]model.BotQuota{{TgId: 42, Total: 9, Banked: 2, Adopted: true}}).Error; err != nil {
		t.Fatalf("a row cannot be saved over the old one: %v", err)
	}
	var n int64
	if err := db.Model(&model.BotQuota{}).Count(&n).Error; err != nil || n != 2 {
		t.Fatalf("%d limits, %v", n, err)
	}
}
