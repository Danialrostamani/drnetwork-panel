package service

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
)

// A node added while the subscription route is on joins the end of it as
// backup only; with the route off, or with none, the setting is left alone.
func TestNewNodeJoinsRoutePlanAsBackup(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "route.db")); err != nil {
		t.Fatal(err)
	}
	db := database.GetDB()
	first := model.Node{Name: "first", Enable: true, BaseUrl: "https://first.example", WebPath: "/app/", Token: "t"}
	if err := db.Create(&first).Error; err != nil {
		t.Fatal(err)
	}
	route, _ := json.Marshal(SubNodeRoute{Enable: true, Entries: []SubNodeRouteEntry{{Id: first.Id, Weight: 100}}})
	if err := db.Create(&model.Setting{Key: "subNodeRoute", Value: string(route)}).Error; err != nil {
		t.Fatal(err)
	}
	add := func(name string) uint {
		t.Helper()
		payload, _ := json.Marshal(model.Node{Name: name, Enable: true, BaseUrl: "https://" + name + ".example", WebPath: "app", Token: "t"})
		tx := db.Begin()
		if err := (&NodeService{}).Save(tx, "new", payload); err != nil {
			tx.Rollback()
			t.Fatal(err)
		}
		if err := tx.Commit().Error; err != nil {
			t.Fatal(err)
		}
		var n model.Node
		if err := db.Where("name = ?", name).First(&n).Error; err != nil {
			t.Fatal(err)
		}
		return n.Id
	}
	read := func() SubNodeRoute {
		t.Helper()
		var setting model.Setting
		if err := db.Where("key = ?", "subNodeRoute").First(&setting).Error; err != nil {
			t.Fatal(err)
		}
		var cfg SubNodeRoute
		if err := json.Unmarshal([]byte(setting.Value), &cfg); err != nil {
			t.Fatal(err)
		}
		return cfg
	}

	second := add("second")
	cfg := read()
	if !cfg.Enable || len(cfg.Entries) != 2 || cfg.Entries[0] != (SubNodeRouteEntry{Id: first.Id, Weight: 100}) || cfg.Entries[1] != (SubNodeRouteEntry{Id: second}) {
		t.Fatalf("route after a new node: %#v", cfg)
	}
	plan := parseNodeRoutePlan(mustJSON(t, cfg), []model.Node{first, {Id: second, Name: "second"}})
	if len(plan.names) != 2 || plan.names[1] != "second" || plan.weight["second"] != 0 {
		t.Fatalf("plan %#v", plan)
	}

	cfg.Enable = false
	off, _ := json.Marshal(cfg)
	if err := db.Model(model.Setting{}).Where("key = ?", "subNodeRoute").Update("value", string(off)).Error; err != nil {
		t.Fatal(err)
	}
	add("third")
	if got := read(); got.Enable || len(got.Entries) != 2 {
		t.Fatalf("route off was changed: %#v", got)
	}
}

func mustJSON(t *testing.T, v interface{}) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
