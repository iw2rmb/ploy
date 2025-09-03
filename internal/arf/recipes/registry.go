package recipes

import "context"

// Recipe represents a minimal recipe descriptor for listing endpoints.
type Recipe struct {
    ID       string   `json:"id"`
    Name     string   `json:"name"`
    Language string   `json:"language,omitempty"`
    Description string `json:"description,omitempty"`
    Pack        string `json:"pack,omitempty"`
    Version     string `json:"version,omitempty"`
    Tags     []string `json:"tags,omitempty"`
}

// Filters for listing/searching recipes (initial slice)
type Filters struct {
    Language string
    Tag      string
}

// Registry is a minimal facade for ARF recipes to enable gradual consolidation.
type Registry interface {
    // Ping verifies registry availability.
    Ping(ctx context.Context) error
    // List returns a list of recipes matching filters.
    List(ctx context.Context, f Filters) ([]Recipe, error)
    // Get returns a single recipe by ID.
    Get(ctx context.Context, id string) (*Recipe, error)
}

// InMemoryRegistry is a no-op implementation used for initial wiring.
type InMemoryRegistry struct{}

func NewInMemory() *InMemoryRegistry { return &InMemoryRegistry{} }

func (r *InMemoryRegistry) Ping(ctx context.Context) error { return nil }

func (r *InMemoryRegistry) List(ctx context.Context, f Filters) ([]Recipe, error) {
    // Empty initial slice; future slices can adapt from storage-backed catalog
    return []Recipe{}, nil
}

func (r *InMemoryRegistry) Get(ctx context.Context, id string) (*Recipe, error) {
    // No data in in-memory initial slice
    return nil, nil
}
