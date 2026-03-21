package proxy

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/things-go/go-socks5/statute"
)

func TestMapDialErrorToReply(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want uint8
	}{
		{
			name: "connection refused",
			err:  errors.New("dial tcp 127.0.0.1:1: connect: connection refused"),
			want: statute.RepConnectionRefused,
		},
		{
			name: "network unreachable",
			err:  errors.New("dial tcp: network is unreachable"),
			want: statute.RepNetworkUnreachable,
		},
		{
			name: "host unreachable fallback",
			err:  errors.New("dial tcp: i/o timeout"),
			want: statute.RepHostUnreachable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := mapDialErrorToReply(tt.err); got != tt.want {
				t.Fatalf("mapDialErrorToReply() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestNormalizeProxyCopyError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		err     error
		wantNil bool
	}{
		{name: "nil", err: nil, wantNil: true},
		{name: "eof", err: io.EOF, wantNil: true},
		{name: "closed", err: net.ErrClosed, wantNil: true},
		{name: "timeout", err: &timeoutError{}, wantNil: true},
		{name: "closed text", err: errors.New("read tcp 127.0.0.1: use of closed network connection"), wantNil: true},
		{name: "unexpected", err: errors.New("boom"), wantNil: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := normalizeProxyCopyError(tt.err)
			if tt.wantNil && err != nil {
				t.Fatalf("normalizeProxyCopyError() = %v, want nil", err)
			}

			if !tt.wantNil && err == nil {
				t.Fatal("normalizeProxyCopyError() = nil, want error")
			}
		})
	}
}

func TestWrapListenerWithHandshakeTimeout(t *testing.T) {
	t.Parallel()

	base := &listenerStub{
		conn: &connStub{},
	}

	wrapped := WrapListenerWithHandshakeTimeout(base, time.Second)
	conn, err := wrapped.Accept()
	if err != nil {
		t.Fatalf("Accept() unexpected error: %v", err)
	}

	stub, ok := conn.(*connStub)
	if !ok {
		t.Fatalf("Accept() returned %T, want *connStub", conn)
	}

	if stub.deadline.IsZero() {
		t.Fatal("deadline was not set")
	}
}

func TestIdleDeadlineConnExtendsDeadline(t *testing.T) {
	t.Parallel()

	client, server := net.Pipe()
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
	})

	conn, err := newIdleDeadlineConn(client, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("newIdleDeadlineConn() unexpected error: %v", err)
	}

	go func() {
		_, _ = server.Write([]byte("ping"))
	}()

	buf := make([]byte, 4)
	if _, err := conn.Read(buf); err != nil {
		t.Fatalf("Read() unexpected error: %v", err)
	}

	if err := server.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("SetReadDeadline() unexpected error: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)

		buf := make([]byte, 4)
		_, _ = io.ReadFull(server, buf)
	}()

	if _, err := conn.Write([]byte("pong")); err != nil {
		t.Fatalf("Write() unexpected error: %v", err)
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("server side did not receive proxied bytes")
	}
}

func TestIdleDeadlineConnTracksReadAndWriteSeparately(t *testing.T) {
	t.Parallel()

	baseConn := &deadlineTrackingConnStub{}
	conn, err := newIdleDeadlineConn(baseConn, time.Second)
	if err != nil {
		t.Fatalf("newIdleDeadlineConn() unexpected error: %v", err)
	}

	initialReadDeadline := baseConn.readDeadline
	initialWriteDeadline := baseConn.writeDeadline

	buf := make([]byte, 1)
	if _, err := conn.Read(buf); err != nil {
		t.Fatalf("Read() unexpected error: %v", err)
	}

	if !baseConn.readDeadline.After(initialReadDeadline) {
		t.Fatal("read deadline was not extended")
	}

	if !baseConn.writeDeadline.Equal(initialWriteDeadline) {
		t.Fatal("write deadline changed on read")
	}

	readDeadlineAfterRead := baseConn.readDeadline

	if _, err := conn.Write([]byte("b")); err != nil {
		t.Fatalf("Write() unexpected error: %v", err)
	}

	if !baseConn.writeDeadline.After(initialWriteDeadline) {
		t.Fatal("write deadline was not extended")
	}

	if !baseConn.readDeadline.Equal(readDeadlineAfterRead) {
		t.Fatal("read deadline changed on write")
	}
}

func TestProxyIdleTimeoutClosesIdleConnection(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	listenCfg := net.ListenConfig{}

	targetListener, err := listenCfg.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start target listener: %v", err)
	}
	t.Cleanup(func() {
		_ = targetListener.Close()
	})

	targetDone := make(chan struct{})
	go func() {
		defer close(targetDone)

		conn, acceptErr := targetListener.Accept()
		if acceptErr != nil {
			return
		}
		defer func() {
			_ = conn.Close()
		}()

		<-time.After(time.Second)
	}()

	server, err := NewServer(NewServerParams{
		Config: Config{
			IdleTimeout: 100 * time.Millisecond,
		},
	})
	if err != nil {
		t.Fatalf("NewServer() unexpected error: %v", err)
	}

	proxyListener, err := listenCfg.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start proxy listener: %v", err)
	}
	proxyListener = WrapListenerWithHandshakeTimeout(proxyListener, time.Second)
	t.Cleanup(func() {
		_ = proxyListener.Close()
	})

	proxyDone := make(chan struct{})
	go func() {
		defer close(proxyDone)
		_ = server.Serve(proxyListener)
	}()

	conn, err := dialViaSocks5NoAuth(proxyListener.Addr().String(), targetListener.Addr().String(), time.Second)
	if err != nil {
		t.Fatalf("failed to connect through proxy: %v", err)
	}
	defer func() {
		_ = conn.Close()
	}()

	time.Sleep(350 * time.Millisecond)

	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("SetReadDeadline() unexpected error: %v", err)
	}

	buf := make([]byte, 1)
	_, err = conn.Read(buf)
	if err == nil {
		t.Fatal("Read() expected connection close after idle timeout")
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		t.Fatalf("Read() timed out instead of observing closed connection: %v", err)
	}

	_ = proxyListener.Close()
	<-proxyDone
	<-targetDone
}

func TestProxyHalfCloseAllowsReply(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	listenCfg := net.ListenConfig{}

	targetListener, err := listenCfg.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start target listener: %v", err)
	}
	t.Cleanup(func() {
		_ = targetListener.Close()
	})

	targetDone := make(chan struct{})
	go func() {
		defer close(targetDone)

		conn, acceptErr := targetListener.Accept()
		if acceptErr != nil {
			return
		}
		defer func() {
			_ = conn.Close()
		}()

		if err := conn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
			t.Errorf("SetDeadline() unexpected error: %v", err)
			return
		}

		payload, readErr := io.ReadAll(conn)
		if readErr != nil {
			t.Errorf("ReadAll() unexpected error: %v", readErr)
			return
		}

		if string(payload) != "ping" {
			t.Errorf("unexpected payload: %q", string(payload))
			return
		}

		if _, writeErr := conn.Write([]byte("pong")); writeErr != nil {
			t.Errorf("Write() unexpected error: %v", writeErr)
		}
	}()

	server, err := NewServer(NewServerParams{
		Config: Config{
			IdleTimeout: time.Second,
		},
		Metrics: noopMetricsObserver{},
	})
	if err != nil {
		t.Fatalf("NewServer() unexpected error: %v", err)
	}

	proxyListener, err := listenCfg.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start proxy listener: %v", err)
	}
	proxyListener = WrapListenerWithHandshakeTimeout(proxyListener, time.Second)
	t.Cleanup(func() {
		_ = proxyListener.Close()
	})

	proxyDone := make(chan struct{})
	go func() {
		defer close(proxyDone)
		_ = server.Serve(proxyListener)
	}()

	conn, err := dialViaSocks5NoAuth(proxyListener.Addr().String(), targetListener.Addr().String(), time.Second)
	if err != nil {
		t.Fatalf("failed to connect through proxy: %v", err)
	}
	defer func() {
		_ = conn.Close()
	}()

	if _, err := conn.Write([]byte("ping")); err != nil {
		t.Fatalf("Write() unexpected error: %v", err)
	}

	closeWriter, ok := conn.(interface{ CloseWrite() error })
	if !ok {
		t.Fatal("client connection does not implement CloseWrite")
	}

	if err := closeWriter.CloseWrite(); err != nil {
		t.Fatalf("CloseWrite() unexpected error: %v", err)
	}

	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline() unexpected error: %v", err)
	}

	reply := make([]byte, 4)
	if _, err := io.ReadFull(conn, reply); err != nil {
		t.Fatalf("ReadFull() unexpected error: %v", err)
	}

	if string(reply) != "pong" {
		t.Fatalf("unexpected reply: %q", string(reply))
	}

	_ = proxyListener.Close()
	<-proxyDone
	<-targetDone
}

func TestProxyAssociateClearsHandshakeDeadline(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	listenCfg := net.ListenConfig{}

	targetAddr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0}
	targetConn, err := net.ListenUDP("udp", targetAddr)
	if err != nil {
		t.Fatalf("failed to start udp target: %v", err)
	}
	t.Cleanup(func() {
		_ = targetConn.Close()
	})

	targetDone := make(chan struct{})
	go func() {
		defer close(targetDone)

		buf := make([]byte, 1024)
		n, remote, readErr := targetConn.ReadFromUDP(buf)
		if readErr != nil {
			return
		}

		if string(buf[:n]) != "ping" {
			t.Errorf("unexpected udp payload: %q", string(buf[:n]))
			return
		}

		if _, writeErr := targetConn.WriteToUDP([]byte("pong"), remote); writeErr != nil {
			t.Errorf("WriteToUDP() unexpected error: %v", writeErr)
		}
	}()

	clientAddr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0}
	clientConn, err := net.ListenUDP("udp", clientAddr)
	if err != nil {
		t.Fatalf("failed to start udp client: %v", err)
	}
	t.Cleanup(func() {
		_ = clientConn.Close()
	})

	server, err := NewServer(NewServerParams{})
	if err != nil {
		t.Fatalf("NewServer() unexpected error: %v", err)
	}

	proxyListener, err := listenCfg.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start proxy listener: %v", err)
	}
	proxyListener = WrapListenerWithHandshakeTimeout(proxyListener, 100*time.Millisecond)
	t.Cleanup(func() {
		_ = proxyListener.Close()
	})

	proxyDone := make(chan struct{})
	go func() {
		defer close(proxyDone)
		_ = server.Serve(proxyListener)
	}()

	controlConn, relayAddr, err := dialViaSocks5AssociateNoAuth(
		proxyListener.Addr().String(),
		clientConn.LocalAddr().(*net.UDPAddr),
		time.Second,
	)
	if err != nil {
		t.Fatalf("failed to establish udp associate: %v", err)
	}
	defer func() {
		_ = controlConn.Close()
	}()

	time.Sleep(250 * time.Millisecond)

	packet, err := statute.NewDatagram(targetConn.LocalAddr().String(), []byte("ping"))
	if err != nil {
		t.Fatalf("NewDatagram() unexpected error: %v", err)
	}

	if _, err := clientConn.WriteToUDP(packet.Bytes(), relayAddr); err != nil {
		t.Fatalf("WriteToUDP() unexpected error: %v", err)
	}

	if err := clientConn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("SetReadDeadline() unexpected error: %v", err)
	}

	buf := make([]byte, 1024)
	n, _, err := clientConn.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("ReadFromUDP() unexpected error: %v", err)
	}

	response, err := statute.ParseDatagram(buf[:n])
	if err != nil {
		t.Fatalf("ParseDatagram() unexpected error: %v", err)
	}

	if string(response.Data) != "pong" {
		t.Fatalf("unexpected udp response: %q", string(response.Data))
	}

	_ = proxyListener.Close()
	<-proxyDone
	<-targetDone
}

func dialViaSocks5NoAuth(proxyAddr, targetAddr string, timeout time.Duration) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	dialer := &net.Dialer{Timeout: timeout}

	conn, err := dialer.DialContext(ctx, "tcp", proxyAddr)
	if err != nil {
		return nil, err
	}

	if err = conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		_ = conn.Close()
		return nil, err
	}

	if err = socks5NoAuthGreeting(conn); err != nil {
		_ = conn.Close()
		return nil, err
	}

	if err = socks5Connect(conn, targetAddr); err != nil {
		_ = conn.Close()
		return nil, err
	}

	if err = conn.SetDeadline(time.Time{}); err != nil {
		_ = conn.Close()
		return nil, err
	}

	return conn, nil
}

func dialViaSocks5AssociateNoAuth(
	proxyAddr string,
	clientAddr *net.UDPAddr,
	timeout time.Duration,
) (net.Conn, *net.UDPAddr, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	dialer := &net.Dialer{Timeout: timeout}

	conn, err := dialer.DialContext(ctx, "tcp", proxyAddr)
	if err != nil {
		return nil, nil, err
	}

	if err = conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		_ = conn.Close()
		return nil, nil, err
	}

	if err = socks5NoAuthGreeting(conn); err != nil {
		_ = conn.Close()
		return nil, nil, err
	}

	relayAddr, err := socks5Associate(conn, clientAddr)
	if err != nil {
		_ = conn.Close()
		return nil, nil, err
	}

	if err = conn.SetDeadline(time.Time{}); err != nil {
		_ = conn.Close()
		return nil, nil, err
	}

	return conn, relayAddr, nil
}

func socks5NoAuthGreeting(conn net.Conn) error {
	if _, err := conn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		return err
	}

	resp := make([]byte, 2)
	if _, err := io.ReadFull(conn, resp); err != nil {
		return err
	}

	if resp[0] != 0x05 || resp[1] != 0x00 {
		return fmt.Errorf("unexpected greeting response: %v", resp)
	}

	return nil
}

func socks5Connect(conn net.Conn, targetAddr string) error {
	host, rawPort, err := net.SplitHostPort(targetAddr)
	if err != nil {
		return err
	}

	port, err := strconv.ParseUint(rawPort, 10, 16)
	if err != nil {
		return err
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("invalid target host: %s", host)
	}

	ip = ip.To4()
	if ip == nil {
		return errors.New("only ipv4 is supported in test helper")
	}

	req := []byte{0x05, 0x01, 0x00, 0x01}
	req = append(req, ip...)
	req = binary.BigEndian.AppendUint16(req, uint16(port))

	if _, err := conn.Write(req); err != nil {
		return err
	}

	resp := make([]byte, 10)
	if _, err := io.ReadFull(conn, resp); err != nil {
		return err
	}

	if resp[0] != 0x05 {
		return fmt.Errorf("unexpected SOCKS version: %d", resp[0])
	}

	if resp[1] != 0x00 {
		return fmt.Errorf("connect failed with status %d", resp[1])
	}

	return nil
}

func socks5Associate(conn net.Conn, clientAddr *net.UDPAddr) (*net.UDPAddr, error) {
	ip := clientAddr.IP.To4()
	if ip == nil {
		return nil, errors.New("only ipv4 is supported in test helper")
	}

	if clientAddr.Port < 0 || clientAddr.Port > 65535 {
		return nil, fmt.Errorf("invalid client udp port: %d", clientAddr.Port)
	}

	req := []byte{0x05, 0x03, 0x00, 0x01}
	req = append(req, ip...)
	req = binary.BigEndian.AppendUint16(req, uint16(clientAddr.Port))

	if _, err := conn.Write(req); err != nil {
		return nil, err
	}

	reply, err := statute.ParseReply(conn)
	if err != nil {
		return nil, err
	}

	if reply.Version != 0x05 {
		return nil, fmt.Errorf("unexpected SOCKS version: %d", reply.Version)
	}

	if reply.Response != 0x00 {
		return nil, fmt.Errorf("associate failed with status %d", reply.Response)
	}

	return &net.UDPAddr{IP: reply.BndAddr.IP, Port: reply.BndAddr.Port}, nil
}

type timeoutError struct{}

func (e *timeoutError) Error() string   { return "i/o timeout" }
func (e *timeoutError) Timeout() bool   { return true }
func (e *timeoutError) Temporary() bool { return true }

type noopMetricsObserver struct{}

func (noopMetricsObserver) ObserveConnectionOpened()                         {}
func (noopMetricsObserver) ObserveConnectionClosed()                         {}
func (noopMetricsObserver) ObserveAuthAttempt(bool)                          {}
func (noopMetricsObserver) ObserveProxyTrafficBytes(int64)                   {}
func (noopMetricsObserver) ObserveProxyTrafficBytesByUsername(string, int64) {}
func (noopMetricsObserver) ObserveDroppedRedisUpdate(string)                 {}

type listenerStub struct {
	conn net.Conn
}

func (l *listenerStub) Accept() (net.Conn, error) { return l.conn, nil }
func (l *listenerStub) Close() error              { return nil }
func (l *listenerStub) Addr() net.Addr            { return &net.TCPAddr{} }

type connStub struct {
	deadline time.Time
}

func (c *connStub) Read(_ []byte) (int, error)         { return 0, io.EOF }
func (c *connStub) Write(p []byte) (int, error)        { return len(p), nil }
func (c *connStub) Close() error                       { return nil }
func (c *connStub) LocalAddr() net.Addr                { return &net.TCPAddr{} }
func (c *connStub) RemoteAddr() net.Addr               { return &net.TCPAddr{} }
func (c *connStub) SetDeadline(t time.Time) error      { c.deadline = t; return nil }
func (c *connStub) SetReadDeadline(t time.Time) error  { c.deadline = t; return nil }
func (c *connStub) SetWriteDeadline(t time.Time) error { c.deadline = t; return nil }

type deadlineTrackingConnStub struct {
	readDeadline  time.Time
	writeDeadline time.Time
}

func (c *deadlineTrackingConnStub) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	p[0] = 'a'

	return 1, nil
}

func (c *deadlineTrackingConnStub) Write(p []byte) (int, error) {
	return len(p), nil
}

func (c *deadlineTrackingConnStub) Close() error         { return nil }
func (c *deadlineTrackingConnStub) LocalAddr() net.Addr  { return &net.TCPAddr{} }
func (c *deadlineTrackingConnStub) RemoteAddr() net.Addr { return &net.TCPAddr{} }
func (c *deadlineTrackingConnStub) SetDeadline(t time.Time) error {
	c.readDeadline, c.writeDeadline = t, t
	return nil
}
func (c *deadlineTrackingConnStub) SetReadDeadline(t time.Time) error { c.readDeadline = t; return nil }
func (c *deadlineTrackingConnStub) SetWriteDeadline(t time.Time) error {
	c.writeDeadline = t
	return nil
}
