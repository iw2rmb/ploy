package events

import (
	"context"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/node/logstream"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		opts    Options
		wantErr bool
	}{
		{
			name: "valid options with defaults",
			opts: Options{
				BufferSize:  0,
				HistorySize: 0,
			},
			wantErr: false,
		},
		{
			name: "valid options with explicit values",
			opts: Options{
				BufferSize:  32,
				HistorySize: 256,
			},
			wantErr: false,
		},
		{
			name: "negative buffer size",
			opts: Options{
				BufferSize:  -1,
				HistorySize: 256,
			},
			wantErr: true,
		},
		{
			name: "negative history size",
			opts: Options{
				BufferSize:  32,
				HistorySize: -1,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, err := New(tt.opts)
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

func TestServiceStartStop(t *testing.T) {
	svc, err := New(Options{
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

func TestServiceHubIntegration(t *testing.T) {
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
