package server

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/iw2rmb/ploy/internal/server/config"
)

const (
	// metricsReadHeaderTimeout caps header read time on the metrics endpoint.
	metricsReadHeaderTimeout = 5 * time.Second
	metricsReadTimeout       = 10 * time.Second
	metricsWriteTimeout      = 10 * time.Second
	metricsIdleTimeout       = 60 * time.Second
	metricsShutdownTimeout   = 5 * time.Second
)

// MetricsServer exposes Prometheus metrics.
type MetricsServer struct {
	mu      sync.Mutex
	cfg     config.MetricsConfig
	server  *http.Server
	running bool
	addr    string
}

// NewMetricsServer constructs the metrics server.
func NewMetricsServer(listen string) *MetricsServer {
	return &MetricsServer{
		cfg: config.MetricsConfig{Listen: listen},
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
	lc := net.ListenConfig{}
	ln, err := lc.Listen(ctx, "tcp", listen)
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: metricsReadHeaderTimeout,
		ReadTimeout:       metricsReadTimeout,
		WriteTimeout:      metricsWriteTimeout,
		IdleTimeout:       metricsIdleTimeout,
	}
	s.server = server
	s.addr = ln.Addr().String()
	s.running = true
	go func() {
		if err := server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
			slog.Error("metrics server stopped unexpectedly", "err", err)
		}
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
	s.running = false
	s.server = nil
	s.addr = ""
	s.mu.Unlock()

	if server != nil {
		shutdownCtx, cancel := context.WithTimeout(ctx, metricsShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
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
