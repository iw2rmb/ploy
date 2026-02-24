package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/iw2rmb/ploy/internal/server/auth"
	"github.com/iw2rmb/ploy/internal/server/config"
)

// HTTPOptions configure the HTTP server.
type HTTPOptions struct {
	Config     config.HTTPConfig
	Authorizer *auth.Authorizer
}

// HTTPServer manages the main API HTTP server with bearer token authentication.
type HTTPServer struct {
	mu         sync.Mutex
	cfg        config.HTTPConfig
	authorizer *auth.Authorizer
	httpServer *http.Server
	listener   net.Listener
	running    bool
	mux        *http.ServeMux
}

// NewHTTPServer constructs a new HTTP server.
func NewHTTPServer(opts HTTPOptions) (*HTTPServer, error) {
	if opts.Authorizer == nil {
		return nil, errors.New("httpserver: authorizer is required")
	}

	return &HTTPServer{
		cfg:        opts.Config,
		authorizer: opts.Authorizer,
		mux:        http.NewServeMux(),
	}, nil
}

// Start begins serving HTTP(S) requests.
func (s *HTTPServer) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return errors.New("httpserver: server already running")
	}

	listener, err := s.listen(ctx)
	if err != nil {
		s.mu.Unlock()
		return err
	}

	srv := &http.Server{
		Handler:           s.mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       s.cfg.ReadTimeout,
		WriteTimeout:      s.cfg.WriteTimeout,
		IdleTimeout:       s.cfg.IdleTimeout,
		// Propagate the provided context to all connections/requests.
		BaseContext: func(net.Listener) context.Context { return ctx },
	}

	// Apply sensible defaults if not configured.
	if srv.ReadTimeout == 0 {
		srv.ReadTimeout = 30 * time.Second
	}
	if srv.WriteTimeout == 0 {
		srv.WriteTimeout = 30 * time.Second
	}
	if srv.IdleTimeout == 0 {
		srv.IdleTimeout = 120 * time.Second
	}

	s.httpServer = srv
	s.listener = listener
	s.running = true
	s.mu.Unlock()

	slog.Info("httpserver started", "addr", listener.Addr().String())

	go func() {
		if err := srv.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			// Avoid logging noise on normal shutdown; Shutdown() causes ErrServerClosed.
			// Some platforms may return net.ErrClosed; treat both as expected.
			if !errors.Is(err, net.ErrClosed) {
				slog.Error("httpserver stopped unexpectedly", "err", err)
			}
		}
	}()

	return nil
}

// Stop gracefully stops the HTTP server.
func (s *HTTPServer) Stop(ctx context.Context) error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return nil
	}

	srv := s.httpServer
	s.httpServer = nil
	s.listener = nil
	s.running = false
	s.mu.Unlock()

	if srv != nil {
		shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("httpserver: shutdown: %w", err)
		}
	}

	slog.Info("httpserver stopped")
	return nil
}

// Addr returns the listener address if the server is running.
func (s *HTTPServer) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener != nil {
		return s.listener.Addr().String()
	}
	return s.cfg.Listen
}

// Handle registers a handler for the given pattern with optional middleware.
// The authorizer middleware will be applied if roles are provided.
func (s *HTTPServer) Handle(pattern string, handler http.Handler, roles ...auth.Role) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(roles) > 0 {
		handler = s.authorizer.Middleware(roles...)(handler)
	}

	s.mux.Handle(pattern, handler)
}

// HandleFunc registers a handler function for the given pattern with optional middleware.
// The authorizer middleware will be applied if roles are provided.
func (s *HTTPServer) HandleFunc(pattern string, handlerFunc http.HandlerFunc, roles ...auth.Role) {
	s.Handle(pattern, handlerFunc, roles...)
}

// Handler returns the underlying HTTP handler (ServeMux) used by the server.
// This is primarily intended for tests that need to exercise the registered
// routes without starting a real listener.
func (s *HTTPServer) Handler() http.Handler {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mux
}

// listen creates a plain TCP listener.
// TLS termination is handled by the load balancer.
func (s *HTTPServer) listen(ctx context.Context) (net.Listener, error) {
	address := s.cfg.Listen
	if address == "" {
		address = ":8080"
	}

	lc := net.ListenConfig{}
	return lc.Listen(ctx, "tcp", address)
}
