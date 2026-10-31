package service

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/config"
)

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"v1.6.3-drnetwork.31", "1.6.3-drnetwork.30", 1},
		{"v1.6.3-drnetwork.30", "1.6.3-drnetwork.30", 0},
		{"v1.6.3-drnetwork.9", "1.6.3-drnetwork.10", -1},
		{"v1.7.0-drnetwork.1", "1.6.3-drnetwork.30", 1},
		{"v1.6.3-drnetwork.1", "1.6.3", 1},
		{"v1.6.3", "1.6.3-drnetwork.1", -1},
		// DrNetwork's own numbers come after every 1.6.3-drnetwork.N.
		{"v32", "1.6.3-drnetwork.31", 1},
		{"v32", "1.6.3-drnetwork.99", 1},
		{"v1.6.3-drnetwork.31", "32", -1},
		{"v33", "32", 1},
		{"v32", "32", 0},
		{"v99", "100", -1},
		{"0.1", "45", 1},
		{"v0.1", "v45", 1},
		{"V.0.1", "0.1", 0},
		{"45", "0.1", -1},
		{"46", "0.2", 1},
		{"v46", "v0.1", 1},
		{"46", "45", 1},
		{"0.2", "46", -1},
		// The line restarted at 1.0 after v52.4.3.
		{"v1.0", "52.4.3", 1},
		{"v52.4.3", "1.0", -1},
		{"v9.9", "1.0", 1},
	}
	for _, c := range cases {
		if got := compareVersions(c.a, c.b); got != c.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

// fakeReleases stands in for GitHub's latest-release API.
func fakeReleases(t *testing.T, tag *atomic.Value) *atomic.Int32 {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/repos/"+updateRepo+"/releases/latest" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprintf(w, `{"tag_name":%q,"html_url":"https://example.com/r","published_at":"2026-10-08T14:29:03Z"}`, tag.Load().(string))
	}))
	t.Cleanup(srv.Close)
	oldBase := updateAPIBase
	updateAPIBase = srv.URL
	resetLatest := func() {
		latestMu.Lock()
		latestCached = latestRelease{}
		latestMu.Unlock()
	}
	resetLatest()
	t.Cleanup(func() {
		updateAPIBase = oldBase
		resetLatest()
	})
	return &calls
}

func stubUpdateHost(t *testing.T, h updateHost, why string) {
	t.Helper()
	old := updateHostCheck
	updateHostCheck = func() (updateHost, string) { return h, why }
	t.Cleanup(func() { updateHostCheck = old })
}

func TestPanelUpdateInfo(t *testing.T) {
	var tag atomic.Value
	tag.Store("v9.9")
	calls := fakeReleases(t, &tag)
	stubUpdateHost(t, updateHost{}, "docker")
	s := &PanelUpdateService{}

	info := s.Info(false)
	if info.Current != config.GetVersion() || info.Latest != "v9.9" || !info.Newer ||
		info.LatestURL != "https://example.com/r" || info.Published != 1791469743 || info.Unsupported != "docker" {
		t.Fatalf("info = %+v", info)
	}
	// The answer is reused until it is old or a check is asked for.
	s.Info(false)
	if calls.Load() != 1 {
		t.Fatalf("GitHub asked %d times", calls.Load())
	}
	tag.Store("v48.2")
	if info = s.Info(true); info.Latest != "v48.2" || info.Newer || calls.Load() != 2 {
		t.Fatalf("after a check: %+v (%d calls)", info, calls.Load())
	}
	// A tag that could not be put on a command line is refused.
	tag.Store("v1; rm -rf /")
	if info = s.Info(true); info.CheckError == "" || info.Latest != "" {
		t.Fatalf("a bad tag was taken: %+v", info)
	}
}

func TestStartPanelUpdate(t *testing.T) {
	var tag atomic.Value
	tag.Store("v9.9")
	fakeReleases(t, &tag)
	dir := t.TempDir()
	stubUpdateHost(t, updateHost{dir: dir, init: "openrc", bash: "/bin/bash"}, "")
	var launched [][]string
	oldLaunch := launchUpdate
	launchUpdate = func(h updateHost, args []string) error {
		launched = append(launched, args)
		return nil
	}
	t.Cleanup(func() { launchUpdate = oldLaunch })
	s := &PanelUpdateService{}

	got, err := s.Start("zhHans", "admin")
	if err != nil || got != "v9.9" {
		t.Fatalf("Start = %q, %v", got, err)
	}
	logPath := filepath.Join(dir, updateLogName)
	want := []string{"v9.9", logPath, updateAPIBase + "/repos/" + updateRepo + "/contents/install.sh?ref=v9.9", "zhcn", githubTokenPath}
	if len(launched) != 1 || !reflect.DeepEqual(launched[0], want) {
		t.Fatalf("launched %q", launched)
	}
	info := s.Info(false)
	if !info.Running || info.Target != "v9.9" || info.From != config.GetVersion() || info.Exit != nil {
		t.Fatalf("while running: %+v", info)
	}
	if _, err := s.Start("en", "admin"); err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("a second update started: %v", err)
	}

	// The runner ends the log with the exit status.
	appendUpdateLog(logPath, "\x1b[0;32mdone\x1b[0m\nEXIT=0\n")
	info = s.Info(false)
	if info.Running || info.Exit == nil || *info.Exit != 0 || info.Log != "done" {
		t.Fatalf("after the update: %+v", info)
	}

	// An update older than the panel, or a panel that cannot update itself,
	// starts nothing.
	tag.Store("v45")
	s.Info(true)
	if _, err := s.Start("en", "admin"); err == nil || !strings.Contains(err.Error(), "up to date") {
		t.Fatalf("an older release was installed: %v", err)
	}
	stubUpdateHost(t, updateHost{}, "path")
	tag.Store("v9.9")
	s.Info(true)
	if _, err := s.Start("en", "admin"); err == nil {
		t.Fatal("an update started where the panel cannot update itself")
	}
	if len(launched) != 1 {
		t.Fatalf("launched %d updates", len(launched))
	}
}

func TestForcePanelUpdateInstallsEvenIfNotNewer(t *testing.T) {
	var tag atomic.Value
	tag.Store("v48.2")
	fakeReleases(t, &tag)
	dir := t.TempDir()
	stubUpdateHost(t, updateHost{dir: dir, init: "openrc", bash: "/bin/bash"}, "")
	var launched [][]string
	oldLaunch := launchUpdate
	launchUpdate = func(h updateHost, args []string) error {
		launched = append(launched, args)
		return nil
	}
	t.Cleanup(func() { launchUpdate = oldLaunch })
	s := &PanelUpdateService{}
	if _, err := s.Start("en", "admin"); err == nil || !strings.Contains(err.Error(), "up to date") {
		t.Fatalf("same version without force: %v", err)
	}
	got, err := s.StartAt("en", "admin", "", true)
	if err != nil || got != "v48.2" {
		t.Fatalf("force latest = %q, %v", got, err)
	}
	_, err = s.StartAt("fa", "admin", "v9.9", true)
	if err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("second force started: %v", err)
	}
	if len(launched) != 1 || launched[0][0] != "v48.2" {
		t.Fatalf("launched %q", launched)
	}
}

func TestReadUpdateLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), updateLogName)
	now := time.Unix(2000, 0)
	text := updateLogHeader + "1990 v2 v1\n" +
		"\x1b[0;33mInstalling...\x1b[0m\n" +
		"s-ui.tar.gz   10%\rs-ui.tar.gz   60%\rs-ui.tar.gz  100%\r\n" +
		"ok\n"
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	st := readUpdateLog(path, now)
	if !st.Running || st.Target != "v2" || st.From != "v1" || st.Started != 1990 || st.Exit != nil {
		t.Fatalf("state = %+v", st)
	}
	if want := "Installing...\ns-ui.tar.gz  100%\nok"; st.Log != want {
		t.Fatalf("log = %q, want %q", st.Log, want)
	}
	// Without an exit status, an update is over after a while all the same.
	if st = readUpdateLog(path, now.Add(updateMaxRun)); st.Running {
		t.Fatal("an update from long ago still runs")
	}
	appendUpdateLog(path, "EXIT=3\n")
	if st = readUpdateLog(path, now); st.Running || st.Exit == nil || *st.Exit != 3 {
		t.Fatalf("after the exit status: %+v", st)
	}
	// Anything else is no update log at all.
	if err := os.WriteFile(path, []byte("hello\nEXIT=0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if st = readUpdateLog(path, now); st.Target != "" || st.Exit != nil {
		t.Fatalf("a foreign file was read: %+v", st)
	}
}

// The runner itself, with a stand-in install script: it is run unattended
// with the tag and the language, everything lands in the log, and the log
// ends with the exit status.
func TestUpdateRunner(t *testing.T) {
	// The runner gets a PATH with only the tools it needs. Without systemctl
	// and rc-service a failed run cannot start a real panel on the machine
	// that runs the tests.
	bin := t.TempDir()
	for _, tool := range []string{"bash", "mktemp", "rm", "curl", "wget"} {
		if p, err := exec.LookPath(tool); err == nil {
			if err := os.Symlink(p, filepath.Join(bin, tool)); err != nil {
				t.Fatal(err)
			}
		}
	}
	bash := filepath.Join(bin, "bash")
	for _, need := range [][]string{{"bash"}, {"mktemp"}, {"rm"}, {"curl"}} {
		found := false
		for _, tool := range need {
			if _, err := os.Lstat(filepath.Join(bin, tool)); err == nil {
				found = true
			}
		}
		if !found {
			t.Skipf("no %s", strings.Join(need, " or "))
		}
	}
	script := "#!/bin/bash\necho \"args: $*\"\necho \"lang: $SUI_LANG\"\nexit ${FAKE_EXIT:-0}\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/install.sh":
		case "/private.sh":
			// A private repository: the API answers with the file only for
			// the token, and only when asked for the raw content.
			if r.Header.Get("Authorization") != "Bearer ghp_secret123" || r.Header.Get("Accept") != "application/vnd.github.raw" {
				http.NotFound(w, r)
				return
			}
		default:
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(script))
	}))
	defer srv.Close()
	tokenFile := filepath.Join(t.TempDir(), "github-token")
	if err := os.WriteFile(tokenFile, []byte("ghp_secret123\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tokfile := ""
	run := func(url string, env ...string) string {
		t.Helper()
		logPath := filepath.Join(t.TempDir(), updateLogName)
		cmd := exec.Command(bash, "-c", updateRunner, "s-ui-update", "v9", logPath, url, "fa", tokfile)
		cmd.Env = append([]string{"PATH=" + bin, "TMPDIR=" + t.TempDir()}, env...)
		_ = cmd.Run()
		b, err := os.ReadFile(logPath)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	out := run(srv.URL + "/install.sh")
	if !strings.Contains(out, "args: --unattended --version v9\n") || !strings.Contains(out, "lang: fa\n") || !strings.HasSuffix(out, "EXIT=0\n") {
		t.Fatalf("log:\n%s", out)
	}
	if out = run(srv.URL+"/install.sh", "FAKE_EXIT=4"); !strings.HasSuffix(out, "EXIT=4\n") {
		t.Fatalf("failed install, log:\n%s", out)
	}
	if out = run(srv.URL + "/missing.sh"); !strings.Contains(out, "could not be downloaded") || !strings.HasSuffix(out, "EXIT=1\n") {
		t.Fatalf("missing script, log:\n%s", out)
	}

	// The private script needs the saved token, which never shows in the log.
	if out = run(srv.URL + "/private.sh"); !strings.Contains(out, "could not be downloaded") {
		t.Fatalf("private script without the token, log:\n%s", out)
	}
	tokfile = tokenFile
	out = run(srv.URL + "/private.sh")
	if !strings.Contains(out, "args: --unattended --version v9\n") || !strings.HasSuffix(out, "EXIT=0\n") {
		t.Fatalf("private script with the token, log:\n%s", out)
	}
	if strings.Contains(out, "ghp_secret123") {
		t.Fatalf("the token was written to the log:\n%s", out)
	}
}
