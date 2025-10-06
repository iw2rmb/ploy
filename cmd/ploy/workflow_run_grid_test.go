package main

import (
	"context"
	"io"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"fmt"

	"github.com/iw2rmb/ploy/internal/workflow/aster"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/grid"
	"github.com/iw2rmb/ploy/internal/workflow/lanes"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
)

func TestHandleWorkflowRunUsesInMemoryGridWhenUnset(t *testing.T) {
	withStubWorkspacePreparer(t)
	fakeRunner := &recordingRunner{}
	prevRunner := runnerExecutor
	prevBusFactory := eventsFactory
	prevManifestLoader := manifestRegistryLoader
	prevManifestDir := manifestConfigDir
	prevLocatorLoader := asterLocatorLoader
	prevAsterDir := asterConfigDir
	prevLaneLoader := laneRegistryLoader
	prevLaneDir := laneConfigDir
	defer func() {
		runnerExecutor = prevRunner
		eventsFactory = prevBusFactory
		manifestRegistryLoader = prevManifestLoader
		manifestConfigDir = prevManifestDir
		asterLocatorLoader = prevLocatorLoader
		asterConfigDir = prevAsterDir
		laneRegistryLoader = prevLaneLoader
		laneConfigDir = prevLaneDir
	}()

	runnerExecutor = fakeRunner
	eventsFactory = func(tenant string) (runner.EventsClient, error) { return contracts.NewInMemoryBus(tenant), nil }
	manifestRegistryLoader = func(dir string) (runner.ManifestCompiler, error) {
		return &stubManifestCompiler{compiled: defaultManifestPayload()}, nil
	}
	manifestConfigDir = "ignored"
	asterLocatorLoader = func(dir string) (aster.Locator, error) { return &recordingLocator{dir: dir}, nil }
	asterConfigDir = "ignored"
	laneRegistryLoader = func(dir string) (laneRegistry, error) {
		desc := lanes.Description{Lane: lanes.Spec{Name: "mods-plan", CacheNamespace: "mods-plan"}, CacheKey: "stub-cache"}
		desc.Lane.Job.Image = "registry.dev/ploy/mods-plan:latest"
		desc.Lane.Job.Command = []string{"mods-plan"}
		desc.Lane.Job.Env = map[string]string{}
		desc.Lane.Job.Resources = lanes.JobResources{CPU: "1000m", Memory: "1Gi"}
		return &fakeLaneRegistry{description: desc}, nil
	}
	laneConfigDir = "ignored"
	t.Setenv("GRID_ENDPOINT", "")

	err := handleWorkflowRun([]string{"--tenant", "acme", "--ticket", "ticket-123"}, io.Discard)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := fakeRunner.opts.Grid.(*runner.InMemoryGrid); !ok {
		t.Fatalf("expected in-memory grid client, got %T", fakeRunner.opts.Grid)
	}
}

func TestHandleWorkflowRunUsesGridEndpointClient(t *testing.T) {
	withStubWorkspacePreparer(t)
	fakeRunner := &recordingRunner{}
	prevRunner := runnerExecutor
	prevBusFactory := eventsFactory
	prevManifestLoader := manifestRegistryLoader
	prevManifestDir := manifestConfigDir
	prevLocatorLoader := asterLocatorLoader
	prevAsterDir := asterConfigDir
	prevLaneLoader := laneRegistryLoader
	prevLaneDir := laneConfigDir
	prevDiscovery := fetchClusterInfoFn
	resetDiscoveryState()
	defer func() {
		runnerExecutor = prevRunner
		eventsFactory = prevBusFactory
		manifestRegistryLoader = prevManifestLoader
		manifestConfigDir = prevManifestDir
		asterLocatorLoader = prevLocatorLoader
		asterConfigDir = prevAsterDir
		laneRegistryLoader = prevLaneLoader
		laneConfigDir = prevLaneDir
		fetchClusterInfoFn = prevDiscovery
	}()

	runnerExecutor = fakeRunner
	var observedConfig integrationConfig
	eventsFactory = func(tenant string) (runner.EventsClient, error) {
		cfg, err := resolveIntegrationConfig(context.Background())
		if err != nil {
			return nil, err
		}
		observedConfig = cfg
		return contracts.NewInMemoryBus(tenant), nil
	}
	manifestRegistryLoader = func(dir string) (runner.ManifestCompiler, error) {
		return &stubManifestCompiler{compiled: defaultManifestPayload()}, nil
	}
	manifestConfigDir = "ignored"
	asterLocatorLoader = func(dir string) (aster.Locator, error) { return &recordingLocator{dir: dir}, nil }
	asterConfigDir = "ignored"
	laneRegistryLoader = func(dir string) (laneRegistry, error) {
		desc := lanes.Description{Lane: lanes.Spec{Name: "mods-plan", CacheNamespace: "mods-plan"}, CacheKey: "stub-cache"}
		desc.Lane.Job.Image = "registry.dev/ploy/mods-plan:latest"
		desc.Lane.Job.Command = []string{"mods-plan"}
		desc.Lane.Job.Env = map[string]string{}
		desc.Lane.Job.Resources = lanes.JobResources{CPU: "1000m", Memory: "1Gi"}
		return &fakeLaneRegistry{description: desc}, nil
	}
	laneConfigDir = "ignored"
	fetchClusterInfoFn = func(ctx context.Context, endpoint string) (clusterInfo, error) {
		return clusterInfo{
			APIEndpoint:   "https://grid.dev",
			JetStreamURLs: []string{"nats://grid.dev:4222", "nats://grid.dev:5222"},
			IPFSGateway:   "https://ipfs.grid.dev",
			Features:      map[string]string{"workspace_api": "enabled"},
			Version:       "2025.9.29",
		}, nil
	}
	t.Setenv("GRID_ENDPOINT", "https://grid.dev")

	err := handleWorkflowRun([]string{"--tenant", "acme", "--ticket", "ticket-123"}, io.Discard)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := fakeRunner.opts.Grid.(*grid.Client); !ok {
		t.Fatalf("expected grid client, got %T", fakeRunner.opts.Grid)
	}
	if observedConfig.JetStreamURL != "nats://grid.dev:4222" {
		t.Fatalf("unexpected primary jetstream url: %q", observedConfig.JetStreamURL)
	}
	if len(observedConfig.JetStreamURLs) != 2 {
		t.Fatalf("unexpected jetstream routes: %+v", observedConfig.JetStreamURLs)
	}
	if observedConfig.APIEndpoint != "https://grid.dev" {
		t.Fatalf("unexpected api endpoint: %q", observedConfig.APIEndpoint)
	}
	if observedConfig.Version != "2025.9.29" {
		t.Fatalf("unexpected version: %q", observedConfig.Version)
	}
	if !observedConfig.FeatureEnabled("workspace_api") {
		t.Fatal("expected workspace_api feature to be enabled")
	}
}

func TestHandleWorkflowRunFailsForInvalidGridEndpoint(t *testing.T) {
	withStubWorkspacePreparer(t)
	prevRunner := runnerExecutor
	prevBusFactory := eventsFactory
	prevManifestLoader := manifestRegistryLoader
	prevManifestDir := manifestConfigDir
	prevLocatorLoader := asterLocatorLoader
	prevAsterDir := asterConfigDir
	prevLaneLoader := laneRegistryLoader
	prevLaneDir := laneConfigDir
	prevDiscovery := fetchClusterInfoFn
	resetDiscoveryState()
	defer func() {
		runnerExecutor = prevRunner
		eventsFactory = prevBusFactory
		manifestRegistryLoader = prevManifestLoader
		manifestConfigDir = prevManifestDir
		asterLocatorLoader = prevLocatorLoader
		asterConfigDir = prevAsterDir
		laneRegistryLoader = prevLaneLoader
		laneConfigDir = prevLaneDir
		fetchClusterInfoFn = prevDiscovery
	}()

	runnerExecutor = &recordingRunner{}
	eventsFactory = func(tenant string) (runner.EventsClient, error) { return contracts.NewInMemoryBus(tenant), nil }
	manifestRegistryLoader = func(dir string) (runner.ManifestCompiler, error) {
		return &stubManifestCompiler{compiled: defaultManifestPayload()}, nil
	}
	manifestConfigDir = "ignored"
	asterLocatorLoader = func(dir string) (aster.Locator, error) { return &recordingLocator{dir: dir}, nil }
	asterConfigDir = "ignored"
	laneRegistryLoader = func(dir string) (laneRegistry, error) {
		desc := lanes.Description{Lane: lanes.Spec{Name: "mods-plan", CacheNamespace: "mods-plan"}, CacheKey: "stub-cache"}
		desc.Lane.Job.Image = "registry.dev/ploy/mods-plan:latest"
		desc.Lane.Job.Command = []string{"mods-plan"}
		desc.Lane.Job.Env = map[string]string{}
		desc.Lane.Job.Resources = lanes.JobResources{CPU: "1000m", Memory: "1Gi"}
		return &fakeLaneRegistry{description: desc}, nil
	}
	laneConfigDir = "ignored"
	fetchClusterInfoFn = func(ctx context.Context, endpoint string) (clusterInfo, error) {
		return clusterInfo{}, fmt.Errorf("invalid endpoint")
	}
	t.Setenv("GRID_ENDPOINT", "://invalid")

	err := handleWorkflowRun([]string{"--tenant", "acme", "--ticket", "ticket-123"}, io.Discard)
	if err == nil {
		t.Fatal("expected error for invalid grid endpoint")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "grid") {
		t.Fatalf("expected grid error context, got %v", err)
	}
}

func TestHandleWorkflowRunFailsWhenJetStreamURLInvalid(t *testing.T) {
	prevFactory := eventsFactory
	prevManifestDir := manifestConfigDir
	prevLaneDir := laneConfigDir
	prevAsterDir := asterConfigDir
	prevDiscovery := fetchClusterInfoFn
	defer func() {
		eventsFactory = prevFactory
		manifestConfigDir = prevManifestDir
		laneConfigDir = prevLaneDir
		asterConfigDir = prevAsterDir
		fetchClusterInfoFn = prevDiscovery
		resetDiscoveryState()
	}()

	eventsFactory = defaultEventsFactory
	resetDiscoveryState()
	fetchClusterInfoFn = func(ctx context.Context, endpoint string) (clusterInfo, error) {
		return clusterInfo{JetStreamURLs: []string{"nats://127.0.0.1:1"}}, nil
	}
	t.Setenv("GRID_ENDPOINT", "https://grid.dev")

	_, file, _, _ := runtime.Caller(0)
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	manifestConfigDir = filepath.Join(repoRoot, "configs", "manifests")
	laneConfigDir = filepath.Join(repoRoot, "..", "ploy-lanes-catalog")
	asterConfigDir = filepath.Join(repoRoot, "configs", "aster")

	err := handleWorkflowRun([]string{"--tenant", "acme", "--ticket", "auto"}, io.Discard)
	if err == nil {
		t.Fatal("expected error when JetStream connection fails")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "jetstream") {
		t.Fatalf("expected jetstream error context, got %v", err)
	}
}
