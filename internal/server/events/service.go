package events

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/logchunk"
	migsapi "github.com/iw2rmb/ploy/internal/migs/api"
	"github.com/iw2rmb/ploy/internal/store"
	logstream "github.com/iw2rmb/ploy/internal/stream"
)

const defaultJobCacheSize = 4096

// Options configures the events service.
type Options struct {
	// BufferSize controls the per-subscriber channel size.
	BufferSize int
	// HistorySize bounds the number of events retained for resumption.
	HistorySize int
	// JobCacheSize bounds the in-memory job context cache used for log enrichment.
	// Zero applies the default size.
	JobCacheSize int
	// Logger for service diagnostics (optional).
	Logger *slog.Logger
	// Store for database persistence (optional; if nil, persistence methods will fail).
	Store store.Store
}

// Service wraps the logstream hub for server-side event streaming
// and coordinates database persistence with SSE fanout.
type Service struct {
	hub      *logstream.Hub
	store    store.Store
	logger   *slog.Logger
	jobCache *jobContextCache
}

// NewService constructs a new events service.
func NewService(opts Options) (*Service, error) {
	if opts.BufferSize < 0 {
		return nil, errors.New("events: buffer size must be non-negative")
	}
	if opts.HistorySize < 0 {
		return nil, errors.New("events: history size must be non-negative")
	}
	if opts.JobCacheSize < 0 {
		return nil, errors.New("events: job cache size must be non-negative")
	}
	jobCacheSize := opts.JobCacheSize
	if jobCacheSize == 0 {
		jobCacheSize = defaultJobCacheSize
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
		hub:      hub,
		store:    opts.Store,
		logger:   logger,
		jobCache: newJobContextCache(jobCacheSize),
	}, nil
}

// Hub returns the underlying logstream hub.
func (s *Service) Hub() *logstream.Hub {
	return s.hub
}

// CreateAndPublishEvent persists an event to the database and publishes it to the SSE hub.
// The runID is used as the streamID for SSE fanout.
// Returns the created event from the database. If persistence fails, an error
// is returned; SSE fanout errors are logged but do not fail the operation.
//
// params.RunID is a KSUID-backed RunID; normalized for hub operations.
func (s *Service) CreateAndPublishEvent(ctx context.Context, params store.CreateEventParams) (store.Event, error) {
	if s.store == nil {
		return store.Event{}, errors.New("events: store not configured")
	}

	// Normalize/validate level using domain LogLevel; default unknown/empty to "info".
	params.Level = normalizeEventLevel(params.Level)

	// Persist to database first.
	event, err := s.store.CreateEvent(ctx, params)
	if err != nil {
		return store.Event{}, fmt.Errorf("persist event: %w", err)
	}

	// Normalize for hub operations.
	runID := domaintypes.RunID(domaintypes.Normalize(params.RunID.String()))
	if runID.IsZero() {
		// DB succeeded but SSE fanout skipped; log and return event.
		s.logger.Warn("event persisted but runID invalid for SSE fanout", "event_id", event.ID)
		return event, nil
	}

	// Fan out to SSE hub.
	if err := s.publishEventToHub(ctx, runID, event); err != nil {
		// Log the error but don't fail the operation since DB write succeeded.
		s.logger.Error("event persisted but SSE fanout failed", "event_id", event.ID, "error", err)
	}

	return event, nil
}

// CreateAndPublishLog publishes a log to the job-scoped SSE stream.
// The log metadata must already be persisted via blobpersist; this method
// only handles SSE fanout. Log data is decoded into per-line stdout LogRecord
// frames and published to the job stream keyed by JobID.
//
// If the log has no JobID, SSE fanout is skipped (job-stream requires a job key).
func (s *Service) CreateAndPublishLog(ctx context.Context, log store.Log, data []byte) error {
	if log.JobID == nil || log.JobID.IsZero() {
		s.logger.Debug("log has no job_id, skipping job-stream SSE fanout", "log_id", log.ID)
		return nil
	}
	jobID := *log.JobID

	if err := s.publishLogToJobStream(ctx, jobID, log, data); err != nil {
		s.logger.Error("job SSE fanout failed", "log_id", log.ID, "job_id", jobID.String(), "error", err)
	}

	return nil
}

// PublishRun publishes a run lifecycle event (queued/running/succeeded/failed/cancelled)
// to the SSE hub. The runID (KSUID-backed RunID) is used as the streamID for SSE fanout.
//
// The payload is intentionally typed as migsapi.RunSummary to enforce a
// JSON-serializable contract at the service boundary and prevent accidental
// non-JSON payloads from being published. Callers should also emit a terminal
// "done" status via Hub().PublishStatus when the run reaches a terminal state
// so SSE clients can terminate streams cleanly. Returns an error if the fanout fails.
func (s *Service) PublishRun(ctx context.Context, runID domaintypes.RunID, payload migsapi.RunSummary) error {
	if runID.IsZero() {
		return logstream.ErrInvalidRunID
	}
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}

	return s.hub.PublishRun(ctx, runID, payload)
}

// publishEventToHub converts a database event to a stage event and publishes it
// to the run-keyed stream. Stage events carry node progress messages (e.g.
// "step started", "gate passed") and are distinct from container log frames.
func (s *Service) publishEventToHub(ctx context.Context, runID domaintypes.RunID, event store.Event) error {
	record := logstream.LogRecord{
		Timestamp: timestampToString(event.Time),
		Stream:    event.Level,
		Line:      event.Message,
	}

	return s.hub.PublishStage(ctx, runID, record)
}

// publishLogToJobStream converts log data to logstream events and publishes
// them to the job-keyed stream. It enriches each LogRecord with execution
// context (node_id, job_id, job_type) by looking up the associated job
// metadata when available.
func (s *Service) publishLogToJobStream(ctx context.Context, jobID domaintypes.JobID, log store.Log, data []byte) error {
	ts := timestampToString(log.CreatedAt)

	// Fetch job metadata to enrich log records with execution context.
	jobCtx := s.loadJobContext(ctx, log.JobID)

	records, err := logchunk.DecodeGzip(data)
	if err != nil {
		rec := logstream.LogRecord{
			Timestamp: ts,
			Stream:    logchunk.StreamStderr,
			Line:      "[log decode error]",
			NodeID:    jobCtx.NodeID,
			JobID:     jobCtx.JobID,
			JobType:   jobCtx.JobType,
		}
		_ = s.hub.PublishJobLog(ctx, jobID, rec)
		return nil
	}

	for _, frame := range records {
		rec := logstream.LogRecord{
			Timestamp: ts,
			Stream:    frame.Stream,
			Line:      frame.Line,
			NodeID:    jobCtx.NodeID,
			JobID:     jobCtx.JobID,
			JobType:   jobCtx.JobType,
		}
		if err := s.hub.PublishJobLog(ctx, jobID, rec); err != nil {
			return err
		}
	}
	return nil
}

// PublishJobRetention emits a retention hint on the job-keyed stream,
// informing SSE clients about log retention metadata for the job.
// Returns ErrInvalidJobID if the job ID is blank.
func (s *Service) PublishJobRetention(ctx context.Context, jobID domaintypes.JobID, hint logstream.RetentionHint) error {
	if jobID.IsZero() {
		return logstream.ErrInvalidJobID
	}
	return s.hub.PublishJobRetention(ctx, jobID, hint)
}

// PublishJobDone emits a terminal done sentinel on the job-keyed stream,
// signaling to SSE clients that the job has completed and the stream
// will close. Returns ErrInvalidJobID if the job ID is blank.
func (s *Service) PublishJobDone(ctx context.Context, jobID domaintypes.JobID, status string) error {
	if jobID.IsZero() {
		return logstream.ErrInvalidJobID
	}
	return s.hub.PublishJobStatus(ctx, jobID, logstream.Status{Status: status})
}
