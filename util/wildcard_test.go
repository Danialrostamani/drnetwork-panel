package util

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

// fixedLabels makes the per-client label readable; the keyed one is tested in
// util/hosts.
func fixedLabels(t *testing.T) {
	t.Helper()
	old := WildcardLabel
	WildcardLabel = func(base, name string) string { return "l-" + name }
	t.Cleanup(func() { WildcardLabel = old })
}

func TestExpandWildcardHost(t *testing.T) {
	fixedLabels(t)
	cases := []struct{ in, want string }{
		{"*.cdn.example.com", "l-ali.cdn.example.com"},
		{" *.CDN.Example.com. ", "l-ali.cdn.example.com"},
		{"cdn.example.com", "cdn.example.com"},
		{"", ""},
		{"*.*.example.com", "*.*.example.com"},
		{"1.2.3.4", "1.2.3.4"},
	}
	for _, c := range cases {
		if got := ExpandWildcardHost(c.in, "ali"); got != c.want {
			t.Errorf("ExpandWildcardHost(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestDefaultWildcardLabelIsAStableDNSLabel(t *testing.T) {
	a := ExpandWildcardHost("*.cdn.example.com", "ali")
	if a != ExpandWildcardHost("*.cdn.example.com", "ali") {
		t.Fatal("label is not stable")
	}
	if a == ExpandWildcardHost("*.cdn.example.com", "reza") {
		t.Fatal("two clients got the same host")
	}
	label := strings.TrimSuffix(a, ".cdn.example.com")
	if label == a || label == "" || len(label) > 63 || strings.ContainsAny(label, ".*_ ") {
		t.Fatalf("bad label in %q", a)
	}
}

func TestExpandWildcardOutboundCopiesSharedMaps(t *testing.T) {
	fixedLabels(t)
	tls := map[string]interface{}{"enabled": true, "server_name": "*.cdn.example.com"}
	headers := map[string]interface{}{"Host": "cdn.example.com"}
	ws := map[string]interface{}{"type": "ws", "path": "/ws", "headers": headers}
	outs := map[string]map[string]interface{}{}
	for _, name := range []string{"ali", "reza"} {
		out := map[string]interface{}{"type": "vless", "server": "*.cdn.example.com", "tls": tls, "transport": ws}
		if !ExpandWildcardOutbound(out, name) {
			t.Fatalf("%s: wildcard server not reported", name)
		}
		outs[name] = out
	}
	for name, out := range outs {
		want := "l-" + name + ".cdn.example.com"
		if out["server"] != want {
			t.Errorf("%s: server %v", name, out["server"])
		}
		if sni := out["tls"].(map[string]interface{})["server_name"]; sni != want {
			t.Errorf("%s: server_name %v", name, sni)
		}
		h := out["transport"].(map[string]interface{})["headers"].(map[string]interface{})
		if h["Host"] != want {
			t.Errorf("%s: ws Host %v", name, h["Host"])
		}
	}
	if tls["server_name"] != "*.cdn.example.com" || headers["Host"] != "cdn.example.com" {
		t.Fatalf("shared maps were changed: %v %v", tls, headers)
	}

	// A server name or host of another domain stays; an empty SNI follows.
	out := map[string]interface{}{
		"server":    "*.cdn.example.com",
		"tls":       map[string]interface{}{"enabled": true},
		"transport": map[string]interface{}{"type": "httpupgrade", "host": "front.example.org"},
	}
	ExpandWildcardOutbound(out, "ali")
	if sni := out["tls"].(map[string]interface{})["server_name"]; sni != "l-ali.cdn.example.com" {
		t.Errorf("empty SNI became %v", sni)
	}
	if h := out["transport"].(map[string]interface{})["host"]; h != "front.example.org" {
		t.Errorf("foreign host became %v", h)
	}

	httpOut := map[string]interface{}{
		"server":    "*.cdn.example.com",
		"transport": map[string]interface{}{"type": "http", "host": []interface{}{"*.cdn.example.com", "other.example.org"}},
	}
	ExpandWildcardOutbound(httpOut, "ali")
	list := httpOut["transport"].(map[string]interface{})["host"].([]interface{})
	if list[0] != "l-ali.cdn.example.com" || list[1] != "other.example.org" {
		t.Errorf("http hosts %v", list)
	}

	plain := map[string]interface{}{"server": "cdn.example.com", "tls": tls}
	if ExpandWildcardOutbound(plain, "ali") || plain["server"] != "cdn.example.com" {
		t.Fatalf("plain server changed: %v", plain)
	}
}

func TestLinkGeneratorGivesEachClientItsOwnHost(t *testing.T) {
	fixedLabels(t)
	transport := map[string]interface{}{"type": "ws", "path": "/ws", "headers": map[string]interface{}{"Host": "*.cdn.example.com"}}
	addrs := json.RawMessage(`[
		{"server":"*.cdn.example.com","server_port":443,"remark":"-cdn","tls":{"enabled":true,"server_name":"*.cdn.example.com"}},
		{"server":"direct.example.com","server_port":8443,"remark":"-direct"}
	]`)
	for _, name := range []string{"ali", "reza"} {
		want := "l-" + name + ".cdn.example.com"

		in := inboundFor(t, "vless", map[string]interface{}{"transport": transport})
		in.Addrs = addrs
		links := LinkGenerator(clientConfigFor(t, "vless", map[string]interface{}{"uuid": "6f1c7d2e-0d6a-4a8e-9a63-1f2b3c4d5e6f"}), in, "panel.example.com", name, name)
		if len(links) != 2 {
			t.Fatalf("%s: %d links", name, len(links))
		}
		u, err := url.Parse(links[0])
		if err != nil {
			t.Fatal(err)
		}
		q := u.Query()
		if u.Hostname() != want || q.Get("host") != want || q.Get("sni") != want {
			t.Errorf("%s: wildcard link %s", name, links[0])
		}
		d, err := url.Parse(links[1])
		if err != nil {
			t.Fatal(err)
		}
		if d.Hostname() != "direct.example.com" || d.Query().Get("host") != "*.cdn.example.com" {
			t.Errorf("%s: plain address changed: %s", name, links[1])
		}

		vin := inboundFor(t, "vmess", map[string]interface{}{"transport": transport})
		vin.Addrs = addrs
		vlinks := LinkGenerator(clientConfigFor(t, "vmess", map[string]interface{}{"uuid": "6f1c7d2e-0d6a-4a8e-9a63-1f2b3c4d5e6f"}), vin, "panel.example.com", name, name)
		if len(vlinks) != 2 {
			t.Fatalf("%s: %d vmess links", name, len(vlinks))
		}
		obj := vmessObject(t, vlinks[0])
		if obj["add"] != want || obj["host"] != want || obj["sni"] != want {
			t.Errorf("%s: vmess wildcard link %v", name, obj)
		}
		if obj := vmessObject(t, vlinks[1]); obj["add"] != "direct.example.com" {
			t.Errorf("%s: vmess plain address %v", name, obj)
		}
	}
}

func vmessObject(t *testing.T, link string) map[string]interface{} {
	t.Helper()
	b64 := strings.TrimPrefix(link, "vmess://")
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		if raw, err = base64.RawStdEncoding.DecodeString(b64); err != nil {
			t.Fatalf("vmess link %q: %v", link, err)
		}
	}
	var obj map[string]interface{}
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatal(err)
	}
	return obj
}
