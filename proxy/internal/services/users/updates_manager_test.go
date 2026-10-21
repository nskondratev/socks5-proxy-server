package users

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type managerUpdaterStub struct {
	updateLastAuthDateFn     func(ctx context.Context, userName string) error
	increaseDataUsageBatchFn func(ctx context.Context, usage map[string]int64) error
}

func (s *managerUpdaterStub) UpdateLastAuthDate(ctx context.Context, userName string) error {
	if s.updateLastAuthDateFn == nil {
		return nil
	}

	return s.updateLastAuthDateFn(ctx, userName)
}

func (s *managerUpdaterStub) IncreaseDataUsageBatch(ctx context.Context, usage map[string]int64) error {
	if s.increaseDataUsageBatchFn == nil {
		return nil
	}

	return s.increaseDataUsageBatchFn(ctx, usage)
}

// usageRecorder collects flushed usage batches in a thread-safe way.
type usageRecorder struct {
	mu      sync.Mutex
	batches []map[string]int64
}

func (r *usageRecorder) record(_ context.Context, usage map[string]int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.batches = append(r.batches, usage)

	return nil
}

func (r *usageRecorder) snapshot() []map[string]int64 {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]map[string]int64(nil), r.batches...)
}

func waitUntil(t *testing.T, timeout time.Duration, cond func() bool, msg string) {
	t.Helper()

	deadline := time.After(timeout)
	for !cond() {
		select {
		case <-deadline:
			t.Fatal(msg)
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func TestNewUpdatesManagerQueueSizeIsAtLeastOne(t *testing.T) {
	t.Parallel()

	manager := NewUpdatesManager(&managerUpdaterStub{}, 0, time.Second)

	if ok := manager.EnqueueLastAuthDateUpdate("alice"); !ok {
		t.Fatal("enqueue auth update should succeed for minimal queue size")
	}
	if ok := manager.EnqueueLastAuthDateUpdate("alice"); ok {
		t.Fatal("second enqueue should fail because queue capacity is one")
	}
}

func TestNewUpdatesManagerUsesDefaultFlushInterval(t *testing.T) {
	t.Parallel()

	manager := NewUpdatesManager(&managerUpdaterStub{}, 1, 0)

	if manager.usageFlushInterval != defaultUsageFlushInterval {
		t.Fatalf("usageFlushInterval = %v, want %v", manager.usageFlushInterval, defaultUsageFlushInterval)
	}
}

func TestEnqueueLastAuthDateUpdate(t *testing.T) {
	t.Parallel()

	manager := NewUpdatesManager(&managerUpdaterStub{}, 1, time.Second)

	if ok := manager.EnqueueLastAuthDateUpdate("alice"); !ok {
		t.Fatal("first enqueue should succeed")
	}

	if ok := manager.EnqueueLastAuthDateUpdate("alice"); ok {
		t.Fatal("second enqueue should fail because queue is full")
	}
}

func TestFlushUsageAggregatesPerUser(t *testing.T) {
	t.Parallel()

	recorder := &usageRecorder{}
	manager := NewUpdatesManager(&managerUpdaterStub{increaseDataUsageBatchFn: recorder.record}, 1, time.Hour)

	manager.AddUsage("alice", 100)
	manager.AddUsage("bob", 7)
	manager.AddUsage("alice", 28)
	manager.AddUsage("bob", 0)
	manager.AddUsage("carol", -5)

	manager.flushUsage(context.Background())

	batches := recorder.snapshot()
	if len(batches) != 1 {
		t.Fatalf("expected exactly one batch, got %d", len(batches))
	}
	if len(batches[0]) != 2 || batches[0]["alice"] != 128 || batches[0]["bob"] != 7 {
		t.Fatalf("unexpected batch: %v", batches[0])
	}

	manager.flushUsage(context.Background())

	if got := len(recorder.snapshot()); got != 1 {
		t.Fatalf("flush without pending usage should not call updater, got %d batches", got)
	}
}

func TestFlushUsageRestoresPendingUsageOnError(t *testing.T) {
	t.Parallel()

	var (
		calls int
		got   map[string]int64
	)

	manager := NewUpdatesManager(&managerUpdaterStub{
		increaseDataUsageBatchFn: func(_ context.Context, usage map[string]int64) error {
			calls++
			if calls == 1 {
				return errors.New("redis unavailable")
			}

			got = usage

			return nil
		},
	}, 1, time.Hour)

	manager.AddUsage("alice", 64)
	manager.flushUsage(context.Background())

	manager.AddUsage("alice", 36)
	manager.flushUsage(context.Background())

	if calls != 2 {
		t.Fatalf("expected two updater calls, got %d", calls)
	}
	if len(got) != 1 || got["alice"] != 100 {
		t.Fatalf("failed batch should be retried with new usage, got %v", got)
	}
}

func TestAddUsageIsSafeForConcurrentUse(t *testing.T) {
	t.Parallel()

	recorder := &usageRecorder{}
	manager := NewUpdatesManager(&managerUpdaterStub{increaseDataUsageBatchFn: recorder.record}, 1, time.Hour)

	const (
		writers = 16
		adds    = 1000
	)

	var wg sync.WaitGroup
	for range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range adds {
				manager.AddUsage("alice", 2)
			}
		}()
	}

	// Flush concurrently with writers to make sure no increments are lost between batches.
	stopFlush := make(chan struct{})
	flushDone := make(chan struct{})
	go func() {
		defer close(flushDone)
		for {
			select {
			case <-stopFlush:
				return
			default:
				manager.flushUsage(context.Background())
			}
		}
	}()

	wg.Wait()
	close(stopFlush)
	<-flushDone
	manager.flushUsage(context.Background())

	var total int64
	for _, batch := range recorder.snapshot() {
		total += batch["alice"]
	}

	if want := int64(writers * adds * 2); total != want {
		t.Fatalf("total flushed usage = %d, want %d", total, want)
	}
}

func TestRunProcessesAuthUpdatesAndFlushesUsagePeriodically(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var (
		mu        sync.Mutex
		authUsers []string
	)
	recorder := &usageRecorder{}

	manager := NewUpdatesManager(&managerUpdaterStub{
		updateLastAuthDateFn: func(_ context.Context, userName string) error {
			mu.Lock()
			defer mu.Unlock()
			authUsers = append(authUsers, userName)
			return nil
		},
		increaseDataUsageBatchFn: recorder.record,
	}, 2, 10*time.Millisecond)

	done := make(chan error, 1)
	go func() {
		done <- manager.Run(ctx)
	}()

	if ok := manager.EnqueueLastAuthDateUpdate("alice"); !ok {
		t.Fatal("enqueue auth update should succeed")
	}
	manager.AddUsage("alice", 128)

	waitUntil(t, time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(authUsers) == 1 && len(recorder.snapshot()) == 1
	}, "updates were not processed in time")

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() unexpected error: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Run() did not stop after context cancel")
	}

	mu.Lock()
	defer mu.Unlock()
	if authUsers[0] != "alice" {
		t.Fatalf("unexpected auth user: %q", authUsers[0])
	}
	if batch := recorder.snapshot()[0]; batch["alice"] != 128 {
		t.Fatalf("unexpected usage batch: %v", batch)
	}
}

func TestRunFlushesPendingUsageOnShutdown(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())

	var flushCtxCanceled atomic.Bool
	recorder := &usageRecorder{}

	manager := NewUpdatesManager(&managerUpdaterStub{
		increaseDataUsageBatchFn: func(ctx context.Context, usage map[string]int64) error {
			flushCtxCanceled.Store(ctx.Err() != nil)
			return recorder.record(ctx, usage)
		},
	}, 1, time.Hour)

	done := make(chan error, 1)
	go func() {
		done <- manager.Run(ctx)
	}()

	manager.AddUsage("alice", 512)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() unexpected error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run() did not stop after context cancel")
	}

	batches := recorder.snapshot()
	if len(batches) != 1 || batches[0]["alice"] != 512 {
		t.Fatalf("pending usage was not flushed on shutdown: %v", batches)
	}
	if flushCtxCanceled.Load() {
		t.Fatal("final flush must not use canceled context")
	}
}

func TestRunContinuesAfterUpdateErrors(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	recorder := &usageRecorder{}

	manager := NewUpdatesManager(&managerUpdaterStub{
		updateLastAuthDateFn: func(_ context.Context, _ string) error {
			return errors.New("redis unavailable")
		},
		increaseDataUsageBatchFn: recorder.record,
	}, 2, 10*time.Millisecond)

	done := make(chan error, 1)
	go func() {
		done <- manager.Run(ctx)
	}()

	if ok := manager.EnqueueLastAuthDateUpdate("alice"); !ok {
		t.Fatal("enqueue auth update should succeed")
	}
	manager.AddUsage("alice", 64)

	waitUntil(t, time.Second, func() bool {
		return len(recorder.snapshot()) > 0
	}, "usage update was not processed")

	cancel()

	if err := <-done; err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
}
