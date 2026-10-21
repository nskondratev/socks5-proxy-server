package metrics

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	QueueAuth = "auth"

	metricsPath     = "/metrics"
	shutdownTimeout = 5 * time.Second
)

type Config struct {
	Enabled      bool
	Port         int
	AuthEnabled  bool
	AuthUsername string
	AuthPassword string
}

type authValidator interface {
	Valid(user, password, userAddr string) bool
}

type Metrics struct {
	config Config
	server *http.Server

	proxyTransferredBytes           prometheus.Counter
	proxyTransferredBytesByUsername *prometheus.CounterVec
	authAttempts                    *prometheus.CounterVec
	droppedRedisUpdates             *prometheus.CounterVec
	newConnections                  prometheus.Counter
	closedConnections               prometheus.Counter
	activeConnections               prometheus.Gauge
}

func New(config Config) (*Metrics, error) {
	metrics := &Metrics{
		config: config,
	}

	if !config.Enabled {
		return metrics, nil
	}

	if config.Port <= 0 || config.Port > 65535 {
		return nil, fmt.Errorf("invalid metrics port: %d", config.Port)
	}

	if config.AuthEnabled && config.AuthUsername == "" {
		return nil, errors.New("metrics auth username is required")
	}

	if config.AuthEnabled && config.AuthPassword == "" {
		return nil, errors.New("metrics auth password is required")
	}

	registry := prometheus.NewRegistry()
	registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	auto := promauto.With(registry)
	metrics.proxyTransferredBytes = auto.NewCounter(prometheus.CounterOpts{
		Name: "proxy_transferred_bytes_total",
		Help: "Total amount of bytes transferred through the proxy.",
	})
	metrics.proxyTransferredBytesByUsername = auto.NewCounterVec(prometheus.CounterOpts{
		Name: "proxy_transferred_bytes_by_username_total",
		Help: "Total amount of bytes transferred through the proxy by username.",
	}, []string{"username"})
	metrics.authAttempts = auto.NewCounterVec(prometheus.CounterOpts{
		Name: "proxy_auth_attempts_total",
		Help: "Total amount of authentication attempts by result.",
	}, []string{"result"})
	metrics.droppedRedisUpdates = auto.NewCounterVec(prometheus.CounterOpts{
		Name: "proxy_redis_updates_dropped_total",
		Help: "Total amount of dropped async Redis updates by queue.",
	}, []string{"queue"})
	metrics.newConnections = auto.NewCounter(prometheus.CounterOpts{
		Name: "proxy_connections_opened_total",
		Help: "Total amount of new proxy connections.",
	})
	metrics.closedConnections = auto.NewCounter(prometheus.CounterOpts{
		Name: "proxy_connections_closed_total",
		Help: "Total amount of closed proxy connections.",
	})
	metrics.activeConnections = auto.NewGauge(prometheus.GaugeOpts{
		Name: "proxy_connections_active",
		Help: "Current amount of active proxy connections.",
	})

	handler := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
	if config.AuthEnabled {
		handler = basicAuth(config.AuthUsername, config.AuthPassword, handler)
	}

	mux := http.NewServeMux()
	mux.Handle(metricsPath, handler)

	metrics.server = &http.Server{
		Addr:              ":" + strconv.Itoa(config.Port),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	return metrics, nil
}

func (m *Metrics) Enabled() bool {
	return m != nil && m.config.Enabled
}

func (m *Metrics) Run(ctx context.Context) error {
	if !m.Enabled() {
		return nil
	}

	errCh := make(chan error, 1)

	go func() {
		if err := m.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}

		close(errCh)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
		defer cancel()

		if err := m.server.Shutdown(shutdownCtx); err != nil {
			return err
		}

		if err := <-errCh; err != nil {
			return err
		}

		return nil
	}
}

func (m *Metrics) ObserveProxyTrafficBytes(dataLen int64) {
	if !m.Enabled() || dataLen <= 0 {
		return
	}

	m.proxyTransferredBytes.Add(float64(dataLen))
}

func (m *Metrics) ObserveProxyTrafficBytesByUsername(username string, dataLen int64) {
	if !m.Enabled() || username == "" || dataLen <= 0 {
		return
	}

	m.proxyTransferredBytesByUsername.WithLabelValues(username).Add(float64(dataLen))
}

func (m *Metrics) ObserveDroppedRedisUpdate(queue string) {
	if !m.Enabled() || queue == "" {
		return
	}

	m.droppedRedisUpdates.WithLabelValues(queue).Inc()
}

func (m *Metrics) ObserveAuthAttempt(valid bool) {
	if !m.Enabled() {
		return
	}

	label := "failure"
	if valid {
		label = "success"
	}

	m.authAttempts.WithLabelValues(label).Inc()
}

type instrumentedAuthValidator struct {
	validator authValidator
	metrics   *Metrics
}

func (m *Metrics) InstrumentAuthValidator(validator authValidator) authValidator {
	if !m.Enabled() || validator == nil {
		return validator
	}

	return &instrumentedAuthValidator{
		validator: validator,
		metrics:   m,
	}
}

func (v *instrumentedAuthValidator) Valid(user, password, userAddr string) bool {
	valid := v.validator.Valid(user, password, userAddr)
	v.metrics.ObserveAuthAttempt(valid)

	return valid
}

func (m *Metrics) ObserveConnectionOpened() {
	if !m.Enabled() {
		return
	}

	m.newConnections.Inc()
	m.activeConnections.Inc()
}

func (m *Metrics) ObserveConnectionClosed() {
	if !m.Enabled() {
		return
	}

	m.closedConnections.Inc()
	m.activeConnections.Dec()
}

func basicAuth(username, password string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedUsername, receivedPassword, ok := r.BasicAuth()
		if !ok || !secureCompare(receivedUsername, username) || !secureCompare(receivedPassword, password) {
			w.Header().Set("WWW-Authenticate", `Basic realm="metrics"`)
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)

			return
		}

		next.ServeHTTP(w, r)
	})
}

func secureCompare(actual, expected string) bool {
	return subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) == 1
}
