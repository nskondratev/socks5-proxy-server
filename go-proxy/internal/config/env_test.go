package config

import (
	"os"
	"testing"

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
