package pprofserver

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
		s, err := New(Config{Enabled: false})
		require.NoError(t, err)
		require.NotNil(t, s)
		assert.False(t, s.Enabled())
	})

	t.Run("invalid port", func(t *testing.T) {
		_, err := New(Config{Enabled: true, Port: 0})
		require.Error(t, err)
		assert.ErrorContains(t, err, "invalid pprof port")
	})

	t.Run("auth username required", func(t *testing.T) {
		_, err := New(Config{
			Enabled:      true,
			Port:         6060,
			AuthEnabled:  true,
			AuthPassword: "secret",
		})
		require.Error(t, err)
		assert.ErrorContains(t, err, "pprof auth username is required")
	})

	t.Run("auth password required", func(t *testing.T) {
		_, err := New(Config{
			Enabled:      true,
			Port:         6060,
			AuthEnabled:  true,
			AuthUsername: "debug",
		})
		require.Error(t, err)
		assert.ErrorContains(t, err, "pprof auth password is required")
	})
}

func TestPprofExposed(t *testing.T) {
	s, err := New(Config{
		Enabled: true,
		Port:    6060,
	})
	require.NoError(t, err)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/debug/pprof/", http.NoBody)
	rec := httptest.NewRecorder()
	s.server.Handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "goroutine?debug=1")
}

func TestPprofAuth(t *testing.T) {
	s, err := New(Config{
		Enabled:      true,
		Port:         6060,
		AuthEnabled:  true,
		AuthUsername: "debug",
		AuthPassword: "secret",
	})
	require.NoError(t, err)

	t.Run("unauthorized without credentials", func(t *testing.T) {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/debug/pprof/", http.NoBody)
		rec := httptest.NewRecorder()

		s.server.Handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
		assert.Equal(t, `Basic realm="pprof"`, rec.Header().Get("WWW-Authenticate"))
	})

	t.Run("unauthorized with invalid credentials", func(t *testing.T) {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/debug/pprof/", http.NoBody)
		req.SetBasicAuth("debug", "bad")
		rec := httptest.NewRecorder()

		s.server.Handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("authorized", func(t *testing.T) {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/debug/pprof/", http.NoBody)
		req.SetBasicAuth("debug", "secret")
		rec := httptest.NewRecorder()

		s.server.Handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "heap?debug=1")
	})
}

func TestRunDisabled(t *testing.T) {
	s, err := New(Config{Enabled: false})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	require.NoError(t, s.Run(ctx))
}

func TestSecureCompare(t *testing.T) {
	assert.True(t, secureCompare("same", "same"))
	assert.False(t, secureCompare("same", "different"))
	assert.False(t, secureCompare(strings.Repeat("a", 5), strings.Repeat("a", 6)))
}
