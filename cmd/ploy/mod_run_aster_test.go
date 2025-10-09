package main

import (
	"bytes"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/aster"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/lanes"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
)

func TestHandleModRunParsesAsterFlags(t *testing.T) {
	t.Setenv("PLOY_ASTER_ENABLE", "1")
	t.Setenv(gridIDEnv, "")
	t.Setenv(gridAPIKeyEnv, "")
	t.Setenv(gridClientStateEnv, t.TempDir())
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
	if err := handleModRun(args, buf); err != nil {
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
