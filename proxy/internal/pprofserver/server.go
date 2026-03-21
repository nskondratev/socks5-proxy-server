package pprofserver

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	httppprof "net/http/pprof"
	"strconv"
	"time"
)

const shutdownTimeout = 5 * time.Second

type Config struct {
	Enabled      bool
	Port         int
	AuthEnabled  bool
	AuthUsername string
	AuthPassword string
}

type Server struct {
	config Config
	server *http.Server
}

func New(config Config) (*Server, error) {
	s := &Server{
		config: config,
	}

	if !config.Enabled {
		return s, nil
	}

	if config.Port <= 0 || config.Port > 65535 {
		return nil, fmt.Errorf("invalid pprof port: %d", config.Port)
	}

	if config.AuthEnabled && config.AuthUsername == "" {
		return nil, errors.New("pprof auth username is required")
	}

	if config.AuthEnabled && config.AuthPassword == "" {
		return nil, errors.New("pprof auth password is required")
	}

	handler := pprofMux()
	if config.AuthEnabled {
		handler = basicAuth(config.AuthUsername, config.AuthPassword, handler)
	}

	s.server = &http.Server{
		Addr:              ":" + strconv.Itoa(config.Port),
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	return s, nil
}

func (s *Server) Enabled() bool {
	return s != nil && s.config.Enabled
}

func (s *Server) Run(ctx context.Context) error {
	if !s.Enabled() {
		return nil
	}

	errCh := make(chan error, 1)

	go func() {
		if err := s.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
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

		if err := s.server.Shutdown(shutdownCtx); err != nil {
			return err
		}

		if err := <-errCh; err != nil {
			return err
		}

		return nil
	}
}

func pprofMux() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/debug/pprof/", httppprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", httppprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", httppprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", httppprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", httppprof.Trace)
	mux.Handle("/debug/pprof/allocs", httppprof.Handler("allocs"))
	mux.Handle("/debug/pprof/block", httppprof.Handler("block"))
	mux.Handle("/debug/pprof/goroutine", httppprof.Handler("goroutine"))
	mux.Handle("/debug/pprof/heap", httppprof.Handler("heap"))
	mux.Handle("/debug/pprof/mutex", httppprof.Handler("mutex"))
	mux.Handle("/debug/pprof/threadcreate", httppprof.Handler("threadcreate"))

	return mux
}

func basicAuth(username, password string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedUsername, receivedPassword, ok := r.BasicAuth()
		if !ok || !secureCompare(receivedUsername, username) || !secureCompare(receivedPassword, password) {
			w.Header().Set("WWW-Authenticate", `Basic realm="pprof"`)
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)

			return
		}

		next.ServeHTTP(w, r)
	})
}

func secureCompare(actual, expected string) bool {
	return subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) == 1
}
