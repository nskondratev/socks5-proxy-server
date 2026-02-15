package createuser

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	goredis "github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

type redisMock struct {
	hGetValue string
	hGetErr   error
	hSetErr   error
	hSetKey   string
	hSetData  []interface{}
}

func (m *redisMock) HGet(_ context.Context, _, _ string) (string, error) {
	return m.hGetValue, m.hGetErr
}

func (m *redisMock) HSet(_ context.Context, key string, values ...interface{}) error {
	m.hSetKey = key
	m.hSetData = values

	return m.hSetErr
}

func TestCommandHandler_Handle(t *testing.T) {
	t.Parallel()

	buf := bytes.NewBuffer(nil)
	redis := &redisMock{hGetErr: goredis.Nil}
	h := New(redis, strings.NewReader("alice\nsecret\n"), buf)

	err := h.Handle(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if redis.hSetKey != userAuthKey {
		t.Fatalf("expected key %q, got %q", userAuthKey, redis.hSetKey)
	}

	if len(redis.hSetData) != 2 {
		t.Fatalf("expected two hset args, got %d", len(redis.hSetData))
	}

	if redis.hSetData[0] != "alice" {
		t.Fatalf("expected username alice, got %v", redis.hSetData[0])
	}

	hashValue, ok := redis.hSetData[1].(string)
	if !ok {
		t.Fatalf("expected hash to be string, got %T", redis.hSetData[1])
	}

	if err = bcrypt.CompareHashAndPassword([]byte(hashValue), []byte("secret")); err != nil {
		t.Fatalf("expected password to be hashed, compare failed: %v", err)
	}

	if !strings.Contains(buf.String(), "User successfully created.") {
		t.Fatalf("expected success output, got %q", buf.String())
	}
}

func TestCommandHandler_HandleUserExists(t *testing.T) {
	t.Parallel()

	h := New(&redisMock{hGetValue: "already-hashed"}, strings.NewReader("alice\nsecret\n"), bytes.NewBuffer(nil))

	err := h.Handle(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCommandHandler_HandleHGetError(t *testing.T) {
	t.Parallel()

	h := New(&redisMock{hGetErr: errors.New("boom")}, strings.NewReader("alice\nsecret\n"), bytes.NewBuffer(nil))

	err := h.Handle(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}
