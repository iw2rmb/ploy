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

// TestPublishTicket verifies that ticket state changes are correctly published
// as SSE events to the stream. It tests various ticket states (pending, running,
// succeeded, failed, cancelled) and validates that the event payload is correctly
// marshaled and delivered.
func TestStream_PublishTicket(t *testing.T) {
	tests := []struct {
		name        string
		runID       string
		state       modsapi.TicketState
		wantErr     bool
		checkEvents bool
	}{
		{
			name:        "publish queued ticket",
			runID:       uuid.New().String(),
			state:       modsapi.TicketStatePending,
			wantErr:     false,
			checkEvents: true,
		},
		{
			name:        "publish running ticket",
			runID:       uuid.New().String(),
			state:       modsapi.TicketStateRunning,
			wantErr:     false,
			checkEvents: true,
		},
		{
			name:        "publish succeeded ticket",
			runID:       uuid.New().String(),
			state:       modsapi.TicketStateSucceeded,
			wantErr:     false,
			checkEvents: true,
		},
		{
			name:        "publish failed ticket",
			runID:       uuid.New().String(),
			state:       modsapi.TicketStateFailed,
			wantErr:     false,
			checkEvents: true,
		},
		{
			name:        "publish cancelled ticket",
			runID:       uuid.New().String(),
			state:       modsapi.TicketStateCancelled,
			wantErr:     false,
			checkEvents: true,
		},
		{
			name:        "empty runID returns error",
			runID:       "",
			state:       modsapi.TicketStatePending,
			wantErr:     true,
			checkEvents: false,
		},
		{
			name:        "whitespace runID returns error",
			runID:       "  \t  ",
			state:       modsapi.TicketStatePending,
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

			payload := modsapi.TicketSummary{
				TicketID:   domaintypes.TicketID("test-ticket-123"),
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

			err = svc.PublishTicket(ctx, tt.runID, payload)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Check if ticket event was published to hub.
			if tt.checkEvents {
				snapshot := svc.Hub().Snapshot(tt.runID)
				if len(snapshot) == 0 {
					t.Fatal("expected ticket event in hub snapshot, got none")
				}
				if snapshot[0].Type != "run" {
					t.Fatalf("expected event type 'ticket', got %s", snapshot[0].Type)
				}

				// Verify the payload is correctly marshaled.
				var decodedPayload modsapi.TicketSummary
				if err := json.Unmarshal(snapshot[0].Data, &decodedPayload); err != nil {
					t.Fatalf("failed to unmarshal ticket payload: %v", err)
				}

				if decodedPayload.TicketID != payload.TicketID {
					t.Fatalf("expected ticket ID %s, got %s", payload.TicketID, decodedPayload.TicketID)
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

// TestPublishTicketWithContext verifies that the PublishTicket method correctly
// handles context cancellation and returns appropriate errors when the context
// is already cancelled before the publish operation begins.
func TestStream_PublishTicketWithContext(t *testing.T) {
	svc, err := New(Options{
		BufferSize:  4,
		HistorySize: 8,
	})
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	runID := uuid.New().String()
	payload := modsapi.TicketSummary{
		TicketID:  domaintypes.TicketID("test-ticket"),
		State:     modsapi.TicketStateRunning,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Stages:    map[string]modsapi.StageStatus{},
	}

	// Test with cancelled context.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = svc.PublishTicket(ctx, runID, payload)
	if err == nil {
		t.Fatal("expected error with cancelled context, got nil")
	}
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled error, got: %v", err)
	}
}
