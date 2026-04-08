package nodeagent

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
	"strings"
	"sync"
	"time"
)

// RunController manages run lifecycle on the node.
type RunController interface {
	StartRun(ctx context.Context, req StartRunRequest) error
	StartAction(ctx context.Context, req StartActionRequest) error
	StopRun(ctx context.Context, req StopRunRequest) error

	// AcquireSlot blocks until a concurrency slot is available or the context
	// is canceled. Returns nil when a slot is acquired, or ctx.Err() if the
	// context was canceled while waiting.
	//
	// Slot ownership:
	//   - On StartRun success (nil error), the controller is responsible for
	//     releasing the slot when the job completes.
	//   - On StartRun failure (non-nil error), the caller must ReleaseSlot()
	//     before returning.
	AcquireSlot(ctx context.Context) error

	// ReleaseSlot frees a previously acquired concurrency slot.
	// Must be called exactly once for each successful AcquireSlot call.
	ReleaseSlot()
}

// Server exposes the node agent API over HTTPS with mTLS.
type Server struct {
	mu         sync.Mutex
	cfg        Config
	controller RunController
	httpServer *http.Server
	listener   net.Listener
	running    bool
}

// NewServer constructs a new node agent HTTP server.
func NewServer(cfg Config, controller RunController) (*Server, error) {
	if controller == nil {
		return nil, errors.New("nodeagent: run controller is required")
	}

	s := &Server{
		cfg:        cfg,
		controller: controller,
	}

	return s, nil
}

// Start begins serving HTTPS requests.
func (s *Server) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return errors.New("nodeagent: server already running")
	}

	listener, err := s.listen()
	if err != nil {
		s.mu.Unlock()
		return err
	}

	mux := http.NewServeMux()
	s.mountRoutes(mux)

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       s.cfg.HTTP.ReadTimeout,
		WriteTimeout:      s.cfg.HTTP.WriteTimeout,
		IdleTimeout:       s.cfg.HTTP.IdleTimeout,
	}

	s.httpServer = srv
	s.listener = listener
	s.running = true
	s.mu.Unlock()

	go func() {
		if err := srv.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("http server stopped", "err", err)
		}
	}()

	return nil
}

// Stop terminates the HTTP server.
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
		shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("nodeagent: shutdown: %w", err)
		}
	}

	return nil
}

// Address returns the bound listener address if the server is running.
func (s *Server) Address() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener != nil {
		return s.listener.Addr().String()
	}
	return ""
}

func (s *Server) listen() (net.Listener, error) {
	address := strings.TrimSpace(s.cfg.HTTP.Listen)
	if address == "" {
		address = ":8444"
	}

	if !s.cfg.HTTP.TLS.Enabled {
		return net.Listen("tcp", address)
	}

	// Load node certificate and key.
	cert, err := tls.LoadX509KeyPair(s.cfg.HTTP.TLS.CertPath, s.cfg.HTTP.TLS.KeyPath)
	if err != nil {
		return nil, fmt.Errorf("load certificate: %w", err)
	}

	// Load CA certificate for client verification.
	caData, err := os.ReadFile(s.cfg.HTTP.TLS.CAPath)
	if err != nil {
		return nil, fmt.Errorf("load ca certificate: %w", err)
	}

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caData) {
		return nil, errors.New("failed to parse ca certificate")
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    caCertPool,
		MinVersion:   tls.VersionTLS13,
	}

	ln, err := tls.Listen("tcp", address, tlsConfig)
	if err != nil {
		return nil, fmt.Errorf("listen: %w", err)
	}

	return ln, nil
}

func (s *Server) mountRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/v1/run/start", s.handleRunStart)
	mux.HandleFunc("/v1/run/stop", s.handleRunStop)
	mux.HandleFunc("/health", s.handleHealth)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}
