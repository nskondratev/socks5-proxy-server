package users

import (
	"context"
	"sync"
	"time"

	"github.com/nskondratev/socks5-proxy-server/proxy/internal/log"
)

const (
	defaultUsageFlushInterval = time.Second
	finalUsageFlushTimeout    = 5 * time.Second
)

type updater interface {
	UpdateLastAuthDate(ctx context.Context, userName string) error
	IncreaseDataUsageBatch(ctx context.Context, usage map[string]int64) error
}

type authUpdateJob struct {
	user string
}

type UpdatesManager struct {
	updater updater

	authUpdates chan authUpdateJob

	// Data usage is aggregated per user in memory and periodically flushed to Redis in one batch,
	// so proxied traffic never waits for Redis and no updates are dropped under load.
	usageFlushInterval time.Duration
	usageMu            sync.Mutex
	pendingUsage       map[string]int64
}

func NewUpdatesManager(updater updater, authQueueSize int, usageFlushInterval time.Duration) *UpdatesManager {
	if authQueueSize < 1 {
		authQueueSize = 1
	}

	if usageFlushInterval <= 0 {
		usageFlushInterval = defaultUsageFlushInterval
	}

	return &UpdatesManager{
		updater:            updater,
		authUpdates:        make(chan authUpdateJob, authQueueSize),
		usageFlushInterval: usageFlushInterval,
		pendingUsage:       make(map[string]int64),
	}
}

func (m *UpdatesManager) EnqueueLastAuthDateUpdate(user string) bool {
	select {
	case m.authUpdates <- authUpdateJob{user: user}:
		return true
	default:
		return false
	}
}

// AddUsage accumulates proxied bytes for user until the next flush to Redis.
func (m *UpdatesManager) AddUsage(user string, dataLen int64) {
	if dataLen <= 0 {
		return
	}

	m.usageMu.Lock()
	m.pendingUsage[user] += dataLen
	m.usageMu.Unlock()
}

func (m *UpdatesManager) Run(ctx context.Context) error {
	ticker := time.NewTicker(m.usageFlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Flush what was accumulated since the last tick, parent context is already canceled.
			flushCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), finalUsageFlushTimeout)
			m.flushUsage(flushCtx)
			cancel()

			return nil
		case job := <-m.authUpdates:
			if err := m.updater.UpdateLastAuthDate(ctx, job.user); err != nil {
				log.Warn(
					ctx,
					"failed to update auth date for user",
					log.String(log.FieldUsername, job.user),
					log.String(log.FieldError, err.Error()),
				)
			}
		case <-ticker.C:
			m.flushUsage(ctx)
		}
	}
}

func (m *UpdatesManager) flushUsage(ctx context.Context) {
	usage := m.takePendingUsage()
	if len(usage) == 0 {
		return
	}

	if err := m.updater.IncreaseDataUsageBatch(ctx, usage); err != nil {
		// Keep usage in memory and retry on the next flush.
		m.restorePendingUsage(usage)

		log.Warn(
			ctx,
			"failed to flush data usage, will retry",
			log.Int("users", len(usage)),
			log.String(log.FieldError, err.Error()),
		)
	}
}

func (m *UpdatesManager) takePendingUsage() map[string]int64 {
	m.usageMu.Lock()
	defer m.usageMu.Unlock()

	if len(m.pendingUsage) == 0 {
		return nil
	}

	usage := m.pendingUsage
	m.pendingUsage = make(map[string]int64, len(usage))

	return usage
}

func (m *UpdatesManager) restorePendingUsage(usage map[string]int64) {
	m.usageMu.Lock()
	defer m.usageMu.Unlock()

	for user, dataLen := range usage {
		m.pendingUsage[user] += dataLen
	}
}
