package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database/model"
)

func TestFilterTarget(t *testing.T) {
	n := &model.Node{BaseUrl: "https://1.2.3.4:2095/app/"}
	if got := filterTarget(n, []model.Inbound{{Options: json.RawMessage(`{}`)}, {Options: json.RawMessage(`{"listen_port":8443}`)}}); got != "1.2.3.4:8443" {
		t.Fatalf("inbound port: %s", got)
	}
	if got := filterTarget(n, nil); got != "1.2.3.4:2095" {
		t.Fatalf("panel port: %s", got)
	}
	if got := filterTarget(&model.Node{BaseUrl: "https://node.example"}, nil); got != "node.example:443" {
		t.Fatalf("default port: %s", got)
	}
}

func TestCheckTCPFromAndVerdicts(t *testing.T) {
	polls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/check-tcp":
			if r.URL.Query().Get("host") != "1.2.3.4:443" || len(r.URL.Query()["node"]) != 3 {
				t.Errorf("query %s", r.URL.RawQuery)
			}
			w.Write([]byte(`{"ok":1,"request_id":"abc"}`))
		case "/check-result/abc":
			polls++
			if polls == 1 {
				w.Write([]byte(`{"ir1":[{"error":"Connection timed out"}],"ir3":null,"ir5":null}`))
				return
			}
			w.Write([]byte(`{"ir1":[{"error":"Connection timed out"}],"ir3":[{"time":0.05,"address":"1.2.3.4"}],"ir5":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	oldBase, oldPoll := checkHostBase, checkHostPoll
	checkHostBase, checkHostPoll = srv.URL, 10*time.Millisecond
	t.Cleanup(func() { checkHostBase, checkHostPoll = oldBase, oldPoll })

	ok, tried, err := checkTCPFrom(context.Background(), "1.2.3.4:443", []string{"ir1", "ir3", "ir5"})
	if err != nil || ok != 1 || tried != 2 || polls != 2 {
		t.Fatalf("ok %d tried %d polls %d err %v", ok, tried, polls, err)
	}

	filterMu.Lock()
	filterStates = map[uint]*filterNodeState{}
	filterHideOn = true
	filterMu.Unlock()
	t.Cleanup(func() {
		filterMu.Lock()
		filterStates, filterHideOn = map[uint]*filterNodeState{}, false
		filterMu.Unlock()
	})
	DrainNodeEvents()
	n := &model.Node{Id: 9, Name: "n"}
	recordFilterResult(n, "x", 0, 3, 1)
	if nodeFilterHidden(9) {
		t.Fatal("filtered after one round")
	}
	recordFilterResult(n, "x", 0, 0, 2) // nobody answered: no verdict
	recordFilterResult(n, "x", 0, 2, 3)
	if !nodeFilterHidden(9) || !nodeFilterState(9) {
		t.Fatal("not filtered after two rounds")
	}
	recordFilterResult(n, "x", 1, 3, 4)
	if nodeFilterState(9) {
		t.Fatal("still filtered after Iran reached it")
	}
	ev := DrainNodeEvents()
	if len(ev) != 2 || ev[0].Kind != "filtered" || ev[1].Kind != "unfiltered" {
		t.Fatalf("events %+v", ev)
	}
}
