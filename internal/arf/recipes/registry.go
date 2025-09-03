package recipes

import "context"

// Registry is a minimal facade for ARF recipes to enable gradual consolidation.
type Registry interface {
    // Ping verifies registry availability.
    Ping(ctx context.Context) error
}

// InMemoryRegistry is a no-op implementation used for initial wiring.
type InMemoryRegistry struct{}

func NewInMemory() *InMemoryRegistry { return &InMemoryRegistry{} }

func (r *InMemoryRegistry) Ping(ctx context.Context) error { return nil }

