package admin

import (
	"context"
	"errors"
	"testing"
)

type redisMock struct {
	hSetErr       error
	hDelErr       error
	hExistsErr    error
	hExistsResult bool
	hSetKey       string
	hSetValues    []interface{}
	hDelKey       string
	hDelFields    []string
	hExistsKey    string
	hExistsField  string
}

func (m *redisMock) HSet(_ context.Context, key string, values ...interface{}) error {
	m.hSetKey = key
	m.hSetValues = values
	return m.hSetErr
}

func (m *redisMock) HDel(_ context.Context, key string, fields ...string) error {
	m.hDelKey = key
	m.hDelFields = fields
	return m.hDelErr
}

func (m *redisMock) HExists(_ context.Context, key, field string) (bool, error) {
	m.hExistsKey = key
	m.hExistsField = field
	return m.hExistsResult, m.hExistsErr
}

func TestService_Add(t *testing.T) {
	r := &redisMock{}
	s := New(r)
	if err := s.Add(context.Background(), "alice"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.hSetKey != userAdminKey {
		t.Fatalf("unexpected key: %s", r.hSetKey)
	}
}

func TestService_Remove(t *testing.T) {
	r := &redisMock{}
	s := New(r)
	if err := s.Remove(context.Background(), "alice"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.hDelKey != userAdminKey {
		t.Fatalf("unexpected key: %s", r.hDelKey)
	}
}

func TestService_IsAdmin(t *testing.T) {
	r := &redisMock{hExistsResult: true}
	s := New(r)
	ok, err := s.IsAdmin(context.Background(), "alice")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected true")
	}
}

func TestService_IsAdminError(t *testing.T) {
	r := &redisMock{hExistsErr: errors.New("boom")}
	s := New(r)
	_, err := s.IsAdmin(context.Background(), "alice")
	if err == nil {
		t.Fatal("expected error")
	}
}
