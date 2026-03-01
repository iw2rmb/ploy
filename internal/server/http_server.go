package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"runtime"
	"runtime/debug"
	"sync"
	"time"

	"github.com/iw2rmb/ploy/internal/server/auth"
	"github.com/iw2rmb/ploy/internal/server/config"
)

const (
	// httpReadHeaderTimeout caps how long the server waits for request headers
	// after accepting a connection. Protects against slowloris-style attacks.
	httpReadHeaderTimeout = 10 * time.Second

	// httpShutdownTimeout bounds the graceful shutdown drain period.
	httpShutdownTimeout = 10 * time.Second
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
		ReadHeaderTimeout: httpReadHeaderTimeout,
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
		shutdownCtx, cancel := context.WithTimeout(ctx, httpShutdownTimeout)
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

// handle is the internal registration method. It applies panic recovery, auth
// middleware, and optional pre-auth wrappers in the correct order:
//
//	panicRecovery → preAuth wrappers → auth → handler
func (s *HTTPServer) handle(pattern string, handler http.Handler, roles []auth.Role, preAuth ...func(http.Handler) http.Handler) {
	if len(roles) > 0 {
		handler = s.authorizer.Middleware(roles...)(handler)
	}
	for i := len(preAuth) - 1; i >= 0; i-- {
		handler = preAuth[i](handler)
	}
	handler = withPanicRecovery(handler)
	s.mux.Handle(pattern, handler)
}

// Handle registers a handler for the given pattern with optional middleware.
// The authorizer middleware will be applied if roles are provided.
func (s *HTTPServer) Handle(pattern string, handler http.Handler, roles ...auth.Role) {
	s.handle(pattern, handler, roles)
}

// HandleFunc registers a handler function for the given pattern with optional middleware.
// The authorizer middleware will be applied if roles are provided.
func (s *HTTPServer) HandleFunc(pattern string, handlerFunc http.HandlerFunc, roles ...auth.Role) {
	s.handle(pattern, handlerFunc, roles)
}

// HandleFuncAllowQueryToken registers a handler that also accepts auth_token
// query parameters for authentication. This is intended for GET endpoints
// serving downloadable content (logs, diffs, specs) where browser/OSC8
// links cannot set Authorization headers.
func (s *HTTPServer) HandleFuncAllowQueryToken(pattern string, handlerFunc http.HandlerFunc, roles ...auth.Role) {
	s.handle(pattern, handlerFunc, roles, auth.WithQueryTokenAllowed)
}

// Handler returns the underlying HTTP handler (ServeMux) used by the server.
// This is primarily intended for tests that need to exercise the registered
// routes without starting a real listener.
func (s *HTTPServer) Handler() http.Handler {
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

func withPanicRecovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				slog.Error("http handler panic recovered",
					"method", r.Method,
					"path", r.URL.Path,
					"panic_type", fmt.Sprintf("%T", recovered),
					"panic_message", panicMessageSafe(recovered),
					"stack", string(debug.Stack()),
				)
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()

		next.ServeHTTP(w, r)
	})
}

func panicMessageSafe(recovered any) (msg string) {
	switch v := recovered.(type) {
	case string:
		return v
	case runtime.Error:
		return "runtime panic"
	case error:
		defer func() {
			if panicErr := recover(); panicErr != nil {
				msg = "error string panicked"
			}
		}()
		return v.Error()
	default:
		return fmt.Sprintf("%T", recovered)
	}
}
