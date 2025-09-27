package main

import (
	"io"
	"reflect"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/workflow/aster"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/lanes"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
)

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
