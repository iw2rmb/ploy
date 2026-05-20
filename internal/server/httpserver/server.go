package httpserver

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

const (
	// readHeaderTimeout caps how long the server waits for request headers
	// after accepting a connection. Protects against slowloris-style attacks.
	readHeaderTimeout = 10 * time.Second

	// shutdownTimeout bounds the graceful shutdown drain period.
	shutdownTimeout = 10 * time.Second
)

// Options configure the HTTP server.
type Options struct {
	Config     config.HTTPConfig
	Authorizer *auth.Authorizer
}

// Server manages the main API HTTP server with bearer token authentication.
type Server struct {
	mu         sync.Mutex
	cfg        config.HTTPConfig
	authorizer *auth.Authorizer
	httpServer *http.Server
	listener   net.Listener
	running    bool
	mux        *http.ServeMux
}

// NewServer constructs a new HTTP server.
func NewServer(opts Options) (*Server, error) {
	if opts.Authorizer == nil {
		return nil, errors.New("httpserver: authorizer is required")
	}

	return &Server{
		cfg:        opts.Config,
		authorizer: opts.Authorizer,
		mux:        http.NewServeMux(),
	}, nil
}

// Start begins serving HTTP(S) requests.
func (s *Server) Start(ctx context.Context) error {
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
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       s.cfg.ReadTimeout,
		WriteTimeout:      s.cfg.WriteTimeout,
		IdleTimeout:       s.cfg.IdleTimeout,
		// Propagate the provided context to all connections/requests.
		BaseContext: func(net.Listener) context.Context { return ctx },
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
func (s *Server) Stop(ctx context.Context) error {
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
		shutdownCtx, cancel := context.WithTimeout(ctx, shutdownTimeout)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("httpserver: shutdown: %w", err)
		}
	}

	slog.Info("httpserver stopped")
	return nil
}

// Addr returns the listener address if the server is running.
func (s *Server) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener != nil {
		return s.listener.Addr().String()
	}
	return s.cfg.Listen
}

// handle is the internal registration method. It applies panic recovery, auth
// middleware, and optional pre-auth wrappers in the correct order:
//
//	panicRecovery -> preAuth wrappers -> auth -> handler
func (s *Server) handle(pattern string, handler http.Handler, roles []auth.Role, preAuth ...func(http.Handler) http.Handler) {
	if len(roles) > 0 {
		handler = s.authorizer.Middleware(roles...)(handler)
	}
	for i := len(preAuth) - 1; i >= 0; i-- {
		handler = preAuth[i](handler)
	}
	handler = withPanicRecovery(handler)
	s.mux.Handle(pattern, handler)
}

// RegisterRoute registers a handler for the given pattern with optional middleware.
// The authorizer middleware will be applied if roles are provided.
func (s *Server) RegisterRoute(pattern string, handler http.Handler, roles ...auth.Role) {
	s.handle(pattern, handler, roles)
}

// RegisterRouteFunc registers a handler function for the given pattern with optional middleware.
// The authorizer middleware will be applied if roles are provided.
func (s *Server) RegisterRouteFunc(pattern string, handlerFunc http.HandlerFunc, roles ...auth.Role) {
	s.handle(pattern, handlerFunc, roles)
}

// RegisterRouteFuncAllowQueryToken registers a handler that also accepts auth_token
// query parameters for authentication. This is intended for GET endpoints
// serving downloadable content (logs, diffs, specs) where browser/OSC8
// links cannot set Authorization headers.
func (s *Server) RegisterRouteFuncAllowQueryToken(pattern string, handlerFunc http.HandlerFunc, roles ...auth.Role) {
	s.handle(pattern, handlerFunc, roles, auth.WithQueryTokenAllowed)
}

// Handler returns the underlying HTTP handler (ServeMux) used by the server.
// This is primarily intended for tests that need to exercise the registered
// routes without starting a real listener.
func (s *Server) Handler() http.Handler {
	return s.mux
}

// listen creates a plain TCP listener.
// TLS termination is handled by the load balancer.
func (s *Server) listen(ctx context.Context) (net.Listener, error) {
	address := s.cfg.Listen
	if address == "" {
		address = ":8080"
	}

	lc := net.ListenConfig{}
	return lc.Listen(ctx, "tcp", address)
}
