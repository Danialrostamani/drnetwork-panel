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
	if VersionLabel(v) != "v52.3" {
		t.Errorf("VersionLabel(%q) = %q, want v52.3", v, VersionLabel(v))
	}
	for in, want := range map[string]string{
		"0.1":                "V.0.1",
		"v0.1":               "V.0.1",
		"V.0.1":              "V.0.1",
		"v32":                "v32",
		"45":                 "v45",
		"1.6.3-drnetwork.31": "v1.6.3-drnetwork.31",
		"":                   "",
	} {
		if got := VersionLabel(in); got != want {
			t.Errorf("VersionLabel(%q) = %q, want %q", in, got, want)
		}
	}
	if GetSchemaVersion() == "" {
		t.Fatal("no schema version for the migrations")
	}
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"v1.6.3-drnetwork.31", "1.6.3-drnetwork.30", 1},
		{"v32", "1.6.3-drnetwork.31", 1},
		{"v45", "32", 1},
		{"0.1", "45", 1},
		{"v0.1", "v45", 1},
		{"V.0.1", "0.1", 0},
		{"0.2", "0.1", 1},
		{"45", "0.1", -1},
		{"1.6.3", "0.1", -1},
		{"46", "0.2", 1},
		{"v46", "v0.1", 1},
		{"46", "45", 1},
		{"0.2", "46", -1},
		// Dotted 48.0 is the same line as V.0.2 on a v0.2 panel (two
		// numbers) and newer than both 0.2 and the integer v47 tag.
		{"48.0", "0.2", 1},
		{"v48.0", "v47", 1},
		{"v47", "48.0", -1},
	}
	for _, c := range cases {
		if got := CompareVersions(c.a, c.b); got != c.want {
			t.Errorf("CompareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}
