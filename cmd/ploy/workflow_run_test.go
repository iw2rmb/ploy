package main

import (
	"bytes"
	"errors"
	"github.com/iw2rmb/ploy/internal/workflow/aster"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/grid"
	"github.com/iw2rmb/ploy/internal/workflow/lanes"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
	"io"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestHandleWorkflowRunSupportsAutoTicket(t *testing.T) {
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
	stubCompiler := &stubManifestCompiler{compiled: defaultManifestPayload()}
	manifestRegistryLoader = func(dir string) (runner.ManifestCompiler, error) {
		return stubCompiler, nil
	}
	manifestConfigDir = "ignored"
	asterLocatorLoader = func(dir string) (aster.Locator, error) { return &recordingLocator{dir: dir}, nil }
	asterConfigDir = "ignored"
	laneRegistryLoader = func(dir string) (laneRegistry, error) {
		return &fakeLaneRegistry{description: lanes.Description{Lane: lanes.Spec{Name: "node-wasm", CacheNamespace: "node-wasm"}, CacheKey: "stub-cache"}}, nil
	}
	laneConfigDir = "ignored"

	err := handleWorkflowRun([]string{"--tenant", "acme", "--ticket", "auto"}, io.Discard)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fakeRunner.opts.Ticket != "" {
		t.Fatalf("expected empty ticket for auto claim, got %q", fakeRunner.opts.Ticket)
	}
	if fakeRunner.opts.Tenant != "acme" {
		t.Fatalf("unexpected tenant: %s", fakeRunner.opts.Tenant)
	}
	compiler := fakeRunner.opts.ManifestCompiler
	if compiler == nil {
		t.Fatal("expected manifest compiler to be set")
	}
	if compiler != stubCompiler {
		t.Fatalf("expected stub compiler, got %T", compiler)
	}
	if fakeRunner.opts.CacheComposer == nil {
		t.Fatal("expected cache composer to be configured")
	}
}

func TestHandleWorkflowRunPropagatesRunnerError(t *testing.T) {
	fakeRunner := &recordingRunner{err: errors.New("boom")}
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

	err := handleWorkflowRun([]string{"--tenant", "acme", "--ticket", "ticket-123"}, io.Discard)
	if !errors.Is(err, fakeRunner.err) {
		t.Fatalf("expected runner error, got %v", err)
	}
}

func TestHandleWorkflowRunPropagatesManifestLoaderError(t *testing.T) {
	prevLoader := manifestRegistryLoader
	prevDir := manifestConfigDir
	prevLaneLoader := laneRegistryLoader
	prevLaneDir := laneConfigDir
	defer func() {
		manifestRegistryLoader = prevLoader
		manifestConfigDir = prevDir
		laneRegistryLoader = prevLaneLoader
		laneConfigDir = prevLaneDir
	}()

	manifestRegistryLoader = func(dir string) (runner.ManifestCompiler, error) {
		return nil, errors.New("manifest load failed")
	}
	manifestConfigDir = "ignored"
	laneRegistryLoader = func(dir string) (laneRegistry, error) {
		return &fakeLaneRegistry{description: lanes.Description{Lane: lanes.Spec{Name: "node-wasm", CacheNamespace: "node-wasm"}, CacheKey: "stub-cache"}}, nil
	}
	laneConfigDir = "ignored"

	err := handleWorkflowRun([]string{"--tenant", "acme", "--ticket", "ticket-123"}, io.Discard)
	if err == nil {
		t.Fatal("expected manifest loader error")
	}
	if !strings.Contains(err.Error(), "manifest") {
		t.Fatalf("expected manifest error context, got %v", err)
	}
}

func TestHandleWorkflowRunRequiresTenant(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleWorkflowRun([]string{"--ticket", "auto"}, buf)
	if err == nil {
		t.Fatal("expected error for missing tenant")
	}
	if !strings.Contains(buf.String(), "Usage: ploy workflow run") {
		t.Fatalf("expected usage output, got %q", buf.String())
	}
}

func TestHandleWorkflowRunTrimsExplicitTicket(t *testing.T) {
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

	err := handleWorkflowRun([]string{"--tenant", "acme", "--ticket", "  ticket-456  "}, io.Discard)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fakeRunner.opts.Ticket != "ticket-456" {
		t.Fatalf("expected trimmed ticket, got %q", fakeRunner.opts.Ticket)
	}
}

func TestHandleWorkflowRunConfiguresModsFlags(t *testing.T) {
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

	err := handleWorkflowRun([]string{"--tenant", "acme", "--mods-plan-timeout", "2m30s", "--mods-max-parallel", "5"}, io.Discard)
	if err != nil {
		t.Fatalf("expected Mods flags accepted, got error: %v", err)
	}

	modsField := reflect.ValueOf(fakeRunner.opts).FieldByName("Mods")
	if !modsField.IsValid() {
		t.Fatalf("runner.Options missing Mods field: %#v", fakeRunner.opts)
	}
	timeoutField := modsField.FieldByName("PlanTimeout")
	if !timeoutField.IsValid() {
		t.Fatalf("runner.ModsOptions missing PlanTimeout: %#v", modsField.Interface())
	}
	if duration, ok := timeoutField.Interface().(time.Duration); !ok || duration != 150*time.Second {
		t.Fatalf("expected plan timeout 150s, got %#v", timeoutField.Interface())
	}
	parallelField := modsField.FieldByName("MaxParallel")
	if !parallelField.IsValid() {
		t.Fatalf("runner.ModsOptions missing MaxParallel: %#v", modsField.Interface())
	}
	if maxVal, ok := parallelField.Interface().(int); !ok || maxVal != 5 {
		t.Fatalf("expected max parallel 5, got %#v", parallelField.Interface())
	}
}

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

func TestHandleWorkflowRunParsesAsterFlags(t *testing.T) {
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
	locator := &recordingLocator{}
	asterLocatorLoader = func(dir string) (aster.Locator, error) {
		locator.dir = dir
		return locator, nil
	}
	asterConfigDir = "configs/aster"
	laneRegistryLoader = func(dir string) (laneRegistry, error) {
		return &fakeLaneRegistry{description: lanes.Description{Lane: lanes.Spec{Name: "node-wasm", CacheNamespace: "node-wasm"}, CacheKey: "stub-cache"}}, nil
	}
	laneConfigDir = "ignored"

	buf := &bytes.Buffer{}
	args := []string{"--tenant", "acme", "--aster", "exec", "--aster-step", "build-gate=lint", "--aster-step", "test=off"}
	if err := handleWorkflowRun(args, buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	opts := fakeRunner.opts.Aster
	if opts.Locator != locator {
		t.Fatalf("expected locator to be injected, got %T", opts.Locator)
	}
	if len(opts.AdditionalToggles) != 1 || opts.AdditionalToggles[0] != "exec" {
		t.Fatalf("unexpected additional toggles: %+v", opts.AdditionalToggles)
	}
	buildOverride, ok := opts.StageOverrides["build-gate"]
	if !ok {
		t.Fatalf("expected build override to exist")
	}
	if buildOverride.Disable {
		t.Fatalf("did not expect build to disable Aster: %+v", buildOverride)
	}
	if len(buildOverride.ExtraToggles) != 1 || buildOverride.ExtraToggles[0] != "lint" {
		t.Fatalf("unexpected build override toggles: %+v", buildOverride.ExtraToggles)
	}
	testOverride, ok := opts.StageOverrides["test"]
	if !ok {
		t.Fatalf("expected test override to exist")
	}
	if !testOverride.Disable {
		t.Fatalf("expected test override to disable Aster, got %+v", testOverride)
	}
	if len(testOverride.ExtraToggles) != 0 {
		t.Fatalf("expected no extra toggles for disabled test stage, got %+v", testOverride.ExtraToggles)
	}

	if locator.dir != "configs/aster" {
		t.Fatalf("expected locator to receive directory, got %s", locator.dir)
	}
}
