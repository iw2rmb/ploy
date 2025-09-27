package runner_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/mods"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
)

func TestRunDefaultsStageOutcomeStatus(t *testing.T) {
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	opts := runner.Options{
		Ticket:           "",
		Tenant:           "acme",
		Events:           events,
		Grid:             statuslessGrid{},
		Planner:          runner.NewDefaultPlanner(),
		WorkspaceRoot:    t.TempDir(),
		MaxStageRetries:  1,
		ManifestCompiler: newStubCompiler(),
	}
	if err := runner.Run(context.Background(), opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sequence := extractStageStatuses(events.checkpoints)
	if len(sequence) == 0 || sequence[len(sequence)-1].status != runner.StageStatusCompleted {
		t.Fatalf("expected workflow completion, got %v", sequence)
	}
}

func TestRunSurfacesNonRetryableStageFailure(t *testing.T) {
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	grid := &fakeGrid{
		outcomes: map[string][]runner.StageOutcome{
			modsPlanStage: {{Status: runner.StageStatusCompleted}},
			"build":       {{Status: runner.StageStatusFailed, Retryable: false, Message: "bad cache"}},
		},
	}
	opts := runner.Options{
		Ticket:           "",
		Tenant:           "acme",
		Events:           events,
		Grid:             grid,
		Planner:          runner.NewDefaultPlanner(),
		WorkspaceRoot:    t.TempDir(),
		MaxStageRetries:  1,
		ManifestCompiler: newStubCompiler(),
	}
	err := runner.Run(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error from non-retryable failure")
	}
	if !errors.Is(err, runner.ErrStageFailed) {
		t.Fatalf("expected ErrStageFailed, got %v", err)
	}
	sequence := extractStageStatuses(events.checkpoints)
	if len(sequence) == 0 || sequence[len(sequence)-1].status != runner.StageStatusFailed {
		t.Fatalf("expected last checkpoint to be failed, sequence=%v", sequence)
	}
}

func TestRunUsesFallbackFailureMessage(t *testing.T) {
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	grid := &fakeGrid{
		outcomes: map[string][]runner.StageOutcome{
			modsPlanStage: {{Status: runner.StageStatusCompleted}},
			"build":       {{Status: runner.StageStatusFailed, Retryable: false}},
		},
	}
	opts := runner.Options{
		Ticket:           "",
		Tenant:           "acme",
		Events:           events,
		Grid:             grid,
		Planner:          runner.NewDefaultPlanner(),
		WorkspaceRoot:    t.TempDir(),
		MaxStageRetries:  1,
		ManifestCompiler: newStubCompiler(),
	}
	err := runner.Run(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error for failure")
	}
	if !strings.Contains(err.Error(), "stage failed") {
		t.Fatalf("expected fallback message, got %v", err)
	}
}

func TestRunPropagatesGridError(t *testing.T) {
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	gridErr := errors.New("grid down")
	opts := runner.Options{
		Ticket:           "",
		Tenant:           "acme",
		Events:           events,
		Grid:             errorGrid{err: gridErr},
		Planner:          runner.NewDefaultPlanner(),
		WorkspaceRoot:    t.TempDir(),
		MaxStageRetries:  1,
		ManifestCompiler: newStubCompiler(),
	}
	err := runner.Run(context.Background(), opts)
	if !errors.Is(err, gridErr) {
		t.Fatalf("expected grid error, got %v", err)
	}
}

func TestRunRetriesStageOnce(t *testing.T) {
	withCleanupDeadline(t)
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	grid := &fakeGrid{
		outcomes: map[string][]runner.StageOutcome{
			modsPlanStage: {{Status: runner.StageStatusCompleted}},
			"build": {
				{Status: runner.StageStatusFailed, Retryable: true, Message: "grid transient"},
				{Status: runner.StageStatusCompleted},
			},
			"test": {{Status: runner.StageStatusCompleted}},
		},
	}
	planner := runner.NewDefaultPlanner()
	workspaceRoot := t.TempDir()
	opts := runner.Options{
		Ticket:           "",
		Tenant:           "acme",
		Events:           events,
		Grid:             grid,
		Planner:          planner,
		WorkspaceRoot:    workspaceRoot,
		MaxStageRetries:  1,
		ManifestCompiler: newStubCompiler(),
	}
	if err := runner.Run(context.Background(), opts); err != nil {
		t.Fatalf("run error: %v", err)
	}
	b := gatherStageAttempts(grid.calls, "build")
	if b != 2 {
		t.Fatalf("expected build stage to retry once, got %d attempts", b)
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
		{stage: "build", status: runner.StageStatusRetrying},
		{stage: "build", status: runner.StageStatusRunning},
		{stage: "build", status: runner.StageStatusCompleted},
		{stage: "test", status: runner.StageStatusRunning},
		{stage: "test", status: runner.StageStatusCompleted},
		{stage: "workflow", status: runner.StageStatusCompleted},
	}
	if err := compareSequences(sequence, expected); err != nil {
		t.Fatalf("checkpoint sequence mismatch: %v", err)
	}
}

func TestRunStopsAfterRetryLimit(t *testing.T) {
	withCleanupDeadline(t)
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	grid := &fakeGrid{
		outcomes: map[string][]runner.StageOutcome{
			modsPlanStage: {{Status: runner.StageStatusCompleted}},
			"build":       {{Status: runner.StageStatusFailed, Retryable: true, Message: "still broken"}},
		},
	}
	opts := runner.Options{
		Ticket:           "",
		Tenant:           "acme",
		Events:           events,
		Grid:             grid,
		Planner:          runner.NewDefaultPlanner(),
		WorkspaceRoot:    t.TempDir(),
		MaxStageRetries:  0,
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
