package middleware

import (
	"net/http"

	"github.com/Danialrostamani/drnetwork-panel/util/hosts"

	"github.com/gin-gonic/gin"
)

// Context keys DomainValidator leaves for the handlers: the list entry the
// request's host matched and, for a wildcard entry, the label in its place.
const (
	HostEntryKey = "hostEntry"
	HostLabelKey = "hostLabel"
)

// DomainValidator answers only requests for a host in the list: names, IP
// literals and one-label wildcards ("*.sub.example.com"), separated by commas
// or spaces. A single domain, the old form of the setting, is a list of one.
func DomainValidator(domains string) gin.HandlerFunc {
	entries := hosts.Parse(domains)
	return func(c *gin.Context) {
		entry, label, ok := hosts.Match(entries, c.Request.Host)
		if !ok {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		c.Set(HostEntryKey, entry)
		c.Set(HostLabelKey, label)
		c.Next()
	}
}
