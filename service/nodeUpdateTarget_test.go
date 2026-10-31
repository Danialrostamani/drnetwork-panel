package service

import "testing"

// Nodes were offered an update only when older than the master. With the
// master on the same old release as its nodes, the Nodes page never said a
// newer release was out. A node is now measured against the latest release
// as well, whichever is newer.
func TestNodeUpdateTargetIsTheNewerOfMasterAndRelease(t *testing.T) {
	old := latestTagFor
	t.Cleanup(func() { latestTagFor = old })

	cases := []struct {
		master, latest, node string
		outdated             bool
	}{
		{"52.4.1", "v52.4.3", "52.4.1", true},  // master behind the release too
		{"52.4.1", "v52.4.3", "52.4.3", false}, // node already on the release
		{"52.4.3", "v52.4.1", "52.4.1", true},  // master newer than the release
		{"52.4.3", "", "52.4.3", false},        // GitHub not reachable: master alone
		{"52.4.3", "", "52.4.1", true},
		{"", "v52.4.3", "52.4.1", true},
	}
	for _, c := range cases {
		latestTagFor = func() string { return c.latest }
		target := nodeUpdateTarget(c.master)
		if got := c.node != "" && versionOlder(c.node, target); got != c.outdated {
			t.Errorf("master %q, latest %q, node %q: outdated = %v, want %v (target %q)", c.master, c.latest, c.node, got, c.outdated, target)
		}
	}
}
