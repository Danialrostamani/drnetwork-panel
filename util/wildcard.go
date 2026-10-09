package util

import (
	"strings"

	"github.com/Danialrostamani/drnetwork-panel/util/hosts"
)

// An inbound address written as "*.cdn.example.com" gives every client a host
// of its own under that name: "<label>.cdn.example.com", with a label only the
// panel can work out. When one host is blocked, only one client loses it.

// Private keys the link generator leaves on an address it expanded, for the
// transport host that has to follow the server.
const (
	addrWildBase = "_wildcardBase"
	addrWildHost = "_wildcardHost"
	addrWildName = "_wildcardName"
)

// ExpandWildcardHost is the client's own host for a wildcard address; any
// other host is returned as it is.
func ExpandWildcardHost(host, clientName string) string {
	h := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if !hosts.IsWildcard(h) || strings.Contains(hosts.Base(h), "*") {
		return host
	}
	base := hosts.Base(h)
	return WildcardLabel(base, clientName) + "." + base
}

// wildcardFollow is what a host field next to an expanded address becomes:
// the wildcard itself or its base turns into the client's host, another
// wildcard gets the client's label as well, and anything else -- an empty
// value included -- stays. keepEmpty false fills an empty value too.
func wildcardFollow(v, base, concrete, clientName string, keepEmpty bool) string {
	l := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(v)), ".")
	switch {
	case l == "":
		if keepEmpty {
			return v
		}
		return concrete
	case l == base || l == "*."+base:
		return concrete
	case hosts.IsWildcard(l):
		return ExpandWildcardHost(l, clientName)
	}
	return v
}

// ExpandWildcardOutbound does to a sing-box outbound what the link generator
// does to an address: a wildcard server becomes the client's host, and so do
// the TLS server name and the transport host when they name the same domain.
// It reports whether the server was a wildcard. Maps it changes are copied
// first, since the caller's outbounds share them between addresses.
func ExpandWildcardOutbound(out map[string]interface{}, clientName string) bool {
	server, _ := out["server"].(string)
	concrete := ExpandWildcardHost(server, clientName)
	if concrete == server {
		return false
	}
	base := hosts.Base(strings.TrimSuffix(strings.ToLower(strings.TrimSpace(server)), "."))
	out["server"] = concrete
	if tls, ok := out["tls"].(map[string]interface{}); ok {
		sni, _ := tls["server_name"].(string)
		if s := wildcardFollow(sni, base, concrete, clientName, false); s != sni {
			t := copyMap(tls)
			t["server_name"] = s
			out["tls"] = t
		}
	}
	if tr, ok := out["transport"].(map[string]interface{}); ok {
		t := copyMap(tr)
		changed := false
		switch typ, _ := tr["type"].(string); typ {
		case "ws":
			if headers, ok := tr["headers"].(map[string]interface{}); ok {
				h := copyMap(headers)
				for _, key := range []string{"Host", "host"} {
					if v, ok := headers[key].(string); ok {
						if nv := wildcardFollow(v, base, concrete, clientName, true); nv != v {
							h[key] = nv
							changed = true
						}
					}
				}
				t["headers"] = h
			}
		case "httpupgrade":
			if v, ok := tr["host"].(string); ok {
				if nv := wildcardFollow(v, base, concrete, clientName, true); nv != v {
					t["host"] = nv
					changed = true
				}
			}
		case "http":
			if list, ok := tr["host"].([]interface{}); ok {
				nl := make([]interface{}, len(list))
				for i, item := range list {
					nl[i] = item
					if v, ok := item.(string); ok {
						if nv := wildcardFollow(v, base, concrete, clientName, true); nv != v {
							nl[i] = nv
							changed = true
						}
					}
				}
				t["host"] = nl
			}
		}
		if changed {
			out["transport"] = t
		}
	}
	return true
}

// expandWildcardAddrs gives the client its host under every wildcard address
// of a link, and remembers it for the transport host.
func expandWildcardAddrs(addrs []map[string]interface{}, clientName string) {
	for _, addr := range addrs {
		server, _ := addr["server"].(string)
		concrete := ExpandWildcardHost(server, clientName)
		if concrete == server {
			continue
		}
		base := hosts.Base(strings.TrimSuffix(strings.ToLower(strings.TrimSpace(server)), "."))
		addr["server"] = concrete
		addr[addrWildBase], addr[addrWildHost], addr[addrWildName] = base, concrete, clientName
		if tls, ok := addr["tls"].(map[string]interface{}); ok {
			sni, _ := tls["server_name"].(string)
			if s := wildcardFollow(sni, base, concrete, clientName, false); s != sni {
				t := copyMap(tls)
				t["server_name"] = s
				addr["tls"] = t
			}
		}
	}
}

// addrHostParams makes the transport host of a link follow its address when
// the address was a wildcard.
func addrHostParams(params []LinkParam, addr map[string]interface{}) {
	for i := range params {
		if params[i].Key == "host" {
			params[i].Value = addrHost(params[i].Value, addr)
		}
	}
}

// addrHost is a transport host (a comma list for HTTP) as the address of the
// link needs it.
func addrHost(value string, addr map[string]interface{}) string {
	base, _ := addr[addrWildBase].(string)
	concrete, _ := addr[addrWildHost].(string)
	name, _ := addr[addrWildName].(string)
	if base == "" || concrete == "" || value == "" {
		return value
	}
	parts := strings.Split(value, ",")
	for j, p := range parts {
		parts[j] = wildcardFollow(p, base, concrete, name, true)
	}
	return strings.Join(parts, ",")
}

func copyMap(m map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
