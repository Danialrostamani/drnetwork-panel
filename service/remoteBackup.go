package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/logger"
	"github.com/Danialrostamani/drnetwork-panel/util/common"
)

// Scheduled copies of the database to storage the operator picks: an
// S3-compatible bucket (AWS, Cloudflare R2, MinIO, ArvanCloud...) or a WebDAV
// folder (Nextcloud, a NAS...).

// RemoteBackupConfig is what the settings hold for it.
type RemoteBackupConfig struct {
	// Kind is "s3" or "webdav"; empty turns it off.
	Kind string
	// URL is the bucket as https://endpoint/bucket[/folder], or the WebDAV
	// folder.
	URL    string
	User   string
	Pass   string
	Region string
	// Every is the hours between copies; zero sends only when asked.
	Every int
}

var remoteBackupClient = &http.Client{Timeout: 5 * time.Minute}

var remoteBackupBusy sync.Mutex

func (s *SettingService) RemoteBackupConfig() RemoteBackupConfig {
	c := RemoteBackupConfig{}
	c.Kind, _ = s.getString("backupKind")
	c.URL, _ = s.getString("backupUrl")
	c.User, _ = s.getString("backupUser")
	c.Pass, _ = s.getString("backupPass")
	c.Region, _ = s.getString("backupRegion")
	c.Every, _ = s.getInt("backupEvery")
	c.Kind = strings.ToLower(strings.TrimSpace(c.Kind))
	c.URL = strings.TrimRight(strings.TrimSpace(c.URL), "/")
	if strings.TrimSpace(c.Region) == "" {
		c.Region = "us-east-1"
	}
	return c
}

// RunRemoteBackup sends a copy when one is due. The cron calls it every minute.
func RunRemoteBackup() {
	var s SettingService
	c := s.RemoteBackupConfig()
	if c.Kind == "" || c.Every <= 0 || c.URL == "" {
		return
	}
	last, _ := s.getString("backupLast")
	at, _ := strconv.ParseInt(last, 10, 64)
	if time.Now().Unix()-at < int64(c.Every)*3600 {
		return
	}
	if err := SendRemoteBackup(context.Background()); err != nil {
		logger.Warning("remote backup: ", err)
	}
}

// SendRemoteBackup uploads the database now.
func SendRemoteBackup(ctx context.Context) error {
	if !remoteBackupBusy.TryLock() {
		return common.NewError("a backup is already being sent")
	}
	defer remoteBackupBusy.Unlock()
	var s SettingService
	c := s.RemoteBackupConfig()
	if c.Kind == "" || c.URL == "" {
		return common.NewError("no backup storage is set")
	}
	data, err := database.GetDb("")
	if err != nil {
		return err
	}
	// Recorded before the upload: a storage that keeps failing is retried at
	// the next period, not every minute.
	_ = s.saveSetting("backupLast", strconv.FormatInt(time.Now().Unix(), 10))
	host, _ := os.Hostname()
	if host == "" {
		host = "panel"
	}
	name := fmt.Sprintf("drnetwork-%s-%s.db", host, time.Now().UTC().Format("20060102-150405"))
	if err := uploadBackup(ctx, c, name, data, time.Now()); err != nil {
		return err
	}
	logger.Info("remote backup: sent ", name, " (", len(data), " bytes)")
	return nil
}

func uploadBackup(ctx context.Context, c RemoteBackupConfig, name string, data []byte, now time.Time) error {
	target, err := url.Parse(c.URL + "/" + url.PathEscape(name))
	if err != nil || (target.Scheme != "https" && target.Scheme != "http") || target.Host == "" {
		return common.NewError("invalid backup URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, target.String(), bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.ContentLength = int64(len(data))
	req.Header.Set("Content-Type", "application/octet-stream")
	switch c.Kind {
	case "s3":
		signS3(req, data, c.User, c.Pass, c.Region, now)
	case "webdav":
		if c.User != "" || c.Pass != "" {
			req.SetBasicAuth(c.User, c.Pass)
		}
	default:
		return common.NewError("unknown backup storage <", c.Kind, ">")
	}
	resp, err := remoteBackupClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return fmt.Errorf("storage answered %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return nil
}

// signS3 signs a request with AWS Signature Version 4 for the s3 service.
func signS3(req *http.Request, body []byte, accessKey, secretKey, region string, now time.Time) {
	sum := sha256.Sum256(body)
	payload := hex.EncodeToString(sum[:])
	amzDate := now.UTC().Format("20060102T150405Z")
	day := amzDate[:8]
	req.Header.Set("x-amz-content-sha256", payload)
	req.Header.Set("x-amz-date", amzDate)

	headers := map[string]string{"host": req.URL.Host}
	for k, v := range req.Header {
		lk := strings.ToLower(k)
		if lk == "range" || lk == "content-type" || strings.HasPrefix(lk, "x-amz-") {
			headers[lk] = strings.TrimSpace(strings.Join(v, ","))
		}
	}
	names := make([]string, 0, len(headers))
	for k := range headers {
		names = append(names, k)
	}
	sort.Strings(names)
	var canonHeaders strings.Builder
	for _, k := range names {
		canonHeaders.WriteString(k + ":" + headers[k] + "\n")
	}
	signed := strings.Join(names, ";")

	canonical := strings.Join([]string{
		req.Method,
		s3EscapePath(req.URL.EscapedPath()),
		s3CanonicalQuery(req.URL.Query()),
		canonHeaders.String(),
		signed,
		payload,
	}, "\n")
	scope := day + "/" + region + "/s3/aws4_request"
	hashed := sha256.Sum256([]byte(canonical))
	toSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + hex.EncodeToString(hashed[:])

	mac := func(key []byte, msg string) []byte {
		h := hmac.New(sha256.New, key)
		h.Write([]byte(msg))
		return h.Sum(nil)
	}
	key := mac([]byte("AWS4"+secretKey), day)
	key = mac(key, region)
	key = mac(key, "s3")
	key = mac(key, "aws4_request")
	sig := hex.EncodeToString(mac(key, toSign))
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+accessKey+"/"+scope+", SignedHeaders="+signed+", Signature="+sig)
}

// s3EscapePath escapes a path the way SigV4 wants: every byte but the
// unreserved ones and the slashes.
func s3EscapePath(p string) string {
	raw, err := url.PathUnescape(p)
	if err != nil {
		raw = p
	}
	if raw == "" {
		return "/"
	}
	var b strings.Builder
	for i := 0; i < len(raw); i++ {
		ch := raw[i]
		if ch == '/' || ch == '-' || ch == '_' || ch == '.' || ch == '~' ||
			(ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') {
			b.WriteByte(ch)
		} else {
			fmt.Fprintf(&b, "%%%02X", ch)
		}
	}
	return b.String()
}

func s3CanonicalQuery(q url.Values) string {
	if len(q) == 0 {
		return ""
	}
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		vals := append([]string(nil), q[k]...)
		sort.Strings(vals)
		for _, v := range vals {
			parts = append(parts, strings.ReplaceAll(url.QueryEscape(k), "+", "%20")+"="+strings.ReplaceAll(url.QueryEscape(v), "+", "%20"))
		}
	}
	return strings.Join(parts, "&")
}
