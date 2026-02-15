package users

import (
	"context"
	"fmt"
	"time"
)

const userAuthKey = "user_auth"
const userAuthDateKey = "user_auth_date"

const jsISOStringLayout = "2006-01-02T15:04:05.000Z"

type redis interface {
	HGet(ctx context.Context, key, field string) (string, error)
	HSet(ctx context.Context, key string, values ...interface{}) error
}

type Users struct {
	redis redis
}

func New(redis redis) *Users {
	return &Users{redis: redis}
}

func (u *Users) GetPasswordHash(ctx context.Context, userName string) (string, error) {
	pswd, err := u.redis.HGet(ctx, userAuthKey, userName)
	if err != nil {
		return "", fmt.Errorf("[users] failed to get password: %w", err)
	}

	return pswd, nil
}

func (u *Users) UpdateLastAuthDate(ctx context.Context, userName string) error {
	if err := u.redis.HSet(ctx, userAuthDateKey, userName, time.Now().UTC().Format(jsISOStringLayout)); err != nil {
		return fmt.Errorf("[users] failed to update auth date: %w", err)
	}

	return nil
}
