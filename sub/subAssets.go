package sub

import (
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strings"
	"sync"

	"github.com/Danialrostamani/drnetwork-panel/config"

	"github.com/gin-gonic/gin"
)

// What the status page loads besides itself is served next to it, under
// names no client link uses, so the page needs nothing from another site --
// a CDN that is blocked or slow left it without its font and QR codes.

//go:embed assets/qrcode.min.js
var qrScript string

//go:embed assets/vazirmatn.woff2.b64
var fontB64 string

//go:embed assets/icon192.png.b64
var icon192B64 string

//go:embed assets/icon512.png.b64
var icon512B64 string

const (
	assetQR       = "sui-qr.js"
	assetFont     = "sui-font.woff2"
	assetIcon192  = "sui-icon-192.png"
	assetIcon512  = "sui-icon-512.png"
	assetManifest = "manifest.json"
	assetWorker   = "sw.js"
)

var (
	decodeOnce sync.Once
	decoded    map[string][]byte
)

func binaryAssets() map[string][]byte {
	decodeOnce.Do(func() {
		decoded = map[string][]byte{}
		for name, b64 := range map[string]string{assetFont: fontB64, assetIcon192: icon192B64, assetIcon512: icon512B64} {
			if b, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(b64), "")); err == nil {
				decoded[name] = b
			}
		}
	})
	return decoded
}

// isAsset tells the page's own files from client names.
func isAsset(name string) bool {
	switch name {
	case assetQR, assetFont, assetIcon192, assetIcon512, assetManifest, assetWorker:
		return true
	}
	return false
}

// serveAsset answers a request for one of the page's files; head leaves the
// body out.
func (s *SubHandler) serveAsset(c *gin.Context, name string, head bool) {
	var body []byte
	var kind string
	long := true
	switch name {
	case assetQR:
		body, kind = []byte(qrScript), "application/javascript; charset=utf-8"
	case assetFont:
		body, kind = binaryAssets()[name], "font/woff2"
	case assetIcon192, assetIcon512:
		body, kind = binaryAssets()[name], "image/png"
	case assetManifest:
		body, kind, long = s.manifest(c.Query("id")), "application/manifest+json", false
	case assetWorker:
		body, kind, long = []byte(serviceWorker), "application/javascript; charset=utf-8", false
	}
	if len(body) == 0 {
		c.String(404, "")
		return
	}
	if long {
		c.Header("Cache-Control", "public, max-age=604800")
	} else {
		c.Header("Cache-Control", "no-cache")
	}
	c.Header("X-Content-Type-Options", "nosniff")
	if head {
		c.Header("Content-Type", kind)
		c.Status(200)
		return
	}
	c.Data(200, kind, body)
}

// manifest lets a browser install the page of one client as an app that
// opens on that client.
func (s *SubHandler) manifest(id string) []byte {
	conf := s.SettingService.GetSubPageConf()
	name := conf.Name
	if name == "" {
		name = "وضعیت اشتراک"
	}
	start := "./"
	if id = strings.TrimSpace(id); id != "" && len(id) <= 128 && !strings.ContainsAny(id, "/\\") {
		start = "./" + url.PathEscape(id)
	}
	theme := conf.Accent
	if theme == "" {
		theme = "#0C96B2"
	}
	v := "?v=" + config.GetVersion()
	m := map[string]any{
		"name":             name,
		"short_name":       name,
		"start_url":        start,
		"scope":            "./",
		"display":          "standalone",
		"dir":              "rtl",
		"lang":             "fa",
		"background_color": "#F7F7F8",
		"theme_color":      theme,
		"icons": []map[string]string{
			{"src": assetIcon192 + v, "sizes": "192x192", "type": "image/png", "purpose": "any"},
			{"src": assetIcon512 + v, "sizes": "512x512", "type": "image/png", "purpose": "any"},
		},
	}
	out, _ := json.Marshal(m)
	return out
}

// serviceWorker keeps the last copy of a page and of its files, so an
// installed page still opens with no connection and shows what it last knew.
const serviceWorker = `const CACHE = "sui-page-v1";
self.addEventListener("install", function () { self.skipWaiting(); });
self.addEventListener("activate", function (e) { e.waitUntil(self.clients.claim()); });
function keep(key, res, page) {
  if (!res || !res.ok) return;
  if (page && (res.headers.get("content-type") || "").indexOf("text/html") < 0) return;
  const copy = res.clone();
  caches.open(CACHE).then(function (c) { return c.put(key, copy); }).catch(function () {});
}
self.addEventListener("message", function (e) {
  const d = e.data || {};
  if (d.type !== "cache-self" || !d.href) return;
  e.waitUntil(fetch(d.href, { headers: { Accept: "text/html" }, cache: "no-store" })
    .then(function (res) { keep(d.key || d.href, res, true); }).catch(function () {}));
});
self.addEventListener("fetch", function (e) {
  const r = e.request;
  if (r.method !== "GET") return;
  const u = new URL(r.url);
  if (u.origin !== location.origin) return;
  if (/\/sui-[^/]+$/.test(u.pathname)) {
    e.respondWith(caches.match(r).then(function (hit) {
      return hit || fetch(r).then(function (res) { keep(r, res); return res; });
    }));
    return;
  }
  if (r.mode !== "navigate") return;
  e.respondWith(fetch(r).then(function (res) { keep(u.pathname, res, true); return res; })
    .catch(function () {
      return caches.match(u.pathname).then(function (hit) { return hit || Response.error(); });
    }));
});
`
