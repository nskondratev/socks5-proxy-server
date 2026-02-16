package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/nskondratev/socks5-proxy-server/internal/cli/commands/createadmin"
	"github.com/nskondratev/socks5-proxy-server/internal/cli/commands/createuser"
	"github.com/nskondratev/socks5-proxy-server/internal/cli/commands/deleteadmin"
	"github.com/nskondratev/socks5-proxy-server/internal/cli/commands/deleteuser"
	"github.com/nskondratev/socks5-proxy-server/internal/cli/commands/stats"
	"github.com/nskondratev/socks5-proxy-server/internal/config"
	"github.com/nskondratev/socks5-proxy-server/internal/log"
)

type redis interface {
	HGet(ctx context.Context, key, field string) (string, error)
	HGetAll(ctx context.Context, key string) (map[string]string, error)
	HSet(ctx context.Context, key string, values ...interface{}) error
	HDel(ctx context.Context, key string, fields ...string) error
}

type CLICommandsDeps struct {
	Redis redis
}

type commandHandler interface {
	CanHandle(ctx context.Context, commandName string) bool
	Handle(ctx context.Context) error
}

func HandleCLICommand(ctx context.Context, deps *CLICommandsDeps) (handled bool) {
	if len(os.Args) < 2 || os.Args[1] == "" {
		return handled
	}

	log.SetDefaultWithParams(log.OutputText, log.ParseStringLogLevel(config.LogLevel()))

	handled = true
	commandName := os.Args[1]

	fmt.Printf("In CLI command mode, process command with name: %s\n", commandName)

	commands := []commandHandler{
		createadmin.New(deps.Redis, os.Stdin, os.Stdout),
		createuser.New(deps.Redis, os.Stdin, os.Stdout),
		deleteadmin.New(deps.Redis, os.Stdin, os.Stdout),
		deleteuser.New(deps.Redis, os.Stdin, os.Stdout),
		stats.New(deps.Redis, os.Stdout),
	}

	for i := range commands {
		if commands[i].CanHandle(ctx, commandName) {
			if err := commands[i].Handle(ctx); err != nil {
				fmt.Printf("Failed to handle command %s, error = %s\n", commandName, err.Error())

				return handled
			}

			return handled
		}
	}

	fmt.Printf("Unknown command: %s\n", commandName)

	return handled
}
