package proxy

import (
	"testing"
	"time"
)

func TestLastAuthDateQueue_EnqueueLastAuthDateNonBlockingWhenChannelFull(t *testing.T) {
	t.Parallel()

	ch := make(chan LastAuthDate, 1)
	ch <- LastAuthDate{UserName: "existing", Time: time.Now()}

	queue := NewLastAuthDateQueue(ch)

	done := make(chan struct{})
	go func() {
		queue.EnqueueLastAuthDate("new-user", time.Now())
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("enqueue blocked while channel is full")
	}

	if got := len(ch); got != 1 {
		t.Fatalf("expected channel size 1, got %d", got)
	}

	item := <-ch
	if item.UserName != "existing" {
		t.Fatalf("expected existing item to stay in queue, got %q", item.UserName)
	}
}
