package config

import (
	"time"

	"github.com/nskondratev/socks5-proxy-server/internal/env"
)

func RedisHost() string {
	return env.String("REDIS_HOST", "")
}

func RedisPort() int {
	return env.Int("REDIS_PORT", 6379)
}

func RedisDB() int {
	return env.Int("REDIS_DB", 0)
}

func AuthCacheMaxSize() int {
	return env.Int("AUTH_CACHE_MAX_SIZE", 100)
}

func AuthCacheTTL() time.Duration {
	return env.Duration("AUTH_CACHE_TTL", time.Hour)
}

func LogLevel() string {
	return env.String("LOG_LEVEL", "warning")
}

func TelegramAPIToken() string {
	return env.String("TELEGRAM_API_TOKEN", "")
}

func TelegramUpdateProcessingTimeout() time.Duration {
	return env.Duration("TELEGRAM_UPDATE_PROCESSING_TIMEOUT", time.Minute)
}

func PublicURL() string {
	return env.String("PUBLIC_URL", "")
}

func AppPort() int {
	return env.Int("APP_PORT", 54321)
}
