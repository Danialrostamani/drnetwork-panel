package sub

import (
	"encoding/json"
	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/service"
	"net/http"
	"strings"
	"testing"
)

func TestLiveSnapshotIsFreshAndAccountOnly(t *testing.T) {
	r := pageServer(t)
	for _, n := range []int64{10, 20, 0, 999} {
		database.GetDB().Model(&model.Client{}).Where("name = ?", "alice").Updates(map[string]any{"up": n, "down": 2 * n, "volume": 1000, "enable": false})
		w := fetch(r, http.MethodGet, "/sub/alice?format=live", "*/*")
		var got struct {
			OK                      bool `json:"ok"`
			Enabled                 bool `json:"enabled"`
			Upload, Download, Total int64
		}
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &got) != nil || !got.OK || got.Enabled || got.Upload != n || got.Download != 2*n || got.Total != 1000 {
			t.Fatalf("live: %d %s", w.Code, w.Body.String())
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("live snapshot cached")
		}
		for _, secret := range []string{"config", "inbounds", "links", "password", "token"} {
			if strings.Contains(w.Body.String(), secret) {
				t.Fatal("private account data leaked")
			}
		}
	}
	if w := fetch(r, http.MethodGet, "/sub/missing?format=live", "*/*"); w.Code != 400 {
		t.Fatal("unknown account exposed")
	}
}
func TestSupportDirectWorksWithoutBotAndToolbarActionsAreGone(t *testing.T) {
	r := pageServer(t)
	page := fetch(r, http.MethodGet, "/sub/alice", "text/html").Body.String()
	for _, id := range []string{`id="supportBtn"`, `id="refreshBtn"`, `$("#refreshBtn")`, `$("#supportBtn")`} {
		if strings.Contains(page, id) {
			t.Fatalf("old toolbar reference: %s", id)
		}
	}
	cfg := (&SubHandler{}).pageConfig(&model.Client{Name: "alice"})
	if cfg["supportLink"] != "https://t.me/DrNetWorkSingBox?direct" || cfg["ui"].(map[string]any)["report"] != true {
		t.Fatalf("default direct/report: %+v", cfg)
	}
	if err := (&service.SettingService{}).Save(database.GetDB(), json.RawMessage(`{"subPageConf":"{\"support\":\"https://t.me/OtherChannel?direct\"}"}`)); err != nil {
		t.Fatal(err)
	}
	cfg = (&SubHandler{}).pageConfig(&model.Client{Name: "alice"})
	if cfg["supportLink"] != "https://t.me/OtherChannel?direct" {
		t.Fatal("custom support not applied")
	}
}

func TestLiveSnapshotRequiresTokenForProtectedClient(t *testing.T) {
	r := pageServer(t)
	database.GetDB().Model(&model.Client{}).Where("name = ?", "alice").Update("use_sub_token", true)
	var c model.Client
	database.GetDB().Where("name = ?", "alice").First(&c)
	if w := fetch(r, http.MethodGet, "/sub/alice?format=live", "*/*"); w.Code == 200 {
		t.Fatal("protected name accepted")
	}
	if w := fetch(r, http.MethodGet, "/sub/"+c.SubToken+"?format=live", "*/*"); w.Code != 200 {
		t.Fatalf("valid token: %d", w.Code)
	}
}
