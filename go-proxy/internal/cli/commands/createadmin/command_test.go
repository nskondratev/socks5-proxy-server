package createadmin

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

type adminServiceMock struct {
	addErr   error
	username string
}

func (m *adminServiceMock) Add(_ context.Context, username string) error {
	m.username = username

	return m.addErr
}

func TestCommandHandler_Handle(t *testing.T) {
	t.Parallel()

	buf := bytes.NewBuffer(nil)
	adminService := &adminServiceMock{}
	h := New(adminService, strings.NewReader("alice\n"), buf)

	err := h.Handle(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if adminService.username != "alice" {
		t.Fatalf("expected username alice, got %q", adminService.username)
	}

	if !strings.Contains(buf.String(), "Admin successfully created.") {
		t.Fatalf("expected success output, got %q", buf.String())
	}
}

func TestCommandHandler_HandleAddError(t *testing.T) {
	t.Parallel()

	h := New(&adminServiceMock{addErr: errors.New("boom")}, strings.NewReader("alice\n"), bytes.NewBuffer(nil))

	err := h.Handle(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}
