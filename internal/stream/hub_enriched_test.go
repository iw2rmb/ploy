package logstream

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// TestLogRecordEnrichedFields verifies that enriched fields (NodeID, JobID,
// JobType) marshal correctly through the publish/subscribe round-trip.
// Fields with zero values are omitted due to `omitempty` tags.
func TestLogRecordEnrichedFields(t *testing.T) {
	hub := NewHub(Options{BufferSize: 4, HistorySize: 8})
	ctx := context.Background()
	jobID := domaintypes.NewJobID()

	tests := []struct {
		name   string
		record LogRecord
		want   map[string]any
	}{
		{
			name: "all enriched fields populated",
			record: LogRecord{
				Timestamp: "2025-10-22T12:00:00Z",
				Stream:    "stdout",
				Line:      "hello world",
				NodeID:    "aB3xY9",
				JobID:     jobID,
				JobType:   "mod",
			},
			want: map[string]any{
				"timestamp": "2025-10-22T12:00:00Z",
				"stream":    "stdout",
				"line":      "hello world",
				"node_id":   "aB3xY9",
				"job_id":    jobID.String(),
				"job_type":  "mod",
			},
		},
		{
			name: "omitempty omits zero values",
			record: LogRecord{
				Timestamp: "2025-10-22T12:00:01Z",
				Stream:    "stderr",
				Line:      "minimal record",
			},
			want: map[string]any{
				"timestamp": "2025-10-22T12:00:01Z",
				"stream":    "stderr",
				"line":      "minimal record",
				// node_id, job_id, job_type, next_id should be absent
			},
		},
		{
			name: "partial enrichment",
			record: LogRecord{
				Timestamp: "2025-10-22T12:00:02Z",
				Stream:    "stdout",
				Line:      "partial context",
				NodeID:    "Z9yX3b",
			},
			want: map[string]any{
				"timestamp": "2025-10-22T12:00:02Z",
				"stream":    "stdout",
				"line":      "partial context",
				"node_id":   "Z9yX3b",
				// job_id, job_type, next_id (0) should be absent
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runID := domaintypes.NewRunID()

			if err := hub.PublishLog(ctx, runID, tt.record); err != nil {
				t.Fatalf("publish log: %v", err)
			}

			snapshot := hub.Snapshot(runID)
			if len(snapshot) == 0 {
				t.Fatal("expected event in snapshot")
			}

			evt := snapshot[len(snapshot)-1]
			if evt.Type != domaintypes.SSEEventLog {
				t.Fatalf("expected event type 'log', got %s", evt.Type)
			}

			// Unmarshal into a generic map to check exact JSON shape.
			var got map[string]any
			if err := json.Unmarshal(evt.Data, &got); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}

			// Verify expected keys are present with correct values.
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("field %q: got %v, want %v", k, got[k], v)
				}
			}

			// Verify omitted keys are truly absent.
			omittedKeys := []string{"node_id", "job_id", "job_type"}
			for _, k := range omittedKeys {
				if _, inWant := tt.want[k]; !inWant {
					if _, inGot := got[k]; inGot {
						t.Errorf("field %q should be omitted but was present", k)
					}
				}
			}
		})
	}
}

// TestHubEnrichedLogPayloadSize verifies that enriched log records with maximum
// field sizes are handled correctly. This ensures large frames don't break
// serialization or subscription delivery.
func TestHubEnrichedLogPayloadSize(t *testing.T) {
	t.Parallel()

	hub := NewHub(Options{BufferSize: 8, HistorySize: 16})
	ctx := context.Background()
	runID := domaintypes.NewRunID()
	nodeID := domaintypes.NodeID(domaintypes.NewNodeKey())
	jobID := domaintypes.NewJobID()

	// Create a log record with maximum reasonable field sizes.
	// Real-world logs can have long lines from stack traces, build output, etc.
	longLine := strings.Repeat("X", 8192) // 8KB log line (common for stack traces)

	record := LogRecord{
		Timestamp: "2025-12-01T10:00:00.123456789Z",
		Stream:    "stderr",
		Line:      longLine,
		NodeID:    nodeID,
		JobID:     jobID,
		JobType:   domaintypes.JobTypePreGate,
	}

	// Subscribe before publishing.
	sub, err := hub.Subscribe(ctx, runID, 0)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer sub.Cancel()

	// Publish the large log record.
	if err := hub.PublishLog(ctx, runID, record); err != nil {
		t.Fatalf("publish log: %v", err)
	}

	// Verify subscriber receives the full record.
	select {
	case evt := <-sub.Events:
		if evt.Type != domaintypes.SSEEventLog {
			t.Fatalf("expected event type 'log', got %s", evt.Type)
		}

		// Verify the JSON payload is valid and contains the full line.
		var received LogRecord
		if err := json.Unmarshal(evt.Data, &received); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if received.Line != longLine {
			t.Errorf("line truncated: got %d bytes, want %d", len(received.Line), len(longLine))
		}
		if received.NodeID != record.NodeID {
			t.Errorf("node_id: got %q, want %q", received.NodeID, record.NodeID)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for large payload event")
	}
}
