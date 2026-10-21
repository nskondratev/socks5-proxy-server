//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

const (
	redisUserAuthKey   = "user_auth"
	redisUserAdminKey  = "user_admin"
	redisUserUsageKey  = "user_usage_data"
	redisUserAuthDate  = "user_auth_date"
	redisUserStateKey  = "user_state"
	defaultWaitTimeout = 3 * time.Second
)

type fakeRedis struct {
	mu   sync.RWMutex
	data map[string]map[string]string
}

func newFakeRedis() *fakeRedis {
	return &fakeRedis{
		data: make(map[string]map[string]string),
	}
}

func (r *fakeRedis) HGet(_ context.Context, key, field string) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	fields, ok := r.data[key]
	if !ok {
		return "", goredis.Nil
	}

	value, ok := fields[field]
	if !ok {
		return "", goredis.Nil
	}

	return value, nil
}

func (r *fakeRedis) HGetAll(_ context.Context, key string) (map[string]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	fields, ok := r.data[key]
	if !ok {
		return map[string]string{}, nil
	}

	cp := make(map[string]string, len(fields))
	for k, v := range fields {
		cp[k] = v
	}

	return cp, nil
}

func (r *fakeRedis) HExists(_ context.Context, key, field string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	fields, ok := r.data[key]
	if !ok {
		return false, nil
	}

	_, exists := fields[field]

	return exists, nil
}

func (r *fakeRedis) HSet(_ context.Context, key string, values ...interface{}) error {
	if len(values)%2 != 0 {
		return fmt.Errorf("values count must be even")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	fields, ok := r.data[key]
	if !ok {
		fields = make(map[string]string)
		r.data[key] = fields
	}

	for i := 0; i < len(values); i += 2 {
		field, ok := values[i].(string)
		if !ok {
			return fmt.Errorf("field must be a string, got %T", values[i])
		}

		fields[field] = redisValueToString(values[i+1])
	}

	return nil
}

func redisValueToString(value interface{}) string {
	switch v := value.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	default:
		return fmt.Sprint(v)
	}
}

func (r *fakeRedis) HDel(_ context.Context, key string, fields ...string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	hash, ok := r.data[key]
	if !ok {
		return nil
	}

	for _, field := range fields {
		delete(hash, field)
	}

	return nil
}

func (r *fakeRedis) HIncrByBatch(ctx context.Context, key string, increments map[string]int64) error {
	for field, incr := range increments {
		if err := r.HIncrBy(ctx, key, field, incr); err != nil {
			return err
		}
	}

	return nil
}

func (r *fakeRedis) HIncrBy(_ context.Context, key, field string, incr int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	hash, ok := r.data[key]
	if !ok {
		hash = make(map[string]string)
		r.data[key] = hash
	}

	current := int64(0)
	if raw, ok := hash[field]; ok {
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return fmt.Errorf("failed to parse integer value for %s/%s: %w", key, field, err)
		}
		current = v
	}

	hash[field] = strconv.FormatInt(current+incr, 10)

	return nil
}

func (r *fakeRedis) Del(_ context.Context, keys ...string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, key := range keys {
		delete(r.data, key)
	}

	return nil
}

func mustHashPassword(t *testing.T, password string) string {
	t.Helper()

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}

	return string(hash)
}

func waitFor(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for {
		if condition() {
			return
		}

		if time.Now().After(deadline) {
			t.Fatal("timeout while waiting for condition")
		}

		time.Sleep(10 * time.Millisecond)
	}
}

func startTCPEchoServer(t *testing.T, response []byte) (addr string, received <-chan []byte, stop func()) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start target TCP server: %v", err)
	}

	receivedCh := make(chan []byte, 8)
	done := make(chan struct{})

	go func() {
		for {
			conn, acceptErr := ln.Accept()
			if acceptErr != nil {
				select {
				case <-done:
					return
				default:
				}

				continue
			}

			go func(c net.Conn) {
				defer c.Close()
				_ = c.SetDeadline(time.Now().Add(2 * time.Second))

				buf := make([]byte, 1024)
				n, readErr := c.Read(buf)
				if readErr != nil && !errors.Is(readErr, io.EOF) {
					return
				}

				if n > 0 {
					copyBuf := make([]byte, n)
					copy(copyBuf, buf[:n])
					receivedCh <- copyBuf
				}

				if len(response) > 0 {
					_, _ = c.Write(response)
				}
			}(conn)
		}
	}()

	stopFn := func() {
		close(done)
		_ = ln.Close()
	}

	return ln.Addr().String(), receivedCh, stopFn
}

func dialViaSocks5(proxyAddr, targetAddr, username, password string, timeout time.Duration) (net.Conn, error) {
	conn, err := net.DialTimeout("tcp", proxyAddr, timeout)
	if err != nil {
		return nil, err
	}

	if err = conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		_ = conn.Close()
		return nil, err
	}

	if err = socks5Greeting(conn, username, password); err != nil {
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

func socks5Greeting(conn net.Conn, username, password string) error {
	method := byte(0x00)
	methods := []byte{0x00}

	if username != "" {
		method = 0x02
		methods = []byte{0x00, 0x02}
	}

	req := []byte{0x05, byte(len(methods))}
	req = append(req, methods...)

	if _, err := conn.Write(req); err != nil {
		return err
	}

	resp := make([]byte, 2)
	if _, err := io.ReadFull(conn, resp); err != nil {
		return err
	}

	if resp[0] != 0x05 {
		return fmt.Errorf("unexpected SOCKS version: %d", resp[0])
	}

	if resp[1] == 0xFF {
		return errors.New("proxy rejected auth methods")
	}

	if resp[1] != method {
		return fmt.Errorf("unexpected auth method selected: %d", resp[1])
	}

	if method == 0x02 {
		return socks5UserPassAuth(conn, username, password)
	}

	return nil
}

func socks5UserPassAuth(conn net.Conn, username, password string) error {
	if len(username) > 255 || len(password) > 255 {
		return errors.New("username/password too long")
	}

	req := []byte{0x01, byte(len(username))}
	req = append(req, username...)
	req = append(req, byte(len(password)))
	req = append(req, password...)

	if _, err := conn.Write(req); err != nil {
		return err
	}

	resp := make([]byte, 2)
	if _, err := io.ReadFull(conn, resp); err != nil {
		return err
	}

	if resp[1] != 0x00 {
		return errors.New("username/password auth failed")
	}

	return nil
}

func socks5Connect(conn net.Conn, targetAddr string) error {
	host, rawPort, err := net.SplitHostPort(targetAddr)
	if err != nil {
		return fmt.Errorf("invalid target addr: %w", err)
	}

	port, err := strconv.Atoi(rawPort)
	if err != nil {
		return fmt.Errorf("invalid target port: %w", err)
	}

	if port <= 0 || port > 65535 {
		return fmt.Errorf("port out of range: %d", port)
	}

	addrType, addrPayload, err := encodeAddr(host)
	if err != nil {
		return err
	}

	req := []byte{0x05, 0x01, 0x00, addrType}
	req = append(req, addrPayload...)
	req = append(req, byte(port>>8), byte(port&0xFF))

	if _, err = conn.Write(req); err != nil {
		return err
	}

	header := make([]byte, 4)
	if _, err = io.ReadFull(conn, header); err != nil {
		return err
	}

	if header[0] != 0x05 {
		return fmt.Errorf("unexpected SOCKS version in reply: %d", header[0])
	}

	if header[1] != 0x00 {
		return fmt.Errorf("proxy connect failed with code 0x%x", header[1])
	}

	return consumeBindAddr(conn, header[3])
}

func encodeAddr(host string) (byte, []byte, error) {
	if ip := net.ParseIP(host); ip != nil {
		if ip4 := ip.To4(); ip4 != nil {
			return 0x01, ip4, nil
		}

		ip16 := ip.To16()
		if ip16 == nil {
			return 0, nil, fmt.Errorf("invalid IPv6 host: %s", host)
		}

		return 0x04, ip16, nil
	}

	if len(host) > 255 {
		return 0, nil, fmt.Errorf("domain too long: %s", host)
	}

	payload := []byte{byte(len(host))}
	payload = append(payload, host...)

	return 0x03, payload, nil
}

func consumeBindAddr(conn net.Conn, addrType byte) error {
	toRead := 0

	switch addrType {
	case 0x01:
		toRead = 4 + 2
	case 0x03:
		length := make([]byte, 1)
		if _, err := io.ReadFull(conn, length); err != nil {
			return err
		}
		toRead = int(length[0]) + 2
	case 0x04:
		toRead = 16 + 2
	default:
		return fmt.Errorf("unsupported bind addr type: 0x%x", addrType)
	}

	buf := make([]byte, toRead)
	_, err := io.ReadFull(conn, buf)

	return err
}
