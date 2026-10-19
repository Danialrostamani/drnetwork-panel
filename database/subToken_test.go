package database

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/Danialrostamani/drnetwork-panel/database/model"
)

// The clients from before tokens, or of an older backup, get one at start.
func TestClientsWithoutATokenGetOneAtStart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tok.db")
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a", "b"} {
		c := model.Client{Name: name, Enable: true, Config: json.RawMessage(`{}`), Inbounds: json.RawMessage(`[]`), Links: json.RawMessage(`[]`)}
		if err := db.Create(&c).Error; err != nil {
			t.Fatal(err)
		}
	}
	var kept model.Client
	db.Where("name = ?", "b").First(&kept)
	if err := db.Model(model.Client{}).Where("name = ?", "a").Update("sub_token", "").Error; err != nil {
		t.Fatal(err)
	}
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	var clients []model.Client
	db.Order("name").Find(&clients)
	if len(clients) != 2 || len(clients[0].SubToken) != model.SubTokenLength {
		t.Fatalf("clients %+v", clients)
	}
	if clients[1].SubToken != kept.SubToken {
		t.Errorf("a token that was there changed: %q -> %q", kept.SubToken, clients[1].SubToken)
	}
}
