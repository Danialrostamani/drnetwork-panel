package config

import (
	"regexp"
	"testing"
)

// The release is tagged v<version>, and the installer turns a bare number
// into such a tag, so the version is plain numbers.
func TestVersion(t *testing.T) {
	v := GetVersion()
	if !regexp.MustCompile(`^[0-9]+(\.[0-9]+)*$`).MatchString(v) {
		t.Fatalf("version %q is not plain numbers", v)
	}
	for in, want := range map[string]string{v: "v" + v, "v32": "v32", "1.6.3-drnetwork.31": "v1.6.3-drnetwork.31", "": ""} {
		if got := VersionLabel(in); got != want {
			t.Errorf("VersionLabel(%q) = %q, want %q", in, got, want)
		}
	}
	if GetSchemaVersion() == "" {
		t.Fatal("no schema version for the migrations")
	}
}
