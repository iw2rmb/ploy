package runner_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/manifests"
	"github.com/iw2rmb/ploy/internal/workflow/mods"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
)

func TestRunRequiresEventsClient(t *testing.T) {
	opts := runner.Options{Ticket: "ticket-123", Grid: runner.NewInMemoryGrid(), ManifestCompiler: newStubCompiler()}
	err := runner.Run(context.Background(), opts)
	if !errors.Is(err, runner.ErrEventsClientRequired) {
		t.Fatalf("expected ErrEventsClientRequired, got %v", err)
	}
}

func TestRunRequiresGridClient(t *testing.T) {
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	opts := runner.Options{Ticket: "ticket-123", Events: events, ManifestCompiler: newStubCompiler()}
	err := runner.Run(context.Background(), opts)
	if !errors.Is(err, runner.ErrGridClientRequired) {
		t.Fatalf("expected ErrGridClientRequired, got %v", err)
	}
}

func TestRunRequiresManifestCompiler(t *testing.T) {
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	opts := runner.Options{
		Ticket:          "ticket-123",
		Events:          events,
		Grid:            runner.NewInMemoryGrid(),
		Planner:         runner.NewDefaultPlanner(),
		WorkspaceRoot:   t.TempDir(),
		MaxStageRetries: 1,
	}
	err := runner.Run(context.Background(), opts)
	if !errors.Is(err, runner.ErrManifestCompilerRequired) {
		t.Fatalf("expected ErrManifestCompilerRequired, got %v", err)
	}
}

func TestRunPropagatesManifestCompilationError(t *testing.T) {
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	compilerErr := errors.New("compile failed")
	opts := runner.Options{
		Ticket:           "ticket-123",
		Events:           events,
		Grid:             runner.NewInMemoryGrid(),
		Planner:          runner.NewDefaultPlanner(),
		WorkspaceRoot:    t.TempDir(),
		MaxStageRetries:  1,
		ManifestCompiler: failingCompiler{err: compilerErr},
	}
	err := runner.Run(context.Background(), opts)
	if !errors.Is(err, compilerErr) {
		t.Fatalf("expected compiler error, got %v", err)
	}
}

func TestRunPassesManifestConstraintsToGrid(t *testing.T) {
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	compiler := &recordingCompiler{
		compiled: manifests.Compilation{
			Manifest: manifests.Metadata{Name: "smoke", Version: "2025-09-26"},
			Lanes: manifests.LaneSet{
				Required: []manifests.Lane{{Name: "node-wasm"}, {Name: "go-native"}},
			},
		},
	}
	grid := &fakeGrid{}
	opts := runner.Options{
		Ticket:           "ticket-123",
		Events:           events,
		Grid:             grid,
		Planner:          runner.NewDefaultPlanner(),
		WorkspaceRoot:    t.TempDir(),
		MaxStageRetries:  1,
		ManifestCompiler: compiler,
	}
	if err := runner.Run(context.Background(), opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(grid.calls) == 0 {
		t.Fatal("expected at least one grid call")
	}
	for _, call := range grid.calls {
		if call.stage.Constraints.Manifest.Manifest.Name != "smoke" {
			t.Fatalf("expected manifest on stage, got %+v", call.stage.Constraints.Manifest)
		}
	}
	if compiler.ref.Name != "smoke" || compiler.ref.Version == "" {
		t.Fatalf("expected manifest reference to be captured, got %+v", compiler.ref)
	}
}

func TestRunAcceptsAllowedLaneAssignments(t *testing.T) {
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	compiler := &recordingCompiler{
		compiled: manifests.Compilation{
			Manifest: manifests.Metadata{Name: "smoke", Version: "2025-09-26"},
			Lanes: manifests.LaneSet{
				Required: []manifests.Lane{{Name: "node-wasm"}},
				Allowed:  []manifests.Lane{{Name: "go-native"}, {Name: "gpu-ml"}},
			},
		},
	}
	grid := runner.NewInMemoryGrid()
	opts := runner.Options{
		Ticket:           "ticket-123",
		Events:           events,
		Grid:             grid,
		Planner:          runner.NewDefaultPlanner(),
		WorkspaceRoot:    t.TempDir(),
		MaxStageRetries:  1,
		ManifestCompiler: compiler,
	}
	if err := runner.Run(context.Background(), opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunTreatsNegativeRetriesAsZero(t *testing.T) {
	withCleanupDeadline(t)
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	grid := &fakeGrid{
		outcomes: map[string][]runner.StageOutcome{
			modsPlanStage: {{Status: runner.StageStatusCompleted}},
			"build":       {{Status: runner.StageStatusFailed, Retryable: true, Message: "no more"}},
		},
	}
	opts := runner.Options{
		Ticket:           "",
		Tenant:           "acme",
		Events:           events,
		Grid:             grid,
		Planner:          runner.NewDefaultPlanner(),
		WorkspaceRoot:    t.TempDir(),
		MaxStageRetries:  -3,
		ManifestCompiler: newStubCompiler(),
	}
	err := runner.Run(context.Background(), opts)
	if !errors.Is(err, runner.ErrStageFailed) {
		t.Fatalf("expected stage failure, got %v", err)
	}
	sequence := extractStageStatuses(events.checkpoints)
	expected := []stageStatusEntry{
		{stage: "ticket-claimed", status: runner.StageStatusCompleted},
		{stage: modsPlanStage, status: runner.StageStatusRunning},
		{stage: modsPlanStage, status: runner.StageStatusCompleted},
		{stage: mods.StageNameORWApply, status: runner.StageStatusRunning},
		{stage: mods.StageNameORWApply, status: runner.StageStatusCompleted},
		{stage: mods.StageNameORWGenerate, status: runner.StageStatusRunning},
		{stage: mods.StageNameORWGenerate, status: runner.StageStatusCompleted},
		{stage: mods.StageNameLLMPlan, status: runner.StageStatusRunning},
		{stage: mods.StageNameLLMPlan, status: runner.StageStatusCompleted},
		{stage: mods.StageNameLLMExec, status: runner.StageStatusRunning},
		{stage: mods.StageNameLLMExec, status: runner.StageStatusCompleted},
		{stage: mods.StageNameHuman, status: runner.StageStatusRunning},
		{stage: mods.StageNameHuman, status: runner.StageStatusCompleted},
		{stage: "build", status: runner.StageStatusRunning},
		{stage: "build", status: runner.StageStatusFailed},
		{stage: "workflow", status: runner.StageStatusFailed},
	}
	if err := compareSequences(sequence, expected); err != nil {
		t.Fatalf("checkpoint sequence mismatch: %v", err)
	}
}

func TestRunErrorsWhenWorkspaceRootInvalid(t *testing.T) {
	temp := t.TempDir()
	file := filepath.Join(temp, "lock")
	if err := os.WriteFile(file, []byte("lock"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	opts := runner.Options{
		Ticket:           "",
		Tenant:           "acme",
		Events:           events,
		Grid:             &fakeGrid{},
		Planner:          runner.NewDefaultPlanner(),
		WorkspaceRoot:    filepath.Join(file, "workspace"),
		MaxStageRetries:  1,
		ManifestCompiler: newStubCompiler(),
	}
	err := runner.Run(context.Background(), opts)
	if err == nil || !strings.Contains(err.Error(), "create workspace root") {
		t.Fatalf("expected workspace root error, got %v", err)
	}
}

func TestRunErrorsWhenTicketValidationFails(t *testing.T) {
	events := &recordingEvents{tenant: "acme", invalidTicket: true}
	opts := runner.Options{
		Ticket:           "",
		Tenant:           "acme",
		Events:           events,
		Grid:             &fakeGrid{},
		Planner:          runner.NewDefaultPlanner(),
		WorkspaceRoot:    t.TempDir(),
		MaxStageRetries:  1,
		ManifestCompiler: newStubCompiler(),
	}
	err := runner.Run(context.Background(), opts)
	if !errors.Is(err, runner.ErrTicketValidationFailed) {
		t.Fatalf("expected ticket validation error, got %v", err)
	}
}
