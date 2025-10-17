package runner_test

import (
	"context"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
)

func TestInMemoryGridRecordsInvocations(t *testing.T) {
	grid := runner.NewInMemoryGrid()
	grid.StageOutcomes[buildGateStage] = []runner.StageOutcome{{Status: runner.StageStatusFailed, Retryable: true, Message: "retry-me"}}
	if outcome, err := grid.ExecuteStage(context.Background(), contracts.WorkflowTicket{TicketID: "ticket-1", Manifest: contracts.ManifestReference{Name: "smoke", Version: "2025-09-26"}}, runner.Stage{Name: modsPlanStage, Lane: "node-wasm"}, "/tmp/work"); err != nil {
		t.Fatalf("unexpected error for default outcome: %v", err)
	} else if outcome.Status != runner.StageStatusCompleted {
		t.Fatalf("expected completed outcome, got %+v", outcome)
	}
	outcome, err := grid.ExecuteStage(context.Background(), contracts.WorkflowTicket{TicketID: "ticket-1", Manifest: contracts.ManifestReference{Name: "smoke", Version: "2025-09-26"}}, runner.Stage{Name: buildGateStage, Lane: "build-gate"}, "/tmp/work")
	if err != nil {
		t.Fatalf("unexpected error for configured outcome: %v", err)
	}
	if outcome.Status != runner.StageStatusFailed || !outcome.Retryable {
		t.Fatalf("unexpected outcome: %+v", outcome)
	}
	invocations := grid.Invocations()
	if len(invocations) != 2 {
		t.Fatalf("expected two invocations, got %d", len(invocations))
	}
	if invocations[0].Stage.Name != modsPlanStage || invocations[1].Stage.Name != buildGateStage {
		t.Fatalf("unexpected invocation order: %+v", invocations)
	}
	if invocations[0].Stage.Lane != "node-wasm" || invocations[1].Stage.Lane != "build-gate" {
		t.Fatalf("unexpected lanes recorded: %+v", invocations)
	}
}

func TestInMemoryGridRejectsMissingLane(t *testing.T) {
	grid := runner.NewInMemoryGrid()
	_, err := grid.ExecuteStage(context.Background(), contracts.WorkflowTicket{TicketID: "ticket-1", Manifest: contracts.ManifestReference{Name: "smoke", Version: "2025-09-26"}}, runner.Stage{Name: modsPlanStage}, "/tmp/work")
	if err == nil || !strings.Contains(err.Error(), "lane missing") {
		t.Fatalf("expected lane missing error, got %v", err)
	}
}
