package admin

import (
	"context"
	"fmt"
)

const userAdminKey = "user_admin"

type redis interface {
	HSet(ctx context.Context, key string, values ...interface{}) error
	HDel(ctx context.Context, key string, fields ...string) error
	HExists(ctx context.Context, key, field string) (bool, error)
}

type Service struct {
	redis redis
}

func New(redis redis) *Service {
	return &Service{redis: redis}
}

func (s *Service) Add(ctx context.Context, username string) error {
	if err := s.redis.HSet(ctx, userAdminKey, username, 1); err != nil {
		return fmt.Errorf("[admin] failed to add admin: %w", err)
	}

	return nil
}

func (s *Service) Remove(ctx context.Context, username string) error {
	if err := s.redis.HDel(ctx, userAdminKey, username); err != nil {
		return fmt.Errorf("[admin] failed to remove admin: %w", err)
	}

	return nil
}

func (s *Service) IsAdmin(ctx context.Context, username string) (bool, error) {
	exists, err := s.redis.HExists(ctx, userAdminKey, username)
	if err != nil {
		return false, fmt.Errorf("[admin] failed to check admin: %w", err)
	}

	return exists, nil
}
