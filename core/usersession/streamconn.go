package usersession

import (
	"net"
	"sync"

	N "github.com/sagernet/sing/common/network"
)

// EndOnClose ties a connection from a V2Ray transport to the stream that
// carries it. Some transports -- gRPC above all -- give a connection whose
// Close does nothing on the server side: the stream lives until the handler
// reports it done. Closing the connection to cut a user off (their volume ran
// out, the panel disconnected them) then left the tunnel working. With this,
// closing the connection also ends the stream.
//
// It returns the wrapped connection and the close handler to pass on in place
// of onClose; both end the stream once, whichever comes first.
func EndOnClose(conn net.Conn, onClose N.CloseHandlerFunc) (net.Conn, N.CloseHandlerFunc) {
	if onClose == nil {
		return conn, nil
	}
	once := N.OnceClose(onClose)
	return &streamConn{Conn: conn, end: once}, once
}

type streamConn struct {
	net.Conn
	end  N.CloseHandlerFunc
	once sync.Once
}

func (c *streamConn) Close() error {
	err := c.Conn.Close()
	c.once.Do(func() { c.end(net.ErrClosed) })
	return err
}

func (c *streamConn) Upstream() any { return c.Conn }

func (c *streamConn) ReaderReplaceable() bool { return true }

func (c *streamConn) WriterReplaceable() bool { return true }
