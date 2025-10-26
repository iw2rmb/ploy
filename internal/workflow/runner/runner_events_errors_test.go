package runner_test

import (
	"context"
	"errors"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
)

func TestRunReturnsClaimTicketError(t *testing.T) {
	events := &errorEvents{claimErr: errors.New("claim failed")}
	opts := runner.Options{
		Ticket:           "ticket-123",
		Events:           events,
		Grid:             runner.NewInMemoryGrid(),
		Planner:          runner.NewDefaultPlanner(),
		ManifestCompiler: newStubCompiler(),
	}
	err := runner.Run(context.Background(), opts)
	if !errors.Is(err, events.claimErr) {
		t.Fatalf("expected claim error, got %v", err)
	}
}

func TestRunPropagatesPublishCheckpointError(t *testing.T) {
	events := &errorEvents{
		ticket: contracts.WorkflowTicket{
			SchemaVersion: contracts.SchemaVersion,
			TicketID:      "ticket-123",
			Tenant:        "acme",
		},
		publishErr: errors.New("checkpoint failure"),
	}
	opts := runner.Options{
		Ticket:           "ticket-123",
		Events:           events,
		Grid:             runner.NewInMemoryGrid(),
		Planner:          runner.NewDefaultPlanner(),
		WorkspaceRoot:    t.TempDir(),
		MaxStageRetries:  1,
		ManifestCompiler: newStubCompiler(),
	}
	err := runner.Run(context.Background(), opts)
	if !errors.Is(err, events.publishErr) {
		t.Fatalf("expected publish error, got %v", err)
	}
}

func TestRunPropagatesPublishArtifactError(t *testing.T) {
	events := &errorEvents{
		ticket: contracts.WorkflowTicket{
			SchemaVersion: contracts.SchemaVersion,
			TicketID:      "ticket-123",
			Tenant:        "acme",
		},
		artifactErr: errors.New("artifact failure"),
	}
	grid := runner.NewInMemoryGrid()
	grid.StageOutcomes[modsPlanStage] = []runner.StageOutcome{{
		Status: runner.StageStatusCompleted,
		Artifacts: []runner.Artifact{{
			Name:        "mods-plan",
			ArtifactCID: "cid-mods-plan",
		}},
	}}
	opts := runner.Options{
		Ticket:           "ticket-123",
		Events:           events,
		Grid:             grid,
		Planner:          runner.NewDefaultPlanner(),
		WorkspaceRoot:    t.TempDir(),
		MaxStageRetries:  0,
		ManifestCompiler: newStubCompiler(),
	}
	err := runner.Run(context.Background(), opts)
	if !errors.Is(err, events.artifactErr) {
		t.Fatalf("expected artifact publish error, got %v", err)
	}
}

func TestRunFailsWhenStageCompletionPublishFails(t *testing.T) {
	events := &countingEvents{
		ticket: contracts.WorkflowTicket{SchemaVersion: contracts.SchemaVersion, TicketID: "ticket-123", Tenant: "acme"},
		failAt: 3,
	}
	opts := runner.Options{
		Ticket:           "ticket-123",
		Events:           events,
		Grid:             noStageGrid{},
		Planner:          runner.NewDefaultPlanner(),
		WorkspaceRoot:    t.TempDir(),
		MaxStageRetries:  1,
		ManifestCompiler: newStubCompiler(),
	}
	err := runner.Run(context.Background(), opts)
	if !errors.Is(err, events.err) {
		t.Fatalf("expected publish error, got %v", err)
	}
}

func TestRunFailsWhenFinalPublishFails(t *testing.T) {
	events := &countingEvents{
		ticket: contracts.WorkflowTicket{SchemaVersion: contracts.SchemaVersion, TicketID: "ticket-123", Tenant: "acme"},
		failAt: 8,
	}
	opts := runner.Options{
		Ticket:           "ticket-123",
		Events:           events,
		Grid:             noStageGrid{},
		Planner:          runner.NewDefaultPlanner(),
		WorkspaceRoot:    t.TempDir(),
		MaxStageRetries:  1,
		ManifestCompiler: newStubCompiler(),
	}
	err := runner.Run(context.Background(), opts)
	if !errors.Is(err, events.err) {
		t.Fatalf("expected final publish error, got %v", err)
	}
}
