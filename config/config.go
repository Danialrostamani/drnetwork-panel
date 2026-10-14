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

// release is the DrNetwork version, such as 0.1: what the panel, the bot, the
// nodes and the installer show. It is bumped by hand in the commit that is
// tagged v<release>.
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

// GetVersion returns the DrNetwork version, such as "0.1".
func GetVersion() string {
	return strings.TrimSpace(release)
}

// VersionLabel writes a version the way it is shown. The 0.x line is "V.0.1";
// the older integer line stays "v45".
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

// CompareVersions orders two DrNetwork product versions. The 0.x line (0.1)
// comes after the integer line (v45), which came after 1.6.3-drnetwork.N.
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
	nums := versionNumbers(v)
	if len(nums) <= 1 {
		return 1
	}
	if nums[0] == 1 && nums[1] == 6 {
		return 0
	}
	return 2
}

func versionNumbers(v string) []int {
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
