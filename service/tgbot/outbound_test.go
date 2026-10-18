package tgbot

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// With an outbound set but the core not running, the bot must still reach
// Telegram, through the proxy when one is set, otherwise directly.
func TestOutboundFallsBackWhenCoreIsDown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	defer srv.Close()
	tr := &http.Transport{}
	useOutbound(tr, "proxy-out", nil)
	res, err := (&http.Client{Transport: tr}).Get(srv.URL)
	if err != nil {
		t.Fatalf("direct fallback: %v", err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if string(body) != "ok" {
		t.Fatalf("body = %q", body)
	}
	proxy, _ := url.Parse("http://127.0.0.1:3128")
	tr = &http.Transport{}
	useOutbound(tr, "proxy-out", proxy)
	req, _ := http.NewRequest("GET", "https://api.telegram.org/", nil)
	if got, err := tr.Proxy(req); err != nil || got == nil || got.Host != "127.0.0.1:3128" {
		t.Fatalf("proxy fallback = %v, %v", got, err)
	}
}
