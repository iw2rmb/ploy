package mods

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"

	"github.com/iw2rmb/ploy/internal/mods/plan"
)

// TestServiceClaimStageUsesOptimisticConcurrency ensures only one worker claims a stage.
func TestServiceClaimStageUsesOptimisticConcurrency(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	e, client := newTestEtcd(t)
	defer e.Close()
	defer client.Close()

	service := newTestService(t, client, newFakeScheduler())
	defer func() {
		if err := service.Close(); err != nil {
			t.Fatalf("close service: %v", err)
		}
	}()

	spec := TicketSpec{
		TicketID:   "ticket-claim",
		Submitter:  "bob@example.com",
		Repository: "github.com/example/repo",
		Stages: []StageDefinition{
			{ID: plan.StageNamePlan, MaxAttempts: 2},
		},
	}

	if _, err := service.Submit(ctx, spec); err != nil {
		t.Fatalf("submit ticket: %v", err)
	}

	var success atomic.Int64
	wg := sync.WaitGroup{}
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			request := ClaimStageRequest{
				TicketID: spec.TicketID,
				StageID:  plan.StageNamePlan,
				JobID:    uuid.NewString(),
				NodeID:   "node-claim",
			}
			if _, err := service.ClaimStage(ctx, request); err == nil {
				success.Add(1)
			} else if !errors.Is(err, ErrStageAlreadyClaimed) {
				t.Errorf("unexpected claim error: %v", err)
			}
		}(i)
	}
	wg.Wait()

	if success.Load() != 1 {
		t.Fatalf("expected exactly one claim success, got %d", success.Load())
	}
}
