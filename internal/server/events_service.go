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
	modsapi "github.com/iw2rmb/ploy/internal/migs/api"
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

// Start is a no-op; the hub is ready immediately after construction.
func (s *EventsService) Start(context.Context) error { return nil }

// Stop is a no-op; the hub requires no teardown.
func (s *EventsService) Stop(context.Context) error { return nil }

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

// CreateAndPublishLog publishes a log to the SSE hub. The log metadata must already
// be persisted via blobpersist; this method only handles SSE fanout.
// Log data is decoded into per-line stdout LogRecord frames before fanout
// so clients see structured "log" events.
//
// params.RunID is a KSUID-backed RunID; normalized for hub operations.
func (s *EventsService) CreateAndPublishLog(ctx context.Context, log store.Log, data []byte) error {
	// Normalize for hub operations.
	runID := domaintypes.RunID(domaintypes.Normalize(log.RunID.String()))
	if runID.IsZero() {
		s.logger.Warn("log runID invalid for SSE fanout", "log_id", log.ID)
		return nil
	}

	// Fan out to SSE hub with the provided data bytes.
	// SSE fanout is best-effort; log errors but don't fail the operation
	// since the blob is already persisted and is the source of truth.
	if err := s.publishLogToHubWithBytes(ctx, runID, log, data); err != nil {
		s.logger.Error("SSE fanout failed", "log_id", log.ID, "error", err)
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
func (s *EventsService) PublishRun(ctx context.Context, runID domaintypes.RunID, payload modsapi.RunSummary) error {
	if runID.IsZero() {
		return logstream.ErrInvalidRunID
	}
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}

	return s.hub.PublishRun(ctx, runID, payload)
}

// publishEventToHub converts a database event to a logstream event and publishes it.
func (s *EventsService) publishEventToHub(ctx context.Context, runID domaintypes.RunID, event store.Event) error {
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
// It enriches each LogRecord with execution context (node_id, job_id, job_type)
// by looking up the associated job metadata when available.
func (s *EventsService) publishLogToHubWithBytes(ctx context.Context, runID domaintypes.RunID, log store.Log, data []byte) error {
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
				JobType:   jobCtx.JobType,
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
				JobType:   jobCtx.JobType,
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
		JobType:   jobCtx.JobType,
	}
	return s.hub.PublishLog(ctx, runID, rec)
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

	mt := domaintypes.JobType(domaintypes.Normalize(job.JobType))
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
