package deleteuser

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	goredis "github.com/redis/go-redis/v9"
)

type redisMock struct {
	hGetErr  error
	hDelErr  error
	hDelKey  string
	hDelArgs []string
}

func (m *redisMock) HGet(_ context.Context, _, _ string) (string, error) {
	return "hashed-password", m.hGetErr
}

func (m *redisMock) HDel(_ context.Context, key string, fields ...string) error {
	m.hDelKey = key
	m.hDelArgs = fields

	return m.hDelErr
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

	if redis.hDelKey != userAuthKey {
		t.Fatalf("expected key %q, got %q", userAuthKey, redis.hDelKey)
	}

	if len(redis.hDelArgs) != 1 || redis.hDelArgs[0] != "alice" {
		t.Fatalf("expected username alice, got %v", redis.hDelArgs)
	}

	if !strings.Contains(buf.String(), "User successfully deleted.") {
		t.Fatalf("expected success output, got %q", buf.String())
	}
}

func TestCommandHandler_HandleNotFound(t *testing.T) {
	t.Parallel()

	h := New(&redisMock{hGetErr: goredis.Nil}, strings.NewReader("alice\n"), bytes.NewBuffer(nil))

	err := h.Handle(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCommandHandler_HandleHDelError(t *testing.T) {
	t.Parallel()

	h := New(&redisMock{hDelErr: errors.New("boom")}, strings.NewReader("alice\n"), bytes.NewBuffer(nil))

	err := h.Handle(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}
