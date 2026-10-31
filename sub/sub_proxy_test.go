package sub

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// The limit on guessing subscription names counts against the address a
// request comes from. Behind a reverse proxy on this machine that is the
// forwarded one, and the subscription server used to trust every hop of it --
// so the client's own first entry won, and it could make every guess from a
// new address or name someone else's to have it cut off.
func TestSubServerCountsTheAddressItsProxySaw(t *testing.T) {
	pageServer(t) // database and settings
	engine, err := NewServer().initRouter()
	if err != nil {
		t.Fatal(err)
	}
	engine.GET("/__test/ip", func(c *gin.Context) { c.String(http.StatusOK, subRequestIP(c)) })

	cases := []struct {
		name, remote, forwarded, want string
	}{
		// nginx appends the address it saw to what the client sent.
		{"client-supplied entry ahead of the proxy's", "127.0.0.1:40000", "203.0.113.7, 198.51.100.9", "198.51.100.9"},
		{"proxy on the LAN", "10.0.0.5:40000", "203.0.113.7, 198.51.100.9", "198.51.100.9"},
		{"plain forwarded address", "127.0.0.1:40000", "198.51.100.9", "198.51.100.9"},
		// A direct client's own header is never believed.
		{"direct client", "198.51.100.20:40000", "203.0.113.7", "198.51.100.20"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/__test/ip", nil)
			req.RemoteAddr = tc.remote
			req.Header.Set("X-Forwarded-For", tc.forwarded)
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, req)
			if got := w.Body.String(); got != tc.want {
				t.Errorf("counted as %q, want %q", got, tc.want)
			}
		})
	}
}
