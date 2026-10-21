package redis

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAndClose(t *testing.T) {
	t.Parallel()

	r := New("127.0.0.1", 1, 0)
	require.NotNil(t, r)
	require.NoError(t, r.Close())
}

func TestRedisMethodsReturnErrorsForUnavailableRedis(t *testing.T) {
	t.Parallel()

	r := New("127.0.0.1", 1, 0)
	t.Cleanup(func() {
		_ = r.Close()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err := r.HGet(ctx, "k", "f")
	assert.Error(t, err)

	_, err = r.HGetAll(ctx, "k")
	assert.Error(t, err)

	_, err = r.HExists(ctx, "k", "f")
	assert.Error(t, err)

	err = r.HSet(ctx, "k", "f", "v")
	assert.Error(t, err)

	err = r.HDel(ctx, "k", "f")
	assert.Error(t, err)

	err = r.HIncrBy(ctx, "k", "f", 1)
	assert.Error(t, err)

	err = r.HIncrByBatch(ctx, "k", map[string]int64{"f": 1})
	assert.Error(t, err)

	err = r.Del(ctx, "k")
	assert.Error(t, err)
}

func TestHIncrByBatchWithEmptyIncrementsIsNoop(t *testing.T) {
	t.Parallel()

	r := New("127.0.0.1", 1, 0)
	t.Cleanup(func() {
		_ = r.Close()
	})

	require.NoError(t, r.HIncrByBatch(context.Background(), "k", nil))
}
