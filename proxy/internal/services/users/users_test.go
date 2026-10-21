package users

import (
	"context"
	"errors"
	"testing"
	"time"
)

type redisStub struct {
	hgetFn         func(ctx context.Context, key, field string) (string, error)
	hgetAllFn      func(ctx context.Context, key string) (map[string]string, error)
	hsetFn         func(ctx context.Context, key string, values ...interface{}) error
	hdelFn         func(ctx context.Context, key string, fields ...string) error
	hincrByBatchFn func(ctx context.Context, key string, increments map[string]int64) error
	delFn          func(ctx context.Context, keys ...string) error
}

func (r *redisStub) HGet(ctx context.Context, key, field string) (string, error) {
	return r.hgetFn(ctx, key, field)
}

func (r *redisStub) HSet(ctx context.Context, key string, values ...interface{}) error {
	return r.hsetFn(ctx, key, values...)
}

func (r *redisStub) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	if r.hgetAllFn == nil {
		return map[string]string{}, nil
	}

	return r.hgetAllFn(ctx, key)
}

func (r *redisStub) HDel(ctx context.Context, key string, fields ...string) error {
	if r.hdelFn == nil {
		return nil
	}

	return r.hdelFn(ctx, key, fields...)
}

func (r *redisStub) HIncrByBatch(ctx context.Context, key string, increments map[string]int64) error {
	return r.hincrByBatchFn(ctx, key, increments)
}

func (r *redisStub) Del(ctx context.Context, keys ...string) error {
	return r.delFn(ctx, keys...)
}

func TestGetPasswordHash(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) *redisStub
		want    string
		wantErr bool
	}{
		{
			name: "success",
			setup: func(t *testing.T) *redisStub {
				t.Helper()
				return &redisStub{hgetFn: func(_ context.Context, key, field string) (string, error) {
					if key != userAuthKey || field != "alice" {
						t.Fatalf("unexpected HGet args: key=%q field=%q", key, field)
					}
					return "hashed", nil
				}}
			},
			want: "hashed",
		},
		{
			name: "redis error",
			setup: func(t *testing.T) *redisStub {
				t.Helper()
				return &redisStub{hgetFn: func(_ context.Context, _, _ string) (string, error) {
					return "", errors.New("redis down")
				}}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			u := New(tt.setup(t))

			got, err := u.GetPasswordHash(context.Background(), "alice")
			if (err != nil) != tt.wantErr {
				t.Fatalf("GetPasswordHash() error = %v, wantErr %v", err, tt.wantErr)
			}

			if got != tt.want {
				t.Fatalf("GetPasswordHash() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestUpdateLastAuthDate(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		var (
			gotKey    string
			gotValues []interface{}
		)
		u := New(&redisStub{hsetFn: func(_ context.Context, key string, values ...interface{}) error {
			gotKey = key
			gotValues = values
			return nil
		}})

		err := u.UpdateLastAuthDate(context.Background(), "alice")
		if err != nil {
			t.Fatalf("UpdateLastAuthDate() unexpected error: %v", err)
		}

		if gotKey != userAuthDateKey {
			t.Fatalf("HSet key = %q, want %q", gotKey, userAuthDateKey)
		}

		if len(gotValues) != 2 {
			t.Fatalf("HSet values count = %d, want 2", len(gotValues))
		}

		if gotValues[0] != "alice" {
			t.Fatalf("HSet field = %v, want alice", gotValues[0])
		}

		isoDate, ok := gotValues[1].(string)
		if !ok {
			t.Fatalf("HSet value type = %T, want string", gotValues[1])
		}

		parsed, parseErr := time.Parse(jsISOStringLayout, isoDate)
		if parseErr != nil {
			t.Fatalf("date %q is not compatible with JS ISO format: %v", isoDate, parseErr)
		}

		if parsed.Location() != time.UTC {
			t.Fatalf("date location = %v, want UTC", parsed.Location())
		}
	})

	t.Run("redis error", func(t *testing.T) {
		t.Parallel()

		u := New(&redisStub{hsetFn: func(_ context.Context, _ string, _ ...interface{}) error {
			return errors.New("write failed")
		}})

		err := u.UpdateLastAuthDate(context.Background(), "alice")
		if err == nil {
			t.Fatal("UpdateLastAuthDate() expected error, got nil")
		}
	})
}

func TestIncreaseDataUsageBatch(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		var (
			gotKey        string
			gotIncrements map[string]int64
		)

		u := New(&redisStub{hincrByBatchFn: func(_ context.Context, key string, increments map[string]int64) error {
			gotKey = key
			gotIncrements = increments
			return nil
		}})

		err := u.IncreaseDataUsageBatch(context.Background(), map[string]int64{"alice": 512, "bob": 64})
		if err != nil {
			t.Fatalf("IncreaseDataUsageBatch() unexpected error: %v", err)
		}

		if gotKey != userUsageDataKey {
			t.Fatalf("HIncrByBatch key = %q, want %q", gotKey, userUsageDataKey)
		}

		if len(gotIncrements) != 2 || gotIncrements["alice"] != 512 || gotIncrements["bob"] != 64 {
			t.Fatalf("HIncrByBatch increments = %v, want alice=512 bob=64", gotIncrements)
		}
	})

	t.Run("redis error", func(t *testing.T) {
		t.Parallel()

		u := New(&redisStub{hincrByBatchFn: func(_ context.Context, _ string, _ map[string]int64) error {
			return errors.New("write failed")
		}})

		err := u.IncreaseDataUsageBatch(context.Background(), map[string]int64{"alice": 512})
		if err == nil {
			t.Fatal("IncreaseDataUsageBatch() expected error, got nil")
		}
	})
}

func TestClearDataUsage(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		var gotKeys []string
		u := New(&redisStub{delFn: func(_ context.Context, keys ...string) error {
			gotKeys = keys
			return nil
		}})

		err := u.ClearDataUsage(context.Background())
		if err != nil {
			t.Fatalf("ClearDataUsage() unexpected error: %v", err)
		}

		if len(gotKeys) != 1 {
			t.Fatalf("Del keys count = %d, want 1", len(gotKeys))
		}

		if gotKeys[0] != userUsageDataKey {
			t.Fatalf("Del key = %q, want %q", gotKeys[0], userUsageDataKey)
		}
	})

	t.Run("redis error", func(t *testing.T) {
		t.Parallel()

		u := New(&redisStub{delFn: func(_ context.Context, _ ...string) error {
			return errors.New("delete failed")
		}})

		err := u.ClearDataUsage(context.Background())
		if err == nil {
			t.Fatal("ClearDataUsage() expected error, got nil")
		}
	})
}
