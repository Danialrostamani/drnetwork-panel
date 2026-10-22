package util

import "testing"

// A Hysteria2 link keeps its obfuscation, salamander or gecko, and drops an
// unknown one.
func TestHy2LinkObfs(t *testing.T) {
	for obfs, want := range map[string]string{"salamander": "salamander", "gecko": "gecko", "other": ""} {
		out, _, err := GetOutbound("hysteria2://pw@example.com:443?obfs="+obfs+"&obfs-password=x&sni=example.com#n", 0)
		if err != nil {
			t.Fatal(err)
		}
		o, ok := (*out)["obfs"].(map[string]interface{})
		if want == "" {
			if ok {
				t.Errorf("%s: obfs = %v, want none", obfs, o)
			}
			continue
		}
		if !ok || o["type"] != want || o["password"] != "x" {
			t.Errorf("%s: obfs = %v", obfs, (*out)["obfs"])
		}
	}
}
