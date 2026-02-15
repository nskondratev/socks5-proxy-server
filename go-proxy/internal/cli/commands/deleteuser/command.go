package deleteuser

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	goredis "github.com/redis/go-redis/v9"
)

const (
	command     = "delete-user"
	userAuthKey = "user_auth"
)

type redis interface {
	HGet(ctx context.Context, key, field string) (string, error)
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
		return fmt.Errorf("[delete-user] redis dependency is not configured")
	}

	fmt.Fprint(h.out, "Input username and press Enter: ")
	username, err := h.readInputLine()
	if err != nil {
		return fmt.Errorf("[delete-user] failed to read username: %w", err)
	}

	if _, err = h.redis.HGet(ctx, userAuthKey, username); errors.Is(err, goredis.Nil) {
		return fmt.Errorf("[delete-user] user with provided username not found")
	} else if err != nil {
		return fmt.Errorf("[delete-user] failed to check if user exists: %w", err)
	}

	if err = h.redis.HDel(ctx, userAuthKey, username); err != nil {
		return fmt.Errorf("[delete-user] failed to delete user: %w", err)
	}

	fmt.Fprintln(h.out, "User successfully deleted.")

	return nil
}

func (h *CommandHandler) readInputLine() (string, error) {
	line, err := h.in.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}

	return strings.TrimSpace(line), nil
}
