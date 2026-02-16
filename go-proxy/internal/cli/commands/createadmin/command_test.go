package createadmin

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

type redisMock struct {
	hSetErr  error
	hSetKey  string
	hSetData []interface{}
}

func (m *redisMock) HSet(_ context.Context, key string, values ...interface{}) error {
	m.hSetKey = key
	m.hSetData = values

	return m.hSetErr
}

func TestCommandHandler_Handle(t *testing.T) {
	t.Parallel()

	buf := bytes.NewBuffer(nil)
	redis := &redisMock{}
	h := New(redis, strings.NewReader("alice\n"), buf)

	err := h.Handle(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if redis.hSetKey != userAdminKey {
		t.Fatalf("expected key %q, got %q", userAdminKey, redis.hSetKey)
	}

	if len(redis.hSetData) != 2 {
		t.Fatalf("expected two hset args, got %d", len(redis.hSetData))
	}

	if redis.hSetData[0] != "alice" {
		t.Fatalf("expected username alice, got %v", redis.hSetData[0])
	}

	if redis.hSetData[1] != 1 {
		t.Fatalf("expected admin marker 1, got %v", redis.hSetData[1])
	}

	if !strings.Contains(buf.String(), "Admin successfully created.") {
		t.Fatalf("expected success output, got %q", buf.String())
	}
}

func TestCommandHandler_HandleHSetError(t *testing.T) {
	t.Parallel()

	h := New(&redisMock{hSetErr: errors.New("boom")}, strings.NewReader("alice\n"), bytes.NewBuffer(nil))

	err := h.Handle(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}
