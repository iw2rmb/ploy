package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/workflow/aster"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/environments"
	"github.com/iw2rmb/ploy/internal/workflow/grid"
	"github.com/iw2rmb/ploy/internal/workflow/lanes"
	"github.com/iw2rmb/ploy/internal/workflow/manifests"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
	"github.com/iw2rmb/ploy/internal/workflow/snapshots"
	server "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

type recordingRunner struct {
	opts runner.Options
	err  error
}

func (r *recordingRunner) Run(ctx context.Context, opts runner.Options) error {
	r.opts = opts
	return r.err
}

func defaultManifestPayload() manifests.Compilation {
	return manifests.Compilation{
		Manifest: manifests.Metadata{Name: "smoke", Version: "2025-09-26"},
		Lanes:    manifests.LaneSet{Required: []manifests.Lane{{Name: "node-wasm"}, {Name: "go-native"}}},
	}
}

type stubManifestCompiler struct {
	compiled manifests.Compilation
	err      error
	ref      contracts.ManifestReference
}

func (s *stubManifestCompiler) Compile(ctx context.Context, ref contracts.ManifestReference) (manifests.Compilation, error) {
	_ = ctx
	s.ref = ref
	return s.compiled, s.err
}

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

func TestExecuteManifestSchemaPrintsJSON(t *testing.T) {
	prevPath := manifestSchemaPath
	_, file, _, _ := runtime.Caller(0)
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	manifestSchemaPath = filepath.Join(repoRoot, "docs", "schemas", "integration_manifest.schema.json")
	defer func() { manifestSchemaPath = prevPath }()

	buf := &bytes.Buffer{}
	err := execute([]string{"manifest", "schema"}, buf)
	if err != nil {
		t.Fatalf("expected schema command to succeed, got %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "\"title\"") {
		t.Fatalf("expected schema output to contain title field, got %q", output)
	}
	if !strings.Contains(output, "integration_manifest.schema.json") {
		t.Fatalf("expected schema output to reference schema file, got %q", output)
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
	args := []string{"--tenant", "acme", "--aster", "exec", "--aster-step", "build=lint", "--aster-step", "test=off"}
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
	buildOverride, ok := opts.StageOverrides["build"]
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

func TestExecuteRequiresCommand(t *testing.T) {
	buf := &bytes.Buffer{}
	err := execute(nil, buf)
	if err == nil {
		t.Fatal("expected error when no command provided")
	}
	if buf.Len() == 0 {
		t.Fatal("expected usage output")
	}
}

func TestExecuteUnknownCommand(t *testing.T) {
	buf := &bytes.Buffer{}
	err := execute([]string{"unknown"}, buf)
	if err == nil {
		t.Fatal("expected error for unknown command")
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("expected unknown command error, got %v", err)
	}
}

func TestHandleWorkflowRequiresSubcommand(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleWorkflow(nil, buf)
	if err == nil {
		t.Fatal("expected error for missing subcommand")
	}
	if !strings.Contains(buf.String(), "Usage: ploy workflow") {
		t.Fatalf("expected workflow usage, got %q", buf.String())
	}
}

func TestPrintHelpers(t *testing.T) {
	buf := &bytes.Buffer{}
	printUsage(buf)
	printWorkflowUsage(buf)
	printWorkflowRunUsage(buf)
	reportError(errors.New("boom"), buf)
	output := buf.String()
	for _, fragment := range []string{"Usage: ploy workflow run", "Usage: ploy workflow", "error: boom"} {
		if !strings.Contains(output, fragment) {
			t.Fatalf("expected output to contain %q, got %q", fragment, output)
		}
	}
}

type recordingLocator struct {
	dir string
}

func (r *recordingLocator) Locate(ctx context.Context, req aster.Request) (aster.Metadata, error) {
	_ = ctx
	return aster.Metadata{Stage: req.Stage, Toggle: req.Toggle}, nil
}

type recordingEnvironmentService struct {
	request environments.Request
	result  environments.Result
	err     error
}

func (r *recordingEnvironmentService) Materialize(ctx context.Context, req environments.Request) (environments.Result, error) {
	r.request = req
	return r.result, r.err
}

func TestHandleEnvironmentMaterializeRequiresCommit(t *testing.T) {
	prevFactory := environmentServiceFactory
	defer func() { environmentServiceFactory = prevFactory }()

	environmentServiceFactory = func(l laneRegistry, s snapshotRegistry) (environmentService, error) {
		return &recordingEnvironmentService{}, nil
	}

	buf := &bytes.Buffer{}
	err := handleEnvironmentMaterialize([]string{"--app", "commit-app", "--tenant", "acme"}, buf)
	if err == nil {
		t.Fatal("expected error when commit SHA is missing")
	}
	if !strings.Contains(buf.String(), "Usage: ploy environment materialize") {
		t.Fatalf("expected usage output, got %q", buf.String())
	}
}

func TestHandleEnvironmentMaterializeRequiresApp(t *testing.T) {
	prevFactory := environmentServiceFactory
	defer func() { environmentServiceFactory = prevFactory }()

	environmentServiceFactory = func(l laneRegistry, s snapshotRegistry) (environmentService, error) {
		return &recordingEnvironmentService{}, nil
	}

	buf := &bytes.Buffer{}
	err := handleEnvironmentMaterialize([]string{"deadbeef", "--tenant", "acme"}, buf)
	if err == nil {
		t.Fatal("expected error when app is missing")
	}
	if !strings.Contains(buf.String(), "Usage: ploy environment materialize") {
		t.Fatalf("expected usage output, got %q", buf.String())
	}
}

func TestHandleEnvironmentMaterializeInvokesService(t *testing.T) {
	prevFactory := environmentServiceFactory
	prevManifestLoader := manifestRegistryLoader
	prevManifestDir := manifestConfigDir
	prevLaneLoader := laneRegistryLoader
	prevLaneDir := laneConfigDir
	prevSnapshotLoader := snapshotRegistryLoader
	prevSnapshotDir := snapshotConfigDir
	defer func() {
		environmentServiceFactory = prevFactory
		manifestRegistryLoader = prevManifestLoader
		manifestConfigDir = prevManifestDir
		laneRegistryLoader = prevLaneLoader
		laneConfigDir = prevLaneDir
		snapshotRegistryLoader = prevSnapshotLoader
		snapshotConfigDir = prevSnapshotDir
	}()

	recorder := &recordingEnvironmentService{
		result: environments.Result{
			App:       "commit-app",
			CommitSHA: "deadbeef",
			DryRun:    true,
			Snapshots: []environments.SnapshotStatus{{Name: "commit-db"}},
			Caches:    []environments.CacheStatus{{Lane: "go-native", CacheKey: "go/go-native@commit=deadbeef@snapshot=pending@manifest=2025-09-26@aster=plan", Hydrated: false}},
		},
	}

	environmentServiceFactory = func(l laneRegistry, s snapshotRegistry) (environmentService, error) {
		return recorder, nil
	}

	laneRegistryLoader = func(dir string) (laneRegistry, error) { return nil, nil }
	laneConfigDir = "ignored"
	snapshotRegistryLoader = func(dir string) (snapshotRegistry, error) { return nil, nil }
	snapshotConfigDir = "ignored"

	manifestRegistryLoader = func(dir string) (runner.ManifestCompiler, error) {
		return &stubManifestCompiler{compiled: manifests.Compilation{
			Manifest: manifests.Metadata{Name: "commit-app", Version: "2025-09-26"},
			Fixtures: manifests.FixtureSet{Required: []manifests.Fixture{{Name: "postgres", Reference: "snapshot:commit-db"}}},
		}}, nil
	}
	manifestConfigDir = "ignored"

	buf := &bytes.Buffer{}
	err := handleEnvironmentMaterialize([]string{"deadbeef", "--app", "commit-app", "--tenant", "acme", "--dry-run", "--aster", "lint"}, buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if recorder.request.CommitSHA != "deadbeef" {
		t.Fatalf("unexpected commit: %s", recorder.request.CommitSHA)
	}
	if recorder.request.App != "commit-app" {
		t.Fatalf("unexpected app: %s", recorder.request.App)
	}
	if !recorder.request.DryRun {
		t.Fatal("expected dry-run request")
	}
	if len(recorder.request.AsterToggles) != 1 || recorder.request.AsterToggles[0] != "lint" {
		t.Fatalf("unexpected aster toggles: %v", recorder.request.AsterToggles)
	}

	output := buf.String()
	for _, fragment := range []string{"Environment: commit-app", "Mode: dry-run", "commit-db", "go-native"} {
		if !strings.Contains(output, fragment) {
			t.Fatalf("expected output to contain %q, got %q", fragment, output)
		}
	}
}

func TestHandleEnvironmentMaterializePropagatesError(t *testing.T) {
	prevFactory := environmentServiceFactory
	prevManifestLoader := manifestRegistryLoader
	prevManifestDir := manifestConfigDir
	prevLaneLoader := laneRegistryLoader
	prevLaneDir := laneConfigDir
	prevSnapshotLoader := snapshotRegistryLoader
	prevSnapshotDir := snapshotConfigDir
	defer func() {
		environmentServiceFactory = prevFactory
		manifestRegistryLoader = prevManifestLoader
		manifestConfigDir = prevManifestDir
		laneRegistryLoader = prevLaneLoader
		laneConfigDir = prevLaneDir
		snapshotRegistryLoader = prevSnapshotLoader
		snapshotConfigDir = prevSnapshotDir
	}()

	recorder := &recordingEnvironmentService{err: errors.New("boom")}
	environmentServiceFactory = func(l laneRegistry, s snapshotRegistry) (environmentService, error) {
		return recorder, nil
	}

	manifestRegistryLoader = func(dir string) (runner.ManifestCompiler, error) {
		return &stubManifestCompiler{compiled: manifests.Compilation{
			Manifest: manifests.Metadata{Name: "commit-app", Version: "2025-09-26"},
		}}, nil
	}
	manifestConfigDir = "ignored"
	laneRegistryLoader = func(dir string) (laneRegistry, error) { return nil, nil }
	laneConfigDir = "ignored"
	snapshotRegistryLoader = func(dir string) (snapshotRegistry, error) { return nil, nil }
	snapshotConfigDir = "ignored"

	err := handleEnvironmentMaterialize([]string{"deadbeef", "--app", "commit-app", "--tenant", "acme"}, io.Discard)
	if !errors.Is(err, recorder.err) {
		t.Fatalf("expected service error, got %v", err)
	}
}

type fakeLaneRegistry struct {
	description lanes.Description
	err         error
}

func (f *fakeLaneRegistry) Describe(name string, opts lanes.DescribeOptions) (lanes.Description, error) {
	if f.err != nil {
		return lanes.Description{}, f.err
	}
	f.description.Parameters = opts
	return f.description, nil
}

func TestHandleLanesDescribePrintsDetails(t *testing.T) {
	buf := &bytes.Buffer{}
	prevLoader := laneRegistryLoader
	prevDir := laneConfigDir
	defer func() {
		laneRegistryLoader = prevLoader
		laneConfigDir = prevDir
	}()

	desc := lanes.Description{
		Lane: lanes.Spec{
			Name:           "node-wasm",
			Description:    "Node lane",
			RuntimeFamily:  "wasm-node",
			CacheNamespace: "node",
			Commands: lanes.Commands{
				Build: []string{"npm", "ci"},
				Test:  []string{"npm", "test"},
			},
		},
		CacheKey: "node/node-wasm@commit=abc@...",
	}

	laneRegistryLoader = func(dir string) (laneRegistry, error) {
		return &fakeLaneRegistry{description: desc}, nil
	}
	laneConfigDir = "ignored"

	err := handleLanes([]string{"describe", "--lane", "node-wasm", "--commit", "abc"}, buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()
	for _, fragment := range []string{"node-wasm", "wasm-node", "node", "node/node-wasm@commit=abc"} {
		if !strings.Contains(output, fragment) {
			t.Fatalf("expected output to contain %q, got %q", fragment, output)
		}
	}
}

func TestHandleLanesRequiresSubcommand(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleLanes(nil, buf)
	if err == nil {
		t.Fatal("expected error when lanes subcommand missing")
	}
	if !strings.Contains(buf.String(), "Usage: ploy lanes") {
		t.Fatalf("expected usage output, got %q", buf.String())
	}
}

type fakeSnapshotRegistry struct {
	planReport    snapshots.PlanReport
	captureResult snapshots.CaptureResult
	planErr       error
	captureErr    error
}

func (f *fakeSnapshotRegistry) Plan(ctx context.Context, name string) (snapshots.PlanReport, error) {
	return f.planReport, f.planErr
}

func (f *fakeSnapshotRegistry) Capture(ctx context.Context, name string, opts snapshots.CaptureOptions) (snapshots.CaptureResult, error) {
	return f.captureResult, f.captureErr
}

func TestHandleSnapshotPlanPrintsSummary(t *testing.T) {
	buf := &bytes.Buffer{}
	prevLoader := snapshotRegistryLoader
	prevDir := snapshotConfigDir
	defer func() {
		snapshotRegistryLoader = prevLoader
		snapshotConfigDir = prevDir
	}()

	report := snapshots.PlanReport{
		SnapshotName: "dev-db",
		Engine:       "postgres",
		Stripping:    snapshots.RuleSummary{Total: 1, Tables: map[string]int{"users": 1}},
		Masking:      snapshots.RuleSummary{Total: 2, Tables: map[string]int{"users": 2}},
		Synthetic:    snapshots.RuleSummary{Total: 1, Tables: map[string]int{"orders": 1}},
		Highlights:   []string{"mask users.email -> hash"},
	}

	snapshotRegistryLoader = func(dir string) (snapshotRegistry, error) {
		return &fakeSnapshotRegistry{planReport: report}, nil
	}
	snapshotConfigDir = "ignored"

	err := handleSnapshot([]string{"plan", "--snapshot", "dev-db"}, buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()
	for _, fragment := range []string{"Snapshot: dev-db", "Engine: postgres", "Mask Rules: 2"} {
		if !strings.Contains(output, fragment) {
			t.Fatalf("expected output to contain %q, got %q", fragment, output)
		}
	}
}

func TestHandleSnapshotCapturePrintsResult(t *testing.T) {
	buf := &bytes.Buffer{}
	prevLoader := snapshotRegistryLoader
	prevDir := snapshotConfigDir
	defer func() {
		snapshotRegistryLoader = prevLoader
		snapshotConfigDir = prevDir
	}()

	result := snapshots.CaptureResult{
		ArtifactCID: "cid-dev",
		Fingerprint: "fp-123",
		Metadata: snapshots.SnapshotMetadata{
			SnapshotName: "dev-db",
			Tenant:       "acme",
			TicketID:     "ticket-123",
		},
	}

	snapshotRegistryLoader = func(dir string) (snapshotRegistry, error) {
		return &fakeSnapshotRegistry{captureResult: result}, nil
	}
	snapshotConfigDir = "ignored"

	err := handleSnapshot([]string{"capture", "--snapshot", "dev-db", "--tenant", "acme", "--ticket", "ticket-123"}, buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()
	for _, fragment := range []string{"Artifact CID: cid-dev", "Fingerprint: fp-123"} {
		if !strings.Contains(output, fragment) {
			t.Fatalf("expected output to contain %q, got %q", fragment, output)
		}
	}
}

func TestHandleSnapshotCaptureUsesIPFSGatewayWhenConfigured(t *testing.T) {
	buf := &bytes.Buffer{}
	serverCalled := int32(0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&serverCalled, 1)
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/v0/add" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := r.ParseMultipartForm(2 << 20); err != nil {
			t.Fatalf("parse multipart form: %v", err)
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("read file: %v", err)
		}
		defer func() { _ = file.Close() }()
		body, err := io.ReadAll(file)
		if err != nil {
			t.Fatalf("read artifact body: %v", err)
		}
		if !strings.Contains(string(body), "users") {
			t.Fatalf("expected artifact body to contain users table, got %q", string(body))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"Hash":"bafyrehandledcid","Name":"dev-db","Size":"42"}`)
	}))
	defer server.Close()

	dir := t.TempDir()
	fixturePath := filepath.Join(dir, "dev-db.json")
	fixture := `{"users":[{"id":"1","email":"alice@example.com"}]}`
	if err := os.WriteFile(fixturePath, []byte(fixture), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	specContent := `name = "dev-db"
description = "Development database"
[source]
engine = "postgres"
dsn = "postgres://dev"
fixture = "dev-db.json"
`
	specPath := filepath.Join(dir, "dev-db.toml")
	if err := os.WriteFile(specPath, []byte(specContent), 0o600); err != nil {
		t.Fatalf("write spec: %v", err)
	}

	prevDir := snapshotConfigDir
	snapshotConfigDir = dir
	defer func() { snapshotConfigDir = prevDir }()

	t.Setenv("IPFS_GATEWAY", server.URL)

	err := handleSnapshot([]string{"capture", "--snapshot", "dev-db", "--tenant", "acme", "--ticket", "ticket-42"}, buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if atomic.LoadInt32(&serverCalled) == 0 {
		t.Fatal("expected IPFS gateway to be invoked")
	}
	output := buf.String()
	if !strings.Contains(output, "Artifact CID: bafyrehandledcid") {
		t.Fatalf("expected output to include IPFS CID, got %q", output)
	}
}

func TestHandleSnapshotCapturePublishesMetadataToJetStream(t *testing.T) {
	buf := &bytes.Buffer{}

	srv := runJetStreamServer(t)
	t.Cleanup(func() { srv.Shutdown() })

	conn, err := nats.Connect(srv.ClientURL())
	if err != nil {
		t.Fatalf("connect nats: %v", err)
	}
	t.Cleanup(func() { _ = conn.Drain() })

	js, err := conn.JetStream()
	if err != nil {
		t.Fatalf("jetstream context: %v", err)
	}
	if _, err := js.AddStream(&nats.StreamConfig{
		Name:     "PLOY_ARTIFACT",
		Subjects: []string{"ploy.artifact.*"},
	}); err != nil {
		t.Fatalf("add artifact stream: %v", err)
	}

	dir := t.TempDir()
	fixturePath := filepath.Join(dir, "dev-db.json")
	fixture := `{"users":[{"id":"1","email":"alice@example.com"}]}`
	if err := os.WriteFile(fixturePath, []byte(fixture), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	specContent := `name = "dev-db"
description = "Development database"
[source]
engine = "postgres"
dsn = "postgres://dev"
fixture = "dev-db.json"
`
	specPath := filepath.Join(dir, "dev-db.toml")
	if err := os.WriteFile(specPath, []byte(specContent), 0o600); err != nil {
		t.Fatalf("write spec: %v", err)
	}

	prevDir := snapshotConfigDir
	snapshotConfigDir = dir
	t.Cleanup(func() { snapshotConfigDir = prevDir })

	t.Setenv("JETSTREAM_URL", srv.ClientURL())

	err = handleSnapshot([]string{"capture", "--snapshot", "dev-db", "--tenant", "acme", "--ticket", "ticket-77"}, buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msg, err := js.GetMsg("PLOY_ARTIFACT", 1)
	if err != nil {
		t.Fatalf("get metadata msg: %v", err)
	}
	if msg.Subject != "ploy.artifact.ticket-77" {
		t.Fatalf("unexpected metadata subject: %s", msg.Subject)
	}

	var envelope struct {
		SchemaVersion string `json:"schema_version"`
		SnapshotName  string `json:"snapshot_name"`
		ArtifactCID   string `json:"artifact_cid"`
		Tenant        string `json:"tenant"`
		TicketID      string `json:"ticket_id"`
		CapturedAt    string `json:"captured_at"`
	}
	if err := json.Unmarshal(msg.Data, &envelope); err != nil {
		t.Fatalf("decode metadata envelope: %v", err)
	}
	if envelope.SchemaVersion != contracts.SchemaVersion {
		t.Fatalf("unexpected schema version: %s", envelope.SchemaVersion)
	}
	if envelope.SnapshotName != "dev-db" {
		t.Fatalf("snapshot mismatch: %s", envelope.SnapshotName)
	}
	if envelope.Tenant != "acme" || envelope.TicketID != "ticket-77" {
		t.Fatalf("tenant/ticket mismatch: %s/%s", envelope.Tenant, envelope.TicketID)
	}
	if envelope.ArtifactCID == "" {
		t.Fatalf("expected artifact cid in envelope")
	}
	if envelope.CapturedAt == "" {
		t.Fatalf("expected captured_at in envelope")
	}
}

func TestHandleSnapshotRequiresSubcommand(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleSnapshot(nil, buf)
	if err == nil {
		t.Fatal("expected error for missing snapshot subcommand")
	}
	if !strings.Contains(buf.String(), "Usage: ploy snapshot") {
		t.Fatalf("expected snapshot usage, got %q", buf.String())
	}
}

func runJetStreamServer(t *testing.T) *server.Server {
	t.Helper()

	opts := &server.Options{
		JetStream: true,
		Host:      "127.0.0.1",
		Port:      -1,
		StoreDir:  t.TempDir(),
	}

	srv, err := server.NewServer(opts)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	go srv.Start()

	if !srv.ReadyForConnections(5 * time.Second) {
		t.Fatalf("nats server not ready")
	}

	return srv
}
