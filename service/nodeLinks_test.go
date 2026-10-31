package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/util"
)

// TestNodeReplicaLinksMatchLocal makes the links of an inbound the way a node
// reports it to the master and checks they are the links the node makes of it
// itself, and that each one reads back as an outbound.
func TestNodeReplicaLinksMatchLocal(t *testing.T) {
	reality := &model.Tls{Id: 1, Server: json.RawMessage(`{"enabled":true,"server_name":"www.google.com","reality":{"enabled":true,"handshake":{"server":"www.google.com","server_port":443},"private_key":"priv","short_id":["abcd1234"]}}`), Client: json.RawMessage(`{"utls":{"enabled":true,"fingerprint":"chrome"},"reality":{"public_key":"PUBKEY"}}`)}
	plain := &model.Tls{Id: 2, Server: json.RawMessage(`{"enabled":true,"server_name":"sni.example","alpn":["h2","http/1.1"]}`), Client: json.RawMessage(`{"insecure":true}`)}
	cfg := json.RawMessage(`{"vless":{"name":"u1","uuid":"11111111-1111-1111-1111-111111111111","flow":"xtls-rprx-vision"},"vmess":{"name":"u1","uuid":"11111111-1111-1111-1111-111111111111","alterId":0},"trojan":{"name":"u1","password":"pw"},"hysteria2":{"name":"u1","password":"pw"},"tuic":{"name":"u1","uuid":"11111111-1111-1111-1111-111111111111","password":"pw"},"shadowsocks":{"name":"u1","password":"c3MyMDIyIGtleSAzMiBieXRlcyBsb25nIQ=="},"shadowsocks16":{"name":"u1","password":"c3MyMDIyIGtleSAxNg=="},"anytls":{"name":"u1","password":"pw"}}`)
	cases := []struct {
		name string
		in   model.Inbound
	}{
		{"vless-reality", model.Inbound{Type: "vless", Tag: "vr", TlsId: 1, Tls: reality, Options: json.RawMessage(`{"listen":"::","listen_port":443}`)}},
		{"vless-ws-tls", model.Inbound{Type: "vless", Tag: "vw", TlsId: 2, Tls: plain, Options: json.RawMessage(`{"listen":"::","listen_port":8443,"transport":{"type":"ws","path":"/ws"}}`)}},
		{"vless-grpc-reality", model.Inbound{Type: "vless", Tag: "vg", TlsId: 1, Tls: reality, Options: json.RawMessage(`{"listen":"::","listen_port":2053,"transport":{"type":"grpc","service_name":"gs"}}`)}},
		{"vmess-ws", model.Inbound{Type: "vmess", Tag: "vm", Options: json.RawMessage(`{"listen":"::","listen_port":80,"transport":{"type":"ws","path":"/vm"}}`)}},
		{"trojan-tls", model.Inbound{Type: "trojan", Tag: "tr", TlsId: 2, Tls: plain, Options: json.RawMessage(`{"listen":"::","listen_port":2083}`)}},
		{"hy2", model.Inbound{Type: "hysteria2", Tag: "hy", TlsId: 2, Tls: plain, Options: json.RawMessage(`{"listen":"::","listen_port":4443,"up_mbps":100,"down_mbps":100}`)}},
		{"tuic", model.Inbound{Type: "tuic", Tag: "tu", TlsId: 2, Tls: plain, Options: json.RawMessage(`{"listen":"::","listen_port":5443,"congestion_control":"bbr"}`)}},
		{"ss2022", model.Inbound{Type: "shadowsocks", Tag: "ss", Options: json.RawMessage(`{"listen":"::","listen_port":8388,"method":"2022-blake3-aes-256-gcm","password":"c2VydmVyIGtleSAzMiBieXRlcyBsb25nISEhISEhIQ=="}`)}},
		{"ss-aead", model.Inbound{Type: "shadowsocks", Tag: "ssa", Options: json.RawMessage(`{"listen":"::","listen_port":8389,"method":"chacha20-ietf-poly1305"}`)}},
		{"anytls", model.Inbound{Type: "anytls", Tag: "at", TlsId: 2, Tls: plain, Options: json.RawMessage(`{"listen":"::","listen_port":6443}`)}},
	}
	for _, c := range cases {
		in := c.in
		in.Addrs = json.RawMessage(`[]`)
		in.OutJson = json.RawMessage(`{}`)
		if err := util.FillOutJson(&in, "node.example"); err != nil {
			t.Errorf("%s fill: %v", c.name, err)
			continue
		}
		full, _ := in.MarshalFull()
		(*full)["out_json"] = in.OutJson
		raw, _ := json.Marshal(full)
		var anyMap map[string]interface{}
		_ = json.Unmarshal(raw, &anyMap)
		raw, _ = json.Marshal(anyMap)
		nid := uint(1)
		rep, err := buildReplicaInbound(raw, nid)
		if err != nil {
			t.Errorf("%s build: %v", c.name, err)
			continue
		}
		client := &model.Client{Name: "u1", Config: cfg}
		nl := genNodeReplicaLinks(rep, client, "", client.Remark)
		ll := util.LinkGenerator(cfg, &in, "node.example", "", "u1")
		if len(nl) == 0 || strings.Join(nl, "\n") != strings.Join(ll, "\n") {
			t.Errorf("%s: node links %v, local links %v", c.name, nl, ll)
		}
		for _, l := range nl {
			out, tag, err := util.GetOutbound(l, 0)
			if err != nil || out == nil || tag == "" {
				t.Errorf("%s: node link does not parse: %v %q", c.name, err, tag)
				continue
			}
		}
	}
}

func TestNodeReplicaLinksUseNodeHostForLoopback(t *testing.T) {
	client := &model.Client{Name: "u1", Config: json.RawMessage(`{"vless":{"name":"u1","uuid":"11111111-1111-1111-1111-111111111111"}}`)}
	for _, server := range []string{"127.0.0.1", "localhost", "::1", "0.0.0.0", ""} {
		out, _ := json.Marshal(map[string]interface{}{"type": "vless", "tag": "v", "server": server, "server_port": 443})
		replica := &model.Inbound{Type: "vless", Tag: "v", Options: json.RawMessage(`{"listen_port":443}`), OutJson: out, Addrs: json.RawMessage(`[]`)}
		links := genNodeReplicaLinks(replica, client, nodeLinkHost("https://node.example:2095"), client.Remark)
		if len(links) != 1 || !strings.Contains(links[0], "@node.example:443") {
			t.Errorf("server %q: links %v", server, links)
		}
		if links := genNodeReplicaLinks(replica, client, nodeLinkHost("https://127.0.0.1:2095"), client.Remark); len(links) != 0 {
			t.Errorf("server %q with a loopback panel address: links %v", server, links)
		}
	}
	out, _ := json.Marshal(map[string]interface{}{"type": "vless", "tag": "v", "server": "real.example", "server_port": 443})
	replica := &model.Inbound{Type: "vless", Tag: "v", Options: json.RawMessage(`{"listen_port":443}`), OutJson: out, Addrs: json.RawMessage(`[]`)}
	if links := genNodeReplicaLinks(replica, client, "node.example", client.Remark); len(links) != 1 || !strings.Contains(links[0], "@real.example:443") {
		t.Errorf("own address replaced: %v", links)
	}
}
