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
