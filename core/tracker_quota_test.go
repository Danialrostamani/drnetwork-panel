package core

import (
	"net"
	"testing"
	"time"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing/common/buf"
	"github.com/sagernet/sing/common/bufio"
	M "github.com/sagernet/sing/common/metadata"
)

// pump moves n bytes from the client end through the tracked conn, the way an
// upload does, and reports whether they all made it.
func pump(client, tracked net.Conn, n int) bool {
	errc := make(chan error, 1)
	go func() {
		_, err := client.Write(make([]byte, n))
		errc <- err
	}()
	got := 0
	buffer := make([]byte, n)
	for got < n {
		m, err := tracked.Read(buffer[got:])
		got += m
		if err != nil {
			_ = client.Close()
			<-errc
			return false
		}
	}
	return <-errc == nil
}

func waitExhausted(t *testing.T, ch <-chan string, user string) {
	t.Helper()
	select {
	case got := <-ch:
		if got != user {
			t.Fatalf("expected %s to be cut off, got %s", user, got)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("%s was not cut off", user)
	}
}

func closedConn(c net.Conn) bool {
	_ = c.SetReadDeadline(time.Now().Add(time.Second))
	_, err := c.Read(make([]byte, 1))
	return err != nil && !isTimeout(err)
}

func isTimeout(err error) bool {
	ne, ok := err.(net.Error)
	return ok && ne.Timeout()
}

func TestQuotaCutsAtTheLimit(t *testing.T) {
	tracker := NewSessionTracker()
	cut := make(chan string, 8)
	tracker.onExhausted = func(user string) { cut <- user }
	tracker.SetQuotas(map[string]int64{"alice": 10})

	alice, aliceConn := trackConn(t, tracker, "in", "alice")
	defer alice.Close()
	bob, bobConn := trackConn(t, tracker, "in", "bob")
	defer bob.Close()
	defer bobConn.Close()

	if !pump(alice, aliceConn, 6) {
		t.Fatal("alice was cut off before her volume ran out")
	}
	if tracker.Exhausted("alice") {
		t.Fatal("6 of 10 bytes is not the end of the volume")
	}
	// Both directions count: 4 bytes down bring her to the limit.
	go func() { _, _ = aliceConn.Write(make([]byte, 4)) }()
	if _, err := alice.Read(make([]byte, 4)); err != nil {
		t.Fatal(err)
	}
	waitExhausted(t, cut, "alice")
	if !tracker.Exhausted("alice") {
		t.Fatal("alice should be out of volume")
	}
	if !closedConn(aliceConn) {
		t.Fatal("alice's open connection was not cut")
	}
	// A new connection of hers is refused, and only hers.
	again, againConn := trackConn(t, tracker, "other", "alice")
	defer again.Close()
	if !closedConn(againConn) {
		t.Fatal("alice could open a new connection with no volume left")
	}
	if !pump(bob, bobConn, 100) {
		t.Fatal("bob, who has no cap, was cut off")
	}
	for _, s := range tracker.Sessions() {
		if s.User == "alice" {
			t.Fatalf("a session of alice is still listed: %+v", s)
		}
	}
}

func TestQuotaCountsFromTheLastDrain(t *testing.T) {
	tracker := NewSessionTracker()
	cut := make(chan string, 8)
	tracker.onExhausted = func(user string) { cut <- user }

	alice, aliceConn := trackConn(t, tracker, "in", "alice")
	defer alice.Close()
	if !pump(alice, aliceConn, 5) {
		t.Fatal("no cap yet")
	}
	// The database now has these 5 bytes; 3 more are left.
	tracker.GetStats()
	tracker.SetQuotas(map[string]int64{"alice": 3})
	if !pump(alice, aliceConn, 2) {
		t.Fatal("2 of the 3 bytes left were refused")
	}
	if tracker.Exhausted("alice") {
		t.Fatal("1 byte is still left")
	}
	// Traffic since the drain is not in the database yet and still counts.
	tracker.SetQuotas(map[string]int64{"alice": 3})
	if tracker.Exhausted("alice") {
		t.Fatal("the same quota set again must not use up the byte left")
	}
	pump(alice, aliceConn, 1)
	waitExhausted(t, cut, "alice")
	if !closedConn(aliceConn) {
		t.Fatal("alice's connection was not cut at the limit")
	}
}

func TestQuotaRenewalLetsTheUserBackIn(t *testing.T) {
	tracker := NewSessionTracker()
	cut := make(chan string, 8)
	tracker.onExhausted = func(user string) { cut <- user }

	alice, aliceConn := trackConn(t, tracker, "in", "alice")
	defer alice.Close()
	if !pump(alice, aliceConn, 8) {
		t.Fatal("no cap yet")
	}
	tracker.GetStats()
	// Already past the volume when it is set: cut off at once.
	tracker.SetQuotas(map[string]int64{"alice": -1})
	waitExhausted(t, cut, "alice")
	if !closedConn(aliceConn) {
		t.Fatal("alice's connection survived a quota she was already past")
	}

	// Renewed: back in.
	tracker.SetQuotas(map[string]int64{"alice": 1 << 20})
	if tracker.Exhausted("alice") {
		t.Fatal("a renewed user is still out of volume")
	}
	renewed, renewedConn := trackConn(t, tracker, "in", "alice")
	defer renewed.Close()
	defer renewedConn.Close()
	if !pump(renewed, renewedConn, 100) {
		t.Fatal("a renewed user could not use the new volume")
	}

	// Dropped from the map (volume made unlimited): no cap at all.
	tracker.SetQuotas(map[string]int64{"alice": 0})
	waitExhausted(t, cut, "alice")
	tracker.SetQuotas(nil)
	if tracker.Exhausted("alice") {
		t.Fatal("a user without a cap is out of volume")
	}
	free, freeConn := trackConn(t, tracker, "in", "alice")
	defer free.Close()
	defer freeConn.Close()
	if !pump(free, freeConn, 1000) {
		t.Fatal("a user without a cap was refused")
	}
}

func TestQuotaCutsPacketConnections(t *testing.T) {
	tracker := NewSessionTracker()
	cut := make(chan string, 8)
	tracker.onExhausted = func(user string) { cut <- user }
	tracker.SetQuotas(map[string]int64{"alice": 1000})

	server, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	peer, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	metadata := adapter.InboundContext{
		Inbound:     "in",
		User:        "alice",
		Source:      M.SocksaddrFromNet(peer.LocalAddr()),
		Destination: M.ParseSocksaddr("example.com:53"),
	}
	tracked := tracker.RoutedPacketConnection(t.Context(), bufio.NewPacketConn(server), metadata, nil, &testOutbound{tag: "direct"})
	defer tracked.Close()
	to := M.SocksaddrFromNet(peer.LocalAddr())
	if err := tracked.WritePacket(buf.As(make([]byte, 600)), to); err != nil {
		t.Fatal(err)
	}
	if err := tracked.WritePacket(buf.As(make([]byte, 600)), to); err != nil {
		t.Fatal(err)
	}
	waitExhausted(t, cut, "alice")
	if err := tracked.WritePacket(buf.As(make([]byte, 10)), to); err == nil {
		t.Fatal("a packet connection kept working past the volume")
	}
}

// Many connections of one user racing the stats job and quota updates: run
// under -race, and the user must end up cut off everywhere.
func TestQuotaConcurrentUse(t *testing.T) {
	tracker := NewSessionTracker()
	cut := make(chan string, 64)
	tracker.onExhausted = func(user string) { cut <- user }

	const conns = 4
	done := make(chan struct{})
	tracked := make([]net.Conn, conns)
	for i := range conns {
		client, conn := trackConn(t, tracker, "in", "alice")
		defer client.Close()
		tracked[i] = conn
		go func() {
			defer func() { done <- struct{}{} }()
			buffer := make([]byte, 512)
			for {
				if _, err := conn.Read(buffer); err != nil {
					return
				}
			}
		}()
		go func() {
			for {
				if _, err := client.Write(make([]byte, 512)); err != nil {
					return
				}
			}
		}()
	}
	for range 50 {
		tracker.GetStats()
		tracker.SetQuotas(map[string]int64{"alice": 1 << 30})
		time.Sleep(time.Millisecond)
	}
	tracker.GetStats()
	tracker.SetQuotas(map[string]int64{"alice": 4096})
	for range conns {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("a connection of alice survived the end of her volume")
		}
	}
	if !tracker.Exhausted("alice") {
		t.Fatal("alice should be out of volume")
	}
}
