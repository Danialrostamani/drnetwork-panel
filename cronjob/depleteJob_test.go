package cronjob

import (
	"encoding/json"
	"path/filepath"
	"testing"

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
