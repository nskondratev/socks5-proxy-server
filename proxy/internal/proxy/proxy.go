package proxy

import (
	"context"
	"errors"
	"net"
	"time"

	"github.com/things-go/go-socks5"

	"github.com/nskondratev/socks5-proxy-server/proxy/internal/cache"
	"github.com/nskondratev/socks5-proxy-server/proxy/internal/log"
	"github.com/nskondratev/socks5-proxy-server/proxy/internal/metrics"
	proxyadapter "github.com/nskondratev/socks5-proxy-server/proxy/internal/proxy/adapters"
)

type passwordHashGetter interface {
	GetPasswordHash(ctx context.Context, userName string) (string, error)
}

type passwordComparator interface {
	Valid(input, toCompare string) (bool, error)
}

type updatesManager interface {
	EnqueueLastAuthDateUpdate(user string) bool
	EnqueueUsageUpdate(user string, dataLen int64) bool
}

type AuthValidator interface {
	Valid(user, password, userAddr string) bool
}

type metricsObserver interface {
	ObserveConnectionOpened()
	ObserveConnectionClosed()
	ObserveAuthAttempt(valid bool)
	ObserveProxyTrafficBytes(dataLen int64)
	ObserveProxyTrafficBytesByUsername(username string, dataLen int64)
	ObserveDroppedRedisUpdate(queue string)
}

type Config struct {
	RequireAuth      bool
	AuthCacheMaxSize int
	AuthCacheTTL     time.Duration
	IdleTimeout      time.Duration
}

type NewServerParams struct {
	Config Config

	PasswordHashGetter passwordHashGetter
	PasswordComparator passwordComparator
	UpdatesManager     updatesManager
	Metrics            metricsObserver
}

func NewServer(params NewServerParams) (*socks5.Server, error) {
	if err := validateAuthDependencies(params); err != nil {
		return nil, err
	}

	socks5Opts := []socks5.Option{
		socks5.WithDialAndRequest(newDialAndRequest(params)),
		socks5.WithConnectHandle(newConnectHandler(params)),
		socks5.WithAssociateMiddleware(clearHandshakeDeadlineMiddleware),
	}

	if params.Config.RequireAuth {
		socks5Opts = append(socks5Opts, buildAuthOption(params))
	}

	return socks5.NewServer(socks5Opts...), nil
}

func validateAuthDependencies(params NewServerParams) error {
	if !params.Config.RequireAuth {
		return nil
	}

	if params.PasswordHashGetter == nil {
		return errors.New("password hash getter is required when auth is enabled")
	}

	if params.PasswordComparator == nil {
		return errors.New("password comparator is required when auth is enabled")
	}

	return nil
}

func newDialAndRequest(
	params NewServerParams,
) func(ctx context.Context, network, addr string, request *socks5.Request) (net.Conn, error) {
	dialer := &net.Dialer{}

	return proxyadapter.NewDialAndRequest(dialer, func(request *socks5.Request, conn net.Conn) net.Conn {
		return wrapConnection(params, request, conn)
	})
}

func wrapConnection(params NewServerParams, request *socks5.Request, conn net.Conn) net.Conn {
	observeConnectionOpened(params.Metrics)

	onClose := func() {
		observeConnectionClosed(params.Metrics)
	}

	username, ok := UsernameFromRequest(request)
	if !ok {
		return buildConnWithoutUsername(params.Metrics, conn, onClose)
	}

	return buildConnWithUsername(params, username, conn, onClose)
}

func observeConnectionOpened(metricsObserver metricsObserver) {
	if metricsObserver == nil {
		return
	}

	metricsObserver.ObserveConnectionOpened()
}

func observeConnectionClosed(metricsObserver metricsObserver) {
	if metricsObserver == nil {
		return
	}

	metricsObserver.ObserveConnectionClosed()
}

func buildConnWithoutUsername(metricsObserver metricsObserver, conn net.Conn, onClose func()) net.Conn {
	if metricsObserver == nil {
		return conn
	}

	return proxyadapter.NewUsageTrackedConnWithClose(conn, nil, onClose)
}

func buildConnWithUsername(
	params NewServerParams,
	username string,
	conn net.Conn,
	onClose func(),
) net.Conn {
	onData := func(dataLen int64) {
		onProxyData(params, username, dataLen)
	}

	return proxyadapter.NewUsageTrackedConnWithClose(conn, onData, onClose)
}

func onProxyData(params NewServerParams, username string, dataLen int64) {
	if params.Metrics != nil {
		params.Metrics.ObserveProxyTrafficBytes(dataLen)
		params.Metrics.ObserveProxyTrafficBytesByUsername(username, dataLen)
	}

	if params.UpdatesManager == nil {
		return
	}

	if queued := params.UpdatesManager.EnqueueUsageUpdate(username, dataLen); queued {
		return
	}

	if params.Metrics != nil {
		params.Metrics.ObserveDroppedRedisUpdate(metrics.QueueUsage)
	}

	log.Warn(
		context.Background(),
		"failed to enqueue data usage update for user",
		log.String(log.FieldUsername, username),
	)
}

func buildAuthOption(params NewServerParams) socks5.Option {
	validator := instrumentAuthValidator(
		params.Metrics,
		AuthValidator(proxyadapter.NewAuth(params.PasswordHashGetter, params.PasswordComparator)),
	)

	authCredentialValidator := proxyadapter.NewAuthWithCache(
		cache.NewExpirableLRU[proxyadapter.AuthCacheKey, bool](
			params.Config.AuthCacheMaxSize,
			params.Config.AuthCacheTTL,
		),
		validator,
		func(user string) {
			onSuccessfulAuth(params, user)
		},
	)

	return socks5.WithCredential(authCredentialValidator)
}

func instrumentAuthValidator(metricsObserver metricsObserver, validator AuthValidator) AuthValidator {
	if metricsObserver == nil || validator == nil {
		return validator
	}

	return &instrumentedAuthValidator{
		validator: validator,
		metrics:   metricsObserver,
	}
}

type instrumentedAuthValidator struct {
	validator AuthValidator
	metrics   metricsObserver
}

func (v *instrumentedAuthValidator) Valid(user, password, userAddr string) bool {
	valid := v.validator.Valid(user, password, userAddr)
	v.metrics.ObserveAuthAttempt(valid)

	return valid
}

func onSuccessfulAuth(params NewServerParams, user string) {
	if params.UpdatesManager == nil {
		return
	}

	if queued := params.UpdatesManager.EnqueueLastAuthDateUpdate(user); queued {
		return
	}

	if params.Metrics != nil {
		params.Metrics.ObserveDroppedRedisUpdate(metrics.QueueAuth)
	}

	log.Warn(
		context.Background(),
		"failed to enqueue auth date update for user",
		log.String(log.FieldUsername, user),
	)
}

func UsernameFromRequest(request *socks5.Request) (string, bool) {
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
