package runner_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
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
	if len(plan.Stages) != 3 {
		t.Fatalf("expected 3 stages, got %d", len(plan.Stages))
	}
	expectOrder := []string{"mods", "build", "test"}
	expectLanes := []string{"node-wasm", "go-native", "go-native"}
	for i, name := range expectOrder {
		stage := plan.Stages[i]
		if stage.Name != name {
			t.Fatalf("unexpected stage at %d: %s", i, stage.Name)
		}
		if stage.Lane != expectLanes[i] {
			t.Fatalf("unexpected lane for %s: %s", stage.Name, stage.Lane)
		}
	}
	if deps := plan.Stages[1].Dependencies; len(deps) != 1 || deps[0] != "mods" {
		t.Fatalf("build dependencies mismatch: %v", deps)
	}
	if deps := plan.Stages[2].Dependencies; len(deps) != 1 || deps[0] != "build" {
		t.Fatalf("test dependencies mismatch: %v", deps)
	}
}

func TestRunRequiresEventsClient(t *testing.T) {
	opts := runner.Options{Ticket: "ticket-123", Grid: runner.NewInMemoryGrid()}
	err := runner.Run(context.Background(), opts)
	if !errors.Is(err, runner.ErrEventsClientRequired) {
		t.Fatalf("expected ErrEventsClientRequired, got %v", err)
	}
}

func TestRunRequiresGridClient(t *testing.T) {
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	opts := runner.Options{Ticket: "ticket-123", Events: events}
	err := runner.Run(context.Background(), opts)
	if !errors.Is(err, runner.ErrGridClientRequired) {
		t.Fatalf("expected ErrGridClientRequired, got %v", err)
	}
}

func TestRunReturnsClaimTicketError(t *testing.T) {
	events := &errorEvents{claimErr: errors.New("claim failed")}
	opts := runner.Options{
		Ticket:  "ticket-123",
		Events:  events,
		Grid:    runner.NewInMemoryGrid(),
		Planner: runner.NewDefaultPlanner(),
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
		Ticket:          "ticket-123",
		Events:          events,
		Grid:            runner.NewInMemoryGrid(),
		Planner:         runner.NewDefaultPlanner(),
		WorkspaceRoot:   t.TempDir(),
		MaxStageRetries: 1,
	}
	err := runner.Run(context.Background(), opts)
	if !errors.Is(err, events.publishErr) {
		t.Fatalf("expected publish error, got %v", err)
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
		Ticket:          "",
		Tenant:          "acme",
		Events:          events,
		Grid:            &fakeGrid{},
		Planner:         runner.NewDefaultPlanner(),
		WorkspaceRoot:   filepath.Join(file, "workspace"),
		MaxStageRetries: 1,
	}
	err := runner.Run(context.Background(), opts)
	if err == nil || !strings.Contains(err.Error(), "create workspace root") {
		t.Fatalf("expected workspace root error, got %v", err)
	}
}

func TestRunTreatsNegativeRetriesAsZero(t *testing.T) {
	withCleanupDeadline(t)
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	grid := &fakeGrid{
		outcomes: map[string][]runner.StageOutcome{
			"mods":  {{Status: runner.StageStatusCompleted}},
			"build": {{Status: runner.StageStatusFailed, Retryable: true, Message: "no more"}},
		},
	}
	opts := runner.Options{
		Ticket:          "",
		Tenant:          "acme",
		Events:          events,
		Grid:            grid,
		Planner:         runner.NewDefaultPlanner(),
		WorkspaceRoot:   t.TempDir(),
		MaxStageRetries: -3,
	}
	err := runner.Run(context.Background(), opts)
	if !errors.Is(err, runner.ErrStageFailed) {
		t.Fatalf("expected stage failure, got %v", err)
	}
	sequence := extractStageStatuses(events.checkpoints)
	expected := []stageStatusEntry{
		{stage: "ticket-claimed", status: runner.StageStatusCompleted},
		{stage: "mods", status: runner.StageStatusRunning},
		{stage: "mods", status: runner.StageStatusCompleted},
		{stage: "build", status: runner.StageStatusRunning},
		{stage: "build", status: runner.StageStatusFailed},
		{stage: "workflow", status: runner.StageStatusFailed},
	}
	if err := compareSequences(sequence, expected); err != nil {
		t.Fatalf("checkpoint sequence mismatch: %v", err)
	}
}

func TestRunDefaultsStageOutcomeStatus(t *testing.T) {
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	opts := runner.Options{
		Ticket:          "",
		Tenant:          "acme",
		Events:          events,
		Grid:            statuslessGrid{},
		Planner:         runner.NewDefaultPlanner(),
		WorkspaceRoot:   t.TempDir(),
		MaxStageRetries: 1,
	}
	if err := runner.Run(context.Background(), opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sequence := extractStageStatuses(events.checkpoints)
	if len(sequence) == 0 || sequence[len(sequence)-1].status != runner.StageStatusCompleted {
		t.Fatalf("expected workflow completion, got %v", sequence)
	}
}

func TestRunUsesDefaultPlannerWhenNil(t *testing.T) {
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	opts := runner.Options{
		Ticket:          "",
		Tenant:          "acme",
		Events:          events,
		Grid:            noStageGrid{},
		Planner:         nil,
		WorkspaceRoot:   "",
		MaxStageRetries: 1,
	}
	if err := runner.Run(context.Background(), opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sequence := extractStageStatuses(events.checkpoints)
	if len(sequence) != 8 {
		t.Fatalf("expected 8 checkpoints, got %d", len(sequence))
	}
}

func TestRunFailsWhenStageCompletionPublishFails(t *testing.T) {
	events := &countingEvents{
		ticket: contracts.WorkflowTicket{SchemaVersion: contracts.SchemaVersion, TicketID: "ticket-123", Tenant: "acme"},
		failAt: 3,
	}
	opts := runner.Options{
		Ticket:          "ticket-123",
		Events:          events,
		Grid:            noStageGrid{},
		Planner:         runner.NewDefaultPlanner(),
		WorkspaceRoot:   t.TempDir(),
		MaxStageRetries: 1,
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
		Ticket:          "ticket-123",
		Events:          events,
		Grid:            noStageGrid{},
		Planner:         runner.NewDefaultPlanner(),
		WorkspaceRoot:   t.TempDir(),
		MaxStageRetries: 1,
	}
	err := runner.Run(context.Background(), opts)
	if !errors.Is(err, events.err) {
		t.Fatalf("expected final publish error, got %v", err)
	}
}

func TestRunAutoClaimsTicketAndCleansWorkspace(t *testing.T) {
	withCleanupDeadline(t)
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	grid := &fakeGrid{
		outcomes: map[string][]runner.StageOutcome{
			"mods":  {{Status: runner.StageStatusCompleted}},
			"build": {{Status: runner.StageStatusCompleted}},
			"test":  {{Status: runner.StageStatusCompleted}},
		},
	}
	planner := runner.NewDefaultPlanner()
	workspaceRoot := t.TempDir()
	opts := runner.Options{
		Ticket:          "",
		Tenant:          "acme",
		Events:          events,
		Grid:            grid,
		Planner:         planner,
		WorkspaceRoot:   workspaceRoot,
		MaxStageRetries: 1,
	}
	if err := runner.Run(context.Background(), opts); err != nil {
		t.Fatalf("run error: %v", err)
	}
	if len(events.claimedTickets) != 1 || events.claimedTickets[0] != "ticket-123" {
		t.Fatalf("expected auto-claimed ticket, got %v", events.claimedTickets)
	}
	sequence := extractStageStatuses(events.checkpoints)
	expected := []stageStatusEntry{
		{stage: "ticket-claimed", status: runner.StageStatusCompleted},
		{stage: "mods", status: runner.StageStatusRunning},
		{stage: "mods", status: runner.StageStatusCompleted},
		{stage: "build", status: runner.StageStatusRunning},
		{stage: "build", status: runner.StageStatusCompleted},
		{stage: "test", status: runner.StageStatusRunning},
		{stage: "test", status: runner.StageStatusCompleted},
		{stage: "workflow", status: runner.StageStatusCompleted},
	}
	if err := compareSequences(sequence, expected); err != nil {
		t.Fatalf("checkpoint sequence mismatch: %v", err)
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

func TestRunRetriesStageOnce(t *testing.T) {
	withCleanupDeadline(t)
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	grid := &fakeGrid{
		outcomes: map[string][]runner.StageOutcome{
			"mods": {{Status: runner.StageStatusCompleted}},
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
		Ticket:          "",
		Tenant:          "acme",
		Events:          events,
		Grid:            grid,
		Planner:         planner,
		WorkspaceRoot:   workspaceRoot,
		MaxStageRetries: 1,
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
		{stage: "mods", status: runner.StageStatusRunning},
		{stage: "mods", status: runner.StageStatusCompleted},
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
			"mods":  {{Status: runner.StageStatusCompleted}},
			"build": {{Status: runner.StageStatusFailed, Retryable: true, Message: "still broken"}},
		},
	}
	opts := runner.Options{
		Ticket:          "",
		Tenant:          "acme",
		Events:          events,
		Grid:            grid,
		Planner:         runner.NewDefaultPlanner(),
		WorkspaceRoot:   t.TempDir(),
		MaxStageRetries: 0,
	}
	err := runner.Run(context.Background(), opts)
	if !errors.Is(err, runner.ErrStageFailed) {
		t.Fatalf("expected stage failure, got %v", err)
	}
	sequence := extractStageStatuses(events.checkpoints)
	expected := []stageStatusEntry{
		{stage: "ticket-claimed", status: runner.StageStatusCompleted},
		{stage: "mods", status: runner.StageStatusRunning},
		{stage: "mods", status: runner.StageStatusCompleted},
		{stage: "build", status: runner.StageStatusRunning},
		{stage: "build", status: runner.StageStatusFailed},
		{stage: "workflow", status: runner.StageStatusFailed},
	}
	if err := compareSequences(sequence, expected); err != nil {
		t.Fatalf("checkpoint sequence mismatch: %v", err)
	}
}

func TestRunFailsWhenPlannerErrors(t *testing.T) {
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	grid := &fakeGrid{}
	planner := failingPlanner{err: errors.New("planner boom")}
	opts := runner.Options{
		Ticket:          "",
		Tenant:          "acme",
		Events:          events,
		Grid:            grid,
		Planner:         planner,
		WorkspaceRoot:   t.TempDir(),
		MaxStageRetries: 1,
	}
	err := runner.Run(context.Background(), opts)
	if !errors.Is(err, planner.err) {
		t.Fatalf("expected planner error, got %v", err)
	}
}

func TestRunFailsWhenPlannerProducesInvalidStage(t *testing.T) {
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	opts := runner.Options{
		Ticket:          "",
		Tenant:          "acme",
		Events:          events,
		Grid:            &fakeGrid{},
		Planner:         invalidStagePlanner{},
		WorkspaceRoot:   t.TempDir(),
		MaxStageRetries: 1,
	}
	err := runner.Run(context.Background(), opts)
	if !errors.Is(err, runner.ErrCheckpointValidationFailed) {
		t.Fatalf("expected checkpoint validation error, got %v", err)
	}
}

func TestRunFailsWhenPlannerOmitsLane(t *testing.T) {
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	opts := runner.Options{
		Ticket:          "",
		Tenant:          "acme",
		Events:          events,
		Grid:            &fakeGrid{},
		Planner:         missingLanePlanner{},
		WorkspaceRoot:   t.TempDir(),
		MaxStageRetries: 1,
	}
	err := runner.Run(context.Background(), opts)
	if !errors.Is(err, runner.ErrLaneRequired) {
		t.Fatalf("expected ErrLaneRequired, got %v", err)
	}
}

func TestRunErrorsWhenTicketValidationFails(t *testing.T) {
	events := &recordingEvents{tenant: "acme", invalidTicket: true}
	opts := runner.Options{
		Ticket:          "",
		Tenant:          "acme",
		Events:          events,
		Grid:            &fakeGrid{},
		Planner:         runner.NewDefaultPlanner(),
		WorkspaceRoot:   t.TempDir(),
		MaxStageRetries: 1,
	}
	err := runner.Run(context.Background(), opts)
	if !errors.Is(err, runner.ErrTicketValidationFailed) {
		t.Fatalf("expected ticket validation error, got %v", err)
	}
}

func TestRunSurfacesNonRetryableStageFailure(t *testing.T) {
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	grid := &fakeGrid{
		outcomes: map[string][]runner.StageOutcome{
			"mods":  {{Status: runner.StageStatusCompleted}},
			"build": {{Status: runner.StageStatusFailed, Retryable: false, Message: "bad cache"}},
		},
	}
	opts := runner.Options{
		Ticket:          "",
		Tenant:          "acme",
		Events:          events,
		Grid:            grid,
		Planner:         runner.NewDefaultPlanner(),
		WorkspaceRoot:   t.TempDir(),
		MaxStageRetries: 1,
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
			"mods":  {{Status: runner.StageStatusCompleted}},
			"build": {{Status: runner.StageStatusFailed, Retryable: false}},
		},
	}
	opts := runner.Options{
		Ticket:          "",
		Tenant:          "acme",
		Events:          events,
		Grid:            grid,
		Planner:         runner.NewDefaultPlanner(),
		WorkspaceRoot:   t.TempDir(),
		MaxStageRetries: 1,
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
		Ticket:          "",
		Tenant:          "acme",
		Events:          events,
		Grid:            errorGrid{err: gridErr},
		Planner:         runner.NewDefaultPlanner(),
		WorkspaceRoot:   t.TempDir(),
		MaxStageRetries: 1,
	}
	err := runner.Run(context.Background(), opts)
	if !errors.Is(err, gridErr) {
		t.Fatalf("expected grid error, got %v", err)
	}
}

func TestInMemoryGridRecordsInvocations(t *testing.T) {
	grid := runner.NewInMemoryGrid()
	grid.StageOutcomes["build"] = []runner.StageOutcome{{Status: runner.StageStatusFailed, Retryable: true, Message: "retry-me"}}
	if outcome, err := grid.ExecuteStage(context.Background(), contracts.WorkflowTicket{TicketID: "ticket-1"}, runner.Stage{Name: "mods", Lane: "node-wasm"}, "/tmp/work"); err != nil {
		t.Fatalf("unexpected error for default outcome: %v", err)
	} else if outcome.Status != runner.StageStatusCompleted {
		t.Fatalf("expected completed outcome, got %+v", outcome)
	}
	outcome, err := grid.ExecuteStage(context.Background(), contracts.WorkflowTicket{TicketID: "ticket-1"}, runner.Stage{Name: "build", Lane: "go-native"}, "/tmp/work")
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
	if invocations[0].Stage.Name != "mods" || invocations[1].Stage.Name != "build" {
		t.Fatalf("unexpected invocation order: %+v", invocations)
	}
	if invocations[0].Stage.Lane != "node-wasm" || invocations[1].Stage.Lane != "go-native" {
		t.Fatalf("unexpected lanes recorded: %+v", invocations)
	}
}

func TestInMemoryGridRejectsMissingLane(t *testing.T) {
	grid := runner.NewInMemoryGrid()
	_, err := grid.ExecuteStage(context.Background(), contracts.WorkflowTicket{TicketID: "ticket-1"}, runner.Stage{Name: "mods"}, "/tmp/work")
	if err == nil || !strings.Contains(err.Error(), "lane missing") {
		t.Fatalf("expected lane missing error, got %v", err)
	}
}

type errorGrid struct {
	err error
}

func (g errorGrid) ExecuteStage(ctx context.Context, ticket contracts.WorkflowTicket, stage runner.Stage, workspace string) (runner.StageOutcome, error) {
	_ = ctx
	_ = ticket
	_ = stage
	_ = workspace
	return runner.StageOutcome{}, g.err
}

type errorEvents struct {
	ticket     contracts.WorkflowTicket
	claimErr   error
	publishErr error
}

func (e *errorEvents) ClaimTicket(ctx context.Context, ticketID string) (contracts.WorkflowTicket, error) {
	_ = ctx
	if e.claimErr != nil {
		return contracts.WorkflowTicket{}, e.claimErr
	}
	if e.ticket.TicketID == "" {
		e.ticket.TicketID = ticketID
	}
	if e.ticket.SchemaVersion == "" {
		e.ticket.SchemaVersion = contracts.SchemaVersion
	}
	if e.ticket.Tenant == "" {
		e.ticket.Tenant = "acme"
	}
	if strings.TrimSpace(e.ticket.TicketID) == "" {
		e.ticket.TicketID = "ticket-auto"
	}
	return e.ticket, nil
}

func (e *errorEvents) PublishCheckpoint(ctx context.Context, checkpoint contracts.WorkflowCheckpoint) error {
	_ = ctx
	if e.publishErr != nil {
		return e.publishErr
	}
	return nil
}

type statuslessGrid struct{}

func (statuslessGrid) ExecuteStage(ctx context.Context, ticket contracts.WorkflowTicket, stage runner.Stage, workspace string) (runner.StageOutcome, error) {
	_ = ctx
	_ = ticket
	_ = workspace
	return runner.StageOutcome{Stage: stage}, nil
}

type noStageGrid struct{}

func (noStageGrid) ExecuteStage(ctx context.Context, ticket contracts.WorkflowTicket, stage runner.Stage, workspace string) (runner.StageOutcome, error) {
	_ = ctx
	_ = ticket
	_ = workspace
	return runner.StageOutcome{Status: runner.StageStatusCompleted}, nil
}

type countingEvents struct {
	ticket       contracts.WorkflowTicket
	failAt       int
	publishCount int
	err          error
}

func (c *countingEvents) ClaimTicket(ctx context.Context, ticketID string) (contracts.WorkflowTicket, error) {
	_ = ctx
	if c.ticket.SchemaVersion == "" {
		c.ticket.SchemaVersion = contracts.SchemaVersion
	}
	if c.ticket.TicketID == "" {
		c.ticket.TicketID = ticketID
	}
	if c.ticket.Tenant == "" {
		c.ticket.Tenant = "acme"
	}
	return c.ticket, nil
}

func (c *countingEvents) PublishCheckpoint(ctx context.Context, checkpoint contracts.WorkflowCheckpoint) error {
	_ = ctx
	c.publishCount++
	if c.failAt > 0 && c.publishCount == c.failAt {
		if c.err == nil {
			c.err = errors.New("publish checkpoint failure")
		}
		return c.err
	}
	return nil
}

type stageStatusEntry struct {
	stage  string
	status runner.StageStatus
}

func extractStageStatuses(checkpoints []contracts.WorkflowCheckpoint) []stageStatusEntry {
	result := make([]stageStatusEntry, 0, len(checkpoints))
	for _, cp := range checkpoints {
		result = append(result, stageStatusEntry{stage: cp.Stage, status: runner.StageStatus(cp.Status)})
	}
	return result
}

func compareSequences(actual, expected []stageStatusEntry) error {
	if len(actual) != len(expected) {
		return fmt.Errorf("length mismatch: got %d want %d", len(actual), len(expected))
	}
	for i := range actual {
		a := actual[i]
		e := expected[i]
		if a.stage != e.stage || a.status != e.status {
			return fmt.Errorf("entry %d mismatch: got %s/%s want %s/%s", i, a.stage, a.status, e.stage, e.status)
		}
	}
	return nil
}

type recordingEvents struct {
	tenant         string
	nextTicket     string
	invalidTicket  bool
	claimedTickets []string
	checkpoints    []contracts.WorkflowCheckpoint
}

func (r *recordingEvents) ClaimTicket(ctx context.Context, ticketID string) (contracts.WorkflowTicket, error) {
	_ = ctx
	if ticketID == "" {
		ticketID = r.nextTicket
	}
	r.claimedTickets = append(r.claimedTickets, ticketID)
	if r.invalidTicket {
		return contracts.WorkflowTicket{}, nil
	}
	return contracts.WorkflowTicket{
		SchemaVersion: contracts.SchemaVersion,
		TicketID:      ticketID,
		Tenant:        r.tenant,
	}, nil
}

func (r *recordingEvents) PublishCheckpoint(ctx context.Context, checkpoint contracts.WorkflowCheckpoint) error {
	_ = ctx
	r.checkpoints = append(r.checkpoints, checkpoint)
	return nil
}

type gridCall struct {
	stage     string
	workspace string
}

type fakeGrid struct {
	outcomes      map[string][]runner.StageOutcome
	calls         []gridCall
	lastWorkspace string
}

func (g *fakeGrid) ExecuteStage(ctx context.Context, ticket contracts.WorkflowTicket, stage runner.Stage, workspace string) (runner.StageOutcome, error) {
	_ = ctx
	_ = ticket
	g.calls = append(g.calls, gridCall{stage: stage.Name, workspace: workspace})
	g.lastWorkspace = workspace
	queue := g.outcomes[stage.Name]
	if len(queue) == 0 {
		return runner.StageOutcome{Stage: stage, Status: runner.StageStatusCompleted}, nil
	}
	outcome := queue[0]
	g.outcomes[stage.Name] = queue[1:]
	if outcome.Stage.Name == "" {
		outcome.Stage = stage
	}
	return outcome, nil
}

func gatherStageAttempts(calls []gridCall, stage string) int {
	count := 0
	for _, call := range calls {
		if call.stage == stage {
			count++
		}
	}
	return count
}

type failingPlanner struct {
	err error
}

func (f failingPlanner) Build(ctx context.Context, ticket contracts.WorkflowTicket) (runner.ExecutionPlan, error) {
	_ = ctx
	_ = ticket
	return runner.ExecutionPlan{}, f.err
}

type invalidStagePlanner struct{}

func (invalidStagePlanner) Build(ctx context.Context, ticket contracts.WorkflowTicket) (runner.ExecutionPlan, error) {
	_ = ctx
	return runner.ExecutionPlan{
		TicketID: ticket.TicketID,
		Stages: []runner.Stage{
			{Name: "", Kind: runner.StageKindMods, Lane: "node-wasm"},
		},
	}, nil
}

type missingLanePlanner struct{}

func (missingLanePlanner) Build(ctx context.Context, ticket contracts.WorkflowTicket) (runner.ExecutionPlan, error) {
	_ = ctx
	return runner.ExecutionPlan{
		TicketID: ticket.TicketID,
		Stages: []runner.Stage{
			{Name: "mods", Kind: runner.StageKindMods, Lane: ""},
		},
	}, nil
}

func withCleanupDeadline(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		<-ctx.Done()
	})
}
