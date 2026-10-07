package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
)

func TestClusterSessionsAskTheNodes(t *testing.T) {
	resetNodeState(t)
	closed := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch strings.TrimPrefix(r.URL.Path, "/app") {
		case "/apiv2/sessions":
			if r.URL.Query().Get("tag") != "ali" || r.Header.Get("Token") != "t" {
				t.Errorf("query %s", r.URL.RawQuery)
			}
			w.Write([]byte(`{"success":true,"obj":[{"id":"7","user":"ali","source":"5.6.7.8:1234","destination":"x.com:443","createdAt":1,"up":1,"down":2}]}`))
		case "/apiv2/closeSessions":
			r.ParseForm()
			closed = r.PostForm.Get("u")
			w.Write([]byte(`{"success":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	db := database.GetDB()
	up := model.Node{Name: "de", Enable: true, BaseUrl: srv.URL, Token: "t"}
	down := model.Node{Name: "nl", Enable: true, BaseUrl: "http://127.0.0.1:1", Token: "t"}
	db.Create(&up)
	db.Create(&down)
	nodeStatusMu.Lock()
	nodeStatuses[up.Id] = NodeStatus{State: "online"}
	nodeStatuses[down.Id] = NodeStatus{State: "offline"}
	nodeStatusMu.Unlock()

	var s StatsService
	got, err := s.GetClusterSessions("user", "ali")
	if err != nil || len(got) != 1 || got[0].Node != "de" || got[0].Source != "5.6.7.8:1234" || got[0].ID == "7" {
		t.Fatalf("%+v %v", got, err)
	}
	if other, _ := s.GetClusterSessions("inbound", "in-1"); len(other) != 0 {
		t.Fatalf("inbound sessions went to the nodes: %+v", other)
	}
	if err := s.CloseClusterSessions("ali"); err != nil || closed != "ali" {
		t.Fatalf("close: %v %q", err, closed)
	}
}
