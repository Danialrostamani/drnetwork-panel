package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
)

// updateFakeNode answers a panel update the way a node's API does: answer is
// what POST panelUpdate replies, "" for a node too old to know the action.
type updateFakeNode struct {
	mu      sync.Mutex
	answer  string
	langs   []string
	checks  []string
	logins  []string
	logouts int
}

func newUpdateFakeNode(t *testing.T, token, answer string) (*httptest.Server, *updateFakeNode) {
	t.Helper()
	f := &updateFakeNode{answer: answer}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The panel API of the pages, with a login: what a node from
		// DrNetwork 31 to 33 offers for its Update button.
		if strings.HasPrefix(r.URL.Path, "/app/api/") {
			f.mu.Lock()
			defer f.mu.Unlock()
			if r.Method == http.MethodPost && r.Header.Get("X-Requested-With") != "XMLHttpRequest" {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			c, _ := r.Cookie("s-ui")
			in := c != nil && c.Value == "sess-1"
			switch r.URL.Path {
			case "/app/api/login":
				_ = r.ParseForm()
				f.logins = append(f.logins, r.FormValue("user"))
				switch {
				case r.FormValue("user") == "admin" && r.FormValue("pass") == "secret":
					http.SetCookie(w, &http.Cookie{Name: "s-ui", Value: "sess-1", Path: "/app/", Secure: true})
					_, _ = w.Write([]byte(`{"success":true,"msg":""}`))
				case r.FormValue("user") == "totp" && r.FormValue("code") == "":
					_, _ = w.Write([]byte(`{"success":false,"msg":"two-factor code required\n","obj":{"totp":true}}`))
				default:
					_, _ = w.Write([]byte(`{"success":false,"msg":": wrong user or password\n"}`))
				}
			case "/app/api/panelUpdate":
				if !in {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				_ = r.ParseForm()
				f.langs = append(f.langs, r.FormValue("lang"))
				_, _ = w.Write([]byte(`{"success":true,"msg":"panelUpdate","obj":"v34"}`))
			case "/app/api/logout":
				if in {
					f.logouts++
				}
				_, _ = w.Write([]byte(`{"success":true}`))
			default:
				w.WriteHeader(http.StatusNotFound)
			}
			return
		}
		if r.Header.Get("Token") != token {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		if r.URL.Path != "/app/apiv2/panelUpdate" || f.answer == "" {
			_, _ = w.Write([]byte(`{"success":false,"msg":"failed: unknown action:  panelUpdate\n"}`))
			return
		}
		if r.Method == http.MethodGet {
			f.checks = append(f.checks, r.URL.Query().Get("check"))
			_, _ = w.Write([]byte(`{"success":true,"obj":{"current":"33","latest":"v34","newer":true,"running":false}}`))
			return
		}
		_ = r.ParseForm()
		f.langs = append(f.langs, r.FormValue("lang"))
		_, _ = w.Write([]byte(f.answer))
	}))
	t.Cleanup(srv.Close)
	return srv, f
}

func TestNodePanelUpdates(t *testing.T) {
	resetNodeState(t)
	db := database.GetDB()
	mk := func(name, answer string) (model.Node, *updateFakeNode) {
		srv, f := newUpdateFakeNode(t, "tok-"+name, answer)
		n := model.Node{Name: name, Enable: true, BaseUrl: srv.URL, WebPath: "/app/", Token: "tok-" + name}
		if err := db.Create(&n).Error; err != nil {
			t.Fatal(err)
		}
		return n, f
	}
	fresh, freshF := mk("fresh", `{"success":true,"msg":"panelUpdate","obj":"v34"}`)
	current, _ := mk("current", `{"success":false,"msg":"panelUpdate: the panel is up to date:  34\n"}`)
	busy, _ := mk("busy", `{"success":false,"msg":"panelUpdate: an update to  v34  is already running\n"}`)
	docker, _ := mk("docker", `{"success":false,"msg":"panelUpdate: this panel cannot update itself:  docker\n"}`)
	old, oldF := mk("old", "")
	off := model.Node{Name: "off", Enable: false, BaseUrl: "http://127.0.0.1:1", Token: "t"}
	db.Create(&off)
	db.Model(&off).Update("enable", false) // Create takes false for the default

	svc := NodeSyncService{}
	res, err := svc.NodeActionWith([]uint{fresh.Id, current.Id, busy.Id, docker.Id, old.Id, off.Id}, "updatePanel", "admin", NodeActionOpts{Lang: "fa"})
	if err != nil || len(res) != 6 {
		t.Fatalf("update: %+v, %v", res, err)
	}
	got := map[string]NodeActionResult{}
	for _, r := range res {
		got[r.Name] = r
	}
	if r := got["fresh"]; !r.Ok || r.Note != "v34" {
		t.Fatalf("fresh node: %+v", r)
	}
	if r := got["current"]; !r.Ok || r.Note != "upToDate" {
		t.Fatalf("up-to-date node: %+v", r)
	}
	if r := got["busy"]; !r.Ok || r.Note != "running" {
		t.Fatalf("node already updating: %+v", r)
	}
	if r := got["docker"]; r.Ok || r.Error != "this panel cannot update itself: docker" {
		t.Fatalf("docker node: %+v", r)
	}
	if r := got["old"]; r.Ok || r.Error != ErrNodeUpdateTooOld {
		t.Fatalf("old node: %+v", r)
	}
	if r := got["off"]; r.Ok || r.Error != "node is disabled" {
		t.Fatalf("disabled node: %+v", r)
	}
	freshF.mu.Lock()
	if len(freshF.langs) != 1 || freshF.langs[0] != "fa" {
		t.Fatalf("the node was asked in %v", freshF.langs)
	}
	freshF.mu.Unlock()

	// One node at a time, for the dialog.
	info, err := svc.NodePanelUpdate(fresh.Id, true)
	if err != nil || !strings.Contains(string(info), `"latest":"v34"`) {
		t.Fatalf("info: %s, %v", info, err)
	}
	freshF.mu.Lock()
	if len(freshF.checks) != 1 || freshF.checks[0] != "1" {
		t.Fatalf("check passed as %v", freshF.checks)
	}
	freshF.mu.Unlock()
	if _, err := svc.NodePanelUpdate(old.Id, false); err == nil || err.Error() != ErrNodeUpdateTooOld {
		t.Fatalf("info of an old node: %v", err)
	}

	// A node from DrNetwork 31 to 33 updates by its Update button: the master
	// logs in with the user and password given, and logs out again.
	upd := func(opts NodeActionOpts) NodeActionResult {
		t.Helper()
		opts.Lang = "fa"
		res, err := svc.NodeActionWith([]uint{old.Id}, "updatePanel", "admin", opts)
		if err != nil || len(res) != 1 {
			t.Fatalf("update: %+v, %v", res, err)
		}
		return res[0]
	}
	if r := upd(NodeActionOpts{User: "admin", Pass: "wrong"}); r.Ok || r.Error != "login to the node's panel failed: wrong user or password" {
		t.Fatalf("a wrong password: %+v", r)
	}
	if r := upd(NodeActionOpts{User: "totp", Pass: "x"}); r.Ok || r.Error != ErrNodeLoginTotp {
		t.Fatalf("a two-factor login: %+v", r)
	}
	if r := upd(NodeActionOpts{User: "admin", Pass: "secret"}); !r.Ok || r.Note != "v34" {
		t.Fatalf("an update by login: %+v", r)
	}
	oldF.mu.Lock()
	if len(oldF.langs) != 1 || oldF.langs[0] != "fa" || oldF.logouts != 1 || len(oldF.logins) != 3 {
		t.Fatalf("by login: langs %v, logouts %d, logins %v", oldF.langs, oldF.logouts, oldF.logins)
	}
	oldF.mu.Unlock()
	// A node that updates through the API is never logged in to.
	if r := func() NodeActionResult {
		res, _ := svc.NodeActionWith([]uint{fresh.Id}, "updatePanel", "admin", NodeActionOpts{User: "admin", Pass: "secret"})
		return res[0]
	}(); !r.Ok || r.Note != "v34" {
		t.Fatalf("fresh node with a login: %+v", r)
	}
	freshF.mu.Lock()
	if len(freshF.logins) != 0 {
		t.Fatalf("a node that has the API was logged in to: %v", freshF.logins)
	}
	freshF.mu.Unlock()
}
