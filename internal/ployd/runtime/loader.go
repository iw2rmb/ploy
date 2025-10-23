package runtime

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/iw2rmb/ploy/internal/ployd/config"
	workflowruntime "github.com/iw2rmb/ploy/internal/workflow/runtime"
)

// Factory constructs runtime adapters from plugin configuration.
type Factory interface {
	Name() string
	Build(ctx context.Context, cfg config.RuntimePluginConfig) (workflowruntime.Adapter, error)
}

// Loader installs runtime plugins into the registry.
type Loader struct {
	registry  *workflowruntime.Registry
	mu        sync.Mutex
	factories map[string]Factory
	installed map[string]struct{}
}

// NewLoader constructs a Loader bound to the provided registry.
func NewLoader(registry *workflowruntime.Registry) *Loader {
	return &Loader{
		registry:  registry,
		factories: make(map[string]Factory),
		installed: make(map[string]struct{}),
	}
}

// RegisterFactory registers a plugin factory.
func (l *Loader) RegisterFactory(name string, factory Factory) {
	if factory == nil {
		return
	}
	key := strings.ToLower(strings.TrimSpace(name))
	if key == "" {
		key = strings.ToLower(strings.TrimSpace(factory.Name()))
	}
	if key == "" {
		return
	}
	l.mu.Lock()
	l.factories[key] = factory
	l.mu.Unlock()
}

// Apply installs the configured plugins into the registry.
func (l *Loader) Apply(ctx context.Context, cfg config.RuntimeConfig) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, plugin := range cfg.Plugins {
		name := strings.ToLower(strings.TrimSpace(plugin.Name))
		if name == "" {
			continue
		}
		if plugin.Enabled != nil && !*plugin.Enabled {
			continue
		}
		if _, exists := l.installed[name]; exists {
			continue
		}
		factory, ok := l.factories[name]
		if !ok {
			return fmt.Errorf("runtime: no factory for plugin %s", name)
		}
		adapter, err := factory.Build(ctx, plugin)
		if err != nil {
			return fmt.Errorf("runtime: build plugin %s: %w", name, err)
		}
		if err := l.registry.Register(adapter); err != nil {
			return err
		}
		l.installed[name] = struct{}{}
	}
	return nil
}
