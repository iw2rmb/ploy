package main

import (
	"context"
	"errors"
	"io"
	"testing"

	discovery "github.com/iw2rmb/grid/sdk/discovery/go"
	gridclient "github.com/iw2rmb/grid/sdk/gridclient/go"

	"github.com/iw2rmb/ploy/internal/workflow/aster"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
	"github.com/iw2rmb/ploy/internal/workflow/runtime"
)

func TestHandleModRunUsesLocalRuntimeByDefault(t *testing.T) {
	withStubWorkspacePreparer(t)

	fakeRunner := &recordingRunner{}
	prevRunner := runnerExecutor
	prevEventsFactory := eventsFactory
	prevManifestLoader := manifestRegistryLoader
	prevManifestDir := manifestConfigDir
	prevLocatorLoader := asterLocatorLoader
	prevAsterDir := asterConfigDir
	prevJobComposerFactory := jobComposerFactory
	prevCacheComposerFactory := cacheComposerFactory
	defer func() {
		runnerExecutor = prevRunner
		eventsFactory = prevEventsFactory
		manifestRegistryLoader = prevManifestLoader
		manifestConfigDir = prevManifestDir
		asterLocatorLoader = prevLocatorLoader
		asterConfigDir = prevAsterDir
		jobComposerFactory = prevJobComposerFactory
		cacheComposerFactory = prevCacheComposerFactory
	}()

	t.Setenv(gridIDEnv, "")
	t.Setenv(gridIDFallbackEnv, "")
	t.Setenv(gridAPIKeyEnv, "")
	t.Setenv(gridAPIKeyFallbackEnv, "")
	t.Setenv(gridClientStateEnv, t.TempDir())

	runnerExecutor = fakeRunner
	eventsFactory = func(tenant string) (runner.EventsClient, error) { return contracts.NewInMemoryBus(tenant), nil }
	manifestRegistryLoader = func(dir string) (runner.ManifestCompiler, error) {
		return &stubManifestCompiler{compiled: defaultManifestPayload()}, nil
	}
	manifestConfigDir = "ignored"
	asterLocatorLoader = func(dir string) (aster.Locator, error) { return &recordingLocator{dir: dir}, nil }
	asterConfigDir = "ignored"
	jobComposerFactory = func() runner.JobComposer { return runner.NewStaticJobComposer() }
	cacheComposerFactory = func() runner.CacheComposer { return runner.NewDefaultCacheComposer() }

	if err := handleModRun([]string{"--tenant", "acme", "--ticket", "ticket-123"}, io.Discard); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := fakeRunner.opts.Grid.(*runtime.LocalStepClient); !ok {
		t.Fatalf("expected local step client, got %T", fakeRunner.opts.Grid)
	}
}

func TestHandleModRunUsesSharedGridClient(t *testing.T) {
	withStubWorkspacePreparer(t)

	fakeRunner := &recordingRunner{}
	prevRunner := runnerExecutor
	prevEventsFactory := eventsFactory
	prevManifestLoader := manifestRegistryLoader
	prevManifestDir := manifestConfigDir
	prevLocatorLoader := asterLocatorLoader
	prevAsterDir := asterConfigDir
	prevJobComposerFactory := jobComposerFactory
	prevCacheComposerFactory := cacheComposerFactory
	defer func() {
		runnerExecutor = prevRunner
		eventsFactory = prevEventsFactory
		manifestRegistryLoader = prevManifestLoader
		manifestConfigDir = prevManifestDir
		asterLocatorLoader = prevLocatorLoader
		asterConfigDir = prevAsterDir
		jobComposerFactory = prevJobComposerFactory
		cacheComposerFactory = prevCacheComposerFactory
	}()

	t.Setenv(gridIDEnv, "grid-dev")
	t.Setenv(gridIDFallbackEnv, "grid-dev")
	t.Setenv(gridAPIKeyEnv, "secret")
	t.Setenv(gridAPIKeyFallbackEnv, "secret")
	t.Setenv(gridClientStateEnv, t.TempDir())
	t.Setenv(runtimeAdapterEnv, "grid")

	status := gridclient.Status{
		Beacon: gridclient.BeaconStatus{
			APIEndpoint:      "https://api.grid.dev",
			WorkflowEndpoint: "https://workflow.grid.dev",
		},
		Discovery: discovery.ClusterInfo{
			APIEndpoint:   "https://api.grid.dev",
			JetStreamURLs: []string{"nats://grid.dev:4222"},
			IPFSGateway:   "https://ipfs.grid.dev",
			Features:      map[string]string{"workspace_api": "enabled"},
			Version:       "2025.9.29",
		},
	}
	withGridClientStub(t, newStubGridClient(status))

	runnerExecutor = fakeRunner
	eventsFactory = func(tenant string) (runner.EventsClient, error) { return contracts.NewInMemoryBus(tenant), nil }
	manifestRegistryLoader = func(dir string) (runner.ManifestCompiler, error) {
		return &stubManifestCompiler{compiled: defaultManifestPayload()}, nil
	}
	manifestConfigDir = "ignored"
	asterLocatorLoader = func(dir string) (aster.Locator, error) { return &recordingLocator{dir: dir}, nil }
	asterConfigDir = "ignored"
	jobComposerFactory = func() runner.JobComposer { return runner.NewStaticJobComposer() }
	cacheComposerFactory = func() runner.CacheComposer { return runner.NewDefaultCacheComposer() }

	if err := handleModRun([]string{"--tenant", "acme", "--ticket", "ticket-123"}, io.Discard); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := fakeRunner.opts.Grid.(*runner.InMemoryGrid); ok {
		t.Fatalf("expected shared grid client, received in-memory grid")
	}
}

func TestHandleModRunSurfacingGridErrors(t *testing.T) {
	t.Setenv(gridIDEnv, "grid-dev")
	t.Setenv(gridIDFallbackEnv, "grid-dev")
	t.Setenv(gridAPIKeyEnv, "secret")
	t.Setenv(gridAPIKeyFallbackEnv, "secret")
	t.Setenv(gridClientStateEnv, t.TempDir())
	withStubWorkspacePreparer(t)

	sentinel := errors.New("grid boom")
	prevFactory := gridFactory
	defer func() { gridFactory = prevFactory }()
	gridFactory = func() (runner.GridClient, error) { return nil, sentinel }

	err := handleModRun([]string{"--tenant", "acme", "--ticket", "ticket-123"}, io.Discard)
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected grid error, got %v", err)
	}
}

func TestHandleModRunPopulatesConfigFromDiscovery(t *testing.T) {
	t.Setenv(gridIDEnv, "grid-dev")
	t.Setenv(gridIDFallbackEnv, "grid-dev")
	t.Setenv(gridAPIKeyEnv, "secret")
	t.Setenv(gridAPIKeyFallbackEnv, "secret")
	t.Setenv(gridClientStateEnv, t.TempDir())
	withStubWorkspacePreparer(t)

	status := gridclient.Status{
		Beacon: gridclient.BeaconStatus{
			APIEndpoint:      "https://api.grid.dev",
			WorkflowEndpoint: "https://workflow.grid.dev",
		},
		Discovery: discovery.ClusterInfo{
			APIEndpoint:   "https://api.grid.dev",
			JetStreamURLs: []string{"nats://grid.dev:4222"},
			IPFSGateway:   "https://ipfs.grid.dev",
			Version:       "2025.9.29",
		},
	}
	withGridClientStub(t, newStubGridClient(status))

	receiver := &recordingRunner{}
	prevRunner := runnerExecutor
	prevEventsFactory := eventsFactory
	prevManifestLoader := manifestRegistryLoader
	prevManifestDir := manifestConfigDir
	prevLocatorLoader := asterLocatorLoader
	prevAsterDir := asterConfigDir
	prevJobComposerFactory := jobComposerFactory
	prevCacheComposerFactory := cacheComposerFactory
	defer func() {
		runnerExecutor = prevRunner
		eventsFactory = prevEventsFactory
		manifestRegistryLoader = prevManifestLoader
		manifestConfigDir = prevManifestDir
		asterLocatorLoader = prevLocatorLoader
		asterConfigDir = prevAsterDir
		jobComposerFactory = prevJobComposerFactory
		cacheComposerFactory = prevCacheComposerFactory
	}()

	runnerExecutor = receiver
	eventsFactory = func(tenant string) (runner.EventsClient, error) { return contracts.NewInMemoryBus(tenant), nil }
	manifestRegistryLoader = func(dir string) (runner.ManifestCompiler, error) {
		return &stubManifestCompiler{compiled: defaultManifestPayload()}, nil
	}
	manifestConfigDir = "ignored"
	asterLocatorLoader = func(dir string) (aster.Locator, error) { return &recordingLocator{dir: dir}, nil }
	asterConfigDir = "ignored"
	jobComposerFactory = func() runner.JobComposer { return runner.NewStaticJobComposer() }
	cacheComposerFactory = func() runner.CacheComposer { return runner.NewDefaultCacheComposer() }

	if err := handleModRun([]string{"--tenant", "acme", "--ticket", "ticket-123"}, io.Discard); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg, err := resolveIntegrationConfig(context.Background())
	if err != nil {
		t.Fatalf("resolve config: %v", err)
	}
	if cfg.APIEndpoint != "https://api.grid.dev" {
		t.Fatalf("unexpected API endpoint: %s", cfg.APIEndpoint)
	}
	if cfg.IPFSGateway != "https://ipfs.grid.dev" {
		t.Fatalf("unexpected IPFS gateway: %s", cfg.IPFSGateway)
	}
}
