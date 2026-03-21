package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-co-op/gocron/v2"
	"github.com/joho/godotenv"
	"golang.org/x/sync/errgroup"
	tele "gopkg.in/telebot.v3"

	botbootstrap "github.com/nskondratev/socks5-proxy-server/proxy/internal/bot/bootstrap"
	"github.com/nskondratev/socks5-proxy-server/proxy/internal/cli"
	"github.com/nskondratev/socks5-proxy-server/proxy/internal/config"
	"github.com/nskondratev/socks5-proxy-server/proxy/internal/log"
	"github.com/nskondratev/socks5-proxy-server/proxy/internal/metrics"
	"github.com/nskondratev/socks5-proxy-server/proxy/internal/password"
	pprofserver "github.com/nskondratev/socks5-proxy-server/proxy/internal/pprofserver"
	"github.com/nskondratev/socks5-proxy-server/proxy/internal/proxy"
	"github.com/nskondratev/socks5-proxy-server/proxy/internal/redis"
	"github.com/nskondratev/socks5-proxy-server/proxy/internal/services/users"
)

const telegramBotHTTPClientTimeout = 30 * time.Second

//nolint:gocognit,gocyclo,cyclop,funlen // Startup wiring intentionally keeps orchestration in one place.
func main() {
	_ = godotenv.Load()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	redisCli := redis.New(config.RedisHost(), config.RedisPort(), config.RedisDB())

	usersService := users.New(redisCli)

	// Handle CLI commands, if passed
	if handled := cli.HandleCLICommand(ctx, &cli.CommandsDeps{Redis: redisCli}); handled {
		return
	}

	log.SetDefaultWithParams(log.OutputText, log.ParseStringLogLevel(config.LogLevel()))

	updatesManager := users.NewUpdatesManager(
		usersService,
		config.RedisAuthUpdatesQueueSize(),
		config.RedisUsageUpdatesQueueSize(),
	)

	metricsService, err := metrics.New(metrics.Config{
		Enabled:      config.MetricsEnabled(),
		Port:         config.MetricsPort(),
		AuthEnabled:  config.MetricsAuthEnabled(),
		AuthUsername: config.MetricsAuthUsername(),
		AuthPassword: config.MetricsAuthPassword(),
	})
	if err != nil {
		log.Error(
			ctx,
			"failed to init metrics service",
			log.String(log.FieldError, err.Error()),
		)

		return
	}

	server, err := proxy.NewServer(proxy.NewServerParams{
		Config: proxy.Config{
			RequireAuth:      config.RequireAuth(),
			AuthCacheMaxSize: config.AuthCacheMaxSize(),
			AuthCacheTTL:     config.AuthCacheTTL(),
			IdleTimeout:      config.ProxyIdleTimeout(),
		},
		PasswordHashGetter: usersService,
		PasswordComparator: password.New(),
		UpdatesManager:     updatesManager,
		Metrics:            metricsService,
	})
	if err != nil {
		log.Error(
			ctx,
			"failed to init socks5 server",
			log.String(log.FieldError, err.Error()),
		)

		return
	}

	pprofService, err := pprofserver.New(pprofserver.Config{
		Enabled:      config.PprofEnabled(),
		Port:         config.PprofPort(),
		AuthEnabled:  config.PprofAuthEnabled(),
		AuthUsername: config.PprofAuthUsername(),
		AuthPassword: config.PprofAuthPassword(),
	})
	if err != nil {
		log.Error(
			ctx,
			"failed to init pprof service",
			log.String(log.FieldError, err.Error()),
		)

		return
	}

	listenCfg := net.ListenConfig{}

	proxyListener, err := listenCfg.Listen(ctx, "tcp", ":8000")
	if err != nil {
		log.Error(
			ctx,
			"failed to create socks5 listener",
			log.String(log.FieldError, err.Error()),
		)

		return
	}

	proxyListener = proxy.WrapListenerWithHandshakeTimeout(proxyListener, config.ProxyHandshakeTimeout())

	// Create telegram bot
	var b *tele.Bot

	if config.TelegramBotEnabled() {
		b, err = botbootstrap.New(botbootstrap.NewParams{
			Config: botbootstrap.Config{
				TelegramAuth:            config.TelegramAPIToken(),
				UseWebHooks:             config.TelegramUseWebHooks(),
				PublicURL:               config.PublicURL(),
				WebHookURL:              config.TelegramWebHookURL(),
				BotAppPort:              config.BotAppPort(),
				WebhookTLSCertPath:      config.TelegramWebhookTLSCertPath(),
				WebhookTLSKeyPath:       config.TelegramWebhookTLSKeyPath(),
				UpdateProcessingTimeout: config.TelegramUpdateProcessingTimeout(),
				HTTPClientTimeout:       telegramBotHTTPClientTimeout,
			},
			Redis:        redisCli,
			UsersService: usersService,
		})
		if err != nil {
			log.Error(
				ctx,
				"failed to create telegram bot",
				log.String(log.FieldError, err.Error()),
			)

			return
		}
	} else {
		log.Info(ctx, "telegram bot is disabled")
	}

	scheduler, err := initSchedulerForClearingUsageStats(ctx, usersService)
	if err != nil {
		log.Error(
			ctx,
			"failed to create scheduler for clearing usage stats",
			log.String(log.FieldError, err.Error()),
		)

		return
	}

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		scheduler.Start()

		return nil
	})

	g.Go(func() error {
		log.Info(ctx, "start redis updates manager")

		return updatesManager.Run(ctx)
	})

	if metricsService.Enabled() {
		g.Go(func() error {
			log.Info(
				ctx,
				"start metrics server",
				log.Int("port", config.MetricsPort()),
				log.Bool("auth_enabled", config.MetricsAuthEnabled()),
			)

			return metricsService.Run(ctx)
		})
	}

	if pprofService.Enabled() {
		g.Go(func() error {
			log.Info(
				ctx,
				"start pprof server",
				log.Int("port", config.PprofPort()),
				log.Bool("auth_enabled", config.PprofAuthEnabled()),
			)

			return pprofService.Run(ctx)
		})
	}

	g.Go(func() error {
		// Serve via external listener so we can stop accept loop on shutdown.
		err := server.Serve(proxyListener)
		if err != nil && !errors.Is(err, net.ErrClosed) {
			return err
		}

		return nil
	})

	if b != nil {
		// Poll bot updates
		g.Go(func() error {
			if config.TelegramUseWebHooks() {
				log.Info(ctx, "start receiving updates via webhook")
			} else {
				log.Info(ctx, "start polling updates")
			}

			b.Start()

			return nil
		})
	}

	g.Go(func() error {
		<-ctx.Done()

		if b != nil {
			b.Stop()
		}

		if err := proxyListener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			return err
		}

		return scheduler.Shutdown()
	})

	if err = g.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		log.Error(
			ctx,
			"errgroup finished with error",
			log.String(log.FieldError, err.Error()),
		)

		return
	}

	log.Info(ctx, "exit from app")
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
				log.Error(
					ctx,
					"failed to clear data usage",
					log.String(log.FieldError, err.Error()),
				)
			}
		}),
		gocron.WithContext(ctx),
	); err != nil {
		return nil, fmt.Errorf("failed to create scheduled job: %w", err)
	}

	return s, nil
}
