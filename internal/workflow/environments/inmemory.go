package environments

import (
	"context"
	"sync"
)

// HydrationRecord captures a cache hydration request recorded by InMemoryHydrator.
type HydrationRecord struct {
	Lane     string
	CacheKey string
}

// InMemoryHydrator records cache hydration requests without executing external side effects.
type InMemoryHydrator struct {
	mu    sync.Mutex
	calls []HydrationRecord
}

// NewInMemoryHydrator constructs an in-memory hydrator suitable for tests and CLI stubs.
func NewInMemoryHydrator() *InMemoryHydrator {
	return &InMemoryHydrator{}
}

// HydrateCache records the hydration request.
func (h *InMemoryHydrator) HydrateCache(ctx context.Context, lane string, cacheKey string) error {
	_ = ctx
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, HydrationRecord{Lane: lane, CacheKey: cacheKey})
	return nil
}

// Calls returns a copy of the recorded hydration requests.
func (h *InMemoryHydrator) Calls() []HydrationRecord {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]HydrationRecord, len(h.calls))
	copy(out, h.calls)
	return out
}
