package events

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
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

// CreateAndPublishLog publishes a log to the SSE hub. The log metadata must already
// be persisted via blobpersist; this method only handles SSE fanout.
// Log data is decoded into per-line stdout LogRecord frames before fanout
// so clients see structured "log" events.
//
// params.RunID is a KSUID-backed RunID; normalized for hub operations.
func (s *Service) CreateAndPublishLog(ctx context.Context, log store.Log, data []byte) error {
	// Normalize for hub operations.
	runID := domaintypes.RunID(domaintypes.Normalize(log.RunID.String()))
	if runID.IsZero() {
		s.logger.Warn("log runID invalid for SSE fanout", "log_id", log.ID)
		return nil
	}

	// Fan out to SSE hub with the provided data bytes.
	if err := s.publishLogToHubWithBytes(ctx, runID, log, data); err != nil {
		s.logger.Error("SSE fanout failed", "log_id", log.ID, "error", err)
		return err
	}

	return nil
}

// PublishRun publishes a run lifecycle event (queued/running/succeeded/failed/cancelled)
// to the SSE hub. The runID (KSUID-backed RunID) is used as the streamID for SSE fanout.
//
// The payload is intentionally typed as modsapi.RunSummary to enforce a
// JSON‑serializable contract at the service boundary and prevent accidental
// non‑JSON payloads from being published. Callers should also emit a terminal
// "done" status via Hub().PublishStatus when the run reaches a terminal state
// so SSE clients can terminate streams cleanly. Returns an error if the fanout fails.
func (s *Service) PublishRun(ctx context.Context, runID domaintypes.RunID, payload modsapi.RunSummary) error {
	if runID.IsZero() {
		return logstream.ErrInvalidRunID
	}
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}

	return s.hub.PublishRun(ctx, runID, payload)
}

// publishEventToHub converts a database event to a logstream event and publishes it.
func (s *Service) publishEventToHub(ctx context.Context, runID domaintypes.RunID, event store.Event) error {
	// Convert event to log record format for SSE.
	// Use the event level as stream and message as line.
	record := logstream.LogRecord{
		Timestamp: timestampToString(event.Time),
		Stream:    event.Level,
		Line:      event.Message,
	}

	return s.hub.PublishLog(ctx, runID, record)
}

// publishLogToHubWithBytes converts log data to logstream events and publishes them.
// It enriches each LogRecord with execution context (node_id, job_id, mod_type,
// step_index) by looking up the associated job metadata when available.
func (s *Service) publishLogToHubWithBytes(ctx context.Context, runID domaintypes.RunID, log store.Log, data []byte) error {
	ts := timestampToString(log.CreatedAt)

	// Fetch job metadata to enrich log records with execution context.
	// If the job lookup fails (e.g., job doesn't exist yet or store unavailable),
	// we still publish logs without enrichment to avoid losing data.
	jobCtx := s.loadJobContext(ctx, log.JobID)

	// Attempt to gunzip; if it fails, fall back to raw-as-string single frame.
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err == nil {
		defer func() {
			_ = zr.Close()
		}()
		scanner := bufio.NewScanner(zr)
		// Set a reasonable max token size (256 KiB per line) to avoid memory blowups.
		const maxLine = 256 * 1024
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, maxLine)
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}
			rec := logstream.LogRecord{
				Timestamp: ts,
				Stream:    "stdout",
				Line:      line,
				NodeID:    jobCtx.NodeID,
				JobID:     jobCtx.JobID,
				ModType:   jobCtx.ModType,
				StepIndex: jobCtx.StepIndex,
			}
			if err := s.hub.PublishLog(ctx, runID, rec); err != nil {
				return err
			}
		}
		if scanErr := scanner.Err(); scanErr != nil {
			// On scanner error, emit a fallback lump to avoid total loss.
			rec := logstream.LogRecord{
				Timestamp: ts,
				Stream:    "stdout",
				Line:      "[log decode error]",
				NodeID:    jobCtx.NodeID,
				JobID:     jobCtx.JobID,
				ModType:   jobCtx.ModType,
				StepIndex: jobCtx.StepIndex,
			}
			_ = s.hub.PublishLog(ctx, runID, rec)
		}
		return nil
	}
	// Fallback: publish raw bytes as a single frame (may look garbled to clients).
	rec := logstream.LogRecord{
		Timestamp: ts,
		Stream:    "log",
		Line:      string(data),
		NodeID:    jobCtx.NodeID,
		JobID:     jobCtx.JobID,
		ModType:   jobCtx.ModType,
		StepIndex: jobCtx.StepIndex,
	}
	return s.hub.PublishLog(ctx, runID, rec)
}

// jobContext holds execution context extracted from job metadata.
// Used to enrich log records with node and mod information.
// Uses domain types to preserve type safety end-to-end without lossy casts.
type jobContext struct {
	NodeID    domaintypes.NodeID
	JobID     domaintypes.JobID
	ModType   domaintypes.ModType
	StepIndex domaintypes.StepIndex
}

// loadJobContext fetches job metadata for a given job ID and extracts
// fields needed to enrich log records. Returns an empty context if the
// job ID is nil/empty or the lookup fails (logs are still published without
// enrichment in these cases).
func (s *Service) loadJobContext(ctx context.Context, jobID *domaintypes.JobID) jobContext {
	// If job ID is nil or empty, return empty context.
	if jobID == nil || jobID.IsZero() {
		return jobContext{}
	}
	// If store is not configured, return empty context (log-only mode).
	if s.store == nil {
		return jobContext{}
	}

	job, err := s.store.GetJob(ctx, *jobID)
	if err != nil {
		// Log lookup failure but don't block log publishing.
		s.logger.Debug("job lookup failed for log enrichment",
			"job_id", jobID.String(),
			"error", err)
		return jobContext{}
	}

	// Normalize and validate job metadata before enrichment.
	//
	// job.ID/job.NodeID are typed IDs; job.JobType must satisfy ModType.Validate().
	//
	// Invalid values are omitted from enrichment to keep the emitted SSE payload
	// contract strict.

	jid := job.ID

	var nid domaintypes.NodeID
	if job.NodeID != nil && !job.NodeID.IsZero() {
		nid = *job.NodeID
	}

	mt := domaintypes.ModType(domaintypes.Normalize(job.JobType))
	if !mt.IsZero() {
		if err := mt.Validate(); err != nil {
			s.logger.Debug("invalid mod_type for log enrichment",
				"job_id", job.ID.String(),
				"job_type", job.JobType,
				"error", err,
			)
			mt = ""
		}
	}

	// step_index is no longer persisted on jobs; preserve log enrichment support
	// by reading optional metadata when present.
	var si domaintypes.StepIndex
	var stepMeta struct {
		StepIndex *float64 `json:"step_index,omitempty"`
	}
	if len(job.Meta) > 0 && json.Unmarshal(job.Meta, &stepMeta) == nil && stepMeta.StepIndex != nil {
		si = domaintypes.StepIndex(*stepMeta.StepIndex)
		if !si.Valid() {
			s.logger.Debug("invalid step_index metadata for log enrichment",
				"job_id", job.ID.String(),
				"step_index", *stepMeta.StepIndex,
			)
			si = 0
		}
	}

	return jobContext{
		NodeID:    nid,
		JobID:     jid,
		ModType:   mt,
		StepIndex: si,
	}
}

// timestampToString converts a pgtype.Timestamptz to RFC3339 string.
// Returns empty string if the timestamp is invalid or null.
func timestampToString(ts pgtype.Timestamptz) string {
	if !ts.Valid {
		return ""
	}
	return ts.Time.Format(time.RFC3339)
}

// normalizeEventLevel canonicalizes and validates event level using domain LogLevel.
// It maps unknown or empty values to "info" to keep storage/SSE streams consistent.
func normalizeEventLevel(level string) string {
	s := strings.ToLower(domaintypes.Normalize(level))
	if domaintypes.IsEmpty(s) {
		return domaintypes.LogLevelInfo.String()
	}
	l := domaintypes.LogLevel(s)
	if err := l.Validate(); err != nil {
		return domaintypes.LogLevelInfo.String()
	}
	return l.String()
}
