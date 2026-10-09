package service

import (
	"encoding/json"
	"strings"

	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/util/common"
	"github.com/Danialrostamani/drnetwork-panel/util/hosts"
)

// cdnTransports are the transports a CDN carries: it speaks HTTP to the
// origin, so a raw TCP stream, QUIC or REALITY cannot pass it.
var cdnTransports = map[string]bool{"ws": true, "httpupgrade": true, "grpc": true}

func realityOn(tls interface{}) bool {
	m, _ := tls.(map[string]interface{})
	r, _ := m["reality"].(map[string]interface{})
	on, _ := r["enabled"].(bool)
	return on
}

// checkInboundAddrs refuses addresses that cannot work: a malformed wildcard
// ("*.cdn.example.com" is the only form) and an address marked as behind a
// CDN on an inbound a CDN cannot carry.
func checkInboundAddrs(inbound *model.Inbound) error {
	var addrs []map[string]interface{}
	if len(inbound.Addrs) == 0 || json.Unmarshal(inbound.Addrs, &addrs) != nil {
		return nil
	}
	var options map[string]interface{}
	_ = json.Unmarshal(inbound.Options, &options)
	transport := "tcp"
	if tr, ok := options["transport"].(map[string]interface{}); ok {
		if t, _ := tr["type"].(string); t != "" {
			transport = t
		}
	}
	inboundReality := false
	if inbound.Tls != nil && len(inbound.Tls.Server) > 0 {
		var server map[string]interface{}
		if json.Unmarshal(inbound.Tls.Server, &server) == nil {
			inboundReality = realityOn(server)
		}
	}
	for _, addr := range addrs {
		server, _ := addr["server"].(string)
		name := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(server)), ".")
		if strings.Contains(name, "*") {
			if err := hosts.Check(name); err != nil {
				return common.NewError("address <", server, ">: ", err)
			}
		}
		if cdn, _ := addr["cdn"].(bool); !cdn {
			continue
		}
		switch inbound.Type {
		case "vless", "vmess", "trojan":
		default:
			return common.NewError("address <", server, "> is behind a CDN, which carries only VLESS, VMess or Trojan over WebSocket, HTTPUpgrade or gRPC")
		}
		if !cdnTransports[transport] {
			return common.NewError("address <", server, "> is behind a CDN: the transport must be WebSocket, HTTPUpgrade or gRPC, not ", transport)
		}
		if inboundReality || realityOn(addr["tls"]) {
			return common.NewError("address <", server, "> is behind a CDN, and REALITY does not work through a CDN")
		}
	}
	return nil
}
