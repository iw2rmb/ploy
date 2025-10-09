package main

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	discovery "github.com/iw2rmb/grid/sdk/discovery/go"
	gridclient "github.com/iw2rmb/grid/sdk/gridclient/go"

	"github.com/iw2rmb/ploy/internal/workflow/aster"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/grid"
	"github.com/iw2rmb/ploy/internal/workflow/lanes"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
)

func TestHandleModRunUsesInMemoryGridWhenCredentialsMissing(t *testing.T) {
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
	prevStateDir := t.TempDir()
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

	t.Setenv(gridIDEnv, "")
	t.Setenv(gridAPIKeyEnv, "")
	t.Setenv(gridClientStateEnv, prevStateDir)

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

	err := handleModRun([]string{"--tenant", "acme", "--ticket", "ticket-123"}, io.Discard)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := fakeRunner.opts.Grid.(*runner.InMemoryGrid); !ok {
		t.Fatalf("expected in-memory grid client, got %T", fakeRunner.opts.Grid)
	}
}

func TestHandleModRunUsesSharedGridClient(t *testing.T) {
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

	t.Setenv(gridIDEnv, "grid-dev")
	t.Setenv(gridAPIKeyEnv, "secret")
	t.Setenv(gridClientStateEnv, t.TempDir())

	status := gridclient.Status{
		Beacon: gridclient.BeaconStatus{
			APIEndpoint:      "https://api.grid.dev",
			WorkflowEndpoint: "https://workflow.grid.dev",
		},
		Discovery: discovery.ClusterInfo{
			APIEndpoint:   "https://api.grid.dev",
			JetStreamURLs: []string{"nats://grid.dev:4222", "nats://grid.dev:5222"},
			IPFSGateway:   "https://ipfs.grid.dev",
			Features:      map[string]string{"workspace_api": "enabled"},
			Version:       "2025.9.29",
		},
	}
	withGridClientStub(t, newStubGridClient(status))

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

	err := handleModRun([]string{"--tenant", "acme", "--ticket", "ticket-123"}, io.Discard)
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
	if observedConfig.APIEndpoint != "https://api.grid.dev" {
		t.Fatalf("unexpected api endpoint: %q", observedConfig.APIEndpoint)
	}
	if observedConfig.Version != "2025.9.29" {
		t.Fatalf("unexpected version: %q", observedConfig.Version)
	}
	if !observedConfig.FeatureEnabled("workspace_api") {
		t.Fatal("expected workspace_api feature to be enabled")
	}
}

func TestHandleModRunFailsWhenGridClientUnavailable(t *testing.T) {
	withStubWorkspacePreparer(t)
	prevRunner := runnerExecutor
	prevBusFactory := eventsFactory
	prevManifestLoader := manifestRegistryLoader
	prevManifestDir := manifestConfigDir
	prevLaneLoader := laneRegistryLoader
	prevLaneDir := laneConfigDir
	defer func() {
		runnerExecutor = prevRunner
		eventsFactory = prevBusFactory
		manifestRegistryLoader = prevManifestLoader
		manifestConfigDir = prevManifestDir
		laneRegistryLoader = prevLaneLoader
		laneConfigDir = prevLaneDir
	}()

	t.Setenv(gridIDEnv, "grid-dev")
	t.Setenv(gridAPIKeyEnv, "secret")
	t.Setenv(gridClientStateEnv, t.TempDir())

	sentinel := fmt.Errorf("gridclient: boom")
	prevNew := newGridClient
	resetGridClientState()
	newGridClient = func(context.Context, gridclient.Config) (gridClientAPI, error) {
		return nil, sentinel
	}
	t.Cleanup(func() {
		newGridClient = prevNew
		resetGridClientState()
	})

	runnerExecutor = &recordingRunner{}
	eventsFactory = func(tenant string) (runner.EventsClient, error) { return contracts.NewInMemoryBus(tenant), nil }
	manifestRegistryLoader = func(dir string) (runner.ManifestCompiler, error) {
		return &stubManifestCompiler{compiled: defaultManifestPayload()}, nil
	}
	manifestConfigDir = "ignored"
	laneRegistryLoader = func(dir string) (laneRegistry, error) {
		desc := lanes.Description{Lane: lanes.Spec{Name: "mods-plan", CacheNamespace: "mods-plan"}, CacheKey: "stub-cache"}
		desc.Lane.Job.Image = "registry.dev/ploy/mods-plan:latest"
		desc.Lane.Job.Command = []string{"mods-plan"}
		desc.Lane.Job.Env = map[string]string{}
		desc.Lane.Job.Resources = lanes.JobResources{CPU: "1000m", Memory: "1Gi"}
		return &fakeLaneRegistry{description: desc}, nil
	}
	laneConfigDir = "ignored"

	err := handleModRun([]string{"--tenant", "acme", "--ticket", "ticket-123"}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), sentinel.Error()) {
		t.Fatalf("expected error %v, got %v", sentinel, err)
	}
}

func TestHandleModRunFailsWhenJetStreamURLInvalid(t *testing.T) {
	prevFactory := eventsFactory
	prevManifestDir := manifestConfigDir
	prevLaneDir := laneConfigDir
	prevAsterDir := asterConfigDir
	defer func() {
		eventsFactory = prevFactory
		manifestConfigDir = prevManifestDir
		laneConfigDir = prevLaneDir
		asterConfigDir = prevAsterDir
	}()

	t.Setenv(gridIDEnv, "grid-dev")
	t.Setenv(gridAPIKeyEnv, "secret")
	t.Setenv(gridClientStateEnv, t.TempDir())

	status := gridclient.Status{
		Beacon: gridclient.BeaconStatus{
			APIEndpoint:      "https://api.grid.dev",
			WorkflowEndpoint: "https://workflow.grid.dev",
		},
		Discovery: discovery.ClusterInfo{
			APIEndpoint:   "https://api.grid.dev",
			JetStreamURLs: []string{"nats://127.0.0.1:1"},
			IPFSGateway:   "",
			Features:      map[string]string{},
			Version:       "2025.9.29",
		},
	}
	withGridClientStub(t, newStubGridClient(status))

	eventsFactory = defaultEventsFactory

	_, file, _, _ := runtime.Caller(0)
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	manifestConfigDir = filepath.Join(repoRoot, "configs", "manifests")
	laneConfigDir = filepath.Join(repoRoot, "..", "ploy-lanes-catalog")
	asterConfigDir = filepath.Join(repoRoot, "configs", "aster")

	err := handleModRun([]string{"--tenant", "acme", "--ticket", "auto"}, io.Discard)
	if err == nil {
		t.Fatal("expected error when JetStream connection fails")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "jetstream") {
		t.Fatalf("expected jetstream error context, got %v", err)
	}
}
