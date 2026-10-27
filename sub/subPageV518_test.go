package sub

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/service"
)

func resetStatusCache() {
	statusMu.Lock()
	statusCache = map[string]statusEntry{}
	statusMu.Unlock()
}

// The link says whether a browser comes through one of the panel's servers,
// and how many devices a client with an address limit uses.
func TestTheLinkChecksTheVpn(t *testing.T) {
	r := pageServer(t)
	resetStatusCache()
	database.GetDB().Model(&model.Client{}).Where("name = ?", "alice").Update("limit_ip", 3)
	w := fetch(r, http.MethodGet, "/sub/alice?format=status", "*/*")
	var st pageStatus
	if json.Unmarshal(w.Body.Bytes(), &st) != nil || st.Devices == nil || st.Devices.Limit != 3 {
		t.Fatalf("devices: %s", w.Body.String())
	}
	w = fetch(r, http.MethodGet, "/sub/alice?format=vpn&ip=203.0.113.9", "*/*")
	var got map[string]vpnSide
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &got) != nil || got["ext"].IP != "203.0.113.9" || got["ext"].Ours || got["self"].Ours {
		t.Fatalf("vpn %d %s", w.Code, w.Body.String())
	}
	if w := fetch(r, http.MethodGet, "/sub/alice?format=vpn&ip=not-an-ip", "*/*"); strings.Contains(w.Body.String(), `"ext"`) {
		t.Fatalf("bad ip taken: %s", w.Body.String())
	}
	if err := (&service.SettingService{}).Save(database.GetDB(), json.RawMessage(`{"subPageConf":"{\"hideVpn\":true}"}`)); err != nil {
		t.Fatal(err)
	}
	if w := fetch(r, http.MethodGet, "/sub/alice?format=vpn", "*/*"); w.Code != 404 {
		t.Fatalf("hidden vpn check: %d", w.Code)
	}
}

// A report must be JSON, and goes nowhere without a bot.
func TestTheLinkTakesReports(t *testing.T) {
	r := pageServer(t)
	post := func(body, kind string) int {
		req := httptest.NewRequest(http.MethodPost, "/sub/alice?format=report", strings.NewReader(body))
		req.Header.Set("Content-Type", kind)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w.Code
	}
	if c := post("text=hi", "application/x-www-form-urlencoded"); c != 415 {
		t.Fatalf("form: %d", c)
	}
	if c := post(`{"text":"  "}`, "application/json"); c != 400 {
		t.Fatalf("empty: %d", c)
	}
	if c := post(`{"text":"down"}`, "application/json"); c != 503 {
		t.Fatalf("no bot: %d", c)
	}
	if !reportAllowed("bob", "1.1.1.1", fixedNow) || reportAllowed("bob", "1.1.1.1", fixedNow.Add(reportGap/2)) || !reportAllowed("bob", "1.1.1.1", fixedNow.Add(reportGap)) {
		t.Fatal("one report per client per gap")
	}
	for i, name := range []string{"c1", "c2", "c3", "c4"} {
		if ok := reportAllowed(name, "2.2.2.2", fixedNow); ok != (i < reportPerIP) {
			t.Fatalf("report %d from one address: %v", i, ok)
		}
	}
	if err := (&service.SettingService{}).Save(database.GetDB(), json.RawMessage(`{"subPageConf":"{\"hideReport\":true}"}`)); err != nil {
		t.Fatal(err)
	}
	if c := post(`{"text":"down"}`, "application/json"); c != 404 {
		t.Fatalf("hidden report: %d", c)
	}
}

var fixedNow = mustTime("2030-01-01T00:00:00Z")

func mustTime(v string) time.Time { t, _ := time.Parse(time.RFC3339, v); return t }
