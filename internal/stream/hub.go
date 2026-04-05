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

// ErrInvalidJobID indicates a blank or whitespace-only job ID was provided.
var ErrInvalidJobID = errors.New("logstream: invalid job ID (blank or whitespace)")

// ErrInvalidEventType indicates an unknown SSE event type was provided.
var ErrInvalidEventType = errors.New("logstream: invalid event type")

// Options configures the hub.
type Options struct {
	// BufferSize controls the per-subscriber channel size (default: 32).
	BufferSize int
	// HistorySize bounds the number of events retained for resumption (default: 256, min: BufferSize).
	HistorySize int
}

func (o *Options) normalize() {
	if o.BufferSize <= 0 {
		o.BufferSize = 32
	}
	if o.HistorySize <= 0 {
		o.HistorySize = 256
	}
	if o.HistorySize < o.BufferSize {
		o.HistorySize = o.BufferSize
	}
}

// LogRecord represents a structured log frame.
// The enriched fields (NodeID, JobID, JobType) provide execution
// context so clients can correlate log lines with specific nodes, jobs, and
// Migs pipeline stages. These fields are optional — older or internal-only
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

	// JobType indicates the job phase type (e.g., "pre_gate", "mig", "post_gate", "heal", "re_gate").
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
// Run-keyed streams carry lifecycle events (run, stage, done).
// Job-keyed streams carry container logs (log, done).
type Hub struct {
	mu         sync.RWMutex
	streams    map[domaintypes.RunID]*stream
	jobStreams map[domaintypes.JobID]*stream
	opts       Options
}

// NewHub constructs a log stream hub.
func NewHub(opts Options) *Hub {
	opts.normalize()
	return &Hub{
		streams:    make(map[domaintypes.RunID]*stream),
		jobStreams: make(map[domaintypes.JobID]*stream),
		opts:       opts,
	}
}

func normalizeRunID(runID domaintypes.RunID) domaintypes.RunID {
	return domaintypes.RunID(domaintypes.Normalize(runID.String()))
}

func normalizeJobID(jobID domaintypes.JobID) domaintypes.JobID {
	return domaintypes.JobID(domaintypes.Normalize(jobID.String()))
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
		h.streams[runID] = newStream(h.opts)
	}
	h.mu.Unlock()
	return nil
}

// PublishLog appends a log record to a stream.
// Returns ErrInvalidRunID if the run ID is blank or whitespace-only.
func (h *Hub) PublishLog(ctx context.Context, runID domaintypes.RunID, record LogRecord) error {
	_, err := h.publish(ctx, runID, domaintypes.SSEEventLog, record)
	return err
}

// PublishRetention appends a retention hint to a stream.
// Returns ErrInvalidRunID if the run ID is blank or whitespace-only.
func (h *Hub) PublishRetention(ctx context.Context, runID domaintypes.RunID, hint RetentionHint) error {
	_, err := h.publish(ctx, runID, domaintypes.SSEEventRetention, hint)
	return err
}

// PublishStatus appends a terminal status event and closes the stream.
// Returns ErrInvalidRunID if the run ID is blank or whitespace-only.
func (h *Hub) PublishStatus(ctx context.Context, runID domaintypes.RunID, status Status) error {
	s, err := h.publish(ctx, runID, domaintypes.SSEEventDone, status)
	if err != nil {
		return err
	}
	s.finish()
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
	_, err := h.publish(ctx, runID, domaintypes.SSEEventRun, run)
	return err
}

// PublishStage appends a stage progress event to a run stream.
// Returns ErrInvalidRunID if the run ID is blank or whitespace-only.
func (h *Hub) PublishStage(ctx context.Context, runID domaintypes.RunID, record LogRecord) error {
	_, err := h.publish(ctx, runID, domaintypes.SSEEventStage, record)
	return err
}

func (h *Hub) publish(ctx context.Context, runID domaintypes.RunID, eventType domaintypes.SSEEventType, payload any) (*stream, error) {
	if runID.IsZero() {
		return nil, ErrInvalidRunID
	}
	runID = normalizeRunID(runID)
	if err := eventType.Validate(); err != nil {
		return nil, ErrInvalidEventType
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	s := h.getOrCreate(runID)
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return s, s.publish(Event{Type: eventType, Data: data})
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
	if ctx.Err() != nil {
		return Subscription{}, ctx.Err()
	}
	s := h.getOrCreate(runID)
	sub, history, closed := s.subscribe(sinceID)
	for _, evt := range history {
		if !sub.send(evt) {
			if sub.id >= 0 {
				s.drop(sub.id)
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
			s.drop(sub.id)
		},
	}, nil
}

// Close tears down the stream and removes it from the hub. No-op if the run ID is blank.
func (h *Hub) Close(runID domaintypes.RunID) {
	if runID.IsZero() {
		return
	}
	runID = normalizeRunID(runID)
	h.mu.Lock()
	s, ok := h.streams[runID]
	if ok {
		delete(h.streams, runID)
	}
	h.mu.Unlock()
	if ok {
		s.finish()
	}
}

// CloseAll tears down all streams (run and job) and clears the hub. Safe for graceful shutdown.
func (h *Hub) CloseAll() {
	h.mu.Lock()
	streams := make([]*stream, 0, len(h.streams)+len(h.jobStreams))
	for id, s := range h.streams {
		streams = append(streams, s)
		delete(h.streams, id)
	}
	for id, s := range h.jobStreams {
		streams = append(streams, s)
		delete(h.jobStreams, id)
	}
	h.mu.Unlock()
	for _, s := range streams {
		s.finish()
	}
}

// ---------------------------------------------------------------------------
// Job-keyed stream methods (container log fanout).
// ---------------------------------------------------------------------------

// EnsureJob creates the job stream if it does not already exist.
// Returns ErrInvalidJobID if the job ID is blank or whitespace-only.
func (h *Hub) EnsureJob(jobID domaintypes.JobID) error {
	if jobID.IsZero() {
		return ErrInvalidJobID
	}
	jobID = normalizeJobID(jobID)
	h.mu.Lock()
	if _, exists := h.jobStreams[jobID]; !exists {
		h.jobStreams[jobID] = newStream(h.opts)
	}
	h.mu.Unlock()
	return nil
}

// PublishJobLog appends a log record to a job stream.
// Returns ErrInvalidJobID if the job ID is blank or whitespace-only.
func (h *Hub) PublishJobLog(ctx context.Context, jobID domaintypes.JobID, record LogRecord) error {
	_, err := h.publishJob(ctx, jobID, domaintypes.SSEEventLog, record)
	return err
}

// PublishJobStatus appends a terminal status event and closes the job stream.
// Returns ErrInvalidJobID if the job ID is blank or whitespace-only.
func (h *Hub) PublishJobStatus(ctx context.Context, jobID domaintypes.JobID, status Status) error {
	s, err := h.publishJob(ctx, jobID, domaintypes.SSEEventDone, status)
	if err != nil {
		return err
	}
	s.finish()
	return nil
}

// SubscribeJob registers a consumer for the job stream starting after the provided id.
// Returns ErrInvalidJobID if the job ID is blank or whitespace-only.
func (h *Hub) SubscribeJob(ctx context.Context, jobID domaintypes.JobID, sinceID domaintypes.EventID) (Subscription, error) {
	if jobID.IsZero() {
		return Subscription{}, ErrInvalidJobID
	}
	jobID = normalizeJobID(jobID)
	if !sinceID.Valid() {
		return Subscription{}, errors.New("logstream: invalid since id")
	}
	if ctx.Err() != nil {
		return Subscription{}, ctx.Err()
	}
	s := h.getOrCreateJob(jobID)
	sub, history, closed := s.subscribe(sinceID)
	for _, evt := range history {
		if !sub.send(evt) {
			if sub.id >= 0 {
				s.drop(sub.id)
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
			s.drop(sub.id)
		},
	}, nil
}

// SnapshotJob returns a copy of buffered events for the job stream.
// Returns nil if the job ID is blank or the stream does not exist.
func (h *Hub) SnapshotJob(jobID domaintypes.JobID) []Event {
	if jobID.IsZero() {
		return nil
	}
	jobID = normalizeJobID(jobID)
	h.mu.RLock()
	s := h.jobStreams[jobID]
	h.mu.RUnlock()
	if s == nil {
		return nil
	}
	return s.snapshot()
}

// CloseJob tears down the job stream and removes it from the hub.
func (h *Hub) CloseJob(jobID domaintypes.JobID) {
	if jobID.IsZero() {
		return
	}
	jobID = normalizeJobID(jobID)
	h.mu.Lock()
	s, ok := h.jobStreams[jobID]
	if ok {
		delete(h.jobStreams, jobID)
	}
	h.mu.Unlock()
	if ok {
		s.finish()
	}
}

func (h *Hub) publishJob(ctx context.Context, jobID domaintypes.JobID, eventType domaintypes.SSEEventType, payload any) (*stream, error) {
	if jobID.IsZero() {
		return nil, ErrInvalidJobID
	}
	jobID = normalizeJobID(jobID)
	if err := eventType.Validate(); err != nil {
		return nil, ErrInvalidEventType
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	s := h.getOrCreateJob(jobID)
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return s, s.publish(Event{Type: eventType, Data: data})
}

func (h *Hub) getOrCreateJob(jobID domaintypes.JobID) *stream {
	h.mu.RLock()
	s := h.jobStreams[jobID]
	h.mu.RUnlock()
	if s != nil {
		return s
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	s = h.jobStreams[jobID]
	if s == nil {
		s = newStream(h.opts)
		h.jobStreams[jobID] = s
	}
	return s
}

func (h *Hub) getOrCreate(runID domaintypes.RunID) *stream {
	h.mu.RLock()
	s := h.streams[runID]
	h.mu.RUnlock()
	if s != nil {
		return s
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	s = h.streams[runID]
	if s == nil {
		s = newStream(h.opts)
		h.streams[runID] = s
	}
	return s
}

func (h *Hub) getStream(runID domaintypes.RunID) *stream {
	h.mu.RLock()
	s := h.streams[runID]
	h.mu.RUnlock()
	return s
}

// Snapshot returns a copy of buffered events for the stream.
// Returns nil if the run ID is blank or the stream does not exist.
func (h *Hub) Snapshot(runID domaintypes.RunID) []Event {
	if runID.IsZero() {
		return nil
	}
	runID = normalizeRunID(runID)
	s := h.getStream(runID)
	if s == nil {
		return nil
	}
	return s.snapshot()
}

// ---------------------------------------------------------------------------
// ring is a fixed-capacity circular buffer of Events.
// ---------------------------------------------------------------------------

type ring struct {
	items []Event
	head  int // next write index
	len   int
	cap   int
}

func newRing(capacity int) ring {
	if capacity <= 0 {
		capacity = 1
	}
	return ring{
		items: make([]Event, capacity),
		cap:   capacity,
	}
}

func (r *ring) push(evt Event) {
	r.items[r.head] = evt
	r.head = (r.head + 1) % r.cap
	if r.len < r.cap {
		r.len++
	}
}

// oldest returns the index of the oldest element.
func (r *ring) oldest() int {
	return (r.head - r.len + r.cap) % r.cap
}

// slice returns all elements in insertion order.
func (r *ring) slice() []Event {
	if r.len == 0 {
		return nil
	}
	out := make([]Event, r.len)
	start := r.oldest()
	if start+r.len <= r.cap {
		copy(out, r.items[start:start+r.len])
	} else {
		n := r.cap - start
		copy(out, r.items[start:])
		copy(out[n:], r.items[:r.len-n])
	}
	return out
}

// after returns elements with ID > since in insertion order.
func (r *ring) after(since domaintypes.EventID) []Event {
	if r.len == 0 {
		return nil
	}
	if since <= 0 {
		return r.slice()
	}
	// Walk the ring from oldest to newest, collecting matches.
	var out []Event
	idx := r.oldest()
	for i := 0; i < r.len; i++ {
		evt := r.items[idx]
		if evt.ID > since {
			if out == nil {
				out = make([]Event, 0, r.len-i)
			}
			out = append(out, evt)
		}
		idx = (idx + 1) % r.cap
	}
	return out
}

// ---------------------------------------------------------------------------
// stream is the internal per-run event manager.
// ---------------------------------------------------------------------------

type stream struct {
	opts        Options
	mu          sync.Mutex
	history     ring
	subscribers map[int]*subscriber
	nextEventID domaintypes.EventID
	nextSubID   int
	closed      bool
}

func (s *stream) snapshot() []Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.history.slice()
}

func newStream(opts Options) *stream {
	return &stream{
		opts:        opts,
		history:     newRing(opts.HistorySize),
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
	s.history.push(evt)

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
	sub := newSubscriber(-1, s.opts.BufferSize)
	s.mu.Lock()
	history := s.history.after(sinceID)
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
