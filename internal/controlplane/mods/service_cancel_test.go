package mods

import (
	"context"
	"testing"

	"github.com/iw2rmb/ploy/internal/mods/plan"
)

// TestServiceCancelAndResume validates cancelling tickets and resuming queued stages.
func TestServiceCancelAndResume(t *testing.T) {
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
		TicketID:   "ticket-cancel",
		Submitter:  "eve@example.com",
		Repository: "github.com/example/repo",
		Stages: []StageDefinition{
			{ID: plan.StageNamePlan, MaxAttempts: 1},
			{ID: plan.StageNameHuman, Dependencies: []string{plan.StageNamePlan}, MaxAttempts: 1},
		},
	}
	if _, err := service.Submit(ctx, spec); err != nil {
		t.Fatalf("submit ticket: %v", err)
	}

	if err := service.Cancel(ctx, spec.TicketID); err != nil {
		t.Fatalf("cancel ticket: %v", err)
	}
	status, err := service.TicketStatus(ctx, spec.TicketID)
	if err != nil {
		t.Fatalf("ticket status: %v", err)
	}
	if status.State != TicketStateCancelled && status.State != TicketStateCancelling {
		t.Fatalf("expected cancelling or cancelled, got %s", status.State)
	}

	if _, err := service.Resume(ctx, spec.TicketID); err != nil {
		t.Fatalf("resume ticket: %v", err)
	}
	resumed, err := service.TicketStatus(ctx, spec.TicketID)
	if err != nil {
		t.Fatalf("ticket status after resume: %v", err)
	}
	if resumed.State != TicketStatePending {
		t.Fatalf("expected ticket pending after resume, got %s", resumed.State)
	}
	if len(scheduler.SubmittedJobs()) < 2 {
		t.Fatalf("expected resumed stage enqueued, got %d jobs", len(scheduler.SubmittedJobs()))
	}
}
