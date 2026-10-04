package service

import (
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
)

// What sing-box keeps hold of after an object is replaced, and what it looks up
// afresh. The first group is why a hot swap leaves "WireGuard is not ready yet"
// behind until the core is restarted; the second is safe to swap in place.
func TestTagReferenced(t *testing.T) {
	cases := []struct {
		name   string
		config string
		tag    string
		want   bool
	}{
		{"route.final", `{"route":{"final":"wg"},"endpoints":[{"type":"wireguard","tag":"wg"}]}`, "wg", true},
		{"selector member", `{"outbounds":[{"type":"selector","tag":"sel","outbounds":["a","wg"]}]}`, "wg", true},
		{"selector default", `{"outbounds":[{"type":"selector","tag":"sel","outbounds":["a","b"],"default":"wg"}]}`, "wg", true},
		{"urltest member", `{"outbounds":[{"type":"urltest","tag":"best","outbounds":["wg","b"]}]}`, "wg", true},
		{"outbound detour", `{"outbounds":[{"type":"socks","tag":"s","detour":"wg"}]}`, "wg", true},
		{"endpoint detour", `{"endpoints":[{"type":"wireguard","tag":"w2","detour":"wg"}]}`, "wg", true},
		{"dns server detour", `{"dns":{"servers":[{"tag":"d","type":"udp","server":"1.1.1.1","detour":"wg"}]}}`, "wg", true},
		{"http client detour", `{"http_clients":[{"tag":"default","detour":"wg"}]}`, "wg", true},
		{"experimental", `{"experimental":{"clash_api":{"external_ui_download_detour":"wg"}}}`, "wg", true},
		{"route.final of a direct outbound", `{"route":{"final":"direct"},"outbounds":[{"type":"direct","tag":"direct"}]}`, "direct", true},

		{"route rule outbound", `{"route":{"rules":[{"domain":["x.com"],"action":"route","outbound":"wg"}],"final":"direct"}}`, "wg", false},
		{"nested logical rule", `{"route":{"rules":[{"type":"logical","mode":"and","rules":[{"domain":["x"]},{"outbound":"wg"}],"action":"route","outbound":"wg"}]}}`, "wg", false},
		{"dns rule", `{"dns":{"rules":[{"outbound":["wg"],"server":"local"}]}}`, "wg", false},
		{"its own definition", `{"endpoints":[{"type":"wireguard","tag":"wg","address":["10.0.0.2/32"]}]}`, "wg", false},
		{"its own type", `{"outbounds":[{"type":"direct","tag":"x"}]}`, "direct", false},
		{"panel metadata", `{"endpoints":[{"type":"wireguard","tag":"w","ext":{"note":"wg"}}]}`, "wg", false},
		{"nothing at all", `{"route":{"final":"direct"}}`, "wg", false},
		{"another tag with the same prefix", `{"route":{"final":"wg2"}}`, "wg", false},
		{"inbound rule", `{"route":{"rules":[{"inbound":["wg"],"action":"route","outbound":"direct"}]}}`, "wg", false},

		// A config that cannot be read is treated as referenced: a restart is
		// always safe.
		{"not json", `{`, "wg", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tagReferenced([]byte(tc.config), tc.tag); got != tc.want {
				t.Fatalf("tagReferenced(%s, %q) = %v, want %v", tc.config, tc.tag, got, tc.want)
			}
		})
	}
}

// The delete is refused with a pointer to what still uses the object.
func TestReferenceSaysWhereTheTagIsUsed(t *testing.T) {
	cases := []struct {
		config, want string
	}{
		{`{"route":{"final":"wg"}}`, "route.final"},
		{`{"outbounds":[{"type":"direct","tag":"d"},{"type":"selector","tag":"sel","outbounds":["a","wg"]}]}`, "outbounds[sel].outbounds"},
		{`{"dns":{"servers":[{"tag":"remote","type":"udp","detour":"wg"}]}}`, "dns.servers[remote].detour"},
		{`{"endpoints":[{"type":"wireguard","detour":"wg"}]}`, "endpoints[0].detour"},
	}
	for _, tc := range cases {
		got, found := reference([]byte(tc.config), "wg")
		if !found || got != tc.want {
			t.Errorf("reference(%s) = %q, %v; want %q", tc.config, got, found, tc.want)
		}
	}
}

// liveReferences reads what the panel would hand sing-box, so it sees the
// stored base config and every object table together.
func TestLiveReferencesReadsTheStoredConfig(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "test.db")); err != nil {
		t.Fatal(err)
	}
	s := &ConfigService{}
	if err := s.SettingService.SetConfig(`{"log":{"level":"error"},"route":{"final":"sel","rules":[{"domain":["x.com"],"action":"route","outbound":"wg"}]}}`); err != nil {
		t.Fatal(err)
	}
	// The database starts with a "direct" outbound of its own.
	for _, raw := range []string{
		`{"type":"selector","tag":"sel","outbounds":["direct","tor"],"default":"direct"}`,
		`{"type":"socks","tag":"tor","server":"127.0.0.1","server_port":9050}`,
	} {
		var o model.Outbound
		if err := o.UnmarshalJSON([]byte(raw)); err != nil {
			t.Fatal(err)
		}
		if err := database.GetDB().Create(&o).Error; err != nil {
			t.Fatal(err)
		}
	}

	if !liveReferences("sel") {
		t.Error("route.final is not seen as a reference")
	}
	if !liveReferences("tor") {
		t.Error("a selector member is not seen as a reference")
	}
	if liveReferences("wg") {
		t.Error("a tag used only by a route rule is seen as referenced")
	}

	// Asked while a save holds its transaction open, which is when it is used.
	tx := database.GetDB().Begin()
	defer tx.Rollback()
	if err := tx.Where("tag = ?", "tor").Delete(model.Outbound{}).Error; err != nil {
		t.Fatal(err)
	}
	done := make(chan bool, 1)
	go func() { done <- liveReferences("tor") }()
	select {
	case got := <-done:
		if !got {
			t.Error("the pending delete hid the reference the running core still has")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("liveReferences blocked on the open transaction")
	}
}

// Requests are neither dropped nor repeated for nothing: the old TryLock turned
// a save made while a restart was running into a no-op, so its rules only took
// effect after the next manual restart.
func TestRestartGateRunsOnceMoreForRequestsMadeDuringARun(t *testing.T) {
	var g restartGate
	var runs atomic.Int32
	entered := make(chan struct{})
	release := make(chan struct{})
	do := func() error {
		if runs.Add(1) == 1 {
			close(entered)
			<-release
		}
		return nil
	}

	var wg sync.WaitGroup
	call := func() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = g.run(do)
		}()
	}

	call()
	<-entered // the first restart is underway

	// Three more saves land while it runs.
	for i := 0; i < 3; i++ {
		call()
	}
	deadline := time.Now().Add(5 * time.Second)
	for g.requested.Load() < 4 {
		if time.Now().After(deadline) {
			t.Fatal("the requests never registered")
		}
		time.Sleep(time.Millisecond)
	}
	close(release)
	wg.Wait()

	// One for the first request, and a single one shared by the three that
	// queued behind it: the first run may have read the database before they
	// committed, so it cannot stand in for them.
	if got := runs.Load(); got != 2 {
		t.Fatalf("restarts = %d, want 2", got)
	}
}

func TestRestartGateSkipsARequestCoveredByALaterRun(t *testing.T) {
	var g restartGate
	var runs atomic.Int32
	for i := 0; i < 3; i++ {
		_ = g.run(func() error { runs.Add(1); return nil })
	}
	if got := runs.Load(); got != 3 {
		t.Fatalf("sequential requests ran %d restarts, want one each", got)
	}
}

func TestRestartGateReportsTheErrorAndKeepsGoing(t *testing.T) {
	var g restartGate
	if err := g.run(func() error { return errTest }); err != errTest {
		t.Fatalf("run returned %v, want the restart's own error", err)
	}
	var ran bool
	if err := g.run(func() error { ran = true; return nil }); err != nil || !ran {
		t.Fatalf("a failed restart blocked the next one (err=%v ran=%v)", err, ran)
	}
}

type testError string

func (e testError) Error() string { return string(e) }

const errTest = testError("restart failed")
