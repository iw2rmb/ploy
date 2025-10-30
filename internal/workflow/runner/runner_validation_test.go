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
    opts := runner.Options{Ticket: "ticket-123", Runtime: runner.NewInMemoryGrid(), ManifestCompiler: newStubCompiler()}
	err := runner.Run(context.Background(), opts)
	if !errors.Is(err, runner.ErrEventsClientRequired) {
		t.Fatalf("expected ErrEventsClientRequired, got %v", err)
	}
}

func TestRunRequiresGridClient(t *testing.T) {
events := &recordingEvents{nextTicket: "ticket-123"}
    opts := runner.Options{Ticket: "ticket-123", Events: events, ManifestCompiler: newStubCompiler()}
	err := runner.Run(context.Background(), opts)
	if !errors.Is(err, runner.ErrGridClientRequired) {
		t.Fatalf("expected ErrGridClientRequired, got %v", err)
	}
}

func TestRunRequiresManifestCompiler(t *testing.T) {
events := &recordingEvents{nextTicket: "ticket-123"}
    opts := runner.Options{
		Ticket:          "ticket-123",
		Events:          events,
        Runtime:         runner.NewInMemoryGrid(),
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
events := &recordingEvents{nextTicket: "ticket-123"}
	compilerErr := errors.New("compile failed")
    opts := runner.Options{
		Ticket:           "ticket-123",
		Events:           events,
        Runtime:          runner.NewInMemoryGrid(),
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
events := &recordingEvents{nextTicket: "ticket-123"}
	compiler := &recordingCompiler{
		compiled: manifests.Compilation{
			Manifest:        manifests.Metadata{Name: "smoke", Version: "2025-09-26"},
			ManifestVersion: "v2",
			Lanes: manifests.LaneSet{
				Required: []manifests.Lane{{Name: "node-wasm"}, {Name: "go-native"}},
			},
		},
	}
	grid := &fakeGrid{}
    opts := runner.Options{
		Ticket:           "ticket-123",
		Events:           events,
        Runtime:          grid,
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
events := &recordingEvents{nextTicket: "ticket-123"}
	compiler := &recordingCompiler{
		compiled: manifests.Compilation{
			Manifest:        manifests.Metadata{Name: "smoke", Version: "2025-09-26"},
			ManifestVersion: "v2",
			Lanes: manifests.LaneSet{
				Required: []manifests.Lane{{Name: "node-wasm"}},
				Allowed: []manifests.Lane{
					{Name: "go-native"},
					{Name: "gpu-ml"},
					{Name: "mods-human"},
					{Name: "build-gate"},
					{Name: "static-checks"},
					{Name: "test"},
				},
			},
		},
	}
	grid := runner.NewInMemoryGrid()
    opts := runner.Options{
		Ticket:           "ticket-123",
		Events:           events,
        Runtime:          grid,
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
events := &recordingEvents{nextTicket: "ticket-123"}
	grid := &fakeGrid{
		outcomes: map[string][]runner.StageOutcome{
			modsPlanStage:  {{Status: runner.StageStatusCompleted}},
			buildGateStage: {{Status: runner.StageStatusFailed, Retryable: true, Message: "no more"}},
		},
	}
    opts := runner.Options{
		Ticket:           "",
    // tenant removed
		Events:           events,
        Runtime:          grid,
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
	if len(sequence) == 0 || sequence[0].stage != "ticket-claimed" {
		t.Fatalf("expected ticket-claimed first, got %v", sequence)
	}
	requireStageStatuses(t, sequence, modsPlanStage, []runner.StageStatus{runner.StageStatusRunning, runner.StageStatusCompleted})
	requireStageStatuses(t, sequence, mods.StageNameORWApply, []runner.StageStatus{runner.StageStatusRunning, runner.StageStatusCompleted})
	requireStageStatuses(t, sequence, mods.StageNameORWGenerate, []runner.StageStatus{runner.StageStatusRunning, runner.StageStatusCompleted})
	requireStageStatuses(t, sequence, mods.StageNameLLMPlan, []runner.StageStatus{runner.StageStatusRunning, runner.StageStatusCompleted})
	requireStageStatuses(t, sequence, mods.StageNameLLMExec, []runner.StageStatus{runner.StageStatusRunning, runner.StageStatusCompleted})
	requireStageStatuses(t, sequence, mods.StageNameHuman, []runner.StageStatus{runner.StageStatusRunning, runner.StageStatusCompleted})
	requireStageStatuses(t, sequence, buildGateStage, []runner.StageStatus{runner.StageStatusRunning, runner.StageStatusFailed})
	workflowStatuses := collectStageStatuses(sequence, "workflow")
	if len(workflowStatuses) != 1 || workflowStatuses[0] != runner.StageStatusFailed {
		t.Fatalf("expected workflow failure, got %v", workflowStatuses)
	}
}

func TestRunErrorsWhenWorkspaceRootInvalid(t *testing.T) {
	temp := t.TempDir()
	file := filepath.Join(temp, "lock")
	if err := os.WriteFile(file, []byte("lock"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
events := &recordingEvents{nextTicket: "ticket-123"}
	opts := runner.Options{
		Ticket:           "",
    // tenant removed
		Events:           events,
        Runtime:          &fakeGrid{},
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
events := &recordingEvents{invalidTicket: true}
    opts := runner.Options{
		Ticket:           "",
    // tenant removed
		Events:           events,
        Runtime:          &fakeGrid{},
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
