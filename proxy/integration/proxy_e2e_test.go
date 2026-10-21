//go:build integration

package integration

import (
	"context"
	"errors"
	"io"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/things-go/go-socks5"

	"github.com/nskondratev/socks5-proxy-server/proxy/internal/cache"
	"github.com/nskondratev/socks5-proxy-server/proxy/internal/password"
	proxyadapter "github.com/nskondratev/socks5-proxy-server/proxy/internal/proxy/adapters"
	"github.com/nskondratev/socks5-proxy-server/proxy/internal/services/users"
)

func TestProxyE2E_NoAuth(t *testing.T) {
	redis := newFakeRedis()
	usersService := users.New(redis)
	updatesManager := users.NewUpdatesManager(usersService, 64, 20*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = updatesManager.Run(ctx)
	}()

	targetAddr, received, stopTarget := startTCPEchoServer(t, []byte("PONG"))
	defer stopTarget()

	proxyAddr, stopProxy := startProxyServer(t, usersService, updatesManager, false)
	defer stopProxy()

	conn, err := dialViaSocks5(proxyAddr, targetAddr, "", "", 2*time.Second)
	if err != nil {
		t.Fatalf("failed to connect through proxy without auth: %v", err)
	}
	defer conn.Close()

	if _, err = conn.Write([]byte("ping")); err != nil {
		t.Fatalf("failed to write via proxy: %v", err)
	}

	reply := make([]byte, 4)
	if _, err = io.ReadFull(conn, reply); err != nil {
		t.Fatalf("failed to read reply via proxy: %v", err)
	}

	if string(reply) != "PONG" {
		t.Fatalf("unexpected reply: %q", string(reply))
	}

	select {
	case got := <-received:
		if string(got) != "ping" {
			t.Fatalf("unexpected payload on target: %q", string(got))
		}
	case <-time.After(defaultWaitTimeout):
		t.Fatal("did not receive payload on target server")
	}
}

func TestProxyE2E_WithAuth(t *testing.T) {
	redis := newFakeRedis()
	usersService := users.New(redis)
	updatesManager := users.NewUpdatesManager(usersService, 64, 20*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = updatesManager.Run(ctx)
	}()

	username := "proxy-user"
	passwordValue := "secret-pass"
	if err := redis.HSet(context.Background(), redisUserAuthKey, username, mustHashPassword(t, passwordValue)); err != nil {
		t.Fatalf("failed to seed auth user: %v", err)
	}

	targetAddr, _, stopTarget := startTCPEchoServer(t, []byte("PONG"))
	defer stopTarget()

	proxyAddr, stopProxy := startProxyServer(t, usersService, updatesManager, true)
	defer stopProxy()

	if _, err := dialViaSocks5(proxyAddr, targetAddr, username, "wrong-pass", 2*time.Second); err == nil {
		t.Fatal("expected auth failure for invalid credentials")
	}

	conn, err := dialViaSocks5(proxyAddr, targetAddr, username, passwordValue, 2*time.Second)
	if err != nil {
		t.Fatalf("failed to connect through proxy with auth: %v", err)
	}
	defer conn.Close()

	if _, err = conn.Write([]byte("ping")); err != nil {
		t.Fatalf("failed to write via authenticated proxy: %v", err)
	}

	reply := make([]byte, 4)
	if _, err = io.ReadFull(conn, reply); err != nil {
		t.Fatalf("failed to read reply via authenticated proxy: %v", err)
	}

	if string(reply) != "PONG" {
		t.Fatalf("unexpected reply: %q", string(reply))
	}

	waitFor(t, defaultWaitTimeout, func() bool {
		rawUsage, usageErr := redis.HGet(context.Background(), redisUserUsageKey, username)
		if usageErr != nil {
			return false
		}

		usage, convErr := strconv.ParseInt(rawUsage, 10, 64)
		if convErr != nil {
			return false
		}

		return usage > 0
	})

	waitFor(t, defaultWaitTimeout, func() bool {
		authDate, err := redis.HGet(context.Background(), redisUserAuthDate, username)
		return err == nil && authDate != ""
	})
}

func startProxyServer(
	t *testing.T,
	usersService *users.Users,
	updatesManager *users.UpdatesManager,
	requireAuth bool,
) (addr string, stop func()) {
	t.Helper()

	dialer := &net.Dialer{}

	socks5Opts := []socks5.Option{
		socks5.WithDialAndRequest(func(ctx context.Context, network, addr string, request *socks5.Request) (net.Conn, error) {
			conn, err := dialer.DialContext(ctx, network, addr)
			if err != nil {
				return nil, err
			}

			username, ok := usernameFromRequest(request)
			if !ok {
				return conn, nil
			}

			return proxyadapter.NewUsageTrackedConn(conn, func(dataLen int64) {
				updatesManager.AddUsage(username, dataLen)
			}), nil
		}),
	}

	if requireAuth {
		authCredentialValidator := proxyadapter.NewAuthWithCache(
			cache.NewExpirableLRU[proxyadapter.AuthCacheKey, bool](128, time.Minute),
			proxyadapter.NewAuth(usersService, password.New()),
			func(user string) {
				_ = updatesManager.EnqueueLastAuthDateUpdate(user)
			},
		)

		socks5Opts = append(socks5Opts, socks5.WithCredential(authCredentialValidator))
	}

	server := socks5.NewServer(socks5Opts...)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen for proxy server: %v", err)
	}

	done := make(chan struct{})

	go func() {
		defer close(done)

		err = server.Serve(listener)
		if err != nil && !errors.Is(err, net.ErrClosed) {
			t.Errorf("proxy server exited with error: %v", err)
		}
	}()

	stopFn := func() {
		_ = listener.Close()

		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("timeout while stopping proxy server")
		}
	}

	return listener.Addr().String(), stopFn
}

func usernameFromRequest(request *socks5.Request) (string, bool) {
	if request == nil || request.AuthContext == nil || request.AuthContext.Payload == nil {
		return "", false
	}

	username := request.AuthContext.Payload["Username"]
	if username == "" {
		username = request.AuthContext.Payload["username"]
	}

	if username == "" {
		return "", false
	}

	return username, true
}
