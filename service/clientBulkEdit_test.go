package service

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
)

// The panel's bulk edit sends back the rows of the client list. Each save
// writes the whole row, so a setting that is missing from the list is reset by
// the edit: the list has to carry delay start, auto reset, reset days and the
// next reset.
func TestBulkEditRoundTripKeepsAutoResetAndDelayStart(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "bulk-edit.db")); err != nil {
		t.Fatal(err)
	}
	db := database.GetDB()
	c := model.Client{
		Name: "x", Enable: true, Config: json.RawMessage(`{}`), Inbounds: json.RawMessage(`[]`), Links: json.RawMessage(`[]`),
		AutoReset: true, ResetDays: 7, DelayStart: true, NextReset: 12345, Up: 1, Down: 2, Remark: "r", LimitIp: 4,
	}
	if err := db.Create(&c).Error; err != nil {
		t.Fatal(err)
	}
	listed, err := (&ClientService{}).GetAll()
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(*listed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (&ClientService{}).Save(db, "editbulk", payload, "example.com"); err != nil {
		t.Fatal(err)
	}
	var got model.Client
	if err := db.First(&got, c.Id).Error; err != nil {
		t.Fatal(err)
	}
	if !got.AutoReset || got.ResetDays != 7 || !got.DelayStart || got.NextReset != 12345 || got.Up != 1 || got.Down != 2 || got.Remark != "r" || got.LimitIp != 4 {
		t.Fatalf("a bulk edit round trip changed the client: %+v", got)
	}
}
