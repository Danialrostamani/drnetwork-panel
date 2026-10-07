package service

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/util/common"
)

// Two-factor login with time-based one-time codes (RFC 6238: SHA-1, six
// digits, thirty-second steps), the kind Google Authenticator and the like
// show.

var totpEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// ErrTotpRequired answers a right password from an account that also needs
// its code.
var ErrTotpRequired = common.NewError("enter the two-factor code")

var (
	totpMu sync.Mutex
	// Secrets made for an account that has not confirmed one yet, by username.
	totpPending = map[string]string{}
	// The last time step each account logged in with, so a code works once.
	totpUsed = map[string]int64{}
)

// NewTotpSecret returns a random 160-bit secret in base32.
func NewTotpSecret() string {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return totpEncoding.EncodeToString(b)
}

func totpCode(secret string, step int64) (string, bool) {
	key, err := totpEncoding.DecodeString(strings.ToUpper(strings.TrimRight(strings.ReplaceAll(secret, " ", ""), "=")))
	if err != nil || len(key) == 0 {
		return "", false
	}
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(step))
	mac := hmac.New(sha1.New, key)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	n := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	digits := []byte("000000")
	n %= 1_000_000
	for i := 5; i >= 0; i-- {
		digits[i] = byte('0' + n%10)
		n /= 10
	}
	return string(digits), true
}

// totpMatch finds the step, within one either side of now, the code belongs
// to; zero when none.
func totpMatch(secret, code string, now time.Time) int64 {
	code = strings.TrimSpace(code)
	if len(code) != 6 {
		return 0
	}
	step := now.Unix() / 30
	for _, s := range []int64{step, step - 1, step + 1} {
		if want, ok := totpCode(secret, s); ok && hmac.Equal([]byte(want), []byte(code)) {
			return s
		}
	}
	return 0
}

// checkTotp accepts a code for an account once: a step already used to log in
// does not work again.
func checkTotp(username, secret, code string) bool {
	step := totpMatch(secret, code, time.Now())
	if step == 0 {
		return false
	}
	totpMu.Lock()
	defer totpMu.Unlock()
	if step <= totpUsed[username] {
		return false
	}
	totpUsed[username] = step
	return true
}

// TotpState says whether the account has two-factor login on. When it does
// not, it also hands out a secret to set up and the otpauth link for its QR.
func (s *UserService) TotpState(username string) (enabled bool, secret, uri string, err error) {
	user := &model.User{}
	if err = database.GetDB().Where("username = ?", username).First(user).Error; err != nil {
		return false, "", "", err
	}
	if user.Totp != "" {
		return true, "", "", nil
	}
	totpMu.Lock()
	secret = totpPending[username]
	if secret == "" {
		secret = NewTotpSecret()
		totpPending[username] = secret
	}
	totpMu.Unlock()
	label := url.PathEscape("DrNetwork:" + username)
	uri = "otpauth://totp/" + label + "?secret=" + secret + "&issuer=DrNetwork"
	return false, secret, uri, nil
}

// EnableTotp turns two-factor login on once a code from the secret handed out
// checks.
func (s *UserService) EnableTotp(username, code string) error {
	totpMu.Lock()
	secret := totpPending[username]
	totpMu.Unlock()
	if secret == "" {
		return common.NewError("open the two-factor setup again")
	}
	if !checkTotp(username, secret, code) {
		return common.NewError("wrong code")
	}
	res := database.GetDB().Model(model.User{}).Where("username = ?", username).Update("totp", secret)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return common.NewError("no such user")
	}
	totpMu.Lock()
	delete(totpPending, username)
	totpMu.Unlock()
	return nil
}

// DisableTotp turns two-factor login off; it takes a current code.
func (s *UserService) DisableTotp(username, code string) error {
	user := &model.User{}
	if err := database.GetDB().Where("username = ?", username).First(user).Error; err != nil {
		return err
	}
	if user.Totp == "" {
		return nil
	}
	if !checkTotp(username, user.Totp, code) {
		return common.NewError("wrong code")
	}
	return database.GetDB().Model(model.User{}).Where("id = ?", user.Id).Update("totp", "").Error
}
