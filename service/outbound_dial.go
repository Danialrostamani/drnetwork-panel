package service

import (
	"context"
	"errors"
	"net"
)

// DialOutbound opens a TCP/UDP connection through a live panel outbound.
func DialOutbound(ctx context.Context, tag, network, address string) (net.Conn, error) {
	if tag == "" {
		return nil, errors.New("no outbound")
	}
	if corePtr == nil {
		return nil, errors.New("core not ready")
	}
	return corePtr.DialOutbound(ctx, tag, network, address)
}

// OutboundReady tells whether DialOutbound can use this tag right now: the
// core is running and has the outbound.
func OutboundReady(tag string) bool {
	if tag == "" || corePtr == nil {
		return false
	}
	return corePtr.HasOutbound(tag)
}
