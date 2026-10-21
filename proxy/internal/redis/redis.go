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

func (r *Redis) HExists(ctx context.Context, key, field string) (bool, error) {
	return r.cli.HExists(ctx, key, field).Result()
}

func (r *Redis) HSet(ctx context.Context, key string, values ...interface{}) error {
	return r.cli.HSet(ctx, key, values...).Err()
}

func (r *Redis) HDel(ctx context.Context, key string, fields ...string) error {
	return r.cli.HDel(ctx, key, fields...).Err()
}

func (r *Redis) HIncrBy(ctx context.Context, key, field string, incr int64) error {
	return r.cli.HIncrBy(ctx, key, field, incr).Err()
}

// HIncrByBatch increments several hash fields in a single round trip using a pipeline.
func (r *Redis) HIncrByBatch(ctx context.Context, key string, increments map[string]int64) error {
	if len(increments) == 0 {
		return nil
	}

	_, err := r.cli.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		for field, incr := range increments {
			pipe.HIncrBy(ctx, key, field, incr)
		}

		return nil
	})

	return err
}

func (r *Redis) Del(ctx context.Context, keys ...string) error {
	return r.cli.Del(ctx, keys...).Err()
}
