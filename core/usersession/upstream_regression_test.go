package usersession

import (
	"testing"
	"time"
)

type missingQUICSession struct{}

func (missingQUICSession) Close() error { return ErrNotClosed }
func TestIdleSourcesAreSweptOnNewSource(t *testing.T) {
	r := NewRegistry()
	r.Bind("old", "old")
	r.sources["old"].lastSeen = time.Now().Add(-2 * idleTimeout)
	r.swept = time.Time{}
	r.blocked["expired"] = &block{at: time.Now().Add(-2 * blockTimeout)}
	r.blocked["live"] = &block{at: time.Now()}
	r.Bind("new", "new")
	if _, ok := r.sources["old"]; ok {
		t.Fatal("idle source leaked")
	}
	if _, ok := r.blocked["expired"]; ok {
		t.Fatal("expired block leaked")
	}
	if r.Allowed("live") {
		t.Fatal("live block removed")
	}
}
func TestMissingQUICCloserStillMutesRemovedAndKickedSession(t *testing.T) {
	for _, kick := range []bool{false, true} {
		r := NewRegistry()
		r.Track("source", missingQUICSession{})
		r.Bind("user", "source")
		if kick {
			r.KickUserSessions("user")
		} else {
			r.CloseUsers(nil)
		}
		if r.Allowed("source") {
			t.Fatal("session without a working closer can bypass removal")
		}
		if r.blocked["source"].kick != kick {
			t.Fatal("wrong block lifetime")
		}
	}
}
