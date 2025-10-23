package runtime_test

import (
	"context"
	"testing"

	"github.com/iw2rmb/ploy/internal/ployd/config"
	"github.com/iw2rmb/ploy/internal/ployd/runtime"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
	workflowruntime "github.com/iw2rmb/ploy/internal/workflow/runtime"
)

func TestLoaderRegistersPlugins(t *testing.T) {
	registry := workflowruntime.NewRegistry()
	loader := runtime.NewLoader(registry)
	loader.RegisterFactory("local", &stubFactory{name: "local"})

	cfg := config.RuntimeConfig{
		DefaultAdapter: "local",
		Plugins: []config.RuntimePluginConfig{
			{Name: "local", Config: map[string]any{"path": "/var/lib/ploy"}},
		},
	}
	if err := loader.Apply(context.Background(), cfg); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	adapter, meta, err := registry.Resolve("local")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if meta.Name != "local" {
		t.Fatalf("metadata.Name = %s", meta.Name)
	}
	if adapter == nil {
		t.Fatal("adapter nil")
	}
}

func TestLoaderSkipsDisabledPlugins(t *testing.T) {
	registry := workflowruntime.NewRegistry()
	loader := runtime.NewLoader(registry)
	loader.RegisterFactory("nomad", &stubFactory{name: "nomad"})
	cfg := config.RuntimeConfig{
		Plugins: []config.RuntimePluginConfig{
			{Name: "nomad", Enabled: boolPtr(false)},
		},
	}
	if err := loader.Apply(context.Background(), cfg); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if _, _, err := registry.Resolve("nomad"); err == nil {
		t.Fatal("expected resolve error for disabled plugin")
	}
}

type stubFactory struct {
	name string
}

func (s *stubFactory) Name() string { return s.name }

func (s *stubFactory) Build(ctx context.Context, cfg config.RuntimePluginConfig) (workflowruntime.Adapter, error) {
	_ = ctx
	_ = cfg
	return &stubAdapter{name: s.name}, nil
}

type stubAdapter struct {
	name string
}

func (s *stubAdapter) Metadata() workflowruntime.AdapterMetadata {
	return workflowruntime.AdapterMetadata{Name: s.name}
}

func (s *stubAdapter) Connect(ctx context.Context) (runner.GridClient, error) {
	_ = ctx
	return nil, nil
}

func boolPtr(v bool) *bool { return &v }
