package deleteadmin

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	command      = "delete-admin"
	userAdminKey = "user_admin"
)

type redis interface {
	HDel(ctx context.Context, key string, fields ...string) error
}

type CommandHandler struct {
	redis redis
	in    *bufio.Reader
	out   io.Writer
}

func New(redis redis, in io.Reader, out io.Writer) *CommandHandler {
	if in == nil {
		in = os.Stdin
	}
	if out == nil {
		out = os.Stdout
	}

	return &CommandHandler{redis: redis, in: bufio.NewReader(in), out: out}
}

func (h *CommandHandler) CanHandle(_ context.Context, commandName string) bool {
	return commandName == command
}

func (h *CommandHandler) Handle(ctx context.Context) error {
	if h.redis == nil {
		return fmt.Errorf("[delete-admin] redis dependency is not configured")
	}

	fmt.Fprint(h.out, "Input admin username and press Enter: ")
	username, err := h.readInputLine()
	if err != nil {
		return fmt.Errorf("[delete-admin] failed to read username: %w", err)
	}

	if err = h.redis.HDel(ctx, userAdminKey, username); err != nil {
		return fmt.Errorf("[delete-admin] failed to delete admin: %w", err)
	}

	fmt.Fprintln(h.out, "Admin successfully deleted.")

	return nil
}

func (h *CommandHandler) readInputLine() (string, error) {
	line, err := h.in.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}

	return strings.TrimSpace(line), nil
}
