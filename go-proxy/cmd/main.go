package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/things-go/go-socks5"
	"golang.org/x/sync/errgroup"
	tele "gopkg.in/telebot.v3"

	"github.com/nskondratev/socks5-proxy-server/internal/adapters/proxy"
	"github.com/nskondratev/socks5-proxy-server/internal/bot"
	"github.com/nskondratev/socks5-proxy-server/internal/bot/commands/createuser"
	"github.com/nskondratev/socks5-proxy-server/internal/bot/commands/deleteuser"
	"github.com/nskondratev/socks5-proxy-server/internal/bot/commands/generatepass"
	"github.com/nskondratev/socks5-proxy-server/internal/bot/commands/getusers"
	"github.com/nskondratev/socks5-proxy-server/internal/bot/commands/start"
	"github.com/nskondratev/socks5-proxy-server/internal/bot/commands/usersstats"
	"github.com/nskondratev/socks5-proxy-server/internal/bot/handlers/message"
	mw "github.com/nskondratev/socks5-proxy-server/internal/bot/middleware"
	"github.com/nskondratev/socks5-proxy-server/internal/bot/store"
	"github.com/nskondratev/socks5-proxy-server/internal/cache"
	"github.com/nskondratev/socks5-proxy-server/internal/cli"
	"github.com/nskondratev/socks5-proxy-server/internal/config"
	"github.com/nskondratev/socks5-proxy-server/internal/log"
	"github.com/nskondratev/socks5-proxy-server/internal/password"
	"github.com/nskondratev/socks5-proxy-server/internal/redis"
	"github.com/nskondratev/socks5-proxy-server/internal/services/users"
)

const (
	httpClientTimeout = 30 * time.Second
)

func main() {
	// TODO: implement graceful shutdown
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	redisCli := redis.New(config.RedisHost(), config.RedisPort(), config.RedisDB())

	passwords := password.New()
	usersService := users.New(redisCli)

	// Handle CLI commands, if passed
	if handled := cli.HandleCLICommand(ctx, &cli.CLICommandsDeps{Redis: redisCli}); handled {
		return
	}

	log.SetDefaultWithParams(log.OutputText, log.ParseStringLogLevel(config.LogLevel()))

	authCredentialValidator := proxy.NewAuthWithCache(
		cache.NewExpirableLRU[proxy.AuthCacheKey, bool](config.AuthCacheMaxSize(), config.AuthCacheTTL()),
		proxy.NewAuth(usersService, passwords),
		func(user string) {
			if err := usersService.UpdateLastAuthDate(ctx, user); err != nil {
				slog.LogAttrs(
					ctx,
					slog.LevelWarn,
					"failed to update auth date for user",
					slog.String(log.FieldUsername, user),
					slog.String(log.FieldError, err.Error()),
				)
			}
		},
	)

	dialer := &net.Dialer{}

	// Create a SOCKS5 server
	server := socks5.NewServer(
		// socks5.WithLogger(socks5.NewLogger(log.New(os.Stdout, "socks5: ", log.LstdFlags))),
		socks5.WithDialAndRequest(func(ctx context.Context, network, addr string, request *socks5.Request) (net.Conn, error) {
			conn, err := dialer.DialContext(ctx, network, addr)
			if err != nil {
				return nil, err
			}

			username, ok := getUsernameFromRequest(request)
			if !ok {
				return conn, nil
			}

			return proxy.NewUsageTrackedConn(conn, func(dataLen int64) {
				if err := usersService.IncreaseDataUsage(ctx, username, dataLen); err != nil {
					slog.LogAttrs(
						ctx,
						slog.LevelWarn,
						"failed to increase data usage for user",
						slog.String(log.FieldUsername, username),
						slog.String(log.FieldError, err.Error()),
					)
				}
			}), nil
		}),
		socks5.WithCredential(authCredentialValidator),
	)

	// Create telegram bot
	b, err := initBot(redisCli)
	if err != nil {
		slog.LogAttrs(
			ctx,
			slog.LevelError,
			"failed to create telegram bot",
			slog.String(log.FieldError, err.Error()),
		)

		return
	}

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		// Create SOCKS5 proxy on localhost port 8000
		// TODO: here we can add graceful shutdown by calling method Serve with custom listener
		return server.ListenAndServe("tcp", ":8000")
	})

	// Poll bot updates
	g.Go(func() error {
		slog.LogAttrs(ctx, slog.LevelInfo, "start polling updates")

		b.Start()

		return nil
	})

	g.Go(func() error {
		<-ctx.Done()
		b.Stop()

		return nil
	})

	if err = g.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		slog.LogAttrs(
			ctx,
			slog.LevelError,
			"errgroup finished with error",
			slog.String(log.FieldError, err.Error()),
		)

		return
	}

	slog.LogAttrs(ctx, slog.LevelInfo, "exit from app")
}

func getUsernameFromRequest(request *socks5.Request) (string, bool) {
	if request == nil || request.AuthContext == nil || request.AuthContext.Payload == nil {
		return "", false
	}

	username, ok := request.AuthContext.Payload["Username"]
	if !ok || username == "" {
		username = request.AuthContext.Payload["username"]
	}

	if username == "" {
		return "", false
	}

	return username, true
}

func initBot(redisCli *redis.Redis) (*tele.Bot, error) {
	// Bot
	botConf := tele.Settings{
		Token: config.TelegramAPIToken(),
		Client: &http.Client{
			Timeout: httpClientTimeout,
		},
		OnError: bot.OnErrorCb,
		Poller:  &tele.LongPoller{Timeout: 10 * time.Second},
	}

	b, err := tele.NewBot(botConf)
	if err != nil {
		return nil, fmt.Errorf("failed to create new telegram bot: %w", err)
	}

	b.Use(
		mw.SetTimeoutCtx(config.TelegramUpdateProcessingTimeout()),
		mw.RestrictByAdminUserID(),
	)

	botStore := store.New(redisCli)

	// Commands
	b.Handle(start.Command, start.New(botStore).Handle)
	b.Handle(usersstats.Command, usersstats.New(botStore).Handle)
	b.Handle(createuser.Command, createuser.New(botStore).Handle)
	b.Handle(deleteuser.Command, deleteuser.New(botStore).Handle)
	b.Handle(getusers.Command, getusers.New(botStore).Handle)
	b.Handle(generatepass.Command, generatepass.New().Handle)

	// Messages
	b.Handle(tele.OnText, message.New(botStore).Handle)

	return b, nil
}
