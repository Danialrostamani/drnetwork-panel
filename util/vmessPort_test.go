package util

import (
	"encoding/base64"
	"testing"
)

// A vmess link gives its port and alterId as numbers or as strings; the
// outbound must carry numbers either way, or sing-box refuses the config.
func TestVmessLinkNumbersBecomeInts(t *testing.T) {
	for _, body := range []string{
		`{"v":"2","ps":"a","add":"h.example","port":"8080","id":"11111111-1111-1111-1111-111111111111","aid":"0","net":"ws","path":"/p","tls":""}`,
		`{"v":"2","ps":"a","add":"h.example","port":8080,"id":"11111111-1111-1111-1111-111111111111","aid":0,"net":"tcp","tls":""}`,
	} {
		out, tag, err := GetOutbound("vmess://"+base64.StdEncoding.EncodeToString([]byte(body)), 0)
		if err != nil || tag != "a" {
			t.Fatalf("%s: %v %q", body, err, tag)
		}
		if port, ok := (*out)["server_port"].(int); !ok || port != 8080 {
			t.Errorf("%s: server_port %#v", body, (*out)["server_port"])
		}
		if aid, ok := (*out)["alter_id"].(int); !ok || aid != 0 {
			t.Errorf("%s: alter_id %#v", body, (*out)["alter_id"])
		}
	}
	bad := `{"v":"2","ps":"a","add":"h.example","port":"x","id":"11111111-1111-1111-1111-111111111111","net":"tcp"}`
	if _, _, err := GetOutbound("vmess://"+base64.StdEncoding.EncodeToString([]byte(bad)), 0); err == nil {
		t.Error("a vmess link without a usable port was accepted")
	}
}
