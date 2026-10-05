package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/service"

	"github.com/gin-gonic/gin"
)

type apiAnswer struct {
	Success bool            `json:"success"`
	Msg     string          `json:"msg"`
	Obj     json.RawMessage `json:"obj"`
}

func getV2(t *testing.T, engine *gin.Engine, path, token string) apiAnswer {
	t.Helper()
	srv := httptest.NewServer(engine)
	defer srv.Close()
	req, err := http.NewRequest(http.MethodGet, srv.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Token", token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var out apiAnswer
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("%s: %v (%s)", path, err, body)
	}
	return out
}

// statsTotals is what a master asks of every node for its Telegram bot's Stats
// screen; a master that meets a node without it needs to recognise the answer.
func TestNodeAPIAnswersStatsTotals(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "totals.db")); err != nil {
		t.Fatal(err)
	}
	token, err := (&service.UserService{}).AddToken("admin", 0, "test")
	if err != nil {
		t.Fatal(err)
	}
	engine := gin.New()
	NewAPIv2Handler(engine.Group("/apiv2"))

	at := time.Now().Unix() - 100
	rows := []model.Stats{
		{DateTime: at, Resource: "user", Tag: "alice", Direction: true, Traffic: 7},
		{DateTime: at, Resource: "user", Tag: "alice", Direction: false, Traffic: 9},
		{DateTime: at, Resource: "inbound", Tag: "in1", Direction: true, Traffic: 3},
		{DateTime: at - 5*86400, Resource: "user", Tag: "alice", Direction: true, Traffic: 1000},
	}
	if err := database.GetDB().Create(&rows).Error; err != nil {
		t.Fatal(err)
	}

	since := time.Now().Unix() - 86400
	path := "/apiv2/statsTotals?since=" + itoa(since) + "&bucket=3600"
	got := getV2(t, engine, path, token)
	if !got.Success {
		t.Fatalf("statsTotals failed: %s", got.Msg)
	}
	var sum service.StatsSummary
	if err := json.Unmarshal(got.Obj, &sum); err != nil {
		t.Fatal(err)
	}
	if sum.Since != since || sum.Bucket != 3600 {
		t.Errorf("window = %d/%d", sum.Since, sum.Bucket)
	}
	want := map[string][2]int64{"user alice": {7, 9}, "inbound in1": {3, 0}}
	if len(sum.Totals) != len(want) {
		t.Fatalf("totals = %+v", sum.Totals)
	}
	for _, tot := range sum.Totals {
		if w, ok := want[tot.Resource+" "+tot.Tag]; !ok || w != [2]int64{tot.Up, tot.Down} {
			t.Errorf("total %+v, want %v", tot, want)
		}
	}
	if len(sum.Series) != 1 || sum.Series[0].Tag != "in1" || sum.Series[0].Traffic != 3 || sum.Series[0].At%3600 != 0 {
		t.Errorf("series = %+v", sum.Series)
	}

	// Without the parameters the answer is still sound.
	if got := getV2(t, engine, "/apiv2/statsTotals", token); !got.Success {
		t.Fatalf("statsTotals without a window failed: %s", got.Msg)
	}

	// A token is needed, like for every other action.
	if got := getV2(t, engine, path, ""); got.Success {
		t.Fatal("statsTotals answered without a token")
	}
	if got := getV2(t, engine, path, "wrong"); got.Success {
		t.Fatal("statsTotals answered to a wrong token")
	}

	// The master tells a node that is too old by this answer.
	missing := getV2(t, engine, "/apiv2/noSuchAction", token)
	if missing.Success || !strings.Contains(missing.Msg, "unknown action") {
		t.Fatalf("an unknown action answers %+v, the master looks for \"unknown action\"", missing)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
