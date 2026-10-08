package config

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// release is the DrNetwork version, such as 32: what the panel, the bot, the
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

// GetVersion returns the DrNetwork version, such as "32".
func GetVersion() string {
	return strings.TrimSpace(release)
}

// VersionLabel writes a version the way it is shown, with a v: "v32".
func VersionLabel(v string) string {
	if v != "" && v[0] >= '0' && v[0] <= '9' {
		return "v" + v
	}
	return v
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
