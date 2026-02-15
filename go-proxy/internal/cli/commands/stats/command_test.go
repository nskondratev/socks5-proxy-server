package stats

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

type redisMock struct {
	data map[string]map[string]string
	err  map[string]error
}

func (r *redisMock) HGetAll(_ context.Context, key string) (map[string]string, error) {
	if err, ok := r.err[key]; ok {
		return nil, err
	}

	return r.data[key], nil
}

func TestCommandHandler_Handle(t *testing.T) {
	t.Parallel()

	buf := bytes.NewBuffer(nil)
	h := New(&redisMock{data: map[string]map[string]string{
		dataUsageKey: {
			"alice": "1025",
			"bob":   "256",
		},
		authDateKey: {
			"alice": "2026-01-01T10:00:00Z",
		},
	}}, buf)

	err := h.Handle(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "alice") || !strings.Contains(out, "bob") {
		t.Fatalf("expected users to be printed, got %q", out)
	}

	if strings.Index(out, "alice") > strings.Index(out, "bob") {
		t.Fatalf("expected alice to be listed before bob, got %q", out)
	}

	if !strings.Contains(out, "1.00 KB") {
		t.Fatalf("expected human readable usage for alice, got %q", out)
	}

	if !strings.Contains(out, "-") {
		t.Fatalf("expected missing last login fallback, got %q", out)
	}
}

func TestCommandHandler_HandleRedisError(t *testing.T) {
	t.Parallel()

	h := New(&redisMock{err: map[string]error{dataUsageKey: errors.New("boom")}}, bytes.NewBuffer(nil))

	err := h.Handle(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestFormatBytes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		size int64
		want string
	}{
		{name: "bytes", size: 1024, want: "1024 B"},
		{name: "kb", size: 1025, want: "1.00 KB"},
		{name: "mb", size: 1048577, want: "1.00 MB"},
		{name: "gb", size: 1073741825, want: "1.00 GB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := formatBytes(tt.size)
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}
