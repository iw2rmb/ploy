package jobs

import (
	"sort"
	"sync"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

// State represents a job state.
type State string

const (
	// StateRunning marks an active job.
	StateRunning State = "running"
	// StateSucceeded marks a completed successful job.
	StateSucceeded State = "succeeded"
	// StateFailed marks a completed failed job.
	StateFailed State = "failed"
)

// Record captures node-local job metadata.
type Record struct {
	ID          types.JobID `json:"id"`
	State       State       `json:"state"`
	StartedAt   time.Time   `json:"started_at"`
	CompletedAt time.Time   `json:"completed_at,omitempty"`
	Error       string      `json:"error,omitempty"`
	LogStream   string      `json:"log_stream"`
}

// Options configures the Store.
type Options struct {
	// Capacity bounds the number of records retained.
	Capacity int
}

// Store is a concurrency-safe in-memory job tracker.
type Store struct {
	mu    sync.RWMutex
	cap   int
	byID  map[types.JobID]*Record
	order []types.JobID // newest-first IDs
}

// NewStore constructs a tracker with a bounded capacity (default 128).
func NewStore(opts Options) *Store {
	cap := opts.Capacity
	if cap <= 0 {
		cap = 128
	}
	return &Store{cap: cap, byID: make(map[types.JobID]*Record), order: make([]types.JobID, 0, cap)}
}

// Start records a job start timestamp and running state.
func (s *Store) Start(id types.JobID) {
	if s == nil || id.IsZero() {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if rec, ok := s.byID[id]; ok {
		// Refresh ordering; job already known.
		rec.State = StateRunning
		s.bumpToFrontLocked(id)
		return
	}
	rec := &Record{ID: id, State: StateRunning, StartedAt: time.Now().UTC(), LogStream: id.String()}
	s.byID[id] = rec
	s.order = append([]types.JobID{id}, s.order...)
	if len(s.order) > s.cap {
		// Evict oldest and its record.
		evict := s.order[len(s.order)-1]
		s.order = s.order[:len(s.order)-1]
		delete(s.byID, evict)
	}
}

// Complete marks a job terminal state and stamps completion time.
func (s *Store) Complete(id types.JobID, state State, errMsg string) {
	if s == nil || id.IsZero() {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.byID[id]
	if !ok {
		rec = &Record{ID: id, StartedAt: time.Now().UTC(), LogStream: id.String()}
		s.byID[id] = rec
		s.order = append([]types.JobID{id}, s.order...)
	}
	if state != StateSucceeded && state != StateFailed {
		state = StateFailed
	}
	rec.State = state
	rec.CompletedAt = time.Now().UTC()
	rec.Error = errMsg
	s.bumpToFrontLocked(id)
}

// Get returns a copy of the job record for the given id.
func (s *Store) Get(id types.JobID) (Record, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec, ok := s.byID[id]
	if !ok {
		return Record{}, false
	}
	return *rec, true
}

// List returns newest-first job records (bounded by capacity).
func (s *Store) List() []Record {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Record, 0, len(s.order))
	for _, id := range s.order {
		if rec, ok := s.byID[id]; ok {
			out = append(out, *rec)
		}
	}
	return out
}

func (s *Store) bumpToFrontLocked(id types.JobID) {
	// Remove id from order and re-insert at front.
	idx := -1
	for i, v := range s.order {
		if v == id {
			idx = i
			break
		}
	}
	if idx >= 0 {
		s.order = append(append([]types.JobID{}, s.order[:idx]...), s.order[idx+1:]...)
	}
	s.order = append([]types.JobID{id}, s.order...)
	// Ensure uniqueness if duplicates slipped in.
	uniq := make(map[types.JobID]struct{}, len(s.order))
	dedup := make([]types.JobID, 0, len(s.order))
	for _, v := range s.order {
		if _, seen := uniq[v]; seen {
			continue
		}
		uniq[v] = struct{}{}
		dedup = append(dedup, v)
	}
	// Keep newest-first order stable after de-dup.
	sort.SliceStable(dedup, func(i, j int) bool { return indexOf(s.order, dedup[i]) < indexOf(s.order, dedup[j]) })
	s.order = dedup
}

func indexOf(list []types.JobID, id types.JobID) int {
	for i, v := range list {
		if v == id {
			return i
		}
	}
	return len(list)
}
