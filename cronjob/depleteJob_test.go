package cronjob

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/core"
	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/logger"
	"github.com/Danialrostamani/drnetwork-panel/service"

	"github.com/op/go-logging"
)

// A client past its volume is disabled on the master and the nodes are marked
// for a sync at once, so they stop serving it too.
func TestDepleteJobMarksNodesForSync(t *testing.T) {
	logger.InitLogger(logging.ERROR)
	// A core that is not running: the inbound update has nothing to touch.
	service.NewConfigService(core.NewCore())
	if err := database.InitDB(filepath.Join(t.TempDir(), "d.db")); err != nil {
		t.Fatal(err)
	}
	db := database.GetDB()
	node := model.Node{Name: "nl", Enable: true, BaseUrl: "http://127.0.0.1:1", Token: "t"}
	db.Create(&node)
	in := model.Inbound{Type: "vless", Tag: "nl-in", NodeId: &node.Id, Options: json.RawMessage(`{}`)}
	db.Create(&in)
	inbounds, _ := json.Marshal([]uint{in.Id})
	db.Create(&model.Client{Name: "heavy", Enable: true, Volume: 65 << 30, Up: 30 << 30, Down: 38 << 30, Config: json.RawMessage(`{}`), Inbounds: inbounds, Links: json.RawMessage(`[]`)})

	NewDepleteJob().Run()

	var c model.Client
	db.Where("name = ?", "heavy").First(&c)
	if c.Enable {
		t.Fatal("the client over its volume is still enabled")
	}
	var n model.Node
	db.First(&n, node.Id)
	if !n.Dirty {
		t.Fatal("the nodes were not marked for a sync")
	}
}

// The quick check cuts a client off the moment its usage reaches the volume
// -- not one byte later -- or its time is up, and leaves the others alone.
func TestQuickDepleteJobCutsAtTheLimit(t *testing.T) {
	logger.InitLogger(logging.ERROR)
	service.NewConfigService(core.NewCore())
	if err := database.InitDB(filepath.Join(t.TempDir(), "q.db")); err != nil {
		t.Fatal(err)
	}
	kickDelay = 0
	db := database.GetDB()
	mk := func(name string, volume, used, expiry int64) {
		db.Create(&model.Client{Name: name, Enable: true, Volume: volume, Down: used, Expiry: expiry, Config: json.RawMessage(`{}`), Inbounds: json.RawMessage(`[]`), Links: json.RawMessage(`[]`)})
	}
	now := time.Now().Unix()
	mk("exact", 10<<30, 10<<30, 0)
	mk("left", 10<<30, 9<<30, 0)
	mk("late", 0, 0, now-1)
	mk("time", 0, 0, now+3600)

	var cs service.ClientService
	if !cs.HasDepleted() {
		t.Fatal("HasDepleted missed the clients at their limit")
	}
	NewQuickDepleteJob().Run()
	want := map[string]bool{"exact": false, "left": true, "late": false, "time": true}
	for name, enabled := range want {
		var c model.Client
		db.Where("name = ?", name).First(&c)
		if c.Enable != enabled {
			t.Fatalf("%s: enable = %v, want %v", name, c.Enable, enabled)
		}
	}
	if cs.HasDepleted() {
		t.Fatal("nothing is left to cut off")
	}
}
