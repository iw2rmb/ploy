package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/environments"
	"github.com/iw2rmb/ploy/internal/workflow/lanes"
	"github.com/iw2rmb/ploy/internal/workflow/manifests"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
	"github.com/iw2rmb/ploy/internal/workflow/snapshots"
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
	defer func() {
		runnerExecutor = prevRunner
		eventsFactory = prevBusFactory
		manifestRegistryLoader = prevManifestLoader
		manifestConfigDir = prevManifestDir
	}()

	runnerExecutor = fakeRunner
	eventsFactory = func(tenant string) runner.EventsClient { return contracts.NewInMemoryBus(tenant) }
	stubCompiler := &stubManifestCompiler{compiled: defaultManifestPayload()}
	manifestRegistryLoader = func(dir string) (runner.ManifestCompiler, error) {
		return stubCompiler, nil
	}
	manifestConfigDir = "ignored"

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
}

func TestHandleWorkflowRunPropagatesRunnerError(t *testing.T) {
	fakeRunner := &recordingRunner{err: errors.New("boom")}
	prevRunner := runnerExecutor
	prevBusFactory := eventsFactory
	prevManifestLoader := manifestRegistryLoader
	prevManifestDir := manifestConfigDir
	defer func() {
		runnerExecutor = prevRunner
		eventsFactory = prevBusFactory
		manifestRegistryLoader = prevManifestLoader
		manifestConfigDir = prevManifestDir
	}()

	runnerExecutor = fakeRunner
	eventsFactory = func(tenant string) runner.EventsClient { return contracts.NewInMemoryBus(tenant) }
	manifestRegistryLoader = func(dir string) (runner.ManifestCompiler, error) {
		return &stubManifestCompiler{compiled: defaultManifestPayload()}, nil
	}
	manifestConfigDir = "ignored"

	err := handleWorkflowRun([]string{"--tenant", "acme", "--ticket", "ticket-123"}, io.Discard)
	if !errors.Is(err, fakeRunner.err) {
		t.Fatalf("expected runner error, got %v", err)
	}
}

func TestHandleWorkflowRunPropagatesManifestLoaderError(t *testing.T) {
	prevLoader := manifestRegistryLoader
	prevDir := manifestConfigDir
	defer func() {
		manifestRegistryLoader = prevLoader
		manifestConfigDir = prevDir
	}()

	manifestRegistryLoader = func(dir string) (runner.ManifestCompiler, error) {
		return nil, errors.New("manifest load failed")
	}
	manifestConfigDir = "ignored"

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
	defer func() {
		runnerExecutor = prevRunner
		eventsFactory = prevBusFactory
		manifestRegistryLoader = prevManifestLoader
		manifestConfigDir = prevManifestDir
	}()

	runnerExecutor = fakeRunner
	eventsFactory = func(tenant string) runner.EventsClient { return contracts.NewInMemoryBus(tenant) }
	manifestRegistryLoader = func(dir string) (runner.ManifestCompiler, error) {
		return &stubManifestCompiler{compiled: defaultManifestPayload()}, nil
	}
	manifestConfigDir = "ignored"

	err := handleWorkflowRun([]string{"--tenant", "acme", "--ticket", "  ticket-456  "}, io.Discard)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fakeRunner.opts.Ticket != "ticket-456" {
		t.Fatalf("expected trimmed ticket, got %q", fakeRunner.opts.Ticket)
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
