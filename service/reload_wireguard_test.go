package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"

	M "github.com/sagernet/sing/common/metadata"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// The case that was reported: a WireGuard endpoint that route.final sends
// everything through. It is replaced in the running core when edited, and
// route.final went on pointing at the closed old instance, so every connection
// failed with "WireGuard is not ready yet" until the core was restarted by
// hand. The server and client endpoints are both in this process, talking over
// loopback.
func TestEditingTheEndpointRouteFinalUsesKeepsTrafficFlowing(t *testing.T) {
	if raceEnabled {
		t.Skip("starting a Box trips sing-box's own race in route.NetworkManager; see race_on_test.go")
	}
	s := lifecycleService(t)

	echo, echoPort := wgEchoServer(t)
	_ = echo
	wgPort := freeUDPPort(t)
	serverKey, clientKey := mustKey(t), mustKey(t)

	if err := s.SettingService.SetConfig(fmt.Sprintf(`{"log":{"level":"error"},"route":{"final":"wg-client","rules":[{"inbound":["wg-server"],"action":"route","outbound":"direct","override_address":"127.0.0.1","override_port":%d}]}}`, echoPort)); err != nil {
		t.Fatal(err)
	}
	server := map[string]interface{}{
		"type": "wireguard", "tag": "wg-server", "address": []string{"10.10.0.1/24"},
		"private_key": serverKey.String(), "listen_port": wgPort,
		"peers": []interface{}{map[string]interface{}{"public_key": clientKey.PublicKey().String(), "allowed_ips": []string{"10.10.0.2/32"}}},
	}
	client := map[string]interface{}{
		"type": "wireguard", "tag": "wg-client", "address": []string{"10.10.0.2/24"},
		"private_key": clientKey.String(),
		"peers": []interface{}{map[string]interface{}{
			"address": "127.0.0.1", "port": wgPort, "public_key": serverKey.PublicKey().String(), "allowed_ips": []string{"0.0.0.0/0"},
		}},
	}
	for _, ep := range []map[string]interface{}{server, client} {
		raw, _ := json.Marshal(ep)
		var e model.Endpoint
		if err := e.UnmarshalJSON(raw); err != nil {
			t.Fatal(err)
		}
		if err := database.GetDB().Create(&e).Error; err != nil {
			t.Fatal(err)
		}
	}

	rawConfig, err := s.GetConfig("")
	if err != nil {
		t.Fatal(err)
	}
	if err := corePtr.Start(*rawConfig); err != nil {
		if strings.Contains(err.Error(), "gVisor") {
			t.Skip("built without gVisor, so WireGuard cannot run in-process: ", err)
		}
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = corePtr.Stop() })

	waitForTunnel(t, 30*time.Second)
	before := corePtr.GetInstance()

	var stored model.Endpoint
	if err := database.GetDB().Where("tag = ?", "wg-client").First(&stored).Error; err != nil {
		t.Fatal(err)
	}
	client["id"] = stored.Id
	client["mtu"] = 1300 // any edit
	data, _ := json.Marshal(client)
	if _, err := s.Save("endpoints", "edit", data, "", "tester", "localhost"); err != nil {
		t.Fatalf("edit: %v", err)
	}
	waitForRestart(t, before)

	// With the stale route.final this never succeeded, however long it waited.
	waitForTunnel(t, 30*time.Second)
}

func mustKey(t *testing.T) wgtypes.Key {
	t.Helper()
	k, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func freeUDPPort(t *testing.T) uint16 {
	t.Helper()
	c, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	return uint16(c.LocalAddr().(*net.UDPAddr).Port)
}

func wgEchoServer(t *testing.T) (net.Listener, uint16) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() { defer c.Close(); _, _ = io.Copy(c, c) }()
		}
	}()
	return l, uint16(l.Addr().(*net.TCPAddr).Port)
}

// dialThroughFinal opens a connection the way a client's would leave: through
// the core's default outbound, which is what route.final names, to an address
// inside the tunnel, and checks that the echo behind the far end answers.
func dialThroughFinal() error {
	box := corePtr.GetInstance()
	if box == nil {
		return fmt.Errorf("core not running")
	}
	out := box.Outbound().Default()
	if out == nil {
		return fmt.Errorf("no default outbound")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, err := out.DialContext(ctx, "tcp", M.ParseSocksaddrHostPort("10.10.0.1", 7))
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err = conn.Write([]byte("ping")); err != nil {
		return err
	}
	got := make([]byte, 4)
	if _, err = io.ReadFull(conn, got); err != nil {
		return err
	}
	if string(got) != "ping" {
		return fmt.Errorf("unexpected echo %q", got)
	}
	return nil
}

func waitForTunnel(t *testing.T, within time.Duration) {
	t.Helper()
	deadline := time.Now().Add(within)
	var err error
	for time.Now().Before(deadline) {
		if err = dialThroughFinal(); err == nil {
			return
		}
		time.Sleep(300 * time.Millisecond)
	}
	t.Fatalf("no traffic through route.final within %v: %v", within, err)
}
