package logstream

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/migs/api"
)

// ErrStreamClosed indicates the target stream is closed.
var ErrStreamClosed = errors.New("logstream: stream closed")

// ErrInvalidRunID indicates a blank or whitespace-only run ID was provided.
var ErrInvalidRunID = errors.New("logstream: invalid run ID (blank or whitespace)")

// Options configures the hub.
type Options struct {
	// BufferSize controls the per-subscriber channel size.
	BufferSize int
	// HistorySize bounds the number of events retained for resumption.
	HistorySize int
}

// LogRecord represents a structured log frame.
// The enriched fields (NodeID, JobID, JobType) provide execution
// context so clients can correlate log lines with specific nodes, jobs, and
// Mods pipeline stages. These fields are optional — older or internal-only
// log sources may omit them.
// Uses domain types (NodeID, JobID, JobType) for type-safe identification.
type LogRecord struct {
	Timestamp string `json:"timestamp"`
	Stream    string `json:"stream"`
	Line      string `json:"line"`

	// NodeID identifies the execution node that produced this log line (NanoID-backed).
	// Empty when the source is not node-bound (e.g., hub-generated events).
	NodeID domaintypes.NodeID `json:"node_id,omitempty"`

	// JobID is the identifier of the job that produced this log line (KSUID-backed).
	// Empty for events not tied to a specific job.
	JobID domaintypes.JobID `json:"job_id,omitempty"`

	// JobType indicates the job phase type (e.g., "pre_gate", "mod", "post_gate", "heal", "re_gate").
	// Empty when not applicable or unknown. Uses domain type for type-safe identification.
	JobType domaintypes.JobType `json:"job_type,omitempty"`
}

// RetentionHint carries retention metadata emitted on the stream.
type RetentionHint struct {
	Retained bool            `json:"retained"`
	TTL      string          `json:"ttl"`
	Expires  string          `json:"expires_at"`
	Bundle   domaintypes.CID `json:"bundle_cid,omitempty"`
}

// Status announces terminal stream states.
type Status struct {
	Status string `json:"status"`
}

// Event represents a server-sent event frame produced by the hub.
type Event struct {
	ID   domaintypes.EventID
	Type domaintypes.SSEEventType
	Data []byte
}

// Subscription delivers events to a consumer.
type Subscription struct {
	Events <-chan Event
	cancel func()
}

// Cancel terminates the subscription.
func (s Subscription) Cancel() {
	if s.cancel != nil {
		s.cancel()
	}
}

// Hub manages log streams published by nodes and consumed by SSE clients.
type Hub struct {
	mu      sync.RWMutex
	streams map[domaintypes.RunID]*stream
	opts    normalizedOptions
}

// NewHub constructs a log stream hub.
func NewHub(opts Options) *Hub {
	return &Hub{
		streams: make(map[domaintypes.RunID]*stream),
		opts:    normalizeOptions(opts),
	}
}

func normalizeRunID(runID domaintypes.RunID) domaintypes.RunID {
	return domaintypes.RunID(domaintypes.Normalize(runID.String()))
}

// Ensure creates the stream if it does not already exist.
// Returns an error if the run ID is blank or whitespace-only.
func (h *Hub) Ensure(runID domaintypes.RunID) error {
	if runID.IsZero() {
		return ErrInvalidRunID
	}
	runID = normalizeRunID(runID)
	h.mu.Lock()
	if _, exists := h.streams[runID]; !exists {
		h.streams[runID] = newStream(runID, h.opts)
	}
	h.mu.Unlock()
	return nil
}

// PublishLog appends a log record to a stream.
// Returns ErrInvalidRunID if the run ID is blank or whitespace-only.
func (h *Hub) PublishLog(ctx context.Context, runID domaintypes.RunID, record LogRecord) error {
	return h.publish(ctx, runID, domaintypes.SSEEventLog, record)
}

// PublishRetention appends a retention hint to a stream.
// Returns ErrInvalidRunID if the run ID is blank or whitespace-only.
func (h *Hub) PublishRetention(ctx context.Context, runID domaintypes.RunID, hint RetentionHint) error {
	return h.publish(ctx, runID, domaintypes.SSEEventRetention, hint)
}

// PublishStatus appends a terminal status event to a stream.
// Event types emitted by the hub are:
//   - "log": LogRecord {Timestamp, Stream, Line, NodeID, JobID, JobType}
//   - "retention": RetentionHint {Retained, TTL, Expires, Bundle}
//   - "run": api.RunSummary snapshot
//   - "stage": stage status update
//   - "done": Status {Status: "done"} sentinel for stream completion.
//
// Returns ErrInvalidRunID if the run ID is blank or whitespace-only.
func (h *Hub) PublishStatus(ctx context.Context, runID domaintypes.RunID, status Status) error {
	if err := h.publish(ctx, runID, domaintypes.SSEEventDone, status); err != nil {
		return err
	}
	runID = normalizeRunID(runID)
	stream := h.getStream(runID)
	if stream != nil {
		stream.finish()
	}
	return nil
}

// PublishRun appends a typed run snapshot to a stream.
//
// The payload is strongly typed as api.RunSummary to prevent accidental
// publication of non‑JSON payloads (e.g., raw []byte or strings). The hub
// still performs generic JSON marshaling internally, but this boundary keeps
// the "run" event contract consistent and JSON‑serializable.
//
// Returns ErrInvalidRunID if the run ID is blank or whitespace-only.
func (h *Hub) PublishRun(ctx context.Context, runID domaintypes.RunID, run api.RunSummary) error {
	return h.publish(ctx, runID, domaintypes.SSEEventRun, run)
}

// ErrInvalidEventType indicates an unknown SSE event type was provided.
var ErrInvalidEventType = errors.New("logstream: invalid event type")

func (h *Hub) publish(ctx context.Context, runID domaintypes.RunID, eventType domaintypes.SSEEventType, payload any) error {
	if runID.IsZero() {
		return ErrInvalidRunID
	}
	runID = normalizeRunID(runID)
	if err := eventType.Validate(); err != nil {
		return ErrInvalidEventType
	}
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	stream := h.getOrCreate(runID)
	if stream == nil {
		return nil
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return stream.publish(Event{Type: eventType, Data: data})
}

// Subscribe registers a consumer for the stream starting after the provided id.
// The sinceID must be a valid EventID (non-negative); invalid IDs are rejected.
// Returns ErrInvalidRunID if the run ID is blank or whitespace-only.
func (h *Hub) Subscribe(ctx context.Context, runID domaintypes.RunID, sinceID domaintypes.EventID) (Subscription, error) {
	if runID.IsZero() {
		return Subscription{}, ErrInvalidRunID
	}
	runID = normalizeRunID(runID)
	if !sinceID.Valid() {
		return Subscription{}, errors.New("logstream: invalid since id")
	}
	if ctx != nil && ctx.Err() != nil {
		return Subscription{}, ctx.Err()
	}
	stream := h.getOrCreate(runID)
	if stream == nil {
		return Subscription{}, errors.New("logstream: stream unavailable")
	}
	sub, history, closed := stream.subscribe(sinceID)
	for _, evt := range history {
		if !sub.send(evt) {
			if sub.id >= 0 {
				stream.drop(sub.id)
			}
			break
		}
	}
	if closed {
		sub.close()
		return Subscription{
			Events: sub.ch,
			cancel: func() {},
		}, nil
	}
	return Subscription{
		Events: sub.ch,
		cancel: func() {
			stream.drop(sub.id)
		},
	}, nil
}

// Close tears down the stream. No-op if the run ID is blank.
func (h *Hub) Close(runID domaintypes.RunID) {
	if runID.IsZero() {
		return
	}
	runID = normalizeRunID(runID)
	h.mu.Lock()
	stream, ok := h.streams[runID]
	if ok {
		delete(h.streams, runID)
	}
	h.mu.Unlock()
	if ok {
		stream.finish()
	}
}

func (h *Hub) getOrCreate(runID domaintypes.RunID) *stream {
	h.mu.RLock()
	stream := h.streams[runID]
	h.mu.RUnlock()
	if stream != nil {
		return stream
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	stream = h.streams[runID]
	if stream == nil {
		stream = newStream(runID, h.opts)
		h.streams[runID] = stream
	}
	return stream
}

func (h *Hub) getStream(runID domaintypes.RunID) *stream {
	h.mu.RLock()
	stream := h.streams[runID]
	h.mu.RUnlock()
	return stream
}

// Snapshot returns a copy of buffered events for the stream.
// Returns nil if the run ID is blank or the stream does not exist.
func (h *Hub) Snapshot(runID domaintypes.RunID) []Event {
	if runID.IsZero() {
		return nil
	}
	runID = normalizeRunID(runID)
	stream := h.getStream(runID)
	if stream == nil {
		return nil
	}
	return stream.snapshot()
}

type normalizedOptions struct {
	buffer  int
	history int
}

func normalizeOptions(opts Options) normalizedOptions {
	buffer := opts.BufferSize
	if buffer <= 0 {
		buffer = 32
	}
	history := opts.HistorySize
	if history <= 0 {
		history = 256
	}
	if history < buffer {
		history = buffer
	}
	return normalizedOptions{
		buffer:  buffer,
		history: history,
	}
}

type stream struct {
	id          domaintypes.RunID
	opts        normalizedOptions
	mu          sync.Mutex
	history     []Event
	subscribers map[int]*subscriber
	nextEventID domaintypes.EventID
	nextSubID   int
	closed      bool
}

func (s *stream) snapshot() []Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.history) == 0 {
		return nil
	}
	out := make([]Event, len(s.history))
	copy(out, s.history)
	return out
}

func newStream(id domaintypes.RunID, opts normalizedOptions) *stream {
	return &stream{
		id:          id,
		opts:        opts,
		history:     make([]Event, 0, opts.history),
		subscribers: make(map[int]*subscriber),
	}
}

func (s *stream) publish(evt Event) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return ErrStreamClosed
	}
	s.nextEventID++
	evt.ID = s.nextEventID
	if s.opts.history > 0 {
		s.history = append(s.history, evt)
		if len(s.history) > s.opts.history {
			start := len(s.history) - s.opts.history
			copied := make([]Event, s.opts.history)
			copy(copied, s.history[start:])
			s.history = copied
		}
	}
	snapshot := make([]*subscriber, 0, len(s.subscribers))
	for _, sub := range s.subscribers {
		snapshot = append(snapshot, sub)
	}
	s.mu.Unlock()

	var dropped []int
	for _, sub := range snapshot {
		if !sub.send(evt) {
			dropped = append(dropped, sub.id)
		}
	}
	if len(dropped) > 0 {
		s.mu.Lock()
		for _, id := range dropped {
			if sub, ok := s.subscribers[id]; ok {
				delete(s.subscribers, id)
				sub.close()
			}
		}
		s.mu.Unlock()
	}
	return nil
}

func (s *stream) subscribe(sinceID domaintypes.EventID) (*subscriber, []Event, bool) {
	buffer := s.opts.buffer
	if buffer <= 0 {
		buffer = 1
	}
	sub := newSubscriber(-1, buffer)
	s.mu.Lock()
	history := s.historyAfterLocked(sinceID)
	if s.closed {
		s.mu.Unlock()
		return sub, history, true
	}
	sub.id = s.nextSubID
	s.nextSubID++
	s.subscribers[sub.id] = sub
	s.mu.Unlock()
	return sub, history, false
}

func (s *stream) drop(id int) {
	if id < 0 {
		return
	}
	s.mu.Lock()
	sub, ok := s.subscribers[id]
	if ok {
		delete(s.subscribers, id)
	}
	s.mu.Unlock()
	if ok {
		sub.close()
	}
}

func (s *stream) finish() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	snapshot := make([]*subscriber, 0, len(s.subscribers))
	for id, sub := range s.subscribers {
		snapshot = append(snapshot, sub)
		delete(s.subscribers, id)
	}
	s.mu.Unlock()
	for _, sub := range snapshot {
		sub.close()
	}
}

func (s *stream) historyAfterLocked(since domaintypes.EventID) []Event {
	if since <= 0 {
		return append([]Event(nil), s.history...)
	}
	out := make([]Event, 0, len(s.history))
	for _, evt := range s.history {
		if evt.ID > since {
			out = append(out, evt)
		}
	}
	return out
}

type subscriber struct {
	id   int
	ch   chan Event
	once sync.Once
}

func newSubscriber(id, buffer int) *subscriber {
	return &subscriber{
		id: id,
		ch: make(chan Event, buffer),
	}
}

func (s *subscriber) send(evt Event) bool {
	select {
	case s.ch <- evt:
		return true
	default:
		s.close()
		return false
	}
}

func (s *subscriber) close() {
	s.once.Do(func() {
		close(s.ch)
	})
}
