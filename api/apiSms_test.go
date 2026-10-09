package api

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/service"

	"github.com/gin-gonic/gin"
)

func TestSmsFields(t *testing.T) {
	var mp bytes.Buffer
	w := multipart.NewWriter(&mp)
	_ = w.WriteField("From", "BankMelli")
	_ = w.WriteField("Message", "واریز 1,000")
	_ = w.Close()
	cases := []struct {
		name, ctype, body, text, sender string
	}{
		{"json", "application/json", `{"Text":"واریز 2,000","from":"+98"}`, "واریز 2,000", "+98"},
		{"json number sender", "application/json; charset=utf-8", `{"msg":"deposit 3,000","number":989121234567}`, "deposit 3,000", "989121234567"},
		{"json without type", "", `{"content":"deposit 4,000"}`, "deposit 4,000", ""},
		{"bad json", "application/json", `{"text":`, "", ""},
		{"form", "application/x-www-form-urlencoded", "sms=%D9%88%D8%A7%D8%B1%DB%8C%D8%B2+5%2C000&sender=bank", "واریز 5,000", "bank"},
		{"multipart", w.FormDataContentType(), mp.String(), "واریز 1,000", "BankMelli"},
		{"plain", "text/plain", "واریز 6,000", "واریز 6,000", ""},
		{"brace text", "text/plain", "{not json} واریز 7,000", "{not json} واریز 7,000", ""},
	}
	for _, c := range cases {
		text, sender := smsFields(c.ctype, []byte(c.body))
		if text != c.text || sender != c.sender {
			t.Errorf("%s: got %q %q, want %q %q", c.name, text, sender, c.text, c.sender)
		}
	}
}

func TestSmsHook(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "sms.db")); err != nil {
		t.Fatal(err)
	}
	s := &service.ShopService{}
	r := gin.New()
	r.POST("/hook/sms/:secret", SmsHook)
	post := func(path, ctype, body, sig, ip string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.RemoteAddr = ip + ":5000"
		if ctype != "" {
			req.Header.Set("Content-Type", ctype)
		}
		if sig != "" {
			req.Header.Set("X-Signature", sig)
		}
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}
	// No key: closed to everybody.
	if rec := post("/hook/sms/anything-at-all-123", "text/plain", "واریز 1,000", "", "192.0.2.1"); rec.Code != http.StatusNotFound || rec.Body.Len() != 0 {
		t.Fatalf("closed hook answered %d %q", rec.Code, rec.Body.String())
	}
	if err := s.SetSetting("shopSmsSecret", "short"); err == nil {
		t.Fatal("a short key was saved")
	}
	key := "k3y-for-the-bank-SMS_0123456789"
	if err := s.SetSetting("shopSmsSecret", key); err != nil {
		t.Fatal(err)
	}
	if rec := post("/hook/sms/wrong-key-wrong-key", "text/plain", "واریز 1,000", "", "192.0.2.2"); rec.Code != http.StatusNotFound {
		t.Fatalf("wrong key: %d", rec.Code)
	}
	body := `{"text":"واریز 1,000,000 ریال","from":"bank"}`
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(body))
	good := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if rec := post("/hook/sms/"+key, "application/json", body, "sha256=00", "192.0.2.3"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad signature: %d", rec.Code)
	}
	rec := post("/hook/sms/"+key, "application/json", body, good, "192.0.2.3")
	if rec.Code != http.StatusOK {
		t.Fatalf("signed message: %d %s", rec.Code, rec.Body.String())
	}
	var m struct {
		Success bool
		Obj     service.SmsResult
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil || !m.Success || m.Obj.Status != model.SmsUnmatched || m.Obj.Amount != 100000 { // rials, read as tomans
		t.Fatalf("answer: %s (%v)", rec.Body.String(), err)
	}
	// Unsigned is fine too; the same message again is a repeat.
	rec = post("/hook/sms/"+key, "application/json", body, "", "192.0.2.3")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), model.SmsDuplicate) {
		t.Fatalf("repeat: %d %s", rec.Code, rec.Body.String())
	}
	if rec := post("/hook/sms/"+key, "application/json", `{"sender":"x"}`, "", "192.0.2.3"); rec.Code != http.StatusBadRequest {
		t.Fatalf("no text: %d", rec.Code)
	}
	if rec := post("/hook/sms/"+key, "text/plain", strings.Repeat("x", 20<<10), "", "192.0.2.3"); rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("huge body: %d", rec.Code)
	}
	// Guessing the key locks the guesser out, even with the right key after.
	for i := 0; i < 12; i++ {
		post("/hook/sms/guess-guess-guess-guess", "text/plain", "x", "", "192.0.2.9")
	}
	if rec := post("/hook/sms/"+key, "text/plain", "واریز 2,000", "", "192.0.2.9"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("after guessing: %d", rec.Code)
	}
	if log, _ := s.SmsLog(10); len(log) != 1 || log[0].Sender != "bank" {
		t.Fatalf("log: %+v", log)
	}
}
