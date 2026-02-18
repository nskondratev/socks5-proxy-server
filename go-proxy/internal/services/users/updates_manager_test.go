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
	updateLastAuthDateFn func(ctx context.Context, userName string) error
	increaseDataUsageFn  func(ctx context.Context, userName string, dataLen int64) error
}

func (s *managerUpdaterStub) UpdateLastAuthDate(ctx context.Context, userName string) error {
	if s.updateLastAuthDateFn == nil {
		return nil
	}

	return s.updateLastAuthDateFn(ctx, userName)
}

func (s *managerUpdaterStub) IncreaseDataUsage(ctx context.Context, userName string, dataLen int64) error {
	if s.increaseDataUsageFn == nil {
		return nil
	}

	return s.increaseDataUsageFn(ctx, userName, dataLen)
}

func TestNewUpdatesManagerQueueSizeIsAtLeastOne(t *testing.T) {
	t.Parallel()

	manager := NewUpdatesManager(&managerUpdaterStub{}, 0, -1)

	if ok := manager.EnqueueLastAuthDateUpdate("alice"); !ok {
		t.Fatal("enqueue auth update should succeed for minimal queue size")
	}
	if ok := manager.EnqueueLastAuthDateUpdate("alice"); ok {
		t.Fatal("second enqueue should fail because queue capacity is one")
	}

	if ok := manager.EnqueueUsageUpdate("alice", 1); !ok {
		t.Fatal("enqueue usage update should succeed for minimal queue size")
	}
	if ok := manager.EnqueueUsageUpdate("alice", 1); ok {
		t.Fatal("second enqueue should fail because queue capacity is one")
	}
}

func TestEnqueueLastAuthDateUpdate(t *testing.T) {
	t.Parallel()

	manager := NewUpdatesManager(&managerUpdaterStub{}, 1, 1)

	if ok := manager.EnqueueLastAuthDateUpdate("alice"); !ok {
		t.Fatal("first enqueue should succeed")
	}

	if ok := manager.EnqueueLastAuthDateUpdate("alice"); ok {
		t.Fatal("second enqueue should fail because queue is full")
	}
}

func TestEnqueueUsageUpdate(t *testing.T) {
	t.Parallel()

	manager := NewUpdatesManager(&managerUpdaterStub{}, 1, 1)

	if ok := manager.EnqueueUsageUpdate("alice", 1); !ok {
		t.Fatal("first enqueue should succeed")
	}

	if ok := manager.EnqueueUsageUpdate("alice", 1); ok {
		t.Fatal("second enqueue should fail because queue is full")
	}
}

func TestRunProcessesJobs(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var (
		mu         sync.Mutex
		authUsers  []string
		usageUsers []string
		usageVals  []int64
	)

	manager := NewUpdatesManager(&managerUpdaterStub{
		updateLastAuthDateFn: func(_ context.Context, userName string) error {
			mu.Lock()
			defer mu.Unlock()
			authUsers = append(authUsers, userName)
			return nil
		},
		increaseDataUsageFn: func(_ context.Context, userName string, dataLen int64) error {
			mu.Lock()
			defer mu.Unlock()
			usageUsers = append(usageUsers, userName)
			usageVals = append(usageVals, dataLen)
			return nil
		},
	}, 2, 2)

	done := make(chan error, 1)
	go func() {
		done <- manager.Run(ctx)
	}()

	if ok := manager.EnqueueLastAuthDateUpdate("alice"); !ok {
		t.Fatal("enqueue auth update should succeed")
	}

	if ok := manager.EnqueueUsageUpdate("alice", 128); !ok {
		t.Fatal("enqueue usage update should succeed")
	}

	deadline := time.After(500 * time.Millisecond)
	for {
		mu.Lock()
		processed := len(authUsers) == 1 && len(usageUsers) == 1
		mu.Unlock()
		if processed {
			break
		}

		select {
		case <-deadline:
			t.Fatal("jobs were not processed in time")
		case <-time.After(10 * time.Millisecond):
		}
	}

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
	if usageUsers[0] != "alice" {
		t.Fatalf("unexpected usage user: %q", usageUsers[0])
	}
	if usageVals[0] != 128 {
		t.Fatalf("unexpected usage value: %d", usageVals[0])
	}
}

func TestRunContinuesAfterUpdateErrors(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var usageProcessed int32

	manager := NewUpdatesManager(&managerUpdaterStub{
		updateLastAuthDateFn: func(_ context.Context, _ string) error {
			return errors.New("redis unavailable")
		},
		increaseDataUsageFn: func(_ context.Context, _ string, _ int64) error {
			atomic.AddInt32(&usageProcessed, 1)
			return nil
		},
	}, 2, 2)

	done := make(chan error, 1)
	go func() {
		done <- manager.Run(ctx)
	}()

	if ok := manager.EnqueueLastAuthDateUpdate("alice"); !ok {
		t.Fatal("enqueue auth update should succeed")
	}
	if ok := manager.EnqueueUsageUpdate("alice", 64); !ok {
		t.Fatal("enqueue usage update should succeed")
	}

	deadline := time.After(500 * time.Millisecond)
	for atomic.LoadInt32(&usageProcessed) == 0 {
		select {
		case <-deadline:
			t.Fatal("usage update was not processed")
		case <-time.After(10 * time.Millisecond):
		}
	}

	cancel()
	<-done
}
