package deleteadmin

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

type redisMock struct {
	hDelErr  error
	hDelKey  string
	hDelArgs []string
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

	if redis.hDelKey != userAdminKey {
		t.Fatalf("expected key %q, got %q", userAdminKey, redis.hDelKey)
	}

	if len(redis.hDelArgs) != 1 || redis.hDelArgs[0] != "alice" {
		t.Fatalf("expected username alice, got %v", redis.hDelArgs)
	}

	if !strings.Contains(buf.String(), "Admin successfully deleted.") {
		t.Fatalf("expected success output, got %q", buf.String())
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
