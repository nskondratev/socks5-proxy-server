package users

import (
	"context"
	"log/slog"

	"github.com/nskondratev/socks5-proxy-server/internal/log"
)

type updater interface {
	UpdateLastAuthDate(ctx context.Context, userName string) error
	IncreaseDataUsage(ctx context.Context, userName string, dataLen int64) error
}

type authUpdateJob struct {
	user string
}

type usageUpdateJob struct {
	user    string
	dataLen int64
}

type UpdatesManager struct {
	updater updater

	authUpdates  chan authUpdateJob
	usageUpdates chan usageUpdateJob
}

func NewUpdatesManager(updater updater, authQueueSize, usageQueueSize int) *UpdatesManager {
	if authQueueSize < 1 {
		authQueueSize = 1
	}
	if usageQueueSize < 1 {
		usageQueueSize = 1
	}

	return &UpdatesManager{
		updater:      updater,
		authUpdates:  make(chan authUpdateJob, authQueueSize),
		usageUpdates: make(chan usageUpdateJob, usageQueueSize),
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

func (m *UpdatesManager) EnqueueUsageUpdate(user string, dataLen int64) bool {
	select {
	case m.usageUpdates <- usageUpdateJob{user: user, dataLen: dataLen}:
		return true
	default:
		return false
	}
}

func (m *UpdatesManager) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case job := <-m.authUpdates:
			if err := m.updater.UpdateLastAuthDate(ctx, job.user); err != nil {
				slog.LogAttrs(
					ctx,
					slog.LevelWarn,
					"failed to update auth date for user",
					slog.String(log.FieldUsername, job.user),
					slog.String(log.FieldError, err.Error()),
				)
			}
		case job := <-m.usageUpdates:
			if err := m.updater.IncreaseDataUsage(ctx, job.user, job.dataLen); err != nil {
				slog.LogAttrs(
					ctx,
					slog.LevelWarn,
					"failed to increase data usage for user",
					slog.String(log.FieldUsername, job.user),
					slog.String(log.FieldError, err.Error()),
				)
			}
		}
	}
}
