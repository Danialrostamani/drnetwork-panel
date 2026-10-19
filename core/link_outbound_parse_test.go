package core

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Danialrostamani/drnetwork-panel/util"
	"github.com/sagernet/sing-box/option"
)

// TestSubscriptionLinkOutboundsParse feeds the outbounds the subscriptions make
// from links (node links, external ones) to sing-box's own parser. A vmess link
// with its port as a string gave "server_port":"80", and sing-box refused the
// whole profile over it.
func TestSubscriptionLinkOutboundsParse(t *testing.T) {
	links := []string{
		"anytls://pw@node.example:6443?security=tls&insecure=1&sni=sni.example&alpn=h2,http/1.1#at",
		"hysteria2://pw@node.example:4443?downmbps=100&upmbps=100&security=tls&insecure=1&sni=sni.example&alpn=h2,http/1.1&fastopen=0#hy",
		"ss://MjAyMi1ibGFrZTMtYWVzLTI1Ni1nY206YzJWeWRtVnlJR3RsZVNBek1pQmllWFJsY3lCc2IyNW5JU0VoSVNFaElRPT06YzNNeU1ESXlJR3RsZVNBek1pQmllWFJsY3lCc2IyNW5JUT09@node.example:8388#ss",
		"ss://Y2hhY2hhMjAtaWV0Zi1wb2x5MTMwNTpjM015TURJeUlHdGxlU0F6TWlCaWVYUmxjeUJzYjI1bklRPT0@node.example:8389#ssa",
		"trojan://pw@node.example:2083?type=tcp&security=tls&allowInsecure=1&sni=sni.example&alpn=h2,http/1.1#tr",
		"tuic://11111111-1111-1111-1111-111111111111:pw@node.example:5443?security=tls&insecure=1&sni=sni.example&alpn=h2,http/1.1&congestion_control=bbr#tu",
		"vless://11111111-1111-1111-1111-111111111111@node.example:2053?type=grpc&serviceName=gs&security=reality&pbk=PUBKEY&sid=abcd1234&fp=chrome&sni=www.google.com#vg",
		"vless://11111111-1111-1111-1111-111111111111@node.example:443?type=tcp&security=reality&pbk=PUBKEY&sid=abcd1234&fp=chrome&sni=www.google.com&flow=xtls-rprx-vision#vr",
		"vless://11111111-1111-1111-1111-111111111111@node.example:8443?type=ws&path=%2Fws&security=tls&allowInsecure=1&sni=sni.example&alpn=h2,http/1.1#vw",
		"vmess://eyJhZGQiOiJub2RlLmV4YW1wbGUiLCJhaWQiOjAsImlkIjoiMTExMTExMTEtMTExMS0xMTExLTExMTEtMTExMTExMTExMTExIiwibmV0Ijoid3MiLCJwYXRoIjoiL3ZtIiwicG9ydCI6IjgwIiwicHMiOiJ2bSIsInRscyI6Im5vbmUiLCJ2IjoiMiJ9",
		"hysteria2://pw@node.example:4443?security=tls&sni=a.b&obfs=salamander&obfs-password=x#h",
		"vless://11111111-1111-1111-1111-111111111111@node.example:443?type=xhttp&path=%2Fx&security=tls&sni=a.b#xh",
		"vless://11111111-1111-1111-1111-111111111111@node.example:443?type=httpupgrade&path=%2Fx&host=a.b&security=tls&sni=a.b#hu",
	}
	ctx := Context(context.Background(), InboundRegistry(), OutboundRegistry(),
		EndpointRegistry(), DNSTransportRegistry(), ServiceRegistry(), CertificateProviderRegistry())
	for _, l := range links {
		out, _, err := util.GetOutbound(l, 0)
		if err != nil {
			t.Errorf("%s: getoutbound: %v", l[:12], err)
			continue
		}
		cfg, _ := json.Marshal(map[string]interface{}{"outbounds": []interface{}{out}})
		var opts option.Options
		if err := opts.UnmarshalJSONContext(ctx, cfg); err != nil {
			t.Errorf("%s: %v\n%s", l[:10], err, cfg)
		}
	}
}
