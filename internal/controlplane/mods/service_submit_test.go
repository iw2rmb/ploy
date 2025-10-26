package mods

import (
	"context"
	"testing"

	"github.com/iw2rmb/ploy/internal/mods/plan"
)

// TestServiceSubmitPersistsTicketAndEnqueuesRootStages verifies submit persists tickets and queues root stages.
func TestServiceSubmitPersistsTicketAndEnqueuesRootStages(t *testing.T) {
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
		TicketID:   "ticket-submit",
		Submitter:  "alice@example.com",
		Repository: "github.com/example/repo",
		Stages: []StageDefinition{
			{ID: plan.StageNamePlan, MaxAttempts: 1},
			{ID: plan.StageNameORWApply, Dependencies: []string{plan.StageNamePlan}, MaxAttempts: 1},
		},
	}

	status, err := service.Submit(ctx, spec)
	if err != nil {
		t.Fatalf("submit ticket: %v", err)
	}
	if status.TicketID != spec.TicketID {
		t.Fatalf("unexpected ticket id: got %s want %s", status.TicketID, spec.TicketID)
	}
	if status.State != TicketStatePending {
		t.Fatalf("expected pending ticket state, got %s", status.State)
	}
	if len(status.Stages) != 2 {
		t.Fatalf("expected 2 stages recorded, got %d", len(status.Stages))
	}
	stage := status.Stages[plan.StageNamePlan]
	if stage.State != StageStateQueued {
		t.Fatalf("expected root stage queued, got %s", stage.State)
	}

	jobs := scheduler.SubmittedJobs()
	if len(jobs) != 1 {
		t.Fatalf("expected 1 root job submitted, got %d", len(jobs))
	}
	if jobs[0].TicketID != spec.TicketID {
		t.Fatalf("unexpected job ticket: %s", jobs[0].TicketID)
	}
	if jobs[0].StageID != plan.StageNamePlan {
		t.Fatalf("unexpected job stage: %s", jobs[0].StageID)
	}
}
