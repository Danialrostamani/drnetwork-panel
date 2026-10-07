package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// The GET Object example of the AWS Signature Version 4 documentation.
func TestSignS3MatchesTheAWSExample(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "https://examplebucket.s3.amazonaws.com/test.txt", nil)
	req.Header.Set("Range", "bytes=0-9")
	signS3(req, nil, "AKIAIOSFODNN7EXAMPLE", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", "us-east-1", time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC))
	want := "AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE/20130524/us-east-1/s3/aws4_request, SignedHeaders=host;range;x-amz-content-sha256;x-amz-date, Signature=f0e8bdb87c964420e857bd35b5d6ed310bd44f0170aba48dd91039c6036bdb41"
	if got := req.Header.Get("Authorization"); got != want {
		t.Fatalf("authorization\n got %s\nwant %s", got, want)
	}
}

func TestUploadBackupToWebDAVAndS3(t *testing.T) {
	var gotPath, gotAuth, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotPath, gotAuth, gotBody = r.URL.Path, r.Header.Get("Authorization"), string(b)
		if r.Method != http.MethodPut {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()
	ctx := context.Background()
	if err := uploadBackup(ctx, RemoteBackupConfig{Kind: "webdav", URL: srv.URL + "/dav/backups", User: "u", Pass: "p"}, "x.db", []byte("data"), time.Now()); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/dav/backups/x.db" || !strings.HasPrefix(gotAuth, "Basic ") || gotBody != "data" {
		t.Fatalf("webdav: %s %s %s", gotPath, gotAuth, gotBody)
	}
	if err := uploadBackup(ctx, RemoteBackupConfig{Kind: "s3", URL: srv.URL + "/bucket", User: "AK", Pass: "SK", Region: "auto"}, "y.db", []byte("data"), time.Now()); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/bucket/y.db" || !strings.Contains(gotAuth, "Credential=AK/") || !strings.Contains(gotAuth, "/auto/s3/aws4_request") {
		t.Fatalf("s3: %s %s", gotPath, gotAuth)
	}
	if err := uploadBackup(ctx, RemoteBackupConfig{Kind: "ftp", URL: srv.URL}, "z.db", nil, time.Now()); err == nil {
		t.Fatal("an unknown storage was accepted")
	}
}
