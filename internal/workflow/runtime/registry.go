package runtime

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/iw2rmb/ploy/internal/workflow/runner"
)

// ErrAdapterNotFound is returned when a runtime adapter cannot be resolved.
var ErrAdapterNotFound = errors.New("runtime: adapter not found")

// ErrAdapterDisabled indicates the adapter is intentionally disabled (for example, credentials missing).
var ErrAdapterDisabled = errors.New("runtime: adapter disabled")

// Adapter exposes scheduler integrations capable of executing workflow stages.
type Adapter interface {
    Metadata() AdapterMetadata
    Connect(ctx context.Context) (runner.RuntimeClient, error)
}

// AdapterMetadata describes a runtime adapter.
type AdapterMetadata struct {
	Name        string
	Aliases     []string
	Description string
}

// Registry stores runtime adapters keyed by name/alias.
type Registry struct {
	mu       sync.RWMutex
	entries  map[string]adapterEntry
	ordering []string
}

type adapterEntry struct {
	adapter Adapter
	meta    AdapterMetadata
}

// NewRegistry constructs an empty runtime adapter registry.
func NewRegistry() *Registry {
	return &Registry{
		entries: make(map[string]adapterEntry),
	}
}

// Register installs the provided adapter using its metadata as keys.
func (r *Registry) Register(adapter Adapter) error {
	if r == nil {
		return errors.New("runtime: registry is nil")
	}
	if adapter == nil {
		return errors.New("runtime: adapter is nil")
	}

	meta, err := sanitizeMetadata(adapter.Metadata())
	if err != nil {
		return fmt.Errorf("runtime: invalid adapter metadata: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.entries[meta.Name]; exists {
		return fmt.Errorf("runtime: adapter already registered for %q", meta.Name)
	}
	r.entries[meta.Name] = adapterEntry{adapter: adapter, meta: meta}
	r.ordering = append(r.ordering, meta.Name)
	for _, alias := range meta.Aliases {
		if _, exists := r.entries[alias]; exists {
			return fmt.Errorf("runtime: adapter already registered for %q", alias)
		}
		r.entries[alias] = adapterEntry{adapter: adapter, meta: meta}
	}
	sort.Strings(r.ordering)
	return nil
}

// Resolve looks up an adapter by name or alias, returning its canonical metadata.
func (r *Registry) Resolve(name string) (Adapter, AdapterMetadata, error) {
	if r == nil {
		return nil, AdapterMetadata{}, errors.New("runtime: registry is nil")
	}
	normalized := normalizeKey(name)

	r.mu.RLock()
	defer r.mu.RUnlock()

	if normalized == "" {
		if len(r.ordering) == 0 {
			return nil, AdapterMetadata{}, ErrAdapterNotFound
		}
		entry := r.entries[r.ordering[0]]
		return entry.adapter, entry.meta, nil
	}

	entry, ok := r.entries[normalized]
	if !ok {
		return nil, AdapterMetadata{}, fmt.Errorf("%w: %s", ErrAdapterNotFound, normalized)
	}
	return entry.adapter, entry.meta, nil
}

// List returns adapter metadata sorted by name.
func (r *Registry) List() []AdapterMetadata {
	if r == nil {
		return nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.ordering) == 0 {
		return nil
	}
	catalog := make([]AdapterMetadata, 0, len(r.ordering))
	seen := make(map[string]struct{}, len(r.ordering))
	for _, key := range r.ordering {
		entry, ok := r.entries[key]
		if !ok {
			continue
		}
		if _, exists := seen[entry.meta.Name]; exists {
			continue
		}
		catalog = append(catalog, entry.meta)
		seen[entry.meta.Name] = struct{}{}
	}
	return catalog
}

func sanitizeMetadata(meta AdapterMetadata) (AdapterMetadata, error) {
	name := normalizeKey(meta.Name)
	if name == "" {
		return AdapterMetadata{}, errors.New("name is required")
	}

	aliases := make([]string, 0, len(meta.Aliases))
	aliasSet := make(map[string]struct{})
	for _, alias := range meta.Aliases {
		normalized := normalizeKey(alias)
		if normalized == "" || normalized == name {
			continue
		}
		if _, exists := aliasSet[normalized]; exists {
			continue
		}
		aliasSet[normalized] = struct{}{}
		aliases = append(aliases, normalized)
	}
	sort.Strings(aliases)

	return AdapterMetadata{
		Name:        name,
		Aliases:     aliases,
		Description: strings.TrimSpace(meta.Description),
	}, nil
}

func normalizeKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
