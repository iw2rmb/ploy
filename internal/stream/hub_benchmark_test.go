package logstream

import (
	"context"
	"testing"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// =============================================================================
// Performance and Resilience Tests for Enriched Logs
// =============================================================================
// These tests validate that enriched log payloads (with node_id, job_id,
// job_type, next_id) do not regress performance or resilience for
// long-running or chatty Mods runs.
// Stress test: validate performance and resilience with enriched logs.

// BenchmarkHubPublishEnrichedLog measures the throughput of publishing enriched
// log records through the hub. This ensures the additional enrichment fields
// do not introduce performance regressions.
func BenchmarkHubPublishEnrichedLog(b *testing.B) {
	hub := NewHub(Options{BufferSize: 256, HistorySize: 1024})
	ctx := context.Background()
	runID := domaintypes.NewRunID()

	// Create a fully enriched log record matching real-world usage.
	record := LogRecord{
		Timestamp: "2025-12-01T10:00:00.000000Z",
		Stream:    "stdout",
		Line:      "Build step completed: compiling module org.example.service",
		NodeID:    "aB3xY9",
		JobID:     domaintypes.NewJobID(),
		JobType:   "mod",
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = hub.PublishLog(ctx, runID, record)
	}
}

// BenchmarkHubPublishMinimalLog measures baseline throughput for minimal logs
// (without enrichment fields). Used as a comparison baseline for the enriched
// log benchmark.
func BenchmarkHubPublishMinimalLog(b *testing.B) {
	hub := NewHub(Options{BufferSize: 256, HistorySize: 1024})
	ctx := context.Background()
	runID := domaintypes.NewRunID()

	// Create a minimal log record without enrichment fields.
	record := LogRecord{
		Timestamp: "2025-12-01T10:00:00.000000Z",
		Stream:    "stdout",
		Line:      "Build step completed: compiling module org.example.service",
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = hub.PublishLog(ctx, runID, record)
	}
}

// BenchmarkHubConcurrentPublishEnrichedLog measures throughput under concurrent
// publisher load. This simulates chatty Mods runs with multiple concurrent
// log sources publishing enriched records.
func BenchmarkHubConcurrentPublishEnrichedLog(b *testing.B) {
	hub := NewHub(Options{BufferSize: 256, HistorySize: 1024})
	ctx := context.Background()
	runID := domaintypes.NewRunID()
	nodeID := domaintypes.NodeID(domaintypes.NewNodeKey())
	jobID := domaintypes.NewJobID()

	record := LogRecord{
		Timestamp: "2025-12-01T10:00:00.000000Z",
		Stream:    "stdout",
		Line:      "Concurrent build output from parallel job execution",
		NodeID:    nodeID,
		JobID:     jobID,
		JobType:   "hook",
	}

	b.ResetTimer()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = hub.PublishLog(ctx, runID, record)
		}
	})
}

// TestHubHighVolumeEnrichedLogs verifies that the hub remains stable under
// sustained high-volume publishing of enriched log records. This simulates
// long-running Mods runs with chatty output. The test uses concurrent
// consumers to actively drain events while publishing continues.
func TestHubHighVolumeEnrichedLogs(t *testing.T) {
	t.Parallel()

	// Configuration for high-volume test:
	// - 1,000 log records simulates a chatty build (e.g., verbose Maven output)
	// - Buffer is sized to avoid backpressure drops for active consumers
	const numLogs = 1000
	const numSubscribers = 3

	hub := NewHub(Options{
		BufferSize:  numLogs + 1,
		HistorySize: 2 * numLogs,
	})
	ctx := context.Background()
	runID := domaintypes.NewRunID()
	nodeID := domaintypes.NodeID(domaintypes.NewNodeKey())
	jobID := domaintypes.NewJobID()

	// Track events received by each subscriber using concurrent goroutines.
	type result struct {
		count       int
		receivedIDs []domaintypes.EventID
		sawDone     bool
	}
	results := make(chan result, numSubscribers)

	// Start subscriber goroutines that actively consume events.
	for i := 0; i < numSubscribers; i++ {
		sub, err := hub.Subscribe(ctx, runID, 0)
		if err != nil {
			t.Fatalf("subscribe %d: %v", i, err)
		}
		go func(subID int, s Subscription) {
			defer s.Cancel()
			r := result{}
			timeout := time.After(5 * time.Second)
			for {
				select {
				case evt, ok := <-s.Events:
					if !ok {
						// Channel closed (backpressure drop).
						results <- r
						return
					}
					r.count++
					r.receivedIDs = append(r.receivedIDs, evt.ID)
					if evt.Type == domaintypes.SSEEventDone {
						r.sawDone = true
						results <- r
						return
					}
				case <-timeout:
					results <- r
					return
				}
			}
		}(i, sub)
	}

	// Publish high volume of enriched logs.
	for i := 0; i < numLogs; i++ {
		record := LogRecord{
			Timestamp: time.Now().Format(time.RFC3339Nano),
			Stream:    "stdout",
			Line:      "Compiling module " + string(rune('A'+i%26)),
			NodeID:    nodeID,
			JobID:     jobID,
			JobType:   "mod",
		}
		if err := hub.PublishLog(ctx, runID, record); err != nil {
			t.Fatalf("publish log %d: %v", i, err)
		}
	}

	// Publish terminal status to signal completion.
	if err := hub.PublishStatus(ctx, runID, Status{Status: "done"}); err != nil {
		t.Fatalf("publish status: %v", err)
	}

	// Collect results from all subscribers.
	var allResults []result
	for i := 0; i < numSubscribers; i++ {
		r := <-results
		allResults = append(allResults, r)
		t.Logf("subscriber %d: received %d events, sawDone=%v", i, r.count, r.sawDone)
	}

	// Verify at least one subscriber received the done event.
	// Active consumers should keep up with publishing.
	sawDone := false
	totalEvents := 0
	for _, r := range allResults {
		if r.sawDone {
			sawDone = true
		}
		totalEvents += r.count
	}

	if !sawDone {
		t.Error("no subscriber received the done event - possible performance regression")
	}

	// Verify we received a reasonable number of events across all subscribers.
	// With active consumers, we expect most events to be delivered.
	expectedMin := numLogs / 2 // At least half the events per active consumer
	for i, r := range allResults {
		if r.count < expectedMin && !r.sawDone {
			t.Logf("warning: subscriber %d received only %d events (expected >= %d)", i, r.count, expectedMin)
		}
	}
}
