package events

// This file contains tests for SSE streaming behavior.
// Event storage tests are in service_test.go.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	modsapi "github.com/iw2rmb/ploy/internal/mods/api"
	logstream "github.com/iw2rmb/ploy/internal/stream"
)

// TestServiceHubIntegration verifies that the service hub correctly handles
// SSE event subscription, publishing, and stream closure. It tests the full
// lifecycle: publish log event, subscribe to stream, publish status to close
// stream, and verify all events are received in order.
func TestStream_ServiceHubIntegration(t *testing.T) {
	svc, err := New(Options{
		BufferSize:  4,
		HistorySize: 8,
	})
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	ctx := context.Background()
	hub := svc.Hub()

	// Publish a log event.
	if err := hub.PublishLog(ctx, "test-stream", logstream.LogRecord{
		Timestamp: time.Now().Format(time.RFC3339),
		Stream:    "stdout",
		Line:      "test log line",
	}); err != nil {
		t.Fatalf("failed to publish log: %v", err)
	}

	// Subscribe to the stream.
	sub, err := hub.Subscribe(ctx, "test-stream", 0)
	if err != nil {
		t.Fatalf("failed to subscribe: %v", err)
	}
	defer sub.Cancel()

	// Publish a status event to close the stream.
	if err := hub.PublishStatus(ctx, "test-stream", logstream.Status{
		Status: "completed",
	}); err != nil {
		t.Fatalf("failed to publish status: %v", err)
	}

	// Read events from the subscription.
	events := make([]logstream.Event, 0)
	for evt := range sub.Events {
		events = append(events, evt)
		if evt.Type == "done" {
			break
		}
	}

	// Verify we received both events.
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Type != "log" {
		t.Fatalf("expected first event type 'log', got %s", events[0].Type)
	}
	if events[1].Type != "done" {
		t.Fatalf("expected second event type 'done', got %s", events[1].Type)
	}
}

// TestStream_PublishRun verifies that run state changes are correctly published
// as SSE events to the stream. It tests various run states (pending, running,
// succeeded, failed, cancelled) and validates that the event payload is correctly
// marshaled and delivered.
func TestStream_PublishRun(t *testing.T) {
	tests := []struct {
		name        string
		runID       string
		state       modsapi.RunState
		wantErr     bool
		checkEvents bool
	}{
		{
			name:        "publish queued run",
			runID:       uuid.New().String(),
			state:       modsapi.RunStatePending,
			wantErr:     false,
			checkEvents: true,
		},
		{
			name:        "publish running run",
			runID:       uuid.New().String(),
			state:       modsapi.RunStateRunning,
			wantErr:     false,
			checkEvents: true,
		},
		{
			name:        "publish succeeded run",
			runID:       uuid.New().String(),
			state:       modsapi.RunStateSucceeded,
			wantErr:     false,
			checkEvents: true,
		},
		{
			name:        "publish failed run",
			runID:       uuid.New().String(),
			state:       modsapi.RunStateFailed,
			wantErr:     false,
			checkEvents: true,
		},
		{
			name:        "publish cancelled run",
			runID:       uuid.New().String(),
			state:       modsapi.RunStateCancelled,
			wantErr:     false,
			checkEvents: true,
		},
		{
			name:        "empty runID returns error",
			runID:       "",
			state:       modsapi.RunStatePending,
			wantErr:     true,
			checkEvents: false,
		},
		{
			name:        "whitespace runID returns error",
			runID:       "  \t  ",
			state:       modsapi.RunStatePending,
			wantErr:     true,
			checkEvents: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, err := New(Options{
				BufferSize:  4,
				HistorySize: 8,
			})
			if err != nil {
				t.Fatalf("failed to create service: %v", err)
			}

			ctx := context.Background()
			now := time.Now()

			// Build payload using renamed RunID field instead of TicketID.
			payload := modsapi.RunSummary{
				RunID:      domaintypes.RunID("test-run-123"),
				State:      tt.state,
				Submitter:  "test-user",
				Repository: "test-repo",
				Metadata: map[string]string{
					"key": "value",
				},
				CreatedAt: now,
				UpdatedAt: now,
				Stages: map[string]modsapi.StageStatus{
					"stage-1": {
						State:       modsapi.StageStateQueued,
						Attempts:    0,
						MaxAttempts: 3,
					},
				},
			}

			// Call renamed PublishRun method.
			err = svc.PublishRun(ctx, tt.runID, payload)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Check if run event was published to hub.
			if tt.checkEvents {
				snapshot := svc.Hub().Snapshot(tt.runID)
				if len(snapshot) == 0 {
					t.Fatal("expected run event in hub snapshot, got none")
				}
				if snapshot[0].Type != "run" {
					t.Fatalf("expected event type 'run', got %s", snapshot[0].Type)
				}

				// Verify the payload is correctly marshaled.
				var decodedPayload modsapi.RunSummary
				if err := json.Unmarshal(snapshot[0].Data, &decodedPayload); err != nil {
					t.Fatalf("failed to unmarshal run payload: %v", err)
				}

				// Verify RunID field (renamed from TicketID).
				if decodedPayload.RunID != payload.RunID {
					t.Fatalf("expected run ID %s, got %s", payload.RunID, decodedPayload.RunID)
				}
				if decodedPayload.State != payload.State {
					t.Fatalf("expected state %s, got %s", payload.State, decodedPayload.State)
				}
				if decodedPayload.Submitter != payload.Submitter {
					t.Fatalf("expected submitter %s, got %s", payload.Submitter, decodedPayload.Submitter)
				}
			}
		})
	}
}

// TestStream_PublishRunWithContext verifies that the PublishRun method correctly
// handles context cancellation and returns appropriate errors when the context
// is already cancelled before the publish operation begins.
func TestStream_PublishRunWithContext(t *testing.T) {
	svc, err := New(Options{
		BufferSize:  4,
		HistorySize: 8,
	})
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	runID := uuid.New().String()
	// Build payload using renamed RunID field instead of TicketID.
	payload := modsapi.RunSummary{
		RunID:     domaintypes.RunID("test-run"),
		State:     modsapi.RunStateRunning,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Stages:    map[string]modsapi.StageStatus{},
	}

	// Test with cancelled context.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Call renamed PublishRun method.
	err = svc.PublishRun(ctx, runID, payload)
	if err == nil {
		t.Fatal("expected error with cancelled context, got nil")
	}
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled error, got: %v", err)
	}
}
