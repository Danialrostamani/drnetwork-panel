package core

import (
	"testing"

	"github.com/sagernet/sing-box/adapter"
	M "github.com/sagernet/sing/common/metadata"
)

func TestSiteOf(t *testing.T) {
	for in, want := range map[[2]string]string{
		{"www.google.com", ""}:             "google.com",
		{"rr3---sn-x.googlevideo.com", ""}: "googlevideo.com",
		{"a.b.example.co.uk.", ""}:         "example.co.uk",
		{"WWW.Instagram.COM", ""}:          "instagram.com",
		{"", "1.2.3.4:443"}:                "1.2.3.4",
		{"", "[2001:db8::1]:443"}:          "2001:db8::1",
		{"8.8.8.8", "8.8.8.8:53"}:          "8.8.8.8",
		{"localhost", ""}:                  "localhost",
		{"", ""}:                           "",
	} {
		if got := siteOf(in[0], in[1]); got != want {
			t.Errorf("siteOf(%q, %q) = %q, want %q", in[0], in[1], got, want)
		}
	}
}

// Site counters are drained like the others, and one no live session holds
// is forgotten after its traffic is taken.
func TestSiteCountersDrainAndForget(t *testing.T) {
	tr := NewSessionTracker()
	md := adapter.InboundContext{Inbound: "in", User: "ali", Destination: M.ParseSocksaddr("1.2.3.4:443"), Domain: "www.google.com"}
	s, _, read, write := tr.newSession(md, nil, nil, "tcp")
	for _, f := range read {
		f(10)
	}
	for _, f := range write {
		f(100)
	}
	got := tr.GetSiteStats()
	if len(got) != 1 || got[0].User != "ali" || got[0].Site != "google.com" || got[0].Up != 10 || got[0].Down != 100 {
		t.Fatalf("drain = %+v", got)
	}
	if len(tr.sites) != 1 {
		t.Fatal("a counter a live session holds was forgotten")
	}
	tr.leave(s)
	for _, f := range write {
		f(5)
	}
	if got := tr.GetSiteStats(); len(got) != 1 || got[0].Down != 5 {
		t.Fatalf("last bytes = %+v", got)
	}
	if len(tr.sites) != 0 {
		t.Fatalf("closed session's counter kept: %d", len(tr.sites))
	}
}
