//go:generate minimock -g -i github.com/nskondratev/socks5-proxy-server/proxy/internal/metrics.authValidator -o auth_validator_mock_test.go -n AuthValidatorMock -p metrics

package metrics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		m, err := New(Config{Enabled: false})
		require.NoError(t, err)
		require.NotNil(t, m)
		assert.False(t, m.Enabled())
	})

	t.Run("invalid port", func(t *testing.T) {
		_, err := New(Config{Enabled: true, Port: 0})
		require.Error(t, err)
		assert.ErrorContains(t, err, "invalid metrics port")
	})

	t.Run("auth username required", func(t *testing.T) {
		_, err := New(Config{
			Enabled:      true,
			Port:         9100,
			AuthEnabled:  true,
			AuthPassword: "secret",
		})
		require.Error(t, err)
		assert.ErrorContains(t, err, "metrics auth username is required")
	})

	t.Run("auth password required", func(t *testing.T) {
		_, err := New(Config{
			Enabled:      true,
			Port:         9100,
			AuthEnabled:  true,
			AuthUsername: "admin",
		})
		require.Error(t, err)
		assert.ErrorContains(t, err, "metrics auth password is required")
	})
}

func TestRuntimeAndCustomMetricsExposed(t *testing.T) {
	m, err := New(Config{
		Enabled: true,
		Port:    9100,
	})
	require.NoError(t, err)

	m.ObserveProxyTrafficBytes(42)
	m.ObserveProxyTrafficBytesByUsername("alice", 42)
	m.ObserveAuthAttempt(true)
	m.ObserveAuthAttempt(false)
	m.ObserveDroppedRedisUpdate(QueueUsage)
	m.ObserveConnectionOpened()
	m.ObserveConnectionOpened()
	m.ObserveConnectionClosed()

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/metrics", http.NoBody)
	rec := httptest.NewRecorder()
	m.server.Handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	assert.Contains(t, body, "go_gc_duration_seconds")
	assert.Contains(t, body, "proxy_transferred_bytes_total 42")
	assert.Contains(t, body, `proxy_transferred_bytes_by_username_total{username="alice"} 42`)
	assert.Contains(t, body, `proxy_auth_attempts_total{result="success"} 1`)
	assert.Contains(t, body, `proxy_auth_attempts_total{result="failure"} 1`)
	assert.Contains(t, body, `proxy_redis_updates_dropped_total{queue="usage"} 1`)
	assert.Contains(t, body, "proxy_connections_opened_total 2")
	assert.Contains(t, body, "proxy_connections_closed_total 1")
	assert.Contains(t, body, "proxy_connections_active 1")
}

func TestMetricsAuth(t *testing.T) {
	m, err := New(Config{
		Enabled:      true,
		Port:         9100,
		AuthEnabled:  true,
		AuthUsername: "metrics",
		AuthPassword: "secret",
	})
	require.NoError(t, err)

	t.Run("unauthorized without credentials", func(t *testing.T) {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/metrics", http.NoBody)
		rec := httptest.NewRecorder()

		m.server.Handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
		assert.Equal(t, `Basic realm="metrics"`, rec.Header().Get("WWW-Authenticate"))
	})

	t.Run("unauthorized with invalid credentials", func(t *testing.T) {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/metrics", http.NoBody)
		req.SetBasicAuth("metrics", "bad")
		rec := httptest.NewRecorder()

		m.server.Handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("authorized", func(t *testing.T) {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/metrics", http.NoBody)
		req.SetBasicAuth("metrics", "secret")
		rec := httptest.NewRecorder()

		m.server.Handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "go_gc_duration_seconds")
	})
}

func TestInstrumentAuthValidator(t *testing.T) {
	m, err := New(Config{
		Enabled: true,
		Port:    9100,
	})
	require.NoError(t, err)

	validator := NewAuthValidatorMock(t)
	validator.ValidMock.Expect("alice", "secret", "127.0.0.1").Return(true)

	instrumented := m.InstrumentAuthValidator(validator)
	require.NotNil(t, instrumented)
	assert.True(t, instrumented.Valid("alice", "secret", "127.0.0.1"))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/metrics", http.NoBody)
	rec := httptest.NewRecorder()
	m.server.Handler.ServeHTTP(rec, req)

	body := rec.Body.String()
	assert.Contains(t, body, `proxy_auth_attempts_total{result="success"} 1`)
}

func TestRunDisabled(t *testing.T) {
	m, err := New(Config{Enabled: false})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	require.NoError(t, m.Run(ctx))
}

func TestSecureCompare(t *testing.T) {
	assert.True(t, secureCompare("same", "same"))
	assert.False(t, secureCompare("same", "different"))
	assert.False(t, secureCompare(strings.Repeat("a", 5), strings.Repeat("a", 6)))
}
