package events

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	modsapi "github.com/iw2rmb/ploy/internal/mods/api"
	"github.com/iw2rmb/ploy/internal/store"
	logstream "github.com/iw2rmb/ploy/internal/stream"
	"github.com/jackc/pgx/v5/pgtype"
)

// Options configures the events service.
type Options struct {
	// BufferSize controls the per-subscriber channel size.
	BufferSize int
	// HistorySize bounds the number of events retained for resumption.
	HistorySize int
	// Logger for service diagnostics (optional).
	Logger *slog.Logger
	// Store for database persistence (optional; if nil, persistence methods will fail).
	Store store.Store
}

// Service wraps the logstream hub for server-side event streaming
// and coordinates database persistence with SSE fanout.
type Service struct {
	hub    *logstream.Hub
	store  store.Store
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
		store:  opts.Store,
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

// CreateAndPublishEvent persists an event to the database and publishes it to the SSE hub.
// The runID is used as the streamID for SSE fanout.
// Returns the created event from the database. If persistence fails, an error
// is returned; SSE fanout errors are logged but do not fail the operation.
func (s *Service) CreateAndPublishEvent(ctx context.Context, params store.CreateEventParams) (store.Event, error) {
	if s.store == nil {
		return store.Event{}, errors.New("events: store not configured")
	}

	// Persist to database first.
	event, err := s.store.CreateEvent(ctx, params)
	if err != nil {
		return store.Event{}, fmt.Errorf("persist event: %w", err)
	}

	// Convert runID to string for streamID.
	streamID := uuidToString(params.RunID)
	if streamID == "" {
		// DB succeeded but SSE fanout skipped; log and return event.
		s.logger.Warn("event persisted but runID invalid for SSE fanout", "event_id", event.ID)
		return event, nil
	}

	// Fan out to SSE hub.
	if err := s.publishEventToHub(ctx, streamID, event); err != nil {
		// Log the error but don't fail the operation since DB write succeeded.
		s.logger.Error("event persisted but SSE fanout failed", "event_id", event.ID, "error", err)
	}

	return event, nil
}

// CreateAndPublishLog persists a log chunk to the database and publishes it to the SSE hub.
// The runID is used as the streamID for SSE fanout.
// Returns the created log from the database or an error if either operation fails.
func (s *Service) CreateAndPublishLog(ctx context.Context, params store.CreateLogParams) (store.Log, error) {
	if s.store == nil {
		return store.Log{}, errors.New("events: store not configured")
	}

	// Persist to database first.
	log, err := s.store.CreateLog(ctx, params)
	if err != nil {
		return store.Log{}, fmt.Errorf("persist log: %w", err)
	}

	// Convert runID to string for streamID.
	streamID := uuidToString(params.RunID)
	if streamID == "" {
		// DB succeeded but SSE fanout skipped; log and return.
		s.logger.Warn("log persisted but runID invalid for SSE fanout", "log_id", log.ID)
		return log, nil
	}

	// Fan out to SSE hub.
	if err := s.publishLogToHub(ctx, streamID, log); err != nil {
		// Log the error but don't fail the operation since DB write succeeded.
		s.logger.Error("log persisted but SSE fanout failed", "log_id", log.ID, "error", err)
	}

	return log, nil
}

// PublishTicket publishes a ticket lifecycle event (queued/running/succeeded/failed/cancelled)
// to the SSE hub. The runID is used as the streamID for SSE fanout.
// The payload should contain a modsapi.TicketSummary with current ticket state.
// Returns an error if the fanout fails.
func (s *Service) PublishTicket(ctx context.Context, runID string, payload modsapi.TicketSummary) error {
	if runID == "" {
		return errors.New("events: runID required for ticket publish")
	}
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}

	return s.hub.PublishTicket(ctx, runID, payload)
}

// publishEventToHub converts a database event to a logstream event and publishes it.
func (s *Service) publishEventToHub(ctx context.Context, streamID string, event store.Event) error {
	// Convert event to log record format for SSE.
	// Use the event level as stream and message as line.
	record := logstream.LogRecord{
		Timestamp: timestampToString(event.Time),
		Stream:    event.Level,
		Line:      event.Message,
	}

	return s.hub.PublishLog(ctx, streamID, record)
}

// publishLogToHub converts a database log to a logstream event and publishes it.
func (s *Service) publishLogToHub(ctx context.Context, streamID string, log store.Log) error {
	// Convert log data to string. The data is expected to be text content.
	line := string(log.Data)

	record := logstream.LogRecord{
		Timestamp: timestampToString(log.CreatedAt),
		Stream:    "log",
		Line:      line,
	}

	return s.hub.PublishLog(ctx, streamID, record)
}

// uuidToString converts a pgtype.UUID to its string representation.
// Returns empty string if the UUID is invalid or null.
func uuidToString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	return uuid.UUID(id.Bytes).String()
}

// timestampToString converts a pgtype.Timestamptz to RFC3339 string.
// Returns empty string if the timestamp is invalid or null.
func timestampToString(ts pgtype.Timestamptz) string {
	if !ts.Valid {
		return ""
	}
	return ts.Time.Format(time.RFC3339)
}
