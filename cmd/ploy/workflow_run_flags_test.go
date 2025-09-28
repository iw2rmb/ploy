package main

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/aster"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/lanes"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
)

func TestHandleWorkflowRunSupportsAutoTicket(t *testing.T) {
	t.Setenv("GRID_ENDPOINT", "")
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
	if fakeRunner.opts.JobComposer == nil {
		t.Fatal("expected job composer to be configured")
	}
}

func TestHandleWorkflowRunPropagatesRunnerError(t *testing.T) {
	t.Setenv("GRID_ENDPOINT", "")
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
	t.Setenv("GRID_ENDPOINT", "")
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
	t.Setenv("GRID_ENDPOINT", "")
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
