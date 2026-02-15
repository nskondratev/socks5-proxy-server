package redis

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

type Redis struct {
	cli *redis.Client
}

func New(host string, port, dbNum int) *Redis {
	cli := redis.NewClient(&redis.Options{
		Addr: fmt.Sprintf("%s:%d", host, port),
		DB:   dbNum,
	})

	return &Redis{cli: cli}
}

func (r *Redis) Close() error {
	return r.cli.Close()
}

func (r *Redis) HGet(ctx context.Context, key, field string) (string, error) {
	return r.cli.HGet(ctx, key, field).Result()
}

func (r *Redis) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	return r.cli.HGetAll(ctx, key).Result()
}
