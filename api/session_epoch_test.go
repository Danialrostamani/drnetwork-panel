package api

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

type panelClient struct {
	t   *testing.T
	srv *httptest.Server
	c   *http.Client
}

func newPanelClient(t *testing.T, srv *httptest.Server) *panelClient {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	c := srv.Client()
	c.Jar = jar
	return &panelClient{t: t, srv: srv, c: c}
}

// post sends a form the way the panel's own pages do.
func (p *panelClient) post(action string, form url.Values) (int, Msg) {
	p.t.Helper()
	req, err := http.NewRequest(http.MethodPost, p.srv.URL+"/api/"+action, strings.NewReader(form.Encode()))
	if err != nil {
		p.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", p.srv.URL)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	return p.do(req)
}

func (p *panelClient) get(action string) (int, Msg) {
	p.t.Helper()
	req, err := http.NewRequest(http.MethodGet, p.srv.URL+"/api/"+action, nil)
	if err != nil {
		p.t.Fatal(err)
	}
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	return p.do(req)
}

func (p *panelClient) do(req *http.Request) (int, Msg) {
	p.t.Helper()
	resp, err := p.c.Do(req)
	if err != nil {
		p.t.Fatal(err)
	}
	defer resp.Body.Close()
	var m Msg
	_ = json.NewDecoder(resp.Body).Decode(&m)
	return resp.StatusCode, m
}

func (p *panelClient) login(user, pass string) {
	p.t.Helper()
	if code, m := p.post("login", url.Values{"user": {user}, "pass": {pass}}); code != http.StatusOK || !m.Success {
		p.t.Fatalf("login as %s: %d %+v", user, code, m)
	}
}

// copyCookiesTo gives other the same session cookie, as a stolen copy would.
func (p *panelClient) copyCookiesTo(other *panelClient) {
	u, _ := url.Parse(p.srv.URL)
	other.c.Jar.SetCookies(u, p.c.Jar.Cookies(u))
}

// Renaming the account used to leave the session naming the old user: the
// panel stayed open, but the tokens, two-factor login and a second change
// all looked up a user that no longer existed.
func TestCredentialChangeKeepsTheCurrentSessionWorking(t *testing.T) {
	srv := httptest.NewServer(loginEngine(t))
	defer srv.Close()

	a := newPanelClient(t, srv)
	a.login("admin", "admin")
	if code, m := a.post("changePass", url.Values{"oldPass": {"admin"}, "newUsername": {"root"}, "newPass": {"first-secret"}}); code != http.StatusOK || !m.Success {
		t.Fatalf("change credentials: %d %+v", code, m)
	}

	if code, m := a.get("totp"); code != http.StatusOK || !m.Success {
		t.Fatalf("two-factor state after the rename: %d %+v -- the session still names the old user", code, m)
	}
	if code, m := a.post("changePass", url.Values{"oldPass": {"first-secret"}, "newUsername": {"root"}, "newPass": {"second-secret"}}); code != http.StatusOK || !m.Success {
		t.Fatalf("a second change after the rename: %d %+v", code, m)
	}
	if code, m := a.get("settings"); code != http.StatusOK || !m.Success {
		t.Fatalf("the session that made the changes was signed out: %d %+v", code, m)
	}
}

// A session cookie is signed but names only a user, so a copy taken before the
// password changed used to stay valid until it expired.
func TestCredentialChangeEndsOtherSessions(t *testing.T) {
	srv := httptest.NewServer(loginEngine(t))
	defer srv.Close()

	a := newPanelClient(t, srv)
	a.login("admin", "admin")
	stolen := newPanelClient(t, srv)
	a.copyCookiesTo(stolen)
	other := newPanelClient(t, srv)
	other.login("admin", "admin")

	if code, _ := stolen.get("settings"); code != http.StatusOK {
		t.Fatalf("the copied cookie did not work before the change (status %d); the test proves nothing", code)
	}
	if code, m := a.post("changePass", url.Values{"oldPass": {"admin"}, "newUsername": {"admin"}, "newPass": {"new-secret"}}); code != http.StatusOK || !m.Success {
		t.Fatalf("change password: %d %+v", code, m)
	}

	for name, c := range map[string]*panelClient{"copied cookie": stolen, "other browser": other} {
		if code, _ := c.get("settings"); code != http.StatusUnauthorized {
			t.Errorf("%s: status %d after the password change, want 401", name, code)
		}
	}
	if code, _ := a.get("settings"); code != http.StatusOK {
		t.Errorf("the session that changed the password: status %d, want 200", code)
	}

	// Logging in again with the new password works as before.
	again := newPanelClient(t, srv)
	again.login("admin", "new-secret")
	if code, _ := again.get("settings"); code != http.StatusOK {
		t.Errorf("a new login after the change: status %d, want 200", code)
	}
}

// Only login and logout are open without a session. Any path ending in
// "login" or "logout" used to skip the check.
func TestOnlyLoginAndLogoutAreOpen(t *testing.T) {
	srv := httptest.NewServer(loginEngine(t))
	defer srv.Close()
	anon := newPanelClient(t, srv)

	if code, _ := anon.get("login"); code != http.StatusUnauthorized {
		t.Errorf("GET login without a session: status %d, want 401", code)
	}
	if code, _ := anon.post("logout", nil); code != http.StatusUnauthorized {
		t.Errorf("POST logout without a session: status %d, want 401", code)
	}
	if code, _ := anon.get("settings"); code != http.StatusUnauthorized {
		t.Errorf("settings without a session: status %d, want 401", code)
	}
	if code, m := anon.get("logout"); code != http.StatusOK || !m.Success {
		t.Errorf("GET logout without a session: %d %+v, want it open", code, m)
	}
	if code, _ := anon.post("login", url.Values{"user": {"admin"}, "pass": {"admin"}}); code != http.StatusOK {
		t.Errorf("POST login: status %d, want it open", code)
	}
}
