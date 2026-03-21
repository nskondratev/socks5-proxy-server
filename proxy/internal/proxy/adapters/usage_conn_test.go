package proxy

import (
	"io"
	"net"
	"testing"
	"time"
)

func TestUsageTrackedConn_Read(t *testing.T) {
	t.Parallel()

	client, server := net.Pipe()
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
	})

	const expected = int64(5)
	called := int64(0)
	conn := NewUsageTrackedConn(client, func(dataLen int64) {
		called += dataLen
	})

	go func() {
		_, _ = server.Write([]byte("hello"))
	}()

	buf := make([]byte, 5)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("Read() unexpected error: %v", err)
	}

	if n != int(expected) {
		t.Fatalf("Read() bytes = %d, want %d", n, expected)
	}

	if called != expected {
		t.Fatalf("onData called with %d bytes, want %d", called, expected)
	}
}

func TestUsageTrackedConn_Write(t *testing.T) {
	t.Parallel()

	client, server := net.Pipe()
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
	})

	const expected = int64(4)
	called := int64(0)
	conn := NewUsageTrackedConn(client, func(dataLen int64) {
		called += dataLen
	})

	go func() {
		_ = server.SetReadDeadline(time.Now().Add(time.Second))
		_, _ = io.ReadFull(server, make([]byte, expected))
	}()

	n, err := conn.Write([]byte("ping"))
	if err != nil {
		t.Fatalf("Write() unexpected error: %v", err)
	}

	if n != int(expected) {
		t.Fatalf("Write() bytes = %d, want %d", n, expected)
	}

	if called != expected {
		t.Fatalf("onData called with %d bytes, want %d", called, expected)
	}
}

func TestUsageTrackedConn_Close(t *testing.T) {
	t.Parallel()

	client, server := net.Pipe()
	t.Cleanup(func() {
		_ = server.Close()
	})

	closedCalled := 0
	conn := NewUsageTrackedConnWithClose(client, nil, func() {
		closedCalled++
	})

	if err := conn.Close(); err != nil {
		t.Fatalf("Close() unexpected error: %v", err)
	}

	_ = conn.Close()

	if closedCalled != 1 {
		t.Fatalf("onClose called %d times, want 1", closedCalled)
	}
}

func TestUsageTrackedConn_CloseWrite(t *testing.T) {
	t.Parallel()

	baseConn := &closeWriteConnStub{}
	conn := NewUsageTrackedConn(baseConn, nil)

	writer, ok := conn.(interface{ CloseWrite() error })
	if !ok {
		t.Fatal("connection does not implement CloseWrite")
	}

	if err := writer.CloseWrite(); err != nil {
		t.Fatalf("CloseWrite() unexpected error: %v", err)
	}

	if !baseConn.closeWriteCalled {
		t.Fatal("underlying CloseWrite() was not called")
	}
}

type closeWriteConnStub struct {
	net.Conn
	closeWriteCalled bool
}

func (c *closeWriteConnStub) CloseWrite() error {
	c.closeWriteCalled = true
	return nil
}
