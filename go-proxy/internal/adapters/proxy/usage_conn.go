package proxy

import "net"

type usageTrackedConn struct {
	net.Conn
	onData func(dataLen int64)
}

func NewUsageTrackedConn(conn net.Conn, onData func(dataLen int64)) net.Conn {
	return &usageTrackedConn{Conn: conn, onData: onData}
}

func (c *usageTrackedConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if n > 0 && c.onData != nil {
		c.onData(int64(n))
	}

	return n, err
}

func (c *usageTrackedConn) Write(p []byte) (int, error) {
	n, err := c.Conn.Write(p)
	if n > 0 && c.onData != nil {
		c.onData(int64(n))
	}

	return n, err
}
