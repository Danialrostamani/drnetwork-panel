package api

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/Danialrostamani/drnetwork-panel/service"

	"github.com/gin-gonic/gin"
)

// smsBodyMax caps a forwarded bank message request.
const smsBodyMax = 16 << 10

var (
	smsTextKeys   = []string{"text", "message", "body", "msg", "content", "sms"}
	smsSenderKeys = []string{"from", "sender", "number", "phone", "address"}
)

// SmsHook takes the bank's deposit messages from a forwarder app on the phone
// that gets them. The address carries the shop's SMS key: a wrong key gets a
// bare 404, and repeated failures lock the sender out the way failed logins
// do. A forwarder that can sign may send X-Signature, the hex HMAC-SHA256 of
// the body under the key; when it is there it must match.
func SmsHook(c *gin.Context) {
	key := "sms:" + getRemoteIp(c)
	if locked, _ := service.LoginLockedOut(key); locked {
		c.AbortWithStatus(http.StatusTooManyRequests)
		return
	}
	s := &service.ShopService{}
	secret := s.Settings().SmsSecret
	if secret == "" || subtle.ConstantTimeCompare([]byte(c.Param("secret")), []byte(secret)) != 1 {
		service.NoteLoginFailure(key)
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, smsBodyMax))
	if err != nil {
		c.AbortWithStatus(http.StatusRequestEntityTooLarge)
		return
	}
	if sig := strings.TrimSpace(c.GetHeader("X-Signature")); sig != "" {
		got, err := hex.DecodeString(strings.TrimPrefix(strings.ToLower(sig), "sha256="))
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(body)
		if err != nil || !hmac.Equal(got, mac.Sum(nil)) {
			service.NoteLoginFailure(key)
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
	}
	service.NoteLoginSuccess(key)
	text, sender := smsFields(c.GetHeader("Content-Type"), body)
	if strings.TrimSpace(text) == "" {
		c.JSON(http.StatusBadRequest, Msg{Msg: "no message text"})
		return
	}
	res, err := s.HandleSms(text, sender)
	if err != nil {
		c.JSON(http.StatusBadRequest, Msg{Msg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, Msg{Success: true, Obj: res})
}

// smsFields finds the message text and its sender in a JSON, form or plain
// text body: every forwarder app names its fields its own way.
func smsFields(contentType string, body []byte) (text, sender string) {
	mt, params, _ := mime.ParseMediaType(contentType)
	fields := map[string]string{}
	put := func(k, v string) {
		lk := strings.ToLower(strings.TrimSpace(k))
		if _, ok := fields[lk]; !ok || k == lk {
			fields[lk] = v
		}
	}
	switch {
	case mt == "multipart/form-data":
		form, err := multipart.NewReader(bytes.NewReader(body), params["boundary"]).ReadForm(smsBodyMax)
		if err != nil {
			return "", ""
		}
		defer func() { _ = form.RemoveAll() }()
		for k, v := range form.Value {
			if len(v) > 0 {
				put(k, v[0])
			}
		}
	case mt == "application/x-www-form-urlencoded":
		vals, _ := url.ParseQuery(string(body))
		for k, v := range vals {
			if len(v) > 0 {
				put(k, v[0])
			}
		}
	case mt == "application/json" || bytes.HasPrefix(bytes.TrimSpace(body), []byte("{")):
		var m map[string]any
		if err := json.Unmarshal(body, &m); err != nil {
			if mt == "application/json" {
				return "", ""
			}
			return string(body), ""
		}
		for k, v := range m {
			switch v := v.(type) {
			case string:
				put(k, v)
			case float64:
				put(k, strconv.FormatFloat(v, 'f', -1, 64))
			}
		}
	default:
		return string(body), ""
	}
	return firstField(fields, smsTextKeys), firstField(fields, smsSenderKeys)
}

func firstField(m map[string]string, keys []string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(m[k]); v != "" {
			return v
		}
	}
	return ""
}
