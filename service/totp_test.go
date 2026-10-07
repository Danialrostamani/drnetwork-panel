package service

import (
	"testing"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/util"
)

func TestTotpCodeMatchesRFC6238(t *testing.T) {
	// RFC 6238 appendix B, SHA-1 key "12345678901234567890", last six digits.
	secret := totpEncoding.EncodeToString([]byte("12345678901234567890"))
	for ts, want := range map[int64]string{59: "287082", 1111111109: "081804", 1234567890: "005924", 2000000000: "279037"} {
		if got, _ := totpCode(secret, ts/30); got != want {
			t.Errorf("t=%d: %s, want %s", ts, got, want)
		}
		if totpMatch(secret, want, time.Unix(ts+30, 0)) == 0 {
			t.Errorf("t=%d: the code of the step before is not accepted", ts)
		}
		if totpMatch(secret, want, time.Unix(ts+90, 0)) != 0 {
			t.Errorf("t=%d: a code three steps old is accepted", ts)
		}
	}
}

func TestTotpLoginFlow(t *testing.T) {
	db := database.GetDB()
	hash, _ := util.HashPassword("pw")
	db.Where("username = ?", "two").Delete(&model.User{})
	db.Create(&model.User{Username: "two", Password: hash})
	t.Cleanup(func() { db.Where("username = ?", "two").Delete(&model.User{}) })
	var s UserService
	enabled, secret, uri, err := s.TotpState("two")
	if err != nil || enabled || secret == "" || uri == "" {
		t.Fatalf("setup: %v %v %q %q", err, enabled, secret, uri)
	}
	if err := s.EnableTotp("two", "000000"); err == nil {
		t.Fatal("a wrong code turned it on")
	}
	now, _ := totpCode(secret, time.Now().Unix()/30)
	if err := s.EnableTotp("two", now); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LoginWithCode("two", "pw", "", "10.9.9.1"); err != ErrTotpRequired {
		t.Fatalf("no code: %v", err)
	}
	// The code that turned it on is used up.
	if _, err := s.LoginWithCode("two", "pw", now, "10.9.9.1"); err == nil {
		t.Fatal("a used code logged in")
	}
	next, _ := totpCode(secret, time.Now().Unix()/30+1)
	if u, err := s.LoginWithCode("two", "pw", next, "10.9.9.1"); err != nil || u != "two" {
		t.Fatalf("login: %q %v", u, err)
	}
	if _, err := s.LoginWithCode("two", "bad", next, "10.9.9.2"); err == nil {
		t.Fatal("a wrong password logged in")
	}
	NoteLoginSuccess("10.9.9.1")
	if err := s.DisableTotp("two", "123456"); err == nil {
		t.Fatal("turned off without a code")
	}
	totpMu.Lock()
	delete(totpUsed, "two")
	totpMu.Unlock()
	if err := s.DisableTotp("two", now); err != nil {
		t.Fatal(err)
	}
	if u, err := s.LoginWithCode("two", "pw", "", "10.9.9.1"); err != nil || u != "two" {
		t.Fatalf("after disabling: %q %v", u, err)
	}
}
