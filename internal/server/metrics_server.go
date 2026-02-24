package server

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/iw2rmb/ploy/internal/server/config"
)

// MetricsOptions configure the metrics server.
type MetricsOptions struct {
	Listen string
}

// MetricsServer exposes Prometheus metrics.
type MetricsServer struct {
	mu       sync.Mutex
	cfg      config.MetricsConfig
	listen   string
	server   *http.Server
	listener net.Listener
	running  bool
	addr     string
	parent   context.Context
}

// NewMetricsServer constructs the metrics server.
func NewMetricsServer(opts MetricsOptions) *MetricsServer {
	listen := opts.Listen
	if listen == "" {
		listen = ":9100"
	}
	return &MetricsServer{
		cfg:    config.MetricsConfig{Listen: listen},
		listen: listen,
	}
}

// Start begins serving metrics.
func (s *MetricsServer) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return nil
	}
	listen := s.cfg.Listen
	if listen == "" {
		listen = ":9100"
	}
	// Use ListenConfig with context to satisfy lint/noctx and enable cancellation.
	lc := net.ListenConfig{}
	ln, err := lc.Listen(ctx, "tcp", listen)
	if err != nil {
		return err
	}
	// Expose Prometheus metrics strictly under /metrics.
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
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
func (s *MetricsServer) Stop(ctx context.Context) error {
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
		if err := server.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			// Some platforms return a net.OpError wrapping "use of closed network connection"
			// when the listener has already been closed. Treat it as benign.
			if !strings.Contains(err.Error(), "use of closed network connection") {
				return err
			}
		}
	}
	return nil
}

// Reload applies new configuration.
func (s *MetricsServer) Reload(ctx context.Context, cfg config.MetricsConfig) error {
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
func (s *MetricsServer) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.addr != "" {
		return s.addr
	}
	return s.cfg.Listen
}

// MetricsConfig returns the current metrics configuration.
func (s *MetricsServer) MetricsConfig() config.MetricsConfig {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg
}
