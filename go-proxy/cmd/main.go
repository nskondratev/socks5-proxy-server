package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-co-op/gocron/v2"
	"github.com/joho/godotenv"
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
	"github.com/nskondratev/socks5-proxy-server/internal/services/admin"
	"github.com/nskondratev/socks5-proxy-server/internal/services/users"
)

const (
	httpClientTimeout = 30 * time.Second
)

func main() {
	_ = godotenv.Load()

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

	updatesManager := users.NewUpdatesManager(
		usersService,
		config.RedisAuthUpdatesQueueSize(),
		config.RedisUsageUpdatesQueueSize(),
	)

	dialer := &net.Dialer{}

	socks5Opts := []socks5.Option{
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
				if queued := updatesManager.EnqueueUsageUpdate(username, dataLen); !queued {
					slog.LogAttrs(
						ctx,
						slog.LevelWarn,
						"failed to enqueue data usage update for user",
						slog.String(log.FieldUsername, username),
					)
				}
			}), nil
		}),
	}

	if config.RequireAuth() {
		authCredentialValidator := proxy.NewAuthWithCache(
			cache.NewExpirableLRU[proxy.AuthCacheKey, bool](config.AuthCacheMaxSize(), config.AuthCacheTTL()),
			proxy.NewAuth(usersService, passwords),
			func(user string) {
				if queued := updatesManager.EnqueueLastAuthDateUpdate(user); !queued {
					slog.LogAttrs(
						ctx,
						slog.LevelWarn,
						"failed to enqueue auth date update for user",
						slog.String(log.FieldUsername, user),
					)
				}
			},
		)

		socks5Opts = append(socks5Opts, socks5.WithCredential(authCredentialValidator))
	}

	// Create a SOCKS5 server
	server := socks5.NewServer(socks5Opts...)

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

	scheduler, err := initSchedulerForClearingUsageStats(ctx, usersService)
	if err != nil {
		slog.LogAttrs(
			ctx,
			slog.LevelError,
			"failed to create scheduler for clearing usage stats",
			slog.String(log.FieldError, err.Error()),
		)

		return
	}

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		scheduler.Start()

		return nil
	})

	g.Go(func() error {
		slog.LogAttrs(ctx, slog.LevelInfo, "start redis updates manager")

		return updatesManager.Run(ctx)
	})

	g.Go(func() error {
		// Create SOCKS5 proxy on localhost port 8000
		// TODO: here we can add graceful shutdown by calling method Serve with custom listener
		return server.ListenAndServe("tcp", ":8000")
	})

	// Poll bot updates
	g.Go(func() error {
		if config.TelegramUseWebHooks() {
			slog.LogAttrs(ctx, slog.LevelInfo, "start receiving updates via webhook")
		} else {
			slog.LogAttrs(ctx, slog.LevelInfo, "start polling updates")
		}

		b.Start()

		return nil
	})

	g.Go(func() error {
		<-ctx.Done()
		b.Stop()

		return scheduler.Shutdown()
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
	poller, err := makeTelegramPoller()
	if err != nil {
		return nil, err
	}

	// Bot
	botConf := tele.Settings{
		Token: config.TelegramAPIToken(),
		Client: &http.Client{
			Timeout: httpClientTimeout,
		},
		OnError: bot.OnErrorCb,
		Poller:  poller,
	}

	b, err := tele.NewBot(botConf)
	if err != nil {
		return nil, fmt.Errorf("failed to create new telegram bot: %w", err)
	}

	// Remove any webhooks in case of long polling used
	if !config.TelegramUseWebHooks() {
		if err := b.RemoveWebhook(); err != nil {
			return nil, fmt.Errorf("failed to remove webhook: %w", err)
		}
	}

	adminService := admin.New(redisCli)

	b.Use(
		mw.SetTimeoutCtx(config.TelegramUpdateProcessingTimeout()),
		mw.RestrictByAdminUserID(adminService),
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

func makeTelegramPoller() (tele.Poller, error) {
	if !config.TelegramUseWebHooks() {
		return &tele.LongPoller{Timeout: 10 * time.Second}, nil
	}

	webHookURL, err := buildTelegramWebhookURL()
	if err != nil {
		return nil, err
	}

	webhookEndpoint := &tele.WebhookEndpoint{PublicURL: webHookURL}
	poller := &tele.Webhook{
		Listen:   fmt.Sprintf(":%d", config.BotAppPort()),
		Endpoint: webhookEndpoint,
	}

	certPath := config.TelegramWebhookTLSCertPath()
	keyPath := config.TelegramWebhookTLSKeyPath()
	if certPath == "" || keyPath == "" {
		return poller, nil
	}

	webhookEndpoint.Cert = certPath
	poller.TLS = &tele.WebhookTLS{Cert: certPath, Key: keyPath}

	return poller, nil
}

func buildTelegramWebhookURL() (string, error) {
	parsedPublicURL, err := url.Parse(config.PublicURL())
	if err != nil {
		return "", fmt.Errorf("failed to parse PUBLIC_URL: %w", err)
	}

	if parsedPublicURL.Scheme == "" || parsedPublicURL.Host == "" {
		return "", fmt.Errorf("PUBLIC_URL must be an absolute URL")
	}

	parsedPublicURL.Path = parsedPublicURL.JoinPath(config.TelegramWebHookURL()).Path + config.TelegramAPIToken()

	return parsedPublicURL.String(), nil
}

func initSchedulerForClearingUsageStats(ctx context.Context, usersService *users.Users) (gocron.Scheduler, error) {
	// create a scheduler
	s, err := gocron.NewScheduler()
	if err != nil {
		return nil, fmt.Errorf("failed to create scheduler: %w", err)
	}

	if _, err = s.NewJob(
		gocron.CronJob("0 0 1 * *", false),
		gocron.NewTask(func(ctx context.Context) {
			if err := usersService.ClearDataUsage(ctx); err != nil {
				slog.LogAttrs(
					ctx,
					slog.LevelError,
					"failed to clear data usage",
					slog.String(log.FieldError, err.Error()),
				)
			}
		}),
		gocron.WithContext(ctx),
	); err != nil {
		return nil, fmt.Errorf("failed to create scheduled job: %w", err)
	}

	return s, nil
}
