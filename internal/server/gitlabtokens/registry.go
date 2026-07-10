package gitlabtokens

import (
	"context"
	"strings"
	"sync"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

type Registry struct {
	mu      sync.RWMutex
	entries map[string]*entry
	runHash map[types.RunID]string
}

type entry struct {
	token string
	runs  map[types.RunID]struct{}
}

func NewRegistry() *Registry {
	return &Registry{
		entries: make(map[string]*entry),
		runHash: make(map[types.RunID]string),
	}
}

func (r *Registry) Register(hash, token string, runIDs []types.RunID) {
	if r == nil {
		return
	}
	hash = strings.TrimSpace(hash)
	token = strings.TrimSpace(token)
	if hash == "" || token == "" || len(runIDs) == 0 {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	e := r.entries[hash]
	if e == nil {
		e = &entry{runs: make(map[types.RunID]struct{})}
		r.entries[hash] = e
	}
	e.token = token
	for _, runID := range runIDs {
		if runID.IsZero() {
			continue
		}
		if oldHash := r.runHash[runID]; oldHash != "" && oldHash != hash {
			r.releaseRunLocked(runID)
		}
		e.runs[runID] = struct{}{}
		r.runHash[runID] = hash
	}
}

func (r *Registry) Token(hash string) (string, bool) {
	if r == nil {
		return "", false
	}
	hash = strings.TrimSpace(hash)
	if hash == "" {
		return "", false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	e := r.entries[hash]
	if e == nil || strings.TrimSpace(e.token) == "" {
		return "", false
	}
	return e.token, true
}

func (r *Registry) ReleaseRuns(runIDs []types.RunID) {
	if r == nil || len(runIDs) == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, runID := range runIDs {
		r.releaseRunLocked(runID)
	}
}

func (r *Registry) releaseRunLocked(runID types.RunID) {
	hash := r.runHash[runID]
	if hash == "" {
		return
	}
	delete(r.runHash, runID)
	e := r.entries[hash]
	if e == nil {
		return
	}
	delete(e.runs, runID)
	if len(e.runs) == 0 {
		delete(r.entries, hash)
	}
}

func (r *Registry) ReleaseWave(ctx context.Context, st store.Store, waveID types.WaveID) {
	if r == nil || st == nil || waveID.IsZero() {
		return
	}
	runs, err := st.ListRunsByWave(ctx, waveID)
	if err != nil {
		return
	}
	runIDs := make([]types.RunID, 0, len(runs))
	for _, run := range runs {
		runIDs = append(runIDs, run.ID)
	}
	r.ReleaseRuns(runIDs)
}
