package logstream

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/migs/api"
)

// TestPublishRunTypedPayload verifies that PublishRun accepts only api.RunSummary
// and that the payload marshals correctly through publish/subscribe round-trip.
func TestPublishRunTypedPayload(t *testing.T) {
	hub := NewHub(Options{BufferSize: 4, HistorySize: 8})
	ctx := context.Background()
	runID := domaintypes.NewRunID()

	// Construct a typed RunSummary payload with RunID field.
	run := api.RunSummary{
		RunID:  runID,
		State:  api.RunStateRunning,
		Stages: make(map[domaintypes.JobID]api.StageStatus),
	}

	// Publish the run event using renamed PublishRun method.
	if err := hub.PublishRun(ctx, runID, run); err != nil {
		t.Fatalf("publish run: %v", err)
	}

	// Subscribe and receive the event.
	sub, err := hub.Subscribe(ctx, runID, 0)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer sub.Cancel()

	select {
	case evt := <-sub.Events:
		if evt.Type != domaintypes.SSEEventRun {
			t.Fatalf("expected event type 'run', got %s", evt.Type)
		}
		// Unmarshal and verify the payload.
		var received api.RunSummary
		if err := json.Unmarshal(evt.Data, &received); err != nil {
			t.Fatalf("unmarshal run payload: %v", err)
		}
		if received.RunID != run.RunID {
			t.Fatalf("expected run_id %s, got %s", run.RunID, received.RunID)
		}
		if received.State != run.State {
			t.Fatalf("expected state %s, got %s", run.State, received.State)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for run event")
	}
}
