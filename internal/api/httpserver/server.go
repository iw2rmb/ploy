package httpserver

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/iw2rmb/ploy/internal/api/config"
	"github.com/iw2rmb/ploy/internal/controlplane/auth"
)

// Options configure the HTTP server.
type Options struct {
	Config     config.HTTPConfig
	Authorizer *auth.Authorizer
}

// Server manages the main API HTTP server with TLS/mTLS support.
type Server struct {
	mu         sync.Mutex
	cfg        config.HTTPConfig
	authorizer *auth.Authorizer
	httpServer *http.Server
	listener   net.Listener
	running    bool
	mux        *http.ServeMux
}

// New constructs a new HTTP server.
func New(opts Options) (*Server, error) {
	if opts.Authorizer == nil {
		return nil, errors.New("httpserver: authorizer is required")
	}

	return &Server{
		cfg:        opts.Config,
		authorizer: opts.Authorizer,
		mux:        http.NewServeMux(),
	}, nil
}

// Start begins serving HTTPS requests.
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
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       s.cfg.ReadTimeout,
		WriteTimeout:      s.cfg.WriteTimeout,
		IdleTimeout:       s.cfg.IdleTimeout,
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

	slog.Info("httpserver started", "addr", listener.Addr().String(), "tls", s.cfg.TLS.Enabled)

	go func() {
		if err := srv.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("httpserver stopped unexpectedly", "err", err)
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
	listener := s.listener
	s.httpServer = nil
	s.listener = nil
	s.running = false
	s.mu.Unlock()

	if listener != nil {
		_ = listener.Close()
	}

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
func (s *Server) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener != nil {
		return s.listener.Addr().String()
	}
	return s.cfg.Listen
}

// Handle registers a handler for the given pattern with optional middleware.
// The authorizer middleware will be applied if roles are provided.
func (s *Server) Handle(pattern string, handler http.Handler, roles ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(roles) > 0 {
		handler = s.authorizer.Middleware(roles...)(handler)
	}

	s.mux.Handle(pattern, handler)
}

// HandleFunc registers a handler function for the given pattern with optional middleware.
// The authorizer middleware will be applied if roles are provided.
func (s *Server) HandleFunc(pattern string, handlerFunc http.HandlerFunc, roles ...string) {
	s.Handle(pattern, handlerFunc, roles...)
}

// listen creates a TCP or TLS listener based on configuration.
func (s *Server) listen(ctx context.Context) (net.Listener, error) {
	address := s.cfg.Listen
	if address == "" {
		address = ":8443"
	}

	if !s.cfg.TLS.Enabled {
		lc := net.ListenConfig{}
		return lc.Listen(ctx, "tcp", address)
	}

	// Load server certificate and key.
	cert, err := tls.LoadX509KeyPair(s.cfg.TLS.CertPath, s.cfg.TLS.KeyPath)
	if err != nil {
		return nil, fmt.Errorf("load certificate: %w", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS13,
	}

	// Configure client certificate verification if required.
	if s.cfg.TLS.RequireClientCert {
		if s.cfg.TLS.ClientCAPath == "" {
			return nil, errors.New("httpserver: client_ca path required when require_client_cert is true")
		}

		caData, err := os.ReadFile(s.cfg.TLS.ClientCAPath)
		if err != nil {
			return nil, fmt.Errorf("load client ca certificate: %w", err)
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caData) {
			return nil, errors.New("httpserver: failed to parse client ca certificate")
		}

		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
		tlsConfig.ClientCAs = caCertPool
	}

	ln, err := tls.Listen("tcp", address, tlsConfig)
	if err != nil {
		return nil, fmt.Errorf("listen tls: %w", err)
	}

	return ln, nil
}
