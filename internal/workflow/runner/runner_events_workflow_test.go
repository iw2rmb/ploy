package runner_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/mods"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
)

func TestRunAutoClaimsTicketAndCleansWorkspace(t *testing.T) {
	withCleanupDeadline(t)
events := &recordingEvents{nextTicket: "ticket-123"}
    grid := &fakeRuntime{
		outcomes: map[string][]runner.StageOutcome{
			modsPlanStage:  {{Status: runner.StageStatusCompleted}},
			buildGateStage: {{Status: runner.StageStatusCompleted}},
			"test":         {{Status: runner.StageStatusCompleted}},
		},
	}
	planner := runner.NewDefaultPlanner()
	workspaceRoot := t.TempDir()
    opts := runner.Options{
		Ticket:           "",
    // tenant removed
		Events:           events,
        Runtime:          grid,
		Planner:          planner,
		WorkspaceRoot:    workspaceRoot,
		MaxStageRetries:  1,
		ManifestCompiler: newStubCompiler(),
	}
	if err := runner.Run(context.Background(), opts); err != nil {
		t.Fatalf("run error: %v", err)
	}
	if len(events.claimedTickets) != 1 || events.claimedTickets[0] != "ticket-123" {
		t.Fatalf("expected auto-claimed ticket, got %v", events.claimedTickets)
	}
	sequence := extractStageStatuses(events.checkpoints)
	if len(sequence) == 0 || sequence[0].stage != "ticket-claimed" || sequence[0].status != runner.StageStatusCompleted {
		t.Fatalf("expected ticket-claimed checkpoint first, got %v", sequence)
	}
	requireStageStatuses(t, sequence, modsPlanStage, []runner.StageStatus{runner.StageStatusRunning, runner.StageStatusCompleted})
	requireStageStatuses(t, sequence, mods.StageNameORWApply, []runner.StageStatus{runner.StageStatusRunning, runner.StageStatusCompleted})
	requireStageStatuses(t, sequence, mods.StageNameORWGenerate, []runner.StageStatus{runner.StageStatusRunning, runner.StageStatusCompleted})
	requireStageStatuses(t, sequence, mods.StageNameLLMPlan, []runner.StageStatus{runner.StageStatusRunning, runner.StageStatusCompleted})
	requireStageStatuses(t, sequence, mods.StageNameLLMExec, []runner.StageStatus{runner.StageStatusRunning, runner.StageStatusCompleted})
	requireStageStatuses(t, sequence, mods.StageNameHuman, []runner.StageStatus{runner.StageStatusRunning, runner.StageStatusCompleted})
	requireStageStatuses(t, sequence, buildGateStage, []runner.StageStatus{runner.StageStatusRunning, runner.StageStatusCompleted})
	requireStageStatuses(t, sequence, staticChecksStage, []runner.StageStatus{runner.StageStatusRunning, runner.StageStatusCompleted})
	requireStageStatuses(t, sequence, "test", []runner.StageStatus{runner.StageStatusRunning, runner.StageStatusCompleted})
	workflowStatuses := collectStageStatuses(sequence, "workflow")
	if len(workflowStatuses) != 1 || workflowStatuses[0] != runner.StageStatusCompleted {
		t.Fatalf("expected workflow completion, got %v", workflowStatuses)
	}
	if grid.lastWorkspace == "" {
		t.Fatal("expected workspace to be recorded")
	}
	if !strings.HasPrefix(grid.lastWorkspace, workspaceRoot) {
		t.Fatalf("workspace %q not under root %q", grid.lastWorkspace, workspaceRoot)
	}
	if _, err := os.Stat(grid.lastWorkspace); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected workspace to be deleted, stat err=%v", err)
	}
}
