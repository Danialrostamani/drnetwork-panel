package core

import "testing"

func TestUserTrafficSnapshotDoesNotDrainOrLeakOtherUsers(t *testing.T) {
	st := NewSessionTracker()
	c := st.loadOrCreateCounter(st.users, "alice")
	c.read.Add(123)
	c.write.Add(456)
	other := st.loadOrCreateCounter(st.users, "bob")
	other.read.Add(999)
	for range 3 {
		up, down := st.UserTraffic("alice")
		if up != 123 || down != 456 {
			t.Fatalf("snapshot %d/%d", up, down)
		}
	}
	if up, down := st.UserTraffic("missing"); up != 0 || down != 0 {
		t.Fatal("other user's traffic leaked")
	}
	got := st.GetStats()
	var up, down int64
	for _, s := range *got {
		if s.Tag == "alice" {
			if s.Direction {
				up += s.Traffic
			} else {
				down += s.Traffic
			}
		}
	}
	if up != 123 || down != 456 {
		t.Fatalf("snapshot changed accounting: %d/%d", up, down)
	}
	if up, down = st.UserTraffic("alice"); up != 0 || down != 0 {
		t.Fatal("drained traffic still counted")
	}
	c.read.Add(7)
	if up, down = st.UserTraffic("alice"); up != 7 || down != 0 {
		t.Fatal("new traffic missing")
	}
}
