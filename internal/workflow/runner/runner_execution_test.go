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

type recordingJobComposer struct {
	job   runner.StageJobSpec
	calls []runner.JobComposeRequest
}

func (c *recordingJobComposer) Compose(ctx context.Context, req runner.JobComposeRequest) (runner.StageJobSpec, error) {
	_ = ctx
	c.calls = append(c.calls, req)
	return c.job, nil
}

func TestRunAttachesComposedJobSpecToStages(t *testing.T) {
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	grid := &fakeGrid{outcomes: map[string][]runner.StageOutcome{
		modsPlanStage:  {{Status: runner.StageStatusCompleted}},
		buildGateStage: {{Status: runner.StageStatusCompleted}},
	}}
	composer := &recordingJobComposer{job: runner.StageJobSpec{
		Image:   "registry.dev/build:latest",
		Command: []string{"/bin/build"},
		Env:     map[string]string{"GOFLAGS": "-mod=vendor"},
		Resources: runner.StageJobResources{
			CPU:    "4000m",
			Memory: "8Gi",
		},
		Runtime: "docker",
	}}
	opts := runner.Options{
		Ticket:           "",
		Tenant:           "acme",
		Events:           events,
		Grid:             grid,
		Planner:          runner.NewDefaultPlanner(),
		WorkspaceRoot:    t.TempDir(),
		MaxStageRetries:  0,
		ManifestCompiler: newStubCompiler(),
		JobComposer:      composer,
	}
	if err := runner.Run(context.Background(), opts); err != nil {
		t.Fatalf("run error: %v", err)
	}
	if len(grid.calls) == 0 {
		t.Fatalf("expected grid calls")
	}
	for _, call := range grid.calls {
		if call.stage.Job.Image != composer.job.Image {
			t.Fatalf("expected job image propagated, got %s", call.stage.Job.Image)
		}
		if call.stage.Job.Resources.CPU != composer.job.Resources.CPU {
			t.Fatalf("expected job resources propagated, got %#v", call.stage.Job.Resources)
		}
		if call.stage.Job.Env["GOFLAGS"] != "-mod=vendor" {
			t.Fatalf("expected job env propagated, got %#v", call.stage.Job.Env)
		}
	}
	if len(composer.calls) == 0 {
		t.Fatalf("expected composer invocations")
	}
	for _, req := range composer.calls {
		if strings.TrimSpace(req.Stage.Lane) == "" {
			t.Fatalf("expected stage lane in composer request: %#v", req.Stage)
		}
	}
}

func TestRunSurfacesNonRetryableStageFailure(t *testing.T) {
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	grid := &fakeGrid{
		outcomes: map[string][]runner.StageOutcome{
			modsPlanStage:  {{Status: runner.StageStatusCompleted}},
			buildGateStage: {{Status: runner.StageStatusFailed, Retryable: false, Message: "bad cache"}},
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
			modsPlanStage:  {{Status: runner.StageStatusCompleted}},
			buildGateStage: {{Status: runner.StageStatusFailed, Retryable: false}},
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
			buildGateStage: {
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
	b := gatherStageAttempts(grid.calls, buildGateStage)
	if b != 2 {
		t.Fatalf("expected build stage to retry once, got %d attempts", b)
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
	requireStageStatuses(t, sequence, buildGateStage, []runner.StageStatus{runner.StageStatusRunning, runner.StageStatusRetrying, runner.StageStatusRunning, runner.StageStatusCompleted})
	requireStageStatuses(t, sequence, staticChecksStage, []runner.StageStatus{runner.StageStatusRunning, runner.StageStatusCompleted})
	requireStageStatuses(t, sequence, "test", []runner.StageStatus{runner.StageStatusRunning, runner.StageStatusCompleted})
	workflowStatuses := collectStageStatuses(sequence, "workflow")
	if len(workflowStatuses) != 1 || workflowStatuses[0] != runner.StageStatusCompleted {
		t.Fatalf("expected workflow completion, got %v", workflowStatuses)
	}
}

func TestRunStopsAfterRetryLimit(t *testing.T) {
	withCleanupDeadline(t)
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	grid := &fakeGrid{
		outcomes: map[string][]runner.StageOutcome{
			modsPlanStage:  {{Status: runner.StageStatusCompleted}},
			buildGateStage: {{Status: runner.StageStatusFailed, Retryable: true, Message: "still broken"}},
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
