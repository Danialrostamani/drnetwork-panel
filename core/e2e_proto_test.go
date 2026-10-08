package core

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

// The protocols the panel serves, end to end through a real core: a client
// outbound of the same core connects to the panel's own inbound on loopback
// and fetches a page. Then the user is taken off the inbound, the way the
// panel does it for a client past its volume or expiry, and the open
// connection must drop and a new one must be refused. Run after every
// sing-box upgrade: the inbounds are the panel's own copies of sing-box's.

const (
	e2eUUID  = "b831381d-6324-4d53-ad4f-8cda48b30811"
	e2eUUID2 = "6f2a6a2e-0b3f-4bb4-9b6e-6a8f3b0f7a11"
)

func e2eCert(t *testing.T) (certPath, keyPath string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tpl, &tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	kder, _ := x509.MarshalECPrivateKey(key)
	dir := t.TempDir()
	certPath, keyPath = filepath.Join(dir, "c.pem"), filepath.Join(dir, "k.pem")
	_ = os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600)
	_ = os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kder}), 0o600)
	return certPath, keyPath
}

func e2ePort(t *testing.T, udp bool) int {
	t.Helper()
	if udp {
		c, err := net.ListenPacket("udp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		return c.LocalAddr().(*net.UDPAddr).Port
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

type e2eCase struct {
	name string
	udp  bool
	// in builds the inbound for the users, out the client outbound for u1.
	in  func(port int, users []string) map[string]any
	out func(port int) map[string]any
}

func e2eCases(certPath, keyPath string) []e2eCase {
	serverTLS := func(alpn ...string) map[string]any {
		m := map[string]any{"enabled": true, "certificate_path": certPath, "key_path": keyPath}
		if len(alpn) > 0 {
			m["alpn"] = alpn
		}
		return m
	}
	clientTLS := func(alpn ...string) map[string]any {
		m := map[string]any{"enabled": true, "insecure": true, "server_name": "localhost"}
		if len(alpn) > 0 {
			m["alpn"] = alpn
		}
		return m
	}
	uuidOf := map[string]string{"u1": e2eUUID, "u2": e2eUUID2}
	ssKey := map[string]string{"u1": base64.StdEncoding.EncodeToString([]byte("0123456789abcdef")), "u2": base64.StdEncoding.EncodeToString([]byte("fedcba9876543210"))}
	ssServer := base64.StdEncoding.EncodeToString([]byte("serverkey-16byte"))
	users := func(names []string, f func(string) map[string]any) []map[string]any {
		out := make([]map[string]any, 0, len(names))
		for _, n := range names {
			u := f(n)
			u["name"] = n
			out = append(out, u)
		}
		return out
	}
	base := func(typ string, port int) map[string]any {
		return map[string]any{"type": typ, "listen": "127.0.0.1", "listen_port": port}
	}
	dial := func(typ string, port int) map[string]any {
		return map[string]any{"type": typ, "server": "127.0.0.1", "server_port": port}
	}
	with := func(m map[string]any, kv ...any) map[string]any {
		for i := 0; i+1 < len(kv); i += 2 {
			m[kv[i].(string)] = kv[i+1]
		}
		return m
	}
	vlessUsers := func(u []string) []map[string]any {
		return users(u, func(n string) map[string]any { return map[string]any{"uuid": uuidOf[n]} })
	}
	pwUsers := func(u []string) []map[string]any {
		return users(u, func(n string) map[string]any { return map[string]any{"password": "pw-" + n} })
	}
	ws := map[string]any{"type": "ws", "path": "/w"}
	grpc := map[string]any{"type": "grpc", "service_name": "g"}
	return []e2eCase{
		{name: "vless", in: func(p int, u []string) map[string]any {
			return with(base("vless", p), "users", vlessUsers(u))
		}, out: func(p int) map[string]any { return with(dial("vless", p), "uuid", e2eUUID) }},
		{name: "vless-tls", in: func(p int, u []string) map[string]any {
			return with(base("vless", p), "tls", serverTLS(), "users", vlessUsers(u))
		}, out: func(p int) map[string]any { return with(dial("vless", p), "uuid", e2eUUID, "tls", clientTLS()) }},
		{name: "vless-ws", in: func(p int, u []string) map[string]any {
			return with(base("vless", p), "transport", ws, "users", vlessUsers(u))
		}, out: func(p int) map[string]any { return with(dial("vless", p), "uuid", e2eUUID, "transport", ws) }},
		{name: "vless-grpc", in: func(p int, u []string) map[string]any {
			return with(base("vless", p), "transport", grpc, "users", vlessUsers(u))
		}, out: func(p int) map[string]any { return with(dial("vless", p), "uuid", e2eUUID, "transport", grpc) }},
		{name: "vless-httpupgrade", in: func(p int, u []string) map[string]any {
			return with(base("vless", p), "transport", map[string]any{"type": "httpupgrade", "path": "/u"}, "users", vlessUsers(u))
		}, out: func(p int) map[string]any {
			return with(dial("vless", p), "uuid", e2eUUID, "transport", map[string]any{"type": "httpupgrade", "path": "/u"})
		}},
		{name: "vless-h2", in: func(p int, u []string) map[string]any {
			return with(base("vless", p), "tls", serverTLS("h2"), "transport", map[string]any{"type": "http", "path": "/h"}, "users", vlessUsers(u))
		}, out: func(p int) map[string]any {
			return with(dial("vless", p), "uuid", e2eUUID, "tls", clientTLS("h2"), "transport", map[string]any{"type": "http", "path": "/h"})
		}},
		{name: "vmess", in: func(p int, u []string) map[string]any {
			return with(base("vmess", p), "users", vlessUsers(u))
		}, out: func(p int) map[string]any { return with(dial("vmess", p), "uuid", e2eUUID, "security", "auto") }},
		{name: "vmess-ws", in: func(p int, u []string) map[string]any {
			return with(base("vmess", p), "transport", ws, "users", vlessUsers(u))
		}, out: func(p int) map[string]any {
			return with(dial("vmess", p), "uuid", e2eUUID, "security", "auto", "transport", ws)
		}},
		{name: "vmess-grpc", in: func(p int, u []string) map[string]any {
			return with(base("vmess", p), "transport", grpc, "users", vlessUsers(u))
		}, out: func(p int) map[string]any {
			return with(dial("vmess", p), "uuid", e2eUUID, "security", "auto", "transport", grpc)
		}},
		{name: "trojan", in: func(p int, u []string) map[string]any {
			return with(base("trojan", p), "tls", serverTLS(), "users", pwUsers(u))
		}, out: func(p int) map[string]any { return with(dial("trojan", p), "password", "pw-u1", "tls", clientTLS()) }},
		{name: "trojan-grpc", in: func(p int, u []string) map[string]any {
			return with(base("trojan", p), "tls", serverTLS("h2"), "transport", grpc, "users", pwUsers(u))
		}, out: func(p int) map[string]any {
			return with(dial("trojan", p), "password", "pw-u1", "tls", clientTLS("h2"), "transport", grpc)
		}},
		{name: "shadowsocks-2022", in: func(p int, u []string) map[string]any {
			return with(base("shadowsocks", p), "method", "2022-blake3-aes-128-gcm", "password", ssServer, "users", users(u, func(n string) map[string]any { return map[string]any{"password": ssKey[n]} }))
		}, out: func(p int) map[string]any {
			return with(dial("shadowsocks", p), "method", "2022-blake3-aes-128-gcm", "password", ssServer+":"+ssKey["u1"])
		}},
		{name: "shadowsocks-aead", in: func(p int, u []string) map[string]any {
			return with(base("shadowsocks", p), "method", "aes-128-gcm", "users", pwUsers(u))
		}, out: func(p int) map[string]any {
			return with(dial("shadowsocks", p), "method", "aes-128-gcm", "password", "pw-u1")
		}},
		{name: "anytls", in: func(p int, u []string) map[string]any {
			return with(base("anytls", p), "tls", serverTLS(), "users", pwUsers(u))
		}, out: func(p int) map[string]any { return with(dial("anytls", p), "password", "pw-u1", "tls", clientTLS()) }},
		{name: "snell", in: func(p int, u []string) map[string]any {
			return with(base("snell", p), "version", 6, "psk", "psk-secret-long-enough", "users", users(u, func(n string) map[string]any { return map[string]any{"userkey": "key-" + n} }))
		}, out: func(p int) map[string]any {
			return with(dial("snell", p), "version", 6, "psk", "psk-secret-long-enough", "userkey", "key-u1")
		}},
		{name: "hysteria2", udp: true, in: func(p int, u []string) map[string]any {
			return with(base("hysteria2", p), "tls", serverTLS("h3"), "users", pwUsers(u))
		}, out: func(p int) map[string]any {
			return with(dial("hysteria2", p), "password", "pw-u1", "tls", clientTLS("h3"))
		}},
		{name: "tuic", udp: true, in: func(p int, u []string) map[string]any {
			return with(base("tuic", p), "tls", serverTLS("h3"), "users", users(u, func(n string) map[string]any { return map[string]any{"uuid": uuidOf[n], "password": "pw-" + n} }))
		}, out: func(p int) map[string]any {
			return with(dial("tuic", p), "uuid", e2eUUID, "password", "pw-u1", "tls", clientTLS("h3"))
		}},
		{name: "hysteria", udp: true, in: func(p int, u []string) map[string]any {
			return with(base("hysteria", p), "up_mbps", 100, "down_mbps", 100, "tls", serverTLS("h3"), "users", users(u, func(n string) map[string]any { return map[string]any{"auth_str": "pw-" + n} }))
		}, out: func(p int) map[string]any {
			return with(dial("hysteria", p), "up_mbps", 100, "down_mbps", 100, "auth_str", "pw-u1", "tls", clientTLS("h3"))
		}},
	}
}

func e2eGet(conn net.Conn, host string) (string, error) {
	_ = conn.SetDeadline(time.Now().Add(8 * time.Second))
	if _, err := fmt.Fprintf(conn, "GET / HTTP/1.1\r\nHost: %s\r\n\r\n", host); err != nil {
		return "", err
	}
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	return string(body), err
}

func TestProtocolsEndToEnd(t *testing.T) {
	if raceEnabled {
		t.Skip("starting a Box trips sing-box's own race in route.NetworkManager; see race_on_test.go")
	}
	if testing.Short() {
		t.Skip("end-to-end protocol test")
	}
	web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "drnetwork-ok")
	}))
	defer web.Close()
	target := strings.TrimPrefix(web.URL, "http://")

	certPath, keyPath := e2eCert(t)
	cases := e2eCases(certPath, keyPath)
	ports := map[string]int{}
	var inbounds, outbounds []map[string]any
	outbounds = append(outbounds, map[string]any{"type": "direct", "tag": "direct"})
	for _, c := range cases {
		p := e2ePort(t, c.udp)
		ports[c.name] = p
		in := c.in(p, []string{"u1", "u2"})
		in["tag"] = "in-" + c.name
		inbounds = append(inbounds, in)
		out := c.out(p)
		out["tag"] = "out-" + c.name
		outbounds = append(outbounds, out)
	}
	cfg, _ := json.Marshal(map[string]any{
		"log":       map[string]any{"level": "error"},
		"inbounds":  inbounds,
		"outbounds": outbounds,
		"route":     map[string]any{"final": "direct"},
	})
	core := NewCore()
	if err := core.Start(cfg); err != nil {
		t.Fatalf("core start: %v", err)
	}
	t.Cleanup(func() { _ = core.Stop() })
	box := core.GetInstance()

	dialVia := func(name string) (net.Conn, error) {
		ob, ok := box.Outbound().Outbound("out-" + name)
		if !ok {
			return nil, fmt.Errorf("no outbound")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		return ob.DialContext(ctx, N.NetworkTCP, M.ParseSocksaddr(target))
	}
	fetch := func(name string) error {
		conn, err := dialVia(name)
		if err != nil {
			return err
		}
		defer conn.Close()
		body, err := e2eGet(conn, target)
		if err != nil {
			return err
		}
		if body != "drnetwork-ok" {
			return fmt.Errorf("body %q", body)
		}
		return nil
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tag := "in-" + c.name
			if err := fetch(c.name); err != nil {
				t.Fatalf("fetch through %s: %v", c.name, err)
			}
			// An open connection, used once, for the cut below.
			open, err := dialVia(c.name)
			if err != nil {
				t.Fatalf("dial: %v", err)
			}
			defer open.Close()
			if _, err := e2eGet(open, target); err != nil {
				t.Fatalf("first request on the open connection: %v", err)
			}
			seen := false
			for _, s := range box.SessionTracker().Sessions() {
				if s.Inbound == tag && s.User == "u1" {
					seen = true
				}
			}
			if !seen {
				t.Errorf("no live session of u1 on %s", tag)
			}

			// u1 runs out: the panel puts the inbound back without them.
			in := c.in(ports[c.name], []string{"u2"})
			in["tag"] = tag
			raw, _ := json.Marshal(in)
			keep := map[string]struct{}{"u2": {}}
			handled, err := core.UpdateInboundUsers(raw)
			if err != nil {
				t.Fatalf("update users: %v", err)
			}
			if handled {
				box.SessionTracker().CloseByInboundUsers(tag, keep)
				core.CloseInboundUserSessions(tag, keep)
			} else {
				if err := core.RemoveInbound(tag); err != nil {
					t.Fatalf("remove inbound: %v", err)
				}
				if err := core.AddInbound(raw); err != nil {
					t.Fatalf("add inbound: %v", err)
				}
			}

			if _, err := e2eGet(open, target); err == nil {
				t.Errorf("the open connection of the removed user still works")
			}
			if err := fetch(c.name); err == nil {
				t.Errorf("a removed user could connect again")
			}
		})
	}
}

// TestQuotaEndToEnd: a user who runs out of volume in the middle of a download
// is cut off right there by the core itself, on every protocol, and cannot
// connect again until the volume is renewed.
func TestQuotaEndToEnd(t *testing.T) {
	if raceEnabled {
		t.Skip("starting a Box trips sing-box's own race in route.NetworkManager; see race_on_test.go")
	}
	if testing.Short() {
		t.Skip("end-to-end protocol test")
	}
	const bigSize = 64 << 20
	const quota = 256 << 10
	web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/big" {
			_, _ = io.WriteString(w, "drnetwork-ok")
			return
		}
		w.Header().Set("Content-Length", fmt.Sprint(bigSize))
		chunk := make([]byte, 64<<10)
		for sent := 0; sent < bigSize; sent += len(chunk) {
			if _, err := w.Write(chunk); err != nil {
				return
			}
		}
	}))
	defer web.Close()
	target := strings.TrimPrefix(web.URL, "http://")

	certPath, keyPath := e2eCert(t)
	cases := e2eCases(certPath, keyPath)
	var inbounds, outbounds []map[string]any
	outbounds = append(outbounds, map[string]any{"type": "direct", "tag": "direct"})
	for _, c := range cases {
		p := e2ePort(t, c.udp)
		in := c.in(p, []string{"u1", "u2"})
		in["tag"] = "in-" + c.name
		inbounds = append(inbounds, in)
		out := c.out(p)
		out["tag"] = "out-" + c.name
		outbounds = append(outbounds, out)
	}
	cfg, _ := json.Marshal(map[string]any{
		"log":       map[string]any{"level": "error"},
		"inbounds":  inbounds,
		"outbounds": outbounds,
		"route":     map[string]any{"final": "direct"},
	})
	core := NewCore()
	if err := core.Start(cfg); err != nil {
		t.Fatalf("core start: %v", err)
	}
	t.Cleanup(func() { _ = core.Stop() })
	box := core.GetInstance()
	tracker := box.SessionTracker()

	dialVia := func(name string) (net.Conn, error) {
		ob, ok := box.Outbound().Outbound("out-" + name)
		if !ok {
			return nil, fmt.Errorf("no outbound")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		return ob.DialContext(ctx, N.NetworkTCP, M.ParseSocksaddr(target))
	}
	fetch := func(name string) error {
		conn, err := dialVia(name)
		if err != nil {
			return err
		}
		defer conn.Close()
		body, err := e2eGet(conn, target)
		if err != nil {
			return err
		}
		if body != "drnetwork-ok" {
			return fmt.Errorf("body %q", body)
		}
		return nil
	}
	download := func(name string) (int64, error) {
		conn, err := dialVia(name)
		if err != nil {
			return 0, err
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(20 * time.Second))
		if _, err := fmt.Fprintf(conn, "GET /big HTTP/1.1\r\nHost: %s\r\n\r\n", target); err != nil {
			return 0, err
		}
		return io.Copy(io.Discard, conn)
	}
	u1Quota := func() *userQuota {
		tracker.access.Lock()
		defer tracker.access.Unlock()
		return tracker.quotas["u1"]
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// A fresh volume for this protocol: what the database has, plus
			// quota bytes.
			tracker.GetStats()
			tracker.SetQuotas(map[string]int64{"u1": quota})
			if err := fetch(c.name); err != nil {
				t.Fatalf("fetch with volume left: %v", err)
			}

			got, _ := download(c.name)
			if got >= bigSize {
				t.Fatalf("the whole %d bytes came through a %d byte volume", got, quota)
			}
			q := u1Quota()
			if q == nil || !q.exhausted() {
				t.Fatal("u1 is not out of volume after the download")
			}
			over := q.used.Load() - q.limit.Load()
			t.Logf("received %d bytes; %d bytes past the volume", got, over)
			if over > 16<<20 {
				t.Errorf("%d bytes past the volume", over)
			}
			if err := fetch(c.name); err == nil {
				t.Fatal("a user out of volume could connect again")
			}

			// QUIC sessions are muted for a while after a kick; the renewal is
			// checked on the others.
			if c.udp {
				return
			}
			tracker.GetStats()
			tracker.SetQuotas(map[string]int64{"u1": 1 << 30})
			if err := fetch(c.name); err != nil {
				t.Fatalf("fetch after the volume was renewed: %v", err)
			}
		})
	}
}
