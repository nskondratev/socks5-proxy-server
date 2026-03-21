package proxy

import (
	"net"
	"sync"
)

type usageTrackedConn struct {
	net.Conn
	onData      func(dataLen int64)
	onClose     func()
	onCloseOnce sync.Once
}

type closeWriter interface {
	CloseWrite() error
}

func NewUsageTrackedConn(conn net.Conn, onData func(dataLen int64)) net.Conn {
	return NewUsageTrackedConnWithClose(conn, onData, nil)
}

func NewUsageTrackedConnWithClose(conn net.Conn, onData func(dataLen int64), onClose func()) net.Conn {
	return &usageTrackedConn{
		Conn:    conn,
		onData:  onData,
		onClose: onClose,
	}
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

func (c *usageTrackedConn) Close() error {
	err := c.Conn.Close()
	c.onCloseOnce.Do(func() {
		if c.onClose != nil {
			c.onClose()
		}
	})

	return err
}

func (c *usageTrackedConn) CloseWrite() error {
	writer, ok := c.Conn.(closeWriter)
	if !ok {
		return nil
	}

	return writer.CloseWrite()
}
