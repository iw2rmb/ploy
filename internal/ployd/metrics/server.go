package metrics

import (
	"context"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/iw2rmb/ploy/internal/ployd/config"
)

// Options configure the metrics server.
type Options struct {
	Listen string
}

// Server exposes Prometheus metrics.
type Server struct {
	mu       sync.Mutex
	cfg      config.MetricsConfig
	listen   string
	server   *http.Server
	listener net.Listener
	running  bool
	addr     string
	ctx      context.Context
	parent   context.Context
}

// New constructs the metrics server.
func New(opts Options) *Server {
	listen := opts.Listen
	if listen == "" {
		listen = ":9100"
	}
	return &Server{
		cfg:    config.MetricsConfig{Listen: listen},
		listen: listen,
	}
}

// Start begins serving metrics.
func (s *Server) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return nil
	}
	listen := s.cfg.Listen
	if listen == "" {
		listen = ":9100"
	}
	ln, err := net.Listen("tcp", listen)
	if err != nil {
		return err
	}
	server := &http.Server{Handler: promhttp.Handler()}
	s.listener = ln
	s.server = server
	s.addr = ln.Addr().String()
	s.running = true
	s.parent = ctx
	go func() {
		_ = server.Serve(ln)
	}()
	return nil
}

// Stop stops the metrics server.
func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return nil
	}
	server := s.server
	listener := s.listener
	s.running = false
	s.server = nil
	s.listener = nil
	s.addr = ""
	s.mu.Unlock()

	if listener != nil {
		_ = listener.Close()
	}
	if server != nil {
		shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return err
		}
	}
	return nil
}

// Reload applies new configuration.
func (s *Server) Reload(ctx context.Context, cfg config.MetricsConfig) error {
	s.mu.Lock()
	s.cfg = cfg
	s.mu.Unlock()
	if s.running {
		if err := s.Stop(ctx); err != nil {
			return err
		}
		return s.Start(s.parent)
	}
	return nil
}

// Addr returns the listener address if running.
func (s *Server) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.addr != "" {
		return s.addr
	}
	return s.cfg.Listen
}

// Config returns the current metrics configuration.
func (s *Server) Config() config.MetricsConfig {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg
}
