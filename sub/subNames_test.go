package sub

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/service"
	"github.com/Danialrostamani/drnetwork-panel/util/hosts"

	"github.com/gin-gonic/gin"
)

func TestHeaderText(t *testing.T) {
	b64 := func(s string) string { return "base64:" + base64.StdEncoding.EncodeToString([]byte(s)) }
	for in, want := range map[string]string{
		"Renew before Friday": "Renew before Friday",
		"":                    "",
		"تمدید تا جمعه":       b64("تمدید تا جمعه"),
		"line\nbreak":         b64("line\nbreak"),
		"base64:abc":          b64("base64:abc"),
	} {
		if got := headerText(in); got != want {
			t.Errorf("headerText(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHeaderURL(t *testing.T) {
	for in, want := range map[string]string{
		"https://t.me/support":      "https://t.me/support",
		"https://example.com/a b":   "https://example.com/a%20b",
		"https://example.com/پ":     "https://example.com/%D9%BE",
		"https://example.com/\r\nX": "https://example.com/%0D%0AX",
	} {
		if got := headerURL(in); got != want {
			t.Errorf("headerURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseLinkAndSetLinkName(t *testing.T) {
	obj, _ := json.Marshal(map[string]interface{}{"v": "2", "add": "v.example.com", "port": "8443", "net": "tcp", "type": "http", "ps": "old"})
	vmess := "vmess://" + base64.StdEncoding.EncodeToString(obj)
	cases := []struct {
		uri                       string
		protocol, network, server string
		port                      int
		name                      string
	}{
		{"vless://id@s.example.com:443?type=ws&security=tls#old%20name", "vless", "ws", "s.example.com", 443, "old name"},
		{"hy2://pw@h.example.com:8443?sni=h.example.com#old", "hysteria2", "quic", "h.example.com", 8443, "old"},
		{"trojan://pw@[2001:db8::1]:443#old", "trojan", "tcp", "2001:db8::1", 443, "old"},
		{"ss://YWVzLTEyOC1nY206cHc@ss.example.com:8388#old", "shadowsocks", "tcp", "ss.example.com", 8388, "old"},
		{vmess, "vmess", "http", "v.example.com", 8443, "old"},
	}
	for _, c := range cases {
		info, name, ok := parseLink(c.uri)
		if !ok || info.protocol != c.protocol || info.network != c.network || info.server != c.server || info.port != c.port || name != c.name {
			t.Errorf("parseLink(%q) = %+v %q %v", c.uri, info, name, ok)
			continue
		}
		renamed := setLinkName(c.uri, "علی #1 ✓")
		again, name, ok := parseLink(renamed)
		if !ok || name != "علی #1 ✓" || *again != *info {
			t.Errorf("setLinkName(%q) = %q, read back as %+v %q", c.uri, renamed, again, name)
		}
	}
	if _, _, ok := parseLink("not a link"); ok {
		t.Error("a non-link parsed")
	}
}

// One subscription with the template, the app headers and a wildcard domain,
// the way an app asks for it.
func TestSubscriptionNamesLinksAndSendsAppHeaders(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "names.db")); err != nil {
		t.Fatal(err)
	}
	db := database.GetDB()
	for k, v := range map[string]string{
		"subNameTemplate": "{USER} | {PROTOCOL}-{NETWORK} | {NODE}",
		"subAnnounce":     "تمدید تا جمعه",
		"subSupportUrl":   "https://t.me/drnet support",
		"subDomain":       "*.sub.example.com, sub2.example.com",
	} {
		if err := db.Create(&model.Setting{Key: k, Value: v}).Error; err != nil {
			t.Fatal(err)
		}
	}
	node := model.Node{Name: "de1", Enable: true, BaseUrl: "https://node.example", WebPath: "/app/", Token: "token"}
	if err := db.Create(&node).Error; err != nil {
		t.Fatal(err)
	}
	links, _ := json.Marshal([]map[string]string{
		{"type": "local", "remark": "in1", "uri": "vless://id@srv.example.com:443?type=ws&security=tls#in1-cdn"},
		{"type": "local", "remark": "in2", "uri": "vless://id@srv2.example.com:443?type=ws&security=tls#in2"},
		{"type": "external", "remark": "[de1] in3", "uri": "trojan://pw@de.example.com:443#%5Bde1%5D%20in3"},
		{"type": "external", "remark": "by hand", "uri": "vless://id@x.example.com:443#hand%20made"},
	})
	c := model.Client{Name: "alice", Enable: true, Inbounds: json.RawMessage(`[]`), Links: links, Config: json.RawMessage(`{}`)}
	if err := db.Create(&c).Error; err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	NewSubHandler(r.Group("/sub"))
	get := func(host string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "http://"+host+"/sub/alice", nil)
		req.Header.Set("User-Agent", "Happ/3.0")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}

	own := hosts.Concrete((&service.SettingService{}).GetLabelSecret(), "*.sub.example.com", "alice")
	if w := get(hosts.Concrete((&service.SettingService{}).GetLabelSecret(), "*.sub.example.com", "bob")); w.Code != 400 {
		t.Fatalf("another client's host answered %d", w.Code)
	}
	w := get(own)
	if w.Code != 200 {
		t.Fatalf("own host: %d %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(body)); err == nil {
		body = string(raw)
	}
	var names []string
	for _, l := range strings.Split(strings.TrimSpace(body), "\n") {
		_, name, ok := parseLink(l)
		if !ok {
			t.Fatalf("bad link %q", l)
		}
		names = append(names, name)
	}
	if len(names) != 4 {
		t.Fatalf("links: %q", names)
	}
	if names[0] != "alice | vless-ws" || names[1] == names[0] || !strings.HasPrefix(names[1], "alice | vless-ws") {
		t.Errorf("local names %q %q", names[0], names[1])
	}
	if names[2] != "alice | trojan-tcp | de1" {
		t.Errorf("node link name %q", names[2])
	}
	if names[3] != "hand made" {
		t.Errorf("a link added by hand was renamed: %q", names[3])
	}

	h := w.Header()
	if got := h.Get("Announce"); got != headerText("تمدید تا جمعه") {
		t.Errorf("Announce %q", got)
	}
	if got := h.Get("Support-Url"); got != "https://t.me/drnet%20support" {
		t.Errorf("Support-Url %q", got)
	}
	if got := h.Get("Profile-Web-Page-Url"); !strings.Contains(got, own) || !strings.HasSuffix(got, "/sub/alice") {
		t.Errorf("Profile-Web-Page-Url %q", got)
	}

	// Off and empty send nothing.
	db.Model(&model.Setting{}).Where("key IN ?", []string{"subAnnounce", "subSupportUrl"}).Update("value", "")
	if err := db.Create(&model.Setting{Key: "subWebPage", Value: "false"}).Error; err != nil {
		t.Fatal(err)
	}
	w = get("sub2.example.com")
	if w.Code != 200 {
		t.Fatalf("second domain: %d", w.Code)
	}
	for _, k := range []string{"Announce", "Support-Url", "Profile-Web-Page-Url"} {
		if v := w.Header().Get(k); v != "" {
			t.Errorf("%s still sent: %q", k, v)
		}
	}
}

// The sing-box JSON and the Clash config get the client's own host under a
// wildcard address, and the template's names.
func TestJSONAndClashUseTheClientsHostAndNames(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "json.db")); err != nil {
		t.Fatal(err)
	}
	db := database.GetDB()
	if err := db.Create(&model.Setting{Key: "subNameTemplate", Value: "{USER} {PROTOCOL}-{NETWORK}"}).Error; err != nil {
		t.Fatal(err)
	}
	in := model.Inbound{Type: "vless", Tag: "in1",
		Options: json.RawMessage(`{"listen_port":443,"transport":{"type":"ws","path":"/ws","headers":{"Host":"*.cdn.example.com"}}}`),
		OutJson: json.RawMessage(`{"type":"vless","tag":"in1","server":"panel.example.com","server_port":443,"tls":{"enabled":true,"server_name":"*.cdn.example.com"},"transport":{"type":"ws","path":"/ws","headers":{"Host":"*.cdn.example.com"}}}`),
		Addrs:   json.RawMessage(`[{"server":"*.cdn.example.com","server_port":443,"remark":"-cdn"},{"server":"direct.example.com","server_port":8443,"remark":"-direct"}]`)}
	if err := db.Create(&in).Error; err != nil {
		t.Fatal(err)
	}
	ids, _ := json.Marshal([]uint{in.Id})
	c := model.Client{Name: "alice", Enable: true, Inbounds: ids, Links: json.RawMessage(`[]`),
		Config: json.RawMessage(`{"vless":{"name":"alice","uuid":"11111111-1111-1111-1111-111111111111"}}`)}
	if err := db.Create(&c).Error; err != nil {
		t.Fatal(err)
	}
	own := hosts.Concrete((&service.SettingService{}).GetLabelSecret(), "*.cdn.example.com", "alice")

	raw, _, err := (&JsonService{}).GetJson("alice", "")
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Outbounds []map[string]interface{} `json:"outbounds"`
	}
	if err := json.Unmarshal([]byte(*raw), &cfg); err != nil {
		t.Fatal(err)
	}
	byServer := map[string]map[string]interface{}{}
	for _, o := range cfg.Outbounds {
		if s, _ := o["server"].(string); s != "" {
			byServer[s] = o
		}
	}
	w, d := byServer[own], byServer["direct.example.com"]
	if w == nil || d == nil {
		t.Fatalf("outbounds %v", cfg.Outbounds)
	}
	if sni := w["tls"].(map[string]interface{})["server_name"]; sni != own {
		t.Errorf("server_name %v", sni)
	}
	if host := w["transport"].(map[string]interface{})["headers"].(map[string]interface{})["Host"]; host != own {
		t.Errorf("ws Host %v", host)
	}
	if sni := d["tls"].(map[string]interface{})["server_name"]; sni != "*.cdn.example.com" {
		t.Errorf("the direct address took the wildcard host: %v", sni)
	}
	if w["tag"] != "alice vless-ws" || d["tag"] == w["tag"] || !strings.HasPrefix(d["tag"].(string), "alice vless-ws") {
		t.Errorf("tags %v %v", w["tag"], d["tag"])
	}

	clash, _, err := (&ClashService{}).GetClash("alice")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(*clash, own) || !strings.Contains(*clash, "alice vless-ws") || strings.Contains(*clash, "server: '*.") || strings.Contains(*clash, "server: \"*.") {
		t.Errorf("clash config:\n%s", *clash)
	}
}
