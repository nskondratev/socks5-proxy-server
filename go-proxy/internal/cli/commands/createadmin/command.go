package createadmin

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
	command = "create-admin"
)

type adminService interface {
	Add(ctx context.Context, username string) error
}

type CommandHandler struct {
	adminService adminService
	in           *bufio.Reader
	out          io.Writer
}

func New(adminService adminService, in io.Reader, out io.Writer) *CommandHandler {
	if in == nil {
		in = os.Stdin
	}
	if out == nil {
		out = os.Stdout
	}

	return &CommandHandler{adminService: adminService, in: bufio.NewReader(in), out: out}
}

func (h *CommandHandler) CanHandle(_ context.Context, commandName string) bool {
	return commandName == command
}

func (h *CommandHandler) Handle(ctx context.Context) error {
	if h.adminService == nil {
		return fmt.Errorf("[create-admin] admin service dependency is not configured")
	}

	fmt.Fprint(h.out, "Input admin username and press Enter: ")
	username, err := h.readInputLine()
	if err != nil {
		return fmt.Errorf("[create-admin] failed to read username: %w", err)
	}

	if err = h.adminService.Add(ctx, username); err != nil {
		return fmt.Errorf("[create-admin] failed to create admin: %w", err)
	}

	fmt.Fprintln(h.out, "Admin successfully created.")

	return nil
}

func (h *CommandHandler) readInputLine() (string, error) {
	line, err := h.in.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}

	return strings.TrimSpace(line), nil
}
