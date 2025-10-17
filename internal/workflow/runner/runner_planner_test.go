package runner_test

import (
	"context"
	"errors"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/mods"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
)

func TestDefaultPlannerBuildsOrderedStages(t *testing.T) {
	planner := runner.NewDefaultPlanner()
	ticket := contracts.WorkflowTicket{SchemaVersion: contracts.SchemaVersion, TicketID: "ticket-123", Tenant: "acme"}
	plan, err := planner.Build(context.Background(), ticket)
	if err != nil {
		t.Fatalf("unexpected error building plan: %v", err)
	}
	if plan.TicketID != ticket.TicketID {
		t.Fatalf("plan ticket mismatch: %s", plan.TicketID)
	}
	if len(plan.Stages) != 9 {
		t.Fatalf("expected 9 stages, got %d", len(plan.Stages))
	}
	expectOrder := []string{
		mods.StageNamePlan,
		mods.StageNameORWApply,
		mods.StageNameORWGenerate,
		mods.StageNameLLMPlan,
		mods.StageNameLLMExec,
		mods.StageNameHuman,
		buildGateStage,
		staticChecksStage,
		"test",
	}
	expectLanes := []string{"node-wasm", "node-wasm", "node-wasm", "gpu-ml", "gpu-ml", "mods-human", "build-gate", "static-checks", "test"}
	for i, name := range expectOrder {
		stage := plan.Stages[i]
		if stage.Name != name {
			t.Fatalf("unexpected stage at %d: %s", i, stage.Name)
		}
		if stage.Lane != expectLanes[i] {
			t.Fatalf("unexpected lane for %s: %s", stage.Name, stage.Lane)
		}
	}
	depMap := map[string][]string{
		mods.StageNamePlan:        nil,
		mods.StageNameORWApply:    {mods.StageNamePlan},
		mods.StageNameORWGenerate: {mods.StageNamePlan},
		mods.StageNameLLMPlan:     {mods.StageNamePlan},
		mods.StageNameLLMExec:     {mods.StageNameORWApply, mods.StageNameORWGenerate, mods.StageNameLLMPlan},
		mods.StageNameHuman:       {mods.StageNameLLMExec},
		buildGateStage:            {mods.StageNameHuman},
		staticChecksStage:         {buildGateStage},
		"test":                    {staticChecksStage},
	}
	for _, stage := range plan.Stages {
		expectedDeps, ok := depMap[stage.Name]
		if !ok {
			t.Fatalf("unexpected stage in plan: %s", stage.Name)
		}
		if len(stage.Dependencies) != len(expectedDeps) {
			t.Fatalf("dependency mismatch for %s: got %v want %v", stage.Name, stage.Dependencies, expectedDeps)
		}
		for i, dep := range expectedDeps {
			if stage.Dependencies[i] != dep {
				t.Fatalf("dependency %d for %s: got %s want %s", i, stage.Name, stage.Dependencies[i], dep)
			}
		}
	}
}

func TestRunUsesDefaultPlannerWhenNil(t *testing.T) {
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	opts := runner.Options{
		Ticket:           "",
		Tenant:           "acme",
		Events:           events,
		Grid:             noStageGrid{},
		Planner:          nil,
		WorkspaceRoot:    "",
		MaxStageRetries:  1,
		ManifestCompiler: newStubCompiler(),
	}
	if err := runner.Run(context.Background(), opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sequence := extractStageStatuses(events.checkpoints)
	if len(sequence) != 20 {
		t.Fatalf("expected 20 checkpoints, got %d", len(sequence))
	}
	if sequence[1].stage != modsPlanStage || sequence[1].status != runner.StageStatusRunning {
		t.Fatalf("expected first stage checkpoint to be %s running, got %+v", modsPlanStage, sequence[1])
	}
	last := sequence[len(sequence)-1]
	if last.stage != "workflow" || last.status != runner.StageStatusCompleted {
		t.Fatalf("expected workflow completion, got %+v", last)
	}
}

func TestRunFailsWhenPlannerErrors(t *testing.T) {
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	grid := &fakeGrid{}
	planner := failingPlanner{err: errors.New("planner boom")}
	opts := runner.Options{
		Ticket:           "",
		Tenant:           "acme",
		Events:           events,
		Grid:             grid,
		Planner:          planner,
		WorkspaceRoot:    t.TempDir(),
		MaxStageRetries:  1,
		ManifestCompiler: newStubCompiler(),
	}
	err := runner.Run(context.Background(), opts)
	if !errors.Is(err, planner.err) {
		t.Fatalf("expected planner error, got %v", err)
	}
}

func TestRunFailsWhenPlannerProducesInvalidStage(t *testing.T) {
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	opts := runner.Options{
		Ticket:           "",
		Tenant:           "acme",
		Events:           events,
		Grid:             &fakeGrid{},
		Planner:          invalidStagePlanner{},
		WorkspaceRoot:    t.TempDir(),
		MaxStageRetries:  1,
		ManifestCompiler: newStubCompiler(),
	}
	err := runner.Run(context.Background(), opts)
	if !errors.Is(err, runner.ErrCheckpointValidationFailed) {
		t.Fatalf("expected checkpoint validation error, got %v", err)
	}
}

func TestRunFailsWhenPlannerOmitsLane(t *testing.T) {
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	opts := runner.Options{
		Ticket:           "",
		Tenant:           "acme",
		Events:           events,
		Grid:             &fakeGrid{},
		Planner:          missingLanePlanner{},
		WorkspaceRoot:    t.TempDir(),
		MaxStageRetries:  1,
		ManifestCompiler: newStubCompiler(),
	}
	err := runner.Run(context.Background(), opts)
	if !errors.Is(err, runner.ErrLaneRequired) {
		t.Fatalf("expected ErrLaneRequired, got %v", err)
	}
}
