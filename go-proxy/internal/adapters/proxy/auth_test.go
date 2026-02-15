package proxy

import (
	"context"
	"errors"
	"testing"
	"time"
)

type testPasswordHashGetter struct {
	passwordHash string
	err          error
}

func (g *testPasswordHashGetter) GetPasswordHash(_ context.Context, _ string) (string, error) {
	if g.err != nil {
		return "", g.err
	}

	return g.passwordHash, nil
}

type testPasswordComparator struct {
	valid bool
	err   error
}

func (c *testPasswordComparator) Valid(_, _ string) (bool, error) {
	if c.err != nil {
		return false, c.err
	}

	return c.valid, nil
}

type testLastAuthDateEnqueuer struct {
	calls int
}

func (e *testLastAuthDateEnqueuer) EnqueueLastAuthDate(_ string, _ time.Time) {
	e.calls++
}

func TestAuthValid_EnqueuesOnlyWhenPasswordIsValid(t *testing.T) {
	t.Parallel()

	t.Run("enqueue on valid password", func(t *testing.T) {
		t.Parallel()

		enqueuer := &testLastAuthDateEnqueuer{}
		auth := NewAuth(
			&testPasswordHashGetter{passwordHash: "hash"},
			&testPasswordComparator{valid: true},
			enqueuer,
		)

		if ok := auth.Valid("user", "password", ""); !ok {
			t.Fatal("expected valid auth")
		}

		if enqueuer.calls != 1 {
			t.Fatalf("expected enqueue call count 1, got %d", enqueuer.calls)
		}
	})

	t.Run("no enqueue on invalid password", func(t *testing.T) {
		t.Parallel()

		enqueuer := &testLastAuthDateEnqueuer{}
		auth := NewAuth(
			&testPasswordHashGetter{passwordHash: "hash"},
			&testPasswordComparator{valid: false},
			enqueuer,
		)

		if ok := auth.Valid("user", "password", ""); ok {
			t.Fatal("expected invalid auth")
		}

		if enqueuer.calls != 0 {
			t.Fatalf("expected enqueue call count 0, got %d", enqueuer.calls)
		}
	})

	t.Run("no enqueue when hash getter fails", func(t *testing.T) {
		t.Parallel()

		enqueuer := &testLastAuthDateEnqueuer{}
		auth := NewAuth(
			&testPasswordHashGetter{err: errors.New("boom")},
			&testPasswordComparator{valid: true},
			enqueuer,
		)

		if ok := auth.Valid("user", "password", ""); ok {
			t.Fatal("expected invalid auth")
		}

		if enqueuer.calls != 0 {
			t.Fatalf("expected enqueue call count 0, got %d", enqueuer.calls)
		}
	})
}
