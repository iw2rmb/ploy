package logstream

import (
	"context"
	"errors"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/migs/api"
)

// TestHubRejectsUnknownEventType verifies that the SSEEventType validation
// correctly rejects unknown event types. The hub uses a closed set of allowed
// event types (log, retention, run, stage, done) to prevent drift and
// accidental publication of invalid types.
func TestHubRejectsUnknownEventType(t *testing.T) {
	hub := NewHub(Options{BufferSize: 1, HistorySize: 1})
	ctx := context.Background()
	validRunID := domaintypes.NewRunID()
	invalidRunID := domaintypes.NewRunID()

	if err := hub.publish(ctx, validRunID, domaintypes.SSEEventLog, LogRecord{
		Timestamp: "2025-10-22T12:00:00Z",
		Stream:    "stdout",
		Line:      "hello",
	}); err != nil {
		t.Fatalf("expected valid publish to succeed: %v", err)
	}

	tests := []struct {
		name      string
		eventType domaintypes.SSEEventType
	}{
		{"empty", domaintypes.SSEEventType("")},
		{"unknown", domaintypes.SSEEventType("unknown")},
		{"status", domaintypes.SSEEventType("status")},
		{"uppercase", domaintypes.SSEEventType("LOG")},
		{"whitespace-only", domaintypes.SSEEventType("   ")},
		{"leading/trailing whitespace", domaintypes.SSEEventType(" log ")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := hub.publish(ctx, invalidRunID, tt.eventType, Status{Status: "noop"})
			if !errors.Is(err, ErrInvalidEventType) {
				t.Fatalf("expected ErrInvalidEventType, got %v", err)
			}
			if hub.getStream(invalidRunID) != nil {
				t.Fatalf("expected no stream creation for invalid event type")
			}
		})
	}
}

// TestRunIDRejectedAtStreamBoundary verifies that blank/whitespace run IDs fail
// before publish/subscribe operations. This enforces the typed boundary contract
// where invalid run IDs are rejected at the API layer rather than being silently
// accepted or causing downstream errors.
func TestRunIDRejectedAtStreamBoundary(t *testing.T) {
	hub := NewHub(Options{BufferSize: 4, HistorySize: 8})
	ctx := context.Background()

	// Test cases for invalid run IDs that should be rejected.
	invalidRunIDs := []struct {
		name  string
		runID domaintypes.RunID
	}{
		{"empty", domaintypes.RunID("")},
		{"whitespace-only-space", domaintypes.RunID("   ")},
		{"whitespace-only-tab", domaintypes.RunID("\t\t")},
		{"whitespace-only-newline", domaintypes.RunID("\n")},
		{"whitespace-only-mixed", domaintypes.RunID(" \t\n ")},
	}

	// Test PublishLog rejects invalid run IDs.
	t.Run("PublishLog", func(t *testing.T) {
		for _, tt := range invalidRunIDs {
			t.Run(tt.name, func(t *testing.T) {
				err := hub.PublishLog(ctx, tt.runID, LogRecord{
					Timestamp: "2025-12-01T10:00:00Z",
					Stream:    "stdout",
					Line:      "test line",
				})
				if !errors.Is(err, ErrInvalidRunID) {
					t.Errorf("PublishLog: expected ErrInvalidRunID, got %v", err)
				}
			})
		}
	})

	// Test PublishRetention rejects invalid run IDs.
	t.Run("PublishRetention", func(t *testing.T) {
		for _, tt := range invalidRunIDs {
			t.Run(tt.name, func(t *testing.T) {
				err := hub.PublishRetention(ctx, tt.runID, RetentionHint{
					Retained: true,
					TTL:      "72h",
				})
				if !errors.Is(err, ErrInvalidRunID) {
					t.Errorf("PublishRetention: expected ErrInvalidRunID, got %v", err)
				}
			})
		}
	})

	// Test PublishStatus rejects invalid run IDs.
	t.Run("PublishStatus", func(t *testing.T) {
		for _, tt := range invalidRunIDs {
			t.Run(tt.name, func(t *testing.T) {
				err := hub.PublishStatus(ctx, tt.runID, Status{Status: "completed"})
				if !errors.Is(err, ErrInvalidRunID) {
					t.Errorf("PublishStatus: expected ErrInvalidRunID, got %v", err)
				}
			})
		}
	})

	// Test PublishRun rejects invalid run IDs.
	t.Run("PublishRun", func(t *testing.T) {
		for _, tt := range invalidRunIDs {
			t.Run(tt.name, func(t *testing.T) {
				err := hub.PublishRun(ctx, tt.runID, api.RunSummary{
					RunID: "test-run",
					State: api.RunStateRunning,
				})
				if !errors.Is(err, ErrInvalidRunID) {
					t.Errorf("PublishRun: expected ErrInvalidRunID, got %v", err)
				}
			})
		}
	})

	// Test Subscribe rejects invalid run IDs.
	t.Run("Subscribe", func(t *testing.T) {
		for _, tt := range invalidRunIDs {
			t.Run(tt.name, func(t *testing.T) {
				_, err := hub.Subscribe(ctx, tt.runID, 0)
				if !errors.Is(err, ErrInvalidRunID) {
					t.Errorf("Subscribe: expected ErrInvalidRunID, got %v", err)
				}
			})
		}
	})

	// Test Ensure rejects invalid run IDs.
	t.Run("Ensure", func(t *testing.T) {
		for _, tt := range invalidRunIDs {
			t.Run(tt.name, func(t *testing.T) {
				err := hub.Ensure(tt.runID)
				if !errors.Is(err, ErrInvalidRunID) {
					t.Errorf("Ensure: expected ErrInvalidRunID, got %v", err)
				}
			})
		}
	})

	// Verify that valid run IDs are accepted (contrast test).
	t.Run("ValidRunIDsAccepted", func(t *testing.T) {
		validRunID := domaintypes.NewRunID()

		// Ensure should succeed.
		if err := hub.Ensure(validRunID); err != nil {
			t.Errorf("Ensure: unexpected error for valid run ID: %v", err)
		}

		// PublishLog should succeed.
		if err := hub.PublishLog(ctx, validRunID, LogRecord{
			Timestamp: "2025-12-01T10:00:00Z",
			Stream:    "stdout",
			Line:      "valid log",
		}); err != nil {
			t.Errorf("PublishLog: unexpected error for valid run ID: %v", err)
		}

		// Subscribe should succeed.
		sub, err := hub.Subscribe(ctx, validRunID, 0)
		if err != nil {
			t.Errorf("Subscribe: unexpected error for valid run ID: %v", err)
		} else {
			sub.Cancel()
		}
	})
}
