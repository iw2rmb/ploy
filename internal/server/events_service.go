package server

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"container/list"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	migsapi "github.com/iw2rmb/ploy/internal/migs/api"
	"github.com/iw2rmb/ploy/internal/store"
	logstream "github.com/iw2rmb/ploy/internal/stream"
	"github.com/jackc/pgx/v5/pgtype"
)

const defaultEventsJobCacheSize = 4096

// EventsOptions configures the events service.
type EventsOptions struct {
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

// EventsService wraps the logstream hub for server-side event streaming
// and coordinates database persistence with SSE fanout.
type EventsService struct {
	hub      *logstream.Hub
	store    store.Store
	logger   *slog.Logger
	jobCache *eventsJobContextCache
}

type jobContextCacheEntry struct {
	jobID domaintypes.JobID
	ctx   eventsJobContext
}

type eventsJobContextCache struct {
	mu         sync.Mutex
	maxEntries int
	entries    map[domaintypes.JobID]*list.Element
	lru        *list.List
}

func newEventsJobContextCache(maxEntries int) *eventsJobContextCache {
	return &eventsJobContextCache{
		maxEntries: maxEntries,
		entries:    make(map[domaintypes.JobID]*list.Element, maxEntries),
		lru:        list.New(),
	}
}

func (c *eventsJobContextCache) Get(jobID domaintypes.JobID) (eventsJobContext, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, ok := c.entries[jobID]
	if !ok {
		return eventsJobContext{}, false
	}

	c.lru.MoveToFront(elem)
	entry := elem.Value.(jobContextCacheEntry)
	return entry.ctx, true
}

func (c *eventsJobContextCache) Set(jobID domaintypes.JobID, ctx eventsJobContext) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.entries[jobID]; ok {
		elem.Value = jobContextCacheEntry{
			jobID: jobID,
			ctx:   ctx,
		}
		c.lru.MoveToFront(elem)
		return
	}

	elem := c.lru.PushFront(jobContextCacheEntry{
		jobID: jobID,
		ctx:   ctx,
	})
	c.entries[jobID] = elem

	if c.lru.Len() <= c.maxEntries {
		return
	}

	last := c.lru.Back()
	if last == nil {
		return
	}
	evicted := last.Value.(jobContextCacheEntry)
	delete(c.entries, evicted.jobID)
	c.lru.Remove(last)
}

// NewEventsService constructs a new events service.
func NewEventsService(opts EventsOptions) (*EventsService, error) {
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
		jobCacheSize = defaultEventsJobCacheSize
	}

	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	hub := logstream.NewHub(logstream.Options{
		BufferSize:  opts.BufferSize,
		HistorySize: opts.HistorySize,
	})

	return &EventsService{
		hub:      hub,
		store:    opts.Store,
		logger:   logger,
		jobCache: newEventsJobContextCache(jobCacheSize),
	}, nil
}

// Hub returns the underlying logstream hub.
func (s *EventsService) Hub() *logstream.Hub {
	return s.hub
}

// CreateAndPublishEvent persists an event to the database and publishes it to the SSE hub.
// The runID is used as the streamID for SSE fanout.
// Returns the created event from the database. If persistence fails, an error
// is returned; SSE fanout errors are logged but do not fail the operation.
//
// params.RunID is a KSUID-backed RunID; normalized for hub operations.
func (s *EventsService) CreateAndPublishEvent(ctx context.Context, params store.CreateEventParams) (store.Event, error) {
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
func (s *EventsService) CreateAndPublishLog(ctx context.Context, log store.Log, data []byte) error {
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
// JSON‑serializable contract at the service boundary and prevent accidental
// non‑JSON payloads from being published. Callers should also emit a terminal
// "done" status via Hub().PublishStatus when the run reaches a terminal state
// so SSE clients can terminate streams cleanly. Returns an error if the fanout fails.
func (s *EventsService) PublishRun(ctx context.Context, runID domaintypes.RunID, payload migsapi.RunSummary) error {
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
func (s *EventsService) publishEventToHub(ctx context.Context, runID domaintypes.RunID, event store.Event) error {
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
func (s *EventsService) publishLogToJobStream(ctx context.Context, jobID domaintypes.JobID, log store.Log, data []byte) error {
	ts := timestampToString(log.CreatedAt)

	// Fetch job metadata to enrich log records with execution context.
	jobCtx := s.loadJobContext(ctx, log.JobID)

	// Attempt to gunzip; if it fails, fall back to raw-as-string single frame.
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err == nil {
		defer func() {
			_ = zr.Close()
		}()
		scanner := bufio.NewScanner(zr)
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
				JobType:   jobCtx.JobType,
			}
			if err := s.hub.PublishJobLog(ctx, jobID, rec); err != nil {
				return err
			}
		}
		if scanErr := scanner.Err(); scanErr != nil {
			rec := logstream.LogRecord{
				Timestamp: ts,
				Stream:    "stdout",
				Line:      "[log decode error]",
				NodeID:    jobCtx.NodeID,
				JobID:     jobCtx.JobID,
				JobType:   jobCtx.JobType,
			}
			_ = s.hub.PublishJobLog(ctx, jobID, rec)
		}
		return nil
	}
	// Fallback: publish raw bytes as a single frame.
	rec := logstream.LogRecord{
		Timestamp: ts,
		Stream:    "log",
		Line:      string(data),
		NodeID:    jobCtx.NodeID,
		JobID:     jobCtx.JobID,
		JobType:   jobCtx.JobType,
	}
	return s.hub.PublishJobLog(ctx, jobID, rec)
}

// PublishJobDone emits a terminal done sentinel on the job-keyed stream,
// signaling to SSE clients that the job has completed and the stream
// will close. Returns ErrInvalidJobID if the job ID is blank.
func (s *EventsService) PublishJobDone(ctx context.Context, jobID domaintypes.JobID, status string) error {
	if jobID.IsZero() {
		return logstream.ErrInvalidJobID
	}
	return s.hub.PublishJobStatus(ctx, jobID, logstream.Status{Status: status})
}

// eventsJobContext holds execution context extracted from job metadata.
// Used to enrich log records with node and mig information.
// Uses domain types to preserve type safety end-to-end without lossy casts.
type eventsJobContext struct {
	NodeID  domaintypes.NodeID
	JobID   domaintypes.JobID
	JobType domaintypes.JobType
}

// loadJobContext fetches job metadata for a given job ID and extracts
// fields needed to enrich log records. Returns an empty context if the
// job ID is nil/empty or the lookup fails (logs are still published without
// enrichment in these cases).
func (s *EventsService) loadJobContext(ctx context.Context, jobID *domaintypes.JobID) eventsJobContext {
	// If job ID is nil or empty, return empty context.
	if jobID == nil || jobID.IsZero() {
		return eventsJobContext{}
	}

	// Check cache first to avoid redundant DB lookups during log bursts.
	if cached, ok := s.jobCache.Get(*jobID); ok {
		return cached
	}

	// If store is not configured, return empty context (log-only mode).
	if s.store == nil {
		return eventsJobContext{}
	}

	job, err := s.store.GetJob(ctx, *jobID)
	if err != nil {
		// Log lookup failure but don't block log publishing.
		s.logger.Debug("job lookup failed for log enrichment",
			"job_id", jobID.String(),
			"error", err)
		return eventsJobContext{}
	}

	// Normalize and validate job metadata before enrichment.
	//
	// job.ID/job.NodeID are typed IDs; job.JobType must satisfy JobType.Validate().
	//
	// Invalid values are omitted from enrichment to keep the emitted SSE payload
	// contract strict.

	jid := job.ID

	var nid domaintypes.NodeID
	if job.NodeID != nil && !job.NodeID.IsZero() {
		nid = *job.NodeID
	}

	mt := domaintypes.JobType(domaintypes.Normalize(job.JobType.String()))
	if !mt.IsZero() {
		if err := mt.Validate(); err != nil {
			s.logger.Debug("invalid job_type for log enrichment",
				"job_id", job.ID.String(),
				"job_type", job.JobType,
				"error", err,
			)
			mt = ""
		}
	}

	jctx := eventsJobContext{
		NodeID:  nid,
		JobID:   jid,
		JobType: mt,
	}
	s.jobCache.Set(*jobID, jctx)
	return jctx
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
