package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/util/hosts"
)

func TestSubLabelOKAndClientSubURL(t *testing.T) {
	settingTestDB(t)
	s := &SettingService{}
	if err := s.saveSetting("subDomain", "*.sub.example.com, sub2.example.com"); err != nil {
		t.Fatal(err)
	}
	secret := s.GetLabelSecret()
	if len(secret) == 0 || string(secret) != string(s.GetLabelSecret()) {
		t.Fatal("the label secret is not kept")
	}
	own := hosts.Concrete(secret, "*.sub.example.com", "alice")
	for host, want := range map[string]bool{
		own:                            true,
		strings.ToUpper(own) + ":2096": true,
		hosts.Concrete(secret, "*.sub.example.com", "bob"): false,
		"x." + own:             false,
		"sub2.example.com":     true,
		"sub2.example.com:443": true,
		"panel.example.org":    true,
	} {
		if got := s.SubLabelOK(host, "alice"); got != want {
			t.Errorf("SubLabelOK(%q) = %v, want %v", host, got, want)
		}
	}

	u, err := s.ClientSubURL("alice", "ignored.example.org")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(u, "://"+own) || !strings.HasSuffix(u, "/alice") {
		t.Fatalf("ClientSubURL = %q", u)
	}
	if u, err := s.ClientSubURL("a b", ""); err != nil || !strings.HasSuffix(u, "/a%20b") || strings.Contains(u, "*") {
		t.Fatalf("ClientSubURL(a b) = %q, %v", u, err)
	}
	// The first domain is what new links use; the others keep answering.
	if err := s.saveSetting("subDomain", "sub2.example.com, *.sub.example.com"); err != nil {
		t.Fatal(err)
	}
	if u, _ := s.ClientSubURL("alice", ""); !strings.Contains(u, "://sub2.example.com") {
		t.Fatalf("ClientSubURL after the swap = %q", u)
	}
	if !s.SubLabelOK(own, "alice") || s.SubLabelOK(own, "bob") {
		t.Fatal("the wildcard entry stopped working when it moved down the list")
	}
}

func TestCheckInboundAddrs(t *testing.T) {
	in := func(typ, transport, addrs string) *model.Inbound {
		opts := map[string]interface{}{"listen_port": 443}
		if transport != "" {
			opts["transport"] = map[string]interface{}{"type": transport}
		}
		raw, _ := json.Marshal(opts)
		return &model.Inbound{Type: typ, Options: raw, Addrs: json.RawMessage(addrs)}
	}
	reality := in("vless", "grpc", `[{"server":"cdn.example.com","cdn":true}]`)
	reality.Tls = &model.Tls{Server: json.RawMessage(`{"enabled":true,"reality":{"enabled":true}}`)}
	cases := []struct {
		name    string
		inbound *model.Inbound
		ok      bool
	}{
		{"plain", in("vless", "", `[{"server":"a.example.com"}]`), true},
		{"no addresses", in("vless", "", ``), true},
		{"wildcard", in("trojan", "", `[{"server":"*.cdn.example.com"}]`), true},
		{"star inside", in("vless", "", `[{"server":"a.*.example.com"}]`), false},
		{"two stars", in("vless", "", `[{"server":"*.*.example.com"}]`), false},
		{"cdn ws", in("vless", "ws", `[{"server":"*.cdn.example.com","cdn":true}]`), true},
		{"cdn grpc vmess", in("vmess", "grpc", `[{"server":"cdn.example.com","cdn":true}]`), true},
		{"cdn httpupgrade trojan", in("trojan", "httpupgrade", `[{"server":"cdn.example.com","cdn":true}]`), true},
		{"cdn tcp", in("vless", "", `[{"server":"cdn.example.com","cdn":true}]`), false},
		{"cdn hysteria2", in("hysteria2", "", `[{"server":"cdn.example.com","cdn":true}]`), false},
		{"cdn shadowsocks", in("shadowsocks", "", `[{"server":"cdn.example.com","cdn":true}]`), false},
		{"cdn reality address", in("vless", "ws", `[{"server":"cdn.example.com","cdn":true,"tls":{"enabled":true,"reality":{"enabled":true}}}]`), false},
		{"cdn reality inbound", reality, false},
		{"direct reality", func() *model.Inbound {
			i := in("vless", "", `[{"server":"a.example.com"}]`)
			i.Tls = reality.Tls
			return i
		}(), true},
	}
	for _, c := range cases {
		if err := checkInboundAddrs(c.inbound); (err == nil) != c.ok {
			t.Errorf("%s: err = %v", c.name, err)
		}
	}
}

func TestCheckNewSetting(t *testing.T) {
	db := settingTestDB(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "index.html")
	if err := os.WriteFile(file, []byte("hi"), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		key, value string
		all        map[string]string
		ok         bool
	}{
		{"subDomain", "sub.example.com, *.sub.example.com", nil, true},
		{"subDomain", "*.*.example.com", nil, false},
		{"subDomain", "bad_domain!", nil, false},
		{"subAnnounce", strings.Repeat("ب", 1000), nil, true},
		{"subAnnounce", strings.Repeat("ب", 1001), nil, false},
		{"subSupportUrl", "", nil, true},
		{"subSupportUrl", "https://t.me/drnet", nil, true},
		{"subSupportUrl", "tg://resolve?domain=drnet", nil, true},
		{"subSupportUrl", "javascript:alert(1)", nil, false},
		{"subSupportUrl", "https://x.example\r\nSet-Cookie: a=b", nil, false},
		{"subWebPage", "false", nil, true},
		{"subWebPage", "maybe", nil, false},
		{"subNameTemplate", "", nil, true},
		{"subNameTemplate", "{USER} {PROTOCOL}", nil, true},
		{"subNameTemplate", "no variables", nil, false},
		{"webDecoyDir", "", nil, true},
		{"webDecoyDir", "relative/site", nil, false},
		{"webDecoyDir", "/", nil, false},
		{"webDecoyDir", filepath.Join(dir, "missing"), nil, false},
		{"webDecoyDir", file, nil, false},
		{"webDecoyDir", dir, map[string]string{"webPath": "/"}, false},
		{"webDecoyDir", dir, map[string]string{"webPath": "/app/"}, true},
		{"webDecoyDir", dir, map[string]string{}, true},
	}
	for _, c := range cases {
		all := c.all
		if all == nil {
			all = map[string]string{}
		}
		if err := checkNewSetting(db, c.key, c.value, all); (err == nil) != c.ok {
			t.Errorf("%s=%q: err = %v", c.key, c.value, err)
		}
	}
	// Save runs the checks.
	payload, _ := json.Marshal(map[string]string{"subNameTemplate": "no variables"})
	if err := (&SettingService{}).Save(db, payload); err == nil {
		t.Fatal("Save took a template without variables")
	}
}
