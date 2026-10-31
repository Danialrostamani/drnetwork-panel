package service

import (
	"testing"

	"github.com/Danialrostamani/drnetwork-panel/config"
)

// The home page shows the panel's and the core's versions from the system
// info, which is fetched whatever tiles are picked.
func TestSystemInfoCarriesBothVersions(t *testing.T) {
	info := (&ServerService{}).GetSystemInfo()
	if info["appVersion"] != config.GetVersion() {
		t.Errorf("appVersion = %v, want %q", info["appVersion"], config.GetVersion())
	}
	core, ok := info["coreVersion"].(string)
	if !ok {
		t.Fatalf("coreVersion = %#v, want a string", info["coreVersion"])
	}
	if core != singboxVersion() {
		t.Errorf("coreVersion = %q, want %q", core, singboxVersion())
	}
}
