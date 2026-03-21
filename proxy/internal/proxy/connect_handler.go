package proxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/things-go/go-socks5"
	"github.com/things-go/go-socks5/statute"
)

type connectHandler struct {
	dial        func(ctx context.Context, network, addr string, request *socks5.Request) (net.Conn, error)
	idleTimeout time.Duration
}

type proxyStreams struct {
	clientConn   net.Conn
	clientWriter io.Writer
	clientReader io.Reader
	target       net.Conn
}

func newConnectHandler(
	params NewServerParams,
) func(ctx context.Context, writer io.Writer, request *socks5.Request) error {
	handler := &connectHandler{
		dial:        newDialAndRequest(params),
		idleTimeout: params.Config.IdleTimeout,
	}

	return handler.Handle
}

func (h *connectHandler) Handle(ctx context.Context, writer io.Writer, request *socks5.Request) error {
	target, err := h.dial(ctx, "tcp", request.DestAddr.String(), request)
	if err != nil {
		return replyDialError(writer, request, err)
	}

	defer func() {
		_ = target.Close()
	}()

	streams, err := h.prepareProxyStreams(writer, request.Reader, target)
	if err != nil {
		return err
	}

	if err := socks5.SendReply(streams.clientWriter, statute.RepSuccess, streams.target.LocalAddr()); err != nil {
		return fmt.Errorf("failed to send reply: %w", err)
	}

	return proxyBidirectional(streams.clientConn, streams.clientWriter, streams.clientReader, streams.target)
}

func replyDialError(writer io.Writer, request *socks5.Request, err error) error {
	if replyErr := socks5.SendReply(writer, mapDialErrorToReply(err), nil); replyErr != nil {
		return fmt.Errorf("failed to send reply: %w", replyErr)
	}

	return fmt.Errorf("connect to %v failed: %w", request.RawDestAddr, err)
}

func (h *connectHandler) prepareProxyStreams(
	writer io.Writer,
	reader io.Reader,
	target net.Conn,
) (proxyStreams, error) {
	streams := proxyStreams{
		clientWriter: writer,
		clientReader: reader,
		target:       target,
	}

	clientConn, ok := writer.(net.Conn)
	streams.clientConn = clientConn

	if h.idleTimeout <= 0 {
		if clientConn == nil {
			return streams, nil
		}

		if err := clientConn.SetDeadline(time.Time{}); err != nil {
			return proxyStreams{}, fmt.Errorf("failed to clear client handshake deadline: %w", err)
		}

		return streams, nil
	}

	if !ok {
		return proxyStreams{}, errors.New("connect writer does not implement net.Conn")
	}

	clientDeadline := newIdleDeadlineController(clientConn, h.idleTimeout)
	if err := clientDeadline.Extend(); err != nil {
		return proxyStreams{}, fmt.Errorf("failed to set client idle deadline: %w", err)
	}

	streams.clientReader = &idleDeadlineReader{
		reader: reader,
		touch:  clientDeadline.Extend,
	}
	streams.clientWriter = &idleDeadlineWriter{
		writer: writer,
		touch:  clientDeadline.Extend,
	}

	idleTarget, err := newIdleDeadlineConn(target, h.idleTimeout)
	if err != nil {
		return proxyStreams{}, fmt.Errorf("failed to set upstream idle deadline: %w", err)
	}

	streams.target = idleTarget

	return streams, nil
}

func mapDialErrorToReply(err error) uint8 {
	msg := err.Error()

	switch {
	case strings.Contains(msg, "refused"):
		return statute.RepConnectionRefused
	case strings.Contains(msg, "network is unreachable"):
		return statute.RepNetworkUnreachable
	default:
		return statute.RepHostUnreachable
	}
}

func proxyBidirectional(clientConn net.Conn, clientWriter io.Writer, clientReader io.Reader, target net.Conn) error {
	errCh := make(chan error, 2)

	go func() {
		errCh <- proxyCopy(target, clientReader)
	}()
	go func() {
		errCh <- proxyCopy(clientWriter, target)
	}()

	firstErr := <-errCh

	if clientConn != nil {
		_ = clientConn.Close()
	}

	_ = target.Close()

	secondErr := <-errCh

	if err := normalizeProxyCopyError(firstErr); err != nil {
		return err
	}

	return normalizeProxyCopyError(secondErr)
}

func proxyCopy(dst io.Writer, src io.Reader) error {
	_, err := io.Copy(dst, src)
	if tcpConn, ok := dst.(interface{ CloseWrite() error }); ok {
		_ = tcpConn.CloseWrite()
	}

	return err
}

func normalizeProxyCopyError(err error) error {
	if err == nil || errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
		return nil
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return nil
	}

	if strings.Contains(err.Error(), "use of closed network connection") {
		return nil
	}

	return err
}

func WrapListenerWithHandshakeTimeout(listener net.Listener, timeout time.Duration) net.Listener {
	if listener == nil || timeout <= 0 {
		return listener
	}

	return &handshakeTimeoutListener{
		Listener: listener,
		timeout:  timeout,
	}
}

type handshakeTimeoutListener struct {
	net.Listener
	timeout time.Duration
}

func (l *handshakeTimeoutListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}

	if err := conn.SetDeadline(time.Now().Add(l.timeout)); err != nil {
		_ = conn.Close()

		return nil, err
	}

	return conn, nil
}

type idleDeadlineController struct {
	conn    net.Conn
	timeout time.Duration
	mu      sync.Mutex
}

func newIdleDeadlineController(conn net.Conn, timeout time.Duration) *idleDeadlineController {
	return &idleDeadlineController{
		conn:    conn,
		timeout: timeout,
	}
}

func (c *idleDeadlineController) Extend() error {
	if c == nil || c.conn == nil || c.timeout <= 0 {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	return c.conn.SetDeadline(time.Now().Add(c.timeout))
}

func newIdleDeadlineConn(conn net.Conn, timeout time.Duration) (net.Conn, error) {
	controller := newIdleDeadlineController(conn, timeout)
	if err := controller.Extend(); err != nil {
		return nil, err
	}

	return &idleDeadlineConn{
		Conn:       conn,
		controller: controller,
	}, nil
}

type idleDeadlineConn struct {
	net.Conn
	controller *idleDeadlineController
}

func (c *idleDeadlineConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if n > 0 {
		if touchErr := c.controller.Extend(); touchErr != nil && err == nil {
			err = touchErr
		}
	}

	return n, err
}

func (c *idleDeadlineConn) Write(p []byte) (int, error) {
	n, err := c.Conn.Write(p)
	if n > 0 {
		if touchErr := c.controller.Extend(); touchErr != nil && err == nil {
			err = touchErr
		}
	}

	return n, err
}

type idleDeadlineReader struct {
	reader io.Reader
	touch  func() error
}

func (r *idleDeadlineReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n > 0 && r.touch != nil {
		if touchErr := r.touch(); touchErr != nil && err == nil {
			err = touchErr
		}
	}

	return n, err
}

type idleDeadlineWriter struct {
	writer io.Writer
	touch  func() error
}

func (w *idleDeadlineWriter) Write(p []byte) (int, error) {
	n, err := w.writer.Write(p)
	if n > 0 && w.touch != nil {
		if touchErr := w.touch(); touchErr != nil && err == nil {
			err = touchErr
		}
	}

	return n, err
}
