package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/lanes"
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

func TestHandleWorkflowRunSupportsAutoTicket(t *testing.T) {
	fakeRunner := &recordingRunner{}
	prevRunner := runnerExecutor
	prevBusFactory := eventsFactory
	defer func() {
		runnerExecutor = prevRunner
		eventsFactory = prevBusFactory
	}()

	runnerExecutor = fakeRunner
	eventsFactory = func(tenant string) runner.EventsClient { return contracts.NewInMemoryBus(tenant) }

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
}

func TestHandleWorkflowRunPropagatesRunnerError(t *testing.T) {
	fakeRunner := &recordingRunner{err: errors.New("boom")}
	prevRunner := runnerExecutor
	prevBusFactory := eventsFactory
	defer func() {
		runnerExecutor = prevRunner
		eventsFactory = prevBusFactory
	}()

	runnerExecutor = fakeRunner
	eventsFactory = func(tenant string) runner.EventsClient { return contracts.NewInMemoryBus(tenant) }

	err := handleWorkflowRun([]string{"--tenant", "acme", "--ticket", "ticket-123"}, io.Discard)
	if !errors.Is(err, fakeRunner.err) {
		t.Fatalf("expected runner error, got %v", err)
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
	defer func() {
		runnerExecutor = prevRunner
		eventsFactory = prevBusFactory
	}()

	runnerExecutor = fakeRunner
	eventsFactory = func(tenant string) runner.EventsClient { return contracts.NewInMemoryBus(tenant) }

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
