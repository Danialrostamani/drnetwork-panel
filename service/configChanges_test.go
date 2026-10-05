package service

import (
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
)

func actorsOf(changes []model.Changes) string {
	out := make([]string, 0, len(changes))
	for _, c := range changes {
		out = append(out, c.Actor)
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}

// The bot records "telegram:<administrator's ID>", older versions plain
// "telegram"; asking the history for "telegram" finds all of them and nothing
// that only looks alike.
func TestChangeHistoryFindsEveryTelegramActor(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "changes.db")); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	rows := []model.Changes{
		{DateTime: now, Actor: "telegram", Key: "clients", Action: "edit"},
		{DateTime: now, Actor: "telegram:42", Key: "clients", Action: "edit"},
		{DateTime: now, Actor: "telegram:77", Key: "settings", Action: "edit"},
		{DateTime: now, Actor: "telegram:42", Key: "settings", Action: "edit"},
		{DateTime: now, Actor: "telegramx", Key: "clients", Action: "edit"},
		{DateTime: now, Actor: "telegrams:1", Key: "clients", Action: "edit"},
		{DateTime: now, Actor: "DepleteJob", Key: "clients", Action: "del"},
		{DateTime: now, Actor: "admin", Key: "clients", Action: "edit"},
		{DateTime: now, Actor: "my_telegram", Key: "clients", Action: "edit"},
	}
	if err := database.GetDB().Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	svc := &ConfigService{}

	if got := actorsOf(svc.GetChanges("telegram", "", "50")); got != "telegram,telegram:42,telegram:42,telegram:77" {
		t.Fatalf("telegram = %s", got)
	}
	// The key filter still narrows the family (the OR must not escape it).
	if got := actorsOf(svc.GetChanges("telegram", "settings", "50")); got != "telegram:42,telegram:77" {
		t.Fatalf("telegram + settings = %s", got)
	}
	if got := actorsOf(svc.GetChanges("telegram", "clients", "50")); got != "telegram,telegram:42" {
		t.Fatalf("telegram + clients = %s", got)
	}
	// One administrator, and every other actor, are still matched exactly.
	if got := actorsOf(svc.GetChanges("telegram:42", "", "50")); got != "telegram:42,telegram:42" {
		t.Fatalf("telegram:42 = %s", got)
	}
	if got := actorsOf(svc.GetChanges("telegram:4", "", "50")); got != "" {
		t.Fatalf("a prefix of an ID matched: %s", got)
	}
	if got := actorsOf(svc.GetChanges("admin", "", "50")); got != "admin" {
		t.Fatalf("admin = %s", got)
	}
	if got := actorsOf(svc.GetChanges("DepleteJob", "", "50")); got != "DepleteJob" {
		t.Fatalf("DepleteJob = %s", got)
	}
	if n := len(svc.GetChanges("", "", "50")); n != len(rows) {
		t.Fatalf("no filter = %d rows, want %d", n, len(rows))
	}
	// The count still applies to the family.
	if n := len(svc.GetChanges("telegram", "", "2")); n != 2 {
		t.Fatalf("count 2 gave %d rows", n)
	}
}
