package server

// This file contains tests for service initialization and lifecycle.

import (
	"context"
	"testing"

	"github.com/iw2rmb/ploy/internal/store"
)

// TestStorage_New verifies that the service constructor validates options and
// initializes the service with proper defaults for buffer and history sizes.
func TestStorage_New(t *testing.T) {
	tests := []struct {
		name    string
		opts    EventsOptions
		wantErr bool
	}{
		{
			name: "valid options with defaults",
			opts: EventsOptions{
				BufferSize:  0,
				HistorySize: 0,
			},
			wantErr: false,
		},
		{
			name: "valid options with explicit values",
			opts: EventsOptions{
				BufferSize:  32,
				HistorySize: 256,
			},
			wantErr: false,
		},
		{
			name: "negative buffer size",
			opts: EventsOptions{
				BufferSize:  -1,
				HistorySize: 256,
			},
			wantErr: true,
		},
		{
			name: "negative history size",
			opts: EventsOptions{
				BufferSize:  32,
				HistorySize: -1,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, err := NewEventsService(tt.opts)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if svc == nil {
				t.Fatal("expected service, got nil")
			}
			if svc.Hub() == nil {
				t.Fatal("expected hub, got nil")
			}
		})
	}
}

// TestStorage_ServiceStartStop verifies the service lifecycle, ensuring the service
// can be started and stopped cleanly without errors.
func TestStorage_ServiceStartStop(t *testing.T) {
	svc, err := NewEventsService(EventsOptions{
		BufferSize:  4,
		HistorySize: 8,
	})
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	ctx := context.Background()

	// Start the service.
	if err := svc.Start(ctx); err != nil {
		t.Fatalf("failed to start service: %v", err)
	}

	// Stop the service.
	if err := svc.Stop(ctx); err != nil {
		t.Fatalf("failed to stop service: %v", err)
	}
}

// TestStorage_WithoutStore verifies that the service correctly returns
// errors when attempting to persist events without a configured store.
// CreateAndPublishLog only handles SSE fanout (no store required), so it doesn't fail.
// This ensures proper error handling for services created without database backing.
func TestStorage_WithoutStore(t *testing.T) {
	svc, err := NewEventsService(EventsOptions{
		BufferSize:  4,
		HistorySize: 8,
		Store:       nil,
	})
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	ctx := context.Background()

	// Test CreateAndPublishEvent without store — should fail (requires DB persistence).
	_, err = svc.CreateAndPublishEvent(ctx, store.CreateEventParams{})
	if err == nil {
		t.Fatal("expected error when store not configured, got nil")
	}

	// Test CreateAndPublishLog without store — should NOT fail since it only fans out to SSE.
	// The log metadata is already persisted via blobpersist; this method only handles SSE fanout.
	err = svc.CreateAndPublishLog(ctx, store.Log{}, []byte{})
	if err != nil {
		t.Fatalf("expected no error for CreateAndPublishLog (SSE-only), got: %v", err)
	}
}
