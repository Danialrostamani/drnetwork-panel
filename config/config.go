package config

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"unicode"
)

// release is the DrNetwork version, such as 1.0: what the panel, the bot, the
// nodes and the installer show. It is bumped by hand in the commit that is
// tagged v<release>. The line restarted at 1.0 after v52.4.3; CompareVersions
// puts it after every earlier line (1.6.3-drnetwork.N, V.0.x, v32 to v52), so
// panels and nodes still on 52.x see 1.0 as an update.
//
//go:embed release
var release string

// schemaVersion is the S-UI release whose database layout this code uses. It
// is never shown: the migrations count by it and the database records it, as
// in S-UI itself, so changes taken over from S-UI bring their migrations along.
//
//go:embed version
var schemaVersion string

//go:embed name
var name string

type LogLevel string

const (
	Debug LogLevel = "debug"
	Info  LogLevel = "info"
	Warn  LogLevel = "warn"
	Error LogLevel = "error"
)

// GetVersion returns the DrNetwork version, such as "1.0".
func GetVersion() string {
	return strings.TrimSpace(release)
}

// VersionLabel writes a version the way it is shown. The 0.x line is "V.0.1";
// the product line is "v48.0".
func VersionLabel(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	s := v
	if s[0] == 'v' || s[0] == 'V' {
		s = s[1:]
		s = strings.TrimPrefix(s, ".")
	}
	if strings.HasPrefix(s, "0.") {
		return "V." + s
	}
	if v[0] >= '0' && v[0] <= '9' {
		return "v" + v
	}
	return v
}

// CompareVersions orders two DrNetwork product versions.
// 1.6.3-drnetwork.N, then v32…v45, then V.0.1 / V.0.2, then v46….
func CompareVersions(a, b string) int {
	ea, eb := versionEpoch(a), versionEpoch(b)
	if ea != eb {
		if ea < eb {
			return -1
		}
		return 1
	}
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

func versionEpoch(v string) int {
	if strings.Contains(strings.ToLower(v), "drnetwork") {
		return 0
	}
	nums := rawVersionNumbers(v)
	// S-UI's own numbers, which the oldest nodes report: 1.6.<n>.
	if len(nums) >= 3 && nums[0] == 1 && nums[1] == 6 {
		return 0
	}
	// The product line restarted at 1.0 after v52.4.3. Its versions keep
	// below 32, where the previous line began, and come after all of it. It
	// never uses a three-part 1.6.<n>, which reads as S-UI's.
	if len(nums) > 0 && nums[0] >= 1 && nums[0] < firstOldLineMajor {
		return 2
	}
	return 1
}

// firstOldLineMajor is the first major version of the line that ran from v32
// to v52.4.3; the line that restarted at 1.0 stays below it.
const firstOldLineMajor = 32

func versionNumbers(v string) []int {
	nums := rawVersionNumbers(v)
	// V.0.1 sat after v45 and before v46. Mapping 0.x → 45.x keeps that
	// order, so a panel on 0.1 still sees v46 as an update.
	if len(nums) >= 2 && nums[0] == 0 {
		return append([]int{45}, nums[1:]...)
	}
	return nums
}

func rawVersionNumbers(v string) []int {
	var out []int
	start := -1
	for i, r := range v {
		if unicode.IsDigit(r) {
			if start < 0 {
				start = i
			}
			continue
		}
		if start >= 0 {
			n, err := strconv.Atoi(v[start:i])
			if err != nil {
				n = 0
			}
			out = append(out, n)
			start = -1
		}
	}
	if start >= 0 {
		n, err := strconv.Atoi(v[start:])
		if err != nil {
			n = 0
		}
		out = append(out, n)
	}
	return out
}

// GetSchemaVersion returns the version the database migrations count by.
func GetSchemaVersion() string {
	return strings.TrimSpace(schemaVersion)
}

func GetName() string {
	return strings.TrimSpace(name)
}

func GetLogLevel() LogLevel {
	if IsDebug() {
		return Debug
	}
	logLevel := os.Getenv("SUI_LOG_LEVEL")
	if logLevel == "" {
		return Info
	}
	return LogLevel(logLevel)
}

func IsDebug() bool {
	return os.Getenv("SUI_DEBUG") == "true"
}

func GetDBFolderPath() string {
	dbFolderPath := os.Getenv("SUI_DB_FOLDER")
	if dbFolderPath == "" {
		dir, err := filepath.Abs(filepath.Dir(os.Args[0]))
		if err != nil {
			// Cross-platform fallback path
			if runtime.GOOS == "windows" {
				return "C:\\Program Files\\s-ui\\db"
			}
			return "/usr/local/s-ui/db"
		}
		dbFolderPath = filepath.Join(dir, "db")
	}
	return dbFolderPath
}

func GetDBPath() string {
	return fmt.Sprintf("%s/%s.db", GetDBFolderPath(), GetName())
}
