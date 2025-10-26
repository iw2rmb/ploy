package mods

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/iw2rmb/ploy/internal/mods/plan"
)

// TestServiceProcessJobCompletionSuccessTransitionsStageAndEnqueuesDependents verifies successful completions advance dependents.
func TestServiceProcessJobCompletionSuccessTransitionsStageAndEnqueuesDependents(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	e, client := newTestEtcd(t)
	defer e.Close()
	defer client.Close()

	scheduler := newFakeScheduler()
	service := newTestService(t, client, scheduler)
	defer func() {
		if err := service.Close(); err != nil {
			t.Fatalf("close service: %v", err)
		}
	}()

	spec := TicketSpec{
		TicketID:   "ticket-complete-success",
		Submitter:  "carol@example.com",
		Repository: "github.com/example/repo",
		Stages: []StageDefinition{
			{ID: plan.StageNamePlan, MaxAttempts: 1},
			{ID: plan.StageNameORWApply, Dependencies: []string{plan.StageNamePlan}, MaxAttempts: 1},
			{ID: plan.StageNameLLMExec, Dependencies: []string{plan.StageNameORWApply}, MaxAttempts: 1},
		},
	}
	if _, err := service.Submit(ctx, spec); err != nil {
		t.Fatalf("submit ticket: %v", err)
	}

	jobID := uuid.NewString()
	if _, err := service.ClaimStage(ctx, ClaimStageRequest{
		TicketID: spec.TicketID,
		StageID:  plan.StageNamePlan,
		JobID:    jobID,
		NodeID:   "node-1",
	}); err != nil {
		t.Fatalf("claim stage: %v", err)
	}

	if err := service.ProcessJobCompletion(ctx, JobCompletion{
		TicketID: spec.TicketID,
		StageID:  plan.StageNamePlan,
		JobID:    jobID,
		State:    JobCompletionSucceeded,
	}); err != nil {
		t.Fatalf("process completion: %v", err)
	}

	stageStatus, err := service.StageStatus(ctx, spec.TicketID, plan.StageNamePlan)
	if err != nil {
		t.Fatalf("get stage status: %v", err)
	}
	if stageStatus.State != StageStateSucceeded {
		t.Fatalf("expected stage succeeded, got %s", stageStatus.State)
	}
	if stageStatus.CurrentJobID != "" {
		t.Fatalf("expected current job cleared, got %s", stageStatus.CurrentJobID)
	}

	jobs := scheduler.SubmittedJobs()
	if len(jobs) != 2 {
		t.Fatalf("expected dependent stage enqueued, got %d jobs", len(jobs))
	}
	if jobs[1].StageID != plan.StageNameORWApply {
		t.Fatalf("expected dependent orw-apply queued, got %s", jobs[1].StageID)
	}
}

// TestServiceProcessJobCompletionFailureRetries ensures failures retry until attempts exhaust.
func TestServiceProcessJobCompletionFailureRetries(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	e, client := newTestEtcd(t)
	defer e.Close()
	defer client.Close()

	scheduler := newFakeScheduler()
	service := newTestService(t, client, scheduler)
	defer func() {
		if err := service.Close(); err != nil {
			t.Fatalf("close service: %v", err)
		}
	}()

	spec := TicketSpec{
		TicketID:   "ticket-complete-failure",
		Submitter:  "dave@example.com",
		Repository: "github.com/example/repo",
		Stages: []StageDefinition{
			{ID: plan.StageNamePlan, MaxAttempts: 2},
		},
	}
	if _, err := service.Submit(ctx, spec); err != nil {
		t.Fatalf("submit ticket: %v", err)
	}

	jobID := uuid.NewString()
	if _, err := service.ClaimStage(ctx, ClaimStageRequest{
		TicketID: spec.TicketID,
		StageID:  plan.StageNamePlan,
		JobID:    jobID,
		NodeID:   "node-2",
	}); err != nil {
		t.Fatalf("claim stage: %v", err)
	}

	if err := service.ProcessJobCompletion(ctx, JobCompletion{
		TicketID: spec.TicketID,
		StageID:  plan.StageNamePlan,
		JobID:    jobID,
		State:    JobCompletionFailed,
		Error:    "step failed",
	}); err != nil {
		t.Fatalf("process completion: %v", err)
	}

	stageStatus, err := service.StageStatus(ctx, spec.TicketID, plan.StageNamePlan)
	if err != nil {
		t.Fatalf("stage status: %v", err)
	}
	if stageStatus.State != StageStateQueued {
		t.Fatalf("expected stage re-queued, got %s", stageStatus.State)
	}
	if stageStatus.Attempts != 1 {
		t.Fatalf("expected attempts incremented to 1, got %d", stageStatus.Attempts)
	}

	if len(scheduler.SubmittedJobs()) != 2 {
		t.Fatalf("expected retry job submitted, got %d", len(scheduler.SubmittedJobs()))
	}

	retryJobID := scheduler.SubmittedJobs()[1].JobID
	if _, err := service.ClaimStage(ctx, ClaimStageRequest{
		TicketID: spec.TicketID,
		StageID:  plan.StageNamePlan,
		JobID:    retryJobID,
		NodeID:   "node-2",
	}); err != nil {
		t.Fatalf("claim retry stage: %v", err)
	}
	if err := service.ProcessJobCompletion(ctx, JobCompletion{
		TicketID: spec.TicketID,
		StageID:  plan.StageNamePlan,
		JobID:    retryJobID,
		State:    JobCompletionFailed,
		Error:    "second failure",
	}); err != nil {
		t.Fatalf("process completion retry: %v", err)
	}

	ticketStatus, err := service.TicketStatus(ctx, spec.TicketID)
	if err != nil {
		t.Fatalf("ticket status: %v", err)
	}
	if ticketStatus.State != TicketStateFailed {
		t.Fatalf("expected ticket failed after retries, got %s", ticketStatus.State)
	}
}
