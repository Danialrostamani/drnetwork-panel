package service

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/config"
	"github.com/Danialrostamani/drnetwork-panel/logger"
	"github.com/Danialrostamani/drnetwork-panel/util/common"
)

// The panel updates itself the way an update over SSH does: the install script
// of the latest release runs with --unattended, so it asks nothing and keeps
// the panel's settings and login. It runs outside the panel's own service, so
// that stopping the panel does not stop the update, and it starts the new
// panel at the end. Its output goes to update.log next to the binary, which the
// Settings page shows while the update runs and after the new panel is up.

// Where releases come from. Variables, so that a test install can point them
// elsewhere at link time (-X).
var (
	updateRepo    = "Danialrostamani/drnetwork-panel"
	updateAPIBase = "https://api.github.com"
	updateRawBase = "https://raw.githubusercontent.com"
)

const (
	// updateInstallDir is where install.sh puts the panel. A panel that runs
	// from anywhere else was installed another way, and the script would
	// install a second panel next to it rather than update it.
	updateInstallDir = "/usr/local/s-ui"
	updateLogName    = "update.log"
	updateLogHeader  = "# s-ui update "
	// An update that wrote no exit status for this long is over all the same
	// (the server restarted half-way, say).
	updateMaxRun = 30 * time.Minute
	// How long the latest release GitHub named is trusted, and a failed look-up.
	latestReleaseTTL = 10 * time.Minute
	latestFailedTTL  = time.Minute
	// How much of the log the page gets.
	updateLogTail = 256 << 10
)

// PanelUpdate is what the Settings page shows about updating the panel.
type PanelUpdate struct {
	Current string `json:"current"`
	// Latest is the tag of the newest release, Newer whether it is newer than
	// the running panel; CheckError tells why it is not known.
	Latest     string `json:"latest"`
	LatestURL  string `json:"latestUrl"`
	Published  int64  `json:"published"`
	Newer      bool   `json:"newer"`
	CheckError string `json:"checkError,omitempty"`
	CheckedAt  int64  `json:"checkedAt"`
	// Unsupported tells why this panel cannot update itself: "os", "docker",
	// "root", "path", "tools" or "init". Empty when it can.
	Unsupported string `json:"unsupported,omitempty"`
	// The last update started from the panel, if any.
	Running bool   `json:"running"`
	Target  string `json:"target,omitempty"`
	From    string `json:"from,omitempty"`
	Started int64  `json:"started,omitempty"`
	Exit    *int   `json:"exit,omitempty"`
	Log     string `json:"log,omitempty"`
}

type PanelUpdateService struct{}

type latestRelease struct {
	tag, url  string
	published int64
	err       error
	at        time.Time
}

var (
	// updateMu makes starting an update one at a time.
	updateMu sync.Mutex

	latestMu     sync.Mutex
	latestCached latestRelease

	releaseTagRe    = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)
	versionNumberRe = regexp.MustCompile(`[0-9]+`)
	ansiEscapeRe    = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)
)

func fetchLatestRelease() latestRelease {
	r := latestRelease{at: time.Now()}
	req, err := http.NewRequest(http.MethodGet, updateAPIBase+"/repos/"+updateRepo+"/releases/latest", nil)
	if err != nil {
		r.err = err
		return r
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "drnetwork-panel/"+config.GetFullVersion())
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		r.err = err
		return r
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		r.err = fmt.Errorf("GitHub answered %s", resp.Status)
		return r
	}
	var body struct {
		Tag       string `json:"tag_name"`
		URL       string `json:"html_url"`
		Published string `json:"published_at"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&body); err != nil {
		r.err = fmt.Errorf("unreadable answer from GitHub: %v", err)
		return r
	}
	// The tag ends up in a URL and on the install script's command line.
	if !releaseTagRe.MatchString(body.Tag) {
		r.err = fmt.Errorf("GitHub named an invalid release tag %q", body.Tag)
		return r
	}
	r.tag, r.url = body.Tag, body.URL
	if t, err := time.Parse(time.RFC3339, body.Published); err == nil {
		r.published = t.Unix()
	}
	return r
}

// latestReleaseInfo is the newest release, asked of GitHub again when force is
// set or the last answer is old.
func latestReleaseInfo(force bool) latestRelease {
	latestMu.Lock()
	defer latestMu.Unlock()
	ttl := latestReleaseTTL
	if latestCached.err != nil {
		ttl = latestFailedTTL
	}
	if force || latestCached.at.IsZero() || time.Since(latestCached.at) > ttl {
		latestCached = fetchLatestRelease()
	}
	return latestCached
}

func versionNumbers(v string) []int {
	var out []int
	for _, s := range versionNumberRe.FindAllString(v, -1) {
		n, err := strconv.Atoi(s)
		if err != nil {
			n = 0
		}
		out = append(out, n)
	}
	return out
}

// compareVersions orders two versions by their numbers, read left to right:
// "v1.6.3-drnetwork.31" is 1 6 3 31, newer than "1.6.3-drnetwork.30" and
// than a plain "1.6.3".
func compareVersions(a, b string) int {
	na, nb := versionNumbers(a), versionNumbers(b)
	for i := 0; i < len(na) || i < len(nb); i++ {
		x, y := -1, -1
		if i < len(na) {
			x = na[i]
		}
		if i < len(nb) {
			y = nb[i]
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

// updateHost is how the update runs on this server.
type updateHost struct {
	dir  string // where the panel is installed
	init string // "systemd" or "openrc"
	bash string
}

// updateHostCheck tells how this panel can update itself, or why it cannot.
// Tests replace it.
var updateHostCheck = checkUpdateHost

func checkUpdateHost() (updateHost, string) {
	if runtime.GOOS != "linux" {
		return updateHost{}, "os"
	}
	exe, err := os.Executable()
	if err == nil {
		exe, err = filepath.EvalSymlinks(exe)
	}
	if err != nil || filepath.Dir(exe) != updateInstallDir {
		// The container image keeps the panel elsewhere and is updated by
		// pulling a new image. A script install inside a container (LXC, or
		// a container with its own init) updates like any other server.
		if inContainer() {
			return updateHost{}, "docker"
		}
		return updateHost{}, "path"
	}
	if os.Geteuid() != 0 {
		return updateHost{}, "root"
	}
	h := updateHost{dir: updateInstallDir}
	if h.bash, err = exec.LookPath("bash"); err != nil || (!hasCommand("curl") && !hasCommand("wget")) {
		return updateHost{}, "tools"
	}
	if st, err := os.Stat("/run/systemd/system"); err == nil && st.IsDir() && hasCommand("systemd-run") {
		h.init = "systemd"
	} else if hasCommand("rc-service") {
		h.init = "openrc"
	} else {
		return updateHost{}, "init"
	}
	return h, ""
}

func hasCommand(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// inContainer tells whether the panel runs in a container, where an update is
// a new image rather than new files.
func inContainer() bool {
	for _, f := range []string{"/.dockerenv", "/run/.containerenv"} {
		if _, err := os.Stat(f); err == nil {
			return true
		}
	}
	b, _ := os.ReadFile("/proc/1/cgroup")
	s := string(b)
	return strings.Contains(s, "docker") || strings.Contains(s, "kubepods") || strings.Contains(s, "containerd")
}

// updateRunner runs the update outside the panel. It fetches the release's
// install script and runs it unattended, appends everything to the log and
// ends the log with the exit status. When the install fails it starts the
// panel again: the script stops the panel before it replaces the files, and a
// failure after that must not leave the server without a panel.
const updateRunner = `tag="$1" log="$2" url="$3" lang="$4"
exec >>"$log" 2>&1 </dev/null
dir=$(mktemp -d "${TMPDIR:-/tmp}/s-ui-update.XXXXXXXX") || { echo "EXIT=1"; exit 1; }
cd "$dir" || { echo "EXIT=1"; exit 1; }
rc=1
echo "Downloading $url"
if { command -v curl >/dev/null 2>&1 && curl -fsSL --retry 2 --connect-timeout 20 -o "$dir/install.sh" "$url"; } ||
	{ command -v wget >/dev/null 2>&1 && wget -q -T 20 -O "$dir/install.sh" "$url"; }; then
	SUI_LANG="$lang" bash "$dir/install.sh" --unattended --version "$tag"
	rc=$?
else
	echo "The install script could not be downloaded."
fi
cd / && rm -rf "$dir"
if [ "$rc" -ne 0 ]; then
	if command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]; then
		systemctl start s-ui
	elif command -v rc-service >/dev/null 2>&1; then
		rc-service s-ui start
	fi
fi
echo "EXIT=$rc"
exit "$rc"
`

// launchUpdate starts the runner where stopping the panel does not reach it.
// Under systemd that is a transient unit of its own, since stopping the
// panel's unit ends every process the panel started; under OpenRC a new
// session is enough. Tests replace it.
var launchUpdate = func(h updateHost, args []string) error {
	cmdline := append([]string{"-c", updateRunner, "s-ui-update"}, args...)
	if h.init == "systemd" {
		unit := fmt.Sprintf("--unit=s-ui-update-%d", time.Now().Unix())
		out, err := exec.Command("systemd-run", append([]string{unit, "--description=DrNetwork panel update", h.bash}, cmdline...)...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("systemd-run: %v: %s", err, strings.TrimSpace(string(out)))
		}
		return nil
	}
	return startDetached(h.bash, cmdline)
}

// updateLangs maps the panel's languages to the install script's.
var updateLangs = map[string]string{
	"en": "en", "fa": "fa", "ru": "ru", "vi": "vi",
	"zhcn": "zhcn", "zhHans": "zhcn", "zh-cn": "zhcn",
	"zhtw": "zhtw", "zhHant": "zhtw", "zh-tw": "zhtw",
}

// Start begins updating the panel to the latest release and returns its tag.
// lang is the language the install script writes the log in.
func (s *PanelUpdateService) Start(lang, by string) (string, error) {
	updateMu.Lock()
	defer updateMu.Unlock()
	h, why := updateHostCheck()
	if why != "" {
		return "", common.NewError("this panel cannot update itself: ", why)
	}
	logPath := filepath.Join(h.dir, updateLogName)
	if st := readUpdateLog(logPath, time.Now()); st.Running {
		return "", common.NewError("an update to ", st.Target, " is already running")
	}
	rel := latestReleaseInfo(false)
	if rel.err != nil {
		rel = latestReleaseInfo(true)
	}
	if rel.err != nil {
		return "", common.NewError("the latest release is unknown: ", rel.err.Error())
	}
	current := config.GetFullVersion()
	if compareVersions(rel.tag, current) <= 0 {
		return "", common.NewError("the panel is up to date: ", current)
	}
	scriptLang, ok := updateLangs[lang]
	if !ok {
		scriptLang = "en"
	}
	header := fmt.Sprintf("%s%d %s %s\n", updateLogHeader, time.Now().Unix(), rel.tag, current)
	if err := os.WriteFile(logPath, []byte(header), 0o600); err != nil {
		return "", err
	}
	url := fmt.Sprintf("%s/%s/%s/install.sh", updateRawBase, updateRepo, rel.tag)
	if err := launchUpdate(h, []string{rel.tag, logPath, url, scriptLang}); err != nil {
		appendUpdateLog(logPath, "The update could not be started: "+err.Error()+"\nEXIT=1\n")
		return "", err
	}
	logger.Info("panel update to ", rel.tag, " started by ", by)
	return rel.tag, nil
}

func appendUpdateLog(path, text string) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(text)
}

// Info is the panel's version, the latest release (asked of GitHub again when
// check is set) and the state of the last update.
func (s *PanelUpdateService) Info(check bool) PanelUpdate {
	out := PanelUpdate{Current: config.GetFullVersion()}
	h, why := updateHostCheck()
	out.Unsupported = why
	if why == "" {
		st := readUpdateLog(filepath.Join(h.dir, updateLogName), time.Now())
		out.Running, out.Target, out.From, out.Started, out.Exit, out.Log = st.Running, st.Target, st.From, st.Started, st.Exit, st.Log
	}
	rel := latestReleaseInfo(check)
	out.CheckedAt = rel.at.Unix()
	if rel.err != nil {
		out.CheckError = rel.err.Error()
		return out
	}
	out.Latest, out.LatestURL, out.Published = rel.tag, rel.url, rel.published
	out.Newer = compareVersions(rel.tag, out.Current) > 0
	return out
}

// updateState is what the log of the last update tells.
type updateState struct {
	Running      bool
	Target, From string
	Started      int64
	Exit         *int
	Log          string
}

// readUpdateLog reads the log the last update left: its first line is the
// header Start writes, its last the exit status the runner writes.
func readUpdateLog(path string, now time.Time) updateState {
	var st updateState
	f, err := os.Open(path)
	if err != nil {
		return st
	}
	defer f.Close()
	head := make([]byte, 256)
	n, _ := io.ReadFull(f, head)
	first, _, _ := strings.Cut(string(head[:n]), "\n")
	if !strings.HasPrefix(first, updateLogHeader) {
		return st
	}
	fields := strings.Fields(strings.TrimPrefix(first, updateLogHeader))
	if len(fields) < 3 {
		return st
	}
	st.Started, _ = strconv.ParseInt(fields[0], 10, 64)
	st.Target, st.From = fields[1], fields[2]
	size := int64(0)
	if fi, err := f.Stat(); err == nil {
		size = fi.Size()
	}
	from := size - updateLogTail
	if from < 0 {
		from = 0
	}
	body := make([]byte, size-from)
	if _, err := f.ReadAt(body, from); err != nil && err != io.EOF {
		return st
	}
	lines := strings.Split(string(body), "\n")
	if from > 0 && len(lines) > 0 {
		lines = lines[1:] // cut mid-line
	}
	var kept []string
	for _, line := range lines {
		// A progress bar redraws its line with carriage returns: what is left
		// on screen is the text after the last one.
		line = strings.TrimRight(line, "\r")
		if i := strings.LastIndexByte(line, '\r'); i >= 0 {
			line = line[i+1:]
		}
		line = ansiEscapeRe.ReplaceAllString(line, "")
		switch {
		case strings.HasPrefix(line, updateLogHeader):
			continue
		case strings.HasPrefix(line, "EXIT="):
			if code, err := strconv.Atoi(strings.TrimPrefix(line, "EXIT=")); err == nil {
				st.Exit = &code
			}
			continue
		}
		kept = append(kept, line)
	}
	st.Log = strings.TrimSpace(strings.Join(kept, "\n"))
	st.Running = st.Exit == nil && now.Sub(time.Unix(st.Started, 0)) < updateMaxRun
	return st
}
