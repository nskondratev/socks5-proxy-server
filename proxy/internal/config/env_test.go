package config

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTelegramWebHookURL(t *testing.T) {
	t.Cleanup(func() {
		_ = os.Unsetenv("TELEGRAM_WEBHOOK_URL")
	})

	assert.NoError(t, os.Setenv("TELEGRAM_WEBHOOK_URL", "/new"))
	assert.Equal(t, "/new", TelegramWebHookURL())

	assert.NoError(t, os.Unsetenv("TELEGRAM_WEBHOOK_URL"))
	assert.Equal(t, "/webhook", TelegramWebHookURL())
}

func TestTelegramUseWebHooks(t *testing.T) {
	t.Cleanup(func() {
		_ = os.Unsetenv("TELEGRAM_USE_WEBHOOKS")
	})

	assert.NoError(t, os.Setenv("TELEGRAM_USE_WEBHOOKS", "true"))
	assert.True(t, TelegramUseWebHooks())

	assert.NoError(t, os.Setenv("TELEGRAM_USE_WEBHOOKS", "false"))
	assert.False(t, TelegramUseWebHooks())
}

func TestTelegramBotEnabled(t *testing.T) {
	t.Cleanup(func() {
		_ = os.Unsetenv("TELEGRAM_BOT_ENABLED")
	})

	assert.NoError(t, os.Setenv("TELEGRAM_BOT_ENABLED", "true"))
	assert.True(t, TelegramBotEnabled())

	assert.NoError(t, os.Setenv("TELEGRAM_BOT_ENABLED", "false"))
	assert.False(t, TelegramBotEnabled())

	assert.NoError(t, os.Unsetenv("TELEGRAM_BOT_ENABLED"))
	assert.True(t, TelegramBotEnabled())
}

func TestBotAppPort(t *testing.T) {
	t.Cleanup(func() {
		_ = os.Unsetenv("BOT_APP_PORT")
	})

	assert.NoError(t, os.Setenv("BOT_APP_PORT", "9443"))
	assert.Equal(t, 9443, BotAppPort())

	assert.NoError(t, os.Unsetenv("BOT_APP_PORT"))
	assert.Equal(t, 8443, BotAppPort())
}

func TestTelegramWebhookTLSCertPath(t *testing.T) {
	t.Cleanup(func() {
		_ = os.Unsetenv("TELEGRAM_WEBHOOK_TLS_CERT_PATH")
	})

	assert.NoError(t, os.Setenv("TELEGRAM_WEBHOOK_TLS_CERT_PATH", "ssl/crt.pem"))
	assert.Equal(t, "ssl/crt.pem", TelegramWebhookTLSCertPath())

	assert.NoError(t, os.Unsetenv("TELEGRAM_WEBHOOK_TLS_CERT_PATH"))
	assert.Equal(t, "", TelegramWebhookTLSCertPath())
}

func TestTelegramWebhookTLSKeyPath(t *testing.T) {
	t.Cleanup(func() {
		_ = os.Unsetenv("TELEGRAM_WEBHOOK_TLS_KEY_PATH")
	})

	assert.NoError(t, os.Setenv("TELEGRAM_WEBHOOK_TLS_KEY_PATH", "ssl/key.pem"))
	assert.Equal(t, "ssl/key.pem", TelegramWebhookTLSKeyPath())

	assert.NoError(t, os.Unsetenv("TELEGRAM_WEBHOOK_TLS_KEY_PATH"))
	assert.Equal(t, "", TelegramWebhookTLSKeyPath())
}

func TestOtherGetters(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		t.Setenv("REDIS_HOST", "")
		t.Setenv("REDIS_PORT", "")
		t.Setenv("REDIS_DB", "")
		t.Setenv("REQUIRE_AUTH", "")
		t.Setenv("AUTH_CACHE_MAX_SIZE", "")
		t.Setenv("AUTH_CACHE_TTL", "")
		t.Setenv("PROXY_HANDSHAKE_TIMEOUT", "")
		t.Setenv("PROXY_IDLE_TIMEOUT", "")
		t.Setenv("REDIS_AUTH_UPDATES_QUEUE_SIZE", "")
		t.Setenv("REDIS_USAGE_UPDATES_QUEUE_SIZE", "")
		t.Setenv("LOG_LEVEL", "")
		t.Setenv("TELEGRAM_BOT_ENABLED", "")
		t.Setenv("TELEGRAM_API_TOKEN", "")
		t.Setenv("TELEGRAM_UPDATE_PROCESSING_TIMEOUT", "")
		t.Setenv("PUBLIC_URL", "")
		t.Setenv("APP_PORT", "")
		t.Setenv("METRICS_ENABLED", "")
		t.Setenv("METRICS_PORT", "")
		t.Setenv("METRICS_AUTH_ENABLED", "")
		t.Setenv("METRICS_AUTH_USERNAME", "")
		t.Setenv("METRICS_AUTH_PASSWORD", "")
		t.Setenv("PPROF_ENABLED", "")
		t.Setenv("PPROF_PORT", "")
		t.Setenv("PPROF_AUTH_ENABLED", "")
		t.Setenv("PPROF_AUTH_USERNAME", "")
		t.Setenv("PPROF_AUTH_PASSWORD", "")

		assert.Equal(t, "", RedisHost())
		assert.Equal(t, 6379, RedisPort())
		assert.Equal(t, 0, RedisDB())
		assert.False(t, RequireAuth())
		assert.Equal(t, 100, AuthCacheMaxSize())
		assert.Equal(t, time.Hour, AuthCacheTTL())
		assert.Equal(t, 10*time.Second, ProxyHandshakeTimeout())
		assert.Equal(t, 15*time.Minute, ProxyIdleTimeout())
		assert.Equal(t, 4096, RedisAuthUpdatesQueueSize())
		assert.Equal(t, 16384, RedisUsageUpdatesQueueSize())
		assert.Equal(t, "warning", LogLevel())
		assert.True(t, TelegramBotEnabled())
		assert.Equal(t, "", TelegramAPIToken())
		assert.Equal(t, time.Minute, TelegramUpdateProcessingTimeout())
		assert.Equal(t, "", PublicURL())
		assert.Equal(t, 54321, AppPort())
		assert.False(t, MetricsEnabled())
		assert.Equal(t, 9100, MetricsPort())
		assert.False(t, MetricsAuthEnabled())
		assert.Equal(t, "", MetricsAuthUsername())
		assert.Equal(t, "", MetricsAuthPassword())
		assert.False(t, PprofEnabled())
		assert.Equal(t, 6060, PprofPort())
		assert.False(t, PprofAuthEnabled())
		assert.Equal(t, "", PprofAuthUsername())
		assert.Equal(t, "", PprofAuthPassword())
	})

	t.Run("env values", func(t *testing.T) {
		t.Setenv("REDIS_HOST", "redis")
		t.Setenv("REDIS_PORT", "6380")
		t.Setenv("REDIS_DB", "2")
		t.Setenv("REQUIRE_AUTH", "true")
		t.Setenv("AUTH_CACHE_MAX_SIZE", "50")
		t.Setenv("AUTH_CACHE_TTL", "30m")
		t.Setenv("PROXY_HANDSHAKE_TIMEOUT", "3s")
		t.Setenv("PROXY_IDLE_TIMEOUT", "20m")
		t.Setenv("REDIS_AUTH_UPDATES_QUEUE_SIZE", "100")
		t.Setenv("REDIS_USAGE_UPDATES_QUEUE_SIZE", "200")
		t.Setenv("LOG_LEVEL", "debug")
		t.Setenv("TELEGRAM_BOT_ENABLED", "false")
		t.Setenv("TELEGRAM_API_TOKEN", "token")
		t.Setenv("TELEGRAM_UPDATE_PROCESSING_TIMEOUT", "90s")
		t.Setenv("PUBLIC_URL", "https://example.com")
		t.Setenv("APP_PORT", "1080")
		t.Setenv("METRICS_ENABLED", "true")
		t.Setenv("METRICS_PORT", "9190")
		t.Setenv("METRICS_AUTH_ENABLED", "true")
		t.Setenv("METRICS_AUTH_USERNAME", "metrics")
		t.Setenv("METRICS_AUTH_PASSWORD", "secret")
		t.Setenv("PPROF_ENABLED", "true")
		t.Setenv("PPROF_PORT", "6061")
		t.Setenv("PPROF_AUTH_ENABLED", "true")
		t.Setenv("PPROF_AUTH_USERNAME", "debug")
		t.Setenv("PPROF_AUTH_PASSWORD", "pprof-secret")

		assert.Equal(t, "redis", RedisHost())
		assert.Equal(t, 6380, RedisPort())
		assert.Equal(t, 2, RedisDB())
		assert.True(t, RequireAuth())
		assert.Equal(t, 50, AuthCacheMaxSize())
		assert.Equal(t, 30*time.Minute, AuthCacheTTL())
		assert.Equal(t, 3*time.Second, ProxyHandshakeTimeout())
		assert.Equal(t, 20*time.Minute, ProxyIdleTimeout())
		assert.Equal(t, 100, RedisAuthUpdatesQueueSize())
		assert.Equal(t, 200, RedisUsageUpdatesQueueSize())
		assert.Equal(t, "debug", LogLevel())
		assert.False(t, TelegramBotEnabled())
		assert.Equal(t, "token", TelegramAPIToken())
		assert.Equal(t, 90*time.Second, TelegramUpdateProcessingTimeout())
		assert.Equal(t, "https://example.com", PublicURL())
		assert.Equal(t, 1080, AppPort())
		assert.True(t, MetricsEnabled())
		assert.Equal(t, 9190, MetricsPort())
		assert.True(t, MetricsAuthEnabled())
		assert.Equal(t, "metrics", MetricsAuthUsername())
		assert.Equal(t, "secret", MetricsAuthPassword())
		assert.True(t, PprofEnabled())
		assert.Equal(t, 6061, PprofPort())
		assert.True(t, PprofAuthEnabled())
		assert.Equal(t, "debug", PprofAuthUsername())
		assert.Equal(t, "pprof-secret", PprofAuthPassword())
	})
}
