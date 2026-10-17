package core

import (
	"context"
	"fmt"
	"net"
	"strings"

	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

// DialOutbound opens a connection through a running outbound of the core.
// The Telegram bot uses this when an outbound tag is set in Settings, so
// api.telegram.org can be reached through a VLESS/VMess/Reality config
// instead of a plain HTTP/SOCKS proxy.
func (c *Core) DialOutbound(ctx context.Context, tag, network, address string) (net.Conn, error) {
	box, err := c.running()
	if err != nil {
		return nil, err
	}
	ob, ok := box.outbound.Outbound(tag)
	if !ok {
		return nil, fmt.Errorf("outbound %q not found", tag)
	}
	dest := M.ParseSocksaddr(address)
	if !dest.IsValid() {
		return nil, fmt.Errorf("invalid address %q", address)
	}
	n := N.NetworkTCP
	if strings.HasPrefix(network, "udp") {
		n = N.NetworkUDP
	}
	return ob.DialContext(ctx, n, dest)
}
