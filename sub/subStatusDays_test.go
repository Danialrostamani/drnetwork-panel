package sub

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/service"
)

// The status answer carries the last 30 days and today's daily allowance,
// and the page draws them instead of the old ring.
func TestStatusCarriesTheDays(t *testing.T) {
	r := pageServer(t)
	resetStatusCache()
	db := database.GetDB()
	var alice model.Client
	if err := db.Where("name = ?", "alice").First(&alice).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	today := service.DayKey(now)
	db.Model(&model.Client{}).Where("id = ?", alice.Id).Updates(map[string]interface{}{
		"daily_volume": 1000, "daily_time": 3600, "day_used": 400, "day_online": 600, "day_key": today,
	})
	db.Create(&model.ClientDay{ClientId: alice.Id, Day: today, Up: 100, Down: 300})
	w := fetch(r, http.MethodGet, "/sub/alice?format=status", "*/*")
	var st pageStatus
	if err := json.Unmarshal(w.Body.Bytes(), &st); err != nil {
		t.Fatalf("%v: %s", err, w.Body.String())
	}
	if len(st.Days) != 30 || st.Days[29].Day != today || st.Days[29].Down != 300 {
		t.Fatalf("days: %s", w.Body.String())
	}
	if st.Daily == nil || st.Daily.Volume != 1000 || st.Daily.Used != 400 || st.Daily.Online != 600 || st.Daily.Locked {
		t.Fatalf("daily: %s", w.Body.String())
	}
	for _, want := range []string{"dayChartHtml(days)", "dailyHtml(j.daily)", `"mtr.d30"`} {
		if !strings.Contains(subPageHTML, want) {
			t.Fatalf("page lacks %s", want)
		}
	}
	if strings.Contains(subPageHTML, "usageGauge") {
		t.Fatal("the ring is still on the page")
	}
}
