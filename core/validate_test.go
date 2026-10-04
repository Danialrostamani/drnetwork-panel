package core

import (
	"encoding/json"
	"strings"
	"testing"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

const validateCoreConfig = `{"log":{"level":"error"},"outbounds":[{"type":"direct","tag":"direct"}]}`

func validateTestCore(t *testing.T) *Core {
	t.Helper()
	if raceEnabled {
		t.Skip("starting a Box trips sing-box's own race in route.NetworkManager; see race_on_test.go")
	}
	c := NewCore()
	if err := c.Start([]byte(validateCoreConfig)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Stop() })
	return c
}

// A system-stack WireGuard endpoint: it builds without gVisor, and nothing is
// bound until it is started.
func wgEndpointConfig(t *testing.T, extra map[string]any) []byte {
	t.Helper()
	priv, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	peer, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	cfg := map[string]any{
		"type": "wireguard", "tag": "wg", "system": true,
		"address":     []string{"10.20.0.2/32"},
		"private_key": priv.String(),
		"peers": []any{map[string]any{
			"address": "192.0.2.1", "port": 51820,
			"public_key": peer.PublicKey().String(), "allowed_ips": []string{"0.0.0.0/0"},
		}},
	}
	for k, v := range extra {
		cfg[k] = v
	}
	out, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestValidateEndpointAcceptsAGoodConfigWithoutRegisteringIt(t *testing.T) {
	c := validateTestCore(t)

	if err := c.ValidateEndpoint(wgEndpointConfig(t, nil), ""); err != nil {
		t.Fatalf("a valid endpoint was refused: %v", err)
	}
	if c.CheckOutbound("wg", "http://127.0.0.1:1/").Error != "outbound not found" {
		t.Error("validating registered the endpoint in the running core")
	}
}

func TestValidateEndpointRefusesWhatSingBoxWouldRefuse(t *testing.T) {
	c := validateTestCore(t)

	cases := map[string]map[string]any{
		"bad private key":   {"private_key": "not-a-key"},
		"missing detour":    {"detour": "nowhere"},
		"detour on itself":  {"detour": "wg"},
		"detour and a port": {"detour": "direct", "listen_port": 51820},
	}
	for name, extra := range cases {
		err := c.ValidateEndpoint(wgEndpointConfig(t, extra), "")
		if err == nil {
			t.Errorf("%s: accepted", name)
		}
	}

	if err := c.ValidateEndpoint(wgEndpointConfig(t, map[string]any{"detour": "direct"}), ""); err != nil {
		t.Errorf("a detour through an existing outbound was refused: %v", err)
	}
}

// The object being replaced is about to disappear, so nothing may point at it.
func TestValidateRefusesADependencyOnTheObjectBeingReplaced(t *testing.T) {
	c := validateTestCore(t)

	const viaDirect = `{"type":"socks","tag":"viaold","server":"127.0.0.1","server_port":1080,"detour":"direct"}`
	err := c.ValidateOutbound([]byte(viaDirect), "direct")
	if err == nil || !strings.Contains(err.Error(), "direct") {
		t.Fatalf("an outbound depending on the one being replaced was accepted: %v", err)
	}
	if err := c.ValidateOutbound([]byte(viaDirect), "other"); err != nil {
		t.Fatalf("refused although the replaced object is another one: %v", err)
	}
}

func TestValidateOutboundRefusesAnUnparsableOne(t *testing.T) {
	c := validateTestCore(t)

	if err := c.ValidateOutbound([]byte(`{"type":"socks","tag":"s","server":"127.0.0.1","server_port":"x"}`), ""); err == nil {
		t.Error("an outbound with a non-numeric port was accepted")
	}
	if err := c.ValidateOutbound([]byte(`{"type":"socks","tag":"s","server":"127.0.0.1","server_port":1080}`), ""); err != nil {
		t.Errorf("a valid outbound was refused: %v", err)
	}
	if err := c.ValidateOutbound([]byte(`{"type":"selector","tag":"sel","outbounds":["direct","ghost"]}`), ""); err == nil {
		t.Error("a selector listing an outbound that does not exist was accepted")
	}
}

func TestValidateNeedsARunningCore(t *testing.T) {
	c := NewCore()
	if err := c.ValidateEndpoint(wgEndpointConfig(t, nil), ""); err == nil {
		t.Error("validated against a core that is not running")
	}
}
