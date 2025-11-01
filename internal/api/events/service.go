package events

import (
	"context"
	"errors"
	"log/slog"

	"github.com/iw2rmb/ploy/internal/node/logstream"
)

// Options configures the events service.
type Options struct {
	// BufferSize controls the per-subscriber channel size.
	BufferSize int
	// HistorySize bounds the number of events retained for resumption.
	HistorySize int
	// Logger for service diagnostics (optional).
	Logger *slog.Logger
}

// Service wraps the logstream hub for server-side event streaming.
type Service struct {
	hub    *logstream.Hub
	logger *slog.Logger
}

// New constructs a new events service.
func New(opts Options) (*Service, error) {
	if opts.BufferSize < 0 {
		return nil, errors.New("events: buffer size must be non-negative")
	}
	if opts.HistorySize < 0 {
		return nil, errors.New("events: history size must be non-negative")
	}

	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	hub := logstream.NewHub(logstream.Options{
		BufferSize:  opts.BufferSize,
		HistorySize: opts.HistorySize,
	})

	return &Service{
		hub:    hub,
		logger: logger,
	}, nil
}

// Hub returns the underlying logstream hub.
func (s *Service) Hub() *logstream.Hub {
	return s.hub
}

// Start is a no-op for now; the hub is ready immediately.
func (s *Service) Start(ctx context.Context) error {
	s.logger.Info("events service started")
	return nil
}

// Stop gracefully stops the events service.
func (s *Service) Stop(ctx context.Context) error {
	s.logger.Info("events service stopped")
	return nil
}
