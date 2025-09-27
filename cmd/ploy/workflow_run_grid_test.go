package main

import (
	"io"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/aster"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/grid"
	"github.com/iw2rmb/ploy/internal/workflow/lanes"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
)

func TestHandleWorkflowRunUsesInMemoryGridWhenUnset(t *testing.T) {
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
		return &fakeLaneRegistry{description: lanes.Description{Lane: lanes.Spec{Name: "node-wasm", CacheNamespace: "node-wasm"}, CacheKey: "stub-cache"}}, nil
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
		return &fakeLaneRegistry{description: lanes.Description{Lane: lanes.Spec{Name: "node-wasm", CacheNamespace: "node-wasm"}, CacheKey: "stub-cache"}}, nil
	}
	laneConfigDir = "ignored"
	t.Setenv("GRID_ENDPOINT", "https://grid.dev")

	err := handleWorkflowRun([]string{"--tenant", "acme", "--ticket", "ticket-123"}, io.Discard)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := fakeRunner.opts.Grid.(*grid.Client); !ok {
		t.Fatalf("expected grid client, got %T", fakeRunner.opts.Grid)
	}
}

func TestHandleWorkflowRunFailsForInvalidGridEndpoint(t *testing.T) {
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

	runnerExecutor = &recordingRunner{}
	eventsFactory = func(tenant string) (runner.EventsClient, error) { return contracts.NewInMemoryBus(tenant), nil }
	manifestRegistryLoader = func(dir string) (runner.ManifestCompiler, error) {
		return &stubManifestCompiler{compiled: defaultManifestPayload()}, nil
	}
	manifestConfigDir = "ignored"
	asterLocatorLoader = func(dir string) (aster.Locator, error) { return &recordingLocator{dir: dir}, nil }
	asterConfigDir = "ignored"
	laneRegistryLoader = func(dir string) (laneRegistry, error) {
		return &fakeLaneRegistry{description: lanes.Description{Lane: lanes.Spec{Name: "node-wasm", CacheNamespace: "node-wasm"}, CacheKey: "stub-cache"}}, nil
	}
	laneConfigDir = "ignored"
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
	defer func() {
		eventsFactory = prevFactory
		manifestConfigDir = prevManifestDir
		laneConfigDir = prevLaneDir
		asterConfigDir = prevAsterDir
	}()

	eventsFactory = defaultEventsFactory

	t.Setenv("JETSTREAM_URL", "nats://127.0.0.1:1")

	_, file, _, _ := runtime.Caller(0)
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	manifestConfigDir = filepath.Join(repoRoot, "configs", "manifests")
	laneConfigDir = filepath.Join(repoRoot, "configs", "lanes")
	asterConfigDir = filepath.Join(repoRoot, "configs", "aster")

	err := handleWorkflowRun([]string{"--tenant", "acme", "--ticket", "auto"}, io.Discard)
	if err == nil {
		t.Fatal("expected error when JetStream connection fails")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "jetstream") {
		t.Fatalf("expected jetstream error context, got %v", err)
	}
}
