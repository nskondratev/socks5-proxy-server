package cli

import (
	"context"
	"os"

	"github.com/nskondratev/socks5-proxy-server/proxy/internal/cli/commands/createadmin"
	"github.com/nskondratev/socks5-proxy-server/proxy/internal/cli/commands/createuser"
	"github.com/nskondratev/socks5-proxy-server/proxy/internal/cli/commands/deleteadmin"
	"github.com/nskondratev/socks5-proxy-server/proxy/internal/cli/commands/deleteuser"
	"github.com/nskondratev/socks5-proxy-server/proxy/internal/cli/commands/stats"
	"github.com/nskondratev/socks5-proxy-server/proxy/internal/config"
	"github.com/nskondratev/socks5-proxy-server/proxy/internal/log"
	"github.com/nskondratev/socks5-proxy-server/proxy/internal/services/admin"
	"github.com/nskondratev/socks5-proxy-server/proxy/internal/services/users"
)

type redis interface {
	HGet(ctx context.Context, key, field string) (string, error)
	HGetAll(ctx context.Context, key string) (map[string]string, error)
	HSet(ctx context.Context, key string, values ...interface{}) error
	HDel(ctx context.Context, key string, fields ...string) error
	HIncrByBatch(ctx context.Context, key string, increments map[string]int64) error
	Del(ctx context.Context, keys ...string) error
	HExists(ctx context.Context, key, field string) (bool, error)
}

type CommandsDeps struct {
	Redis redis
}

type commandHandler interface {
	CanHandle(ctx context.Context, commandName string) bool
	Handle(ctx context.Context) error
}

func HandleCLICommand(ctx context.Context, deps *CommandsDeps) (handled bool) {
	if len(os.Args) < 2 || os.Args[1] == "" {
		return handled
	}

	log.SetDefaultWithParams(log.OutputText, log.ParseStringLogLevel(config.LogLevel()))

	handled = true
	commandName := os.Args[1]

	log.Info(
		ctx,
		"in CLI command mode, process command",
		log.String("command", commandName),
	)

	adminService := admin.New(deps.Redis)
	usersService := users.New(deps.Redis)

	commands := []commandHandler{
		createadmin.New(adminService, os.Stdin, os.Stdout),
		createuser.New(usersService, os.Stdin, os.Stdout),
		deleteadmin.New(adminService, os.Stdin, os.Stdout),
		deleteuser.New(usersService, os.Stdin, os.Stdout),
		stats.New(usersService, os.Stdout),
	}

	for i := range commands {
		if commands[i].CanHandle(ctx, commandName) {
			if err := commands[i].Handle(ctx); err != nil {
				log.Error(
					ctx,
					"failed to handle CLI command",
					log.String("command", commandName),
					log.String(log.FieldError, err.Error()),
				)

				return handled
			}

			return handled
		}
	}

	log.Warn(
		ctx,
		"unknown CLI command",
		log.String("command", commandName),
	)

	return handled
}
