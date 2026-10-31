package middleware

// TrustedProxies are the peers whose X-Forwarded-* headers are believed: the
// loopback and private ranges a reverse proxy in front of the panel or the
// subscription server lives in. gin trusts every peer unless told otherwise,
// which lets any client choose the address it is counted as.
var TrustedProxies = []string{
	"127.0.0.0/8", "::1/128",
	"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "fc00::/7",
	"169.254.0.0/16", "fe80::/10",
}
