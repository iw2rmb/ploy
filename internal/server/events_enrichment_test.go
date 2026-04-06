package server

// This file contains tests for log enrichment with job metadata.

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	logstream "github.com/iw2rmb/ploy/internal/stream"
	"github.com/jackc/pgx/v5/pgtype"
)

// gzipData compresses data using gzip and returns the compressed bytes.
func gzipData(t *testing.T, data string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	if _, err := gzWriter.Write([]byte(data)); err != nil {
		t.Fatalf("gzip write failed: %v", err)
	}
	if err := gzWriter.Close(); err != nil {
		t.Fatalf("gzip close failed: %v", err)
	}
	return buf.Bytes()
}

// TestStorage_LogEnrichmentWithJobMetadata verifies that log records are
// enriched with execution context (node_id, job_id, job_type) when job
// metadata is available, and published to the job-keyed stream.
func TestStorage_LogEnrichmentWithJobMetadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		jobName string
		jobType string
	}{
		{name: "mig job", jobName: "build-step", jobType: "mig"},
		{name: "pre_gate job", jobName: "pre-gate", jobType: string(domaintypes.JobTypePreGate)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			runID := domaintypes.NewRunID()
			jobID := domaintypes.NewJobID()
			nodeID := domaintypes.NodeID(domaintypes.NewNodeKey())

			logLine := "Build step completed successfully\n"
			gzippedLog := gzipData(t, logLine)

			mock := &mockStore{
				getJobFunc: func(ctx context.Context, id domaintypes.JobID) (store.Job, error) {
					if id != jobID {
						t.Fatalf("GetJob called with unexpected id: got %v, want %v", id, jobID)
					}
					return store.Job{
						ID:      jobID,
						RunID:   runID,
						Name:    tt.jobName,
						JobType: domaintypes.JobType(tt.jobType),
						Meta:    []byte(`{"next_id":2000}`),
						NodeID:  &nodeID,
					}, nil
				},
			}

			svc := newTestEventsService(t, mock)

			ctx := context.Background()
			logRow := store.Log{
				ID:        1,
				RunID:     runID,
				JobID:     &jobID,
				ChunkNo:   1,
				DataSize:  int64(len(gzippedLog)),
				CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
			}

			if err := svc.CreateAndPublishLog(ctx, logRow, gzippedLog); err != nil {
				t.Fatalf("CreateAndPublishLog failed: %v", err)
			}

			snapshot := svc.Hub().SnapshotJob(jobID)
			if len(snapshot) == 0 {
				t.Fatal("expected log event in job hub snapshot, got none")
			}
			if snapshot[0].Type != domaintypes.SSEEventLog {
				t.Fatalf("expected event type 'log', got %s", snapshot[0].Type)
			}

			var rec logstream.LogRecord
			if err := json.Unmarshal(snapshot[0].Data, &rec); err != nil {
				t.Fatalf("failed to unmarshal log record: %v", err)
			}

			if rec.NodeID != nodeID {
				t.Errorf("node_id: got %q, want %q", rec.NodeID, nodeID)
			}
			if rec.JobID != jobID {
				t.Errorf("job_id: got %q, want %q", rec.JobID, jobID)
			}
			if string(rec.JobType) != tt.jobType {
				t.Errorf("job_type: got %q, want %q", rec.JobType, tt.jobType)
			}

			// Verify nothing was published to the run-keyed stream.
			runSnapshot := svc.Hub().Snapshot(runID)
			if len(runSnapshot) != 0 {
				t.Fatalf("expected no events in run hub snapshot, got %d", len(runSnapshot))
			}
		})
	}
}

// TestStorage_LogEnrichmentWithoutJobID verifies that logs without a valid
// job_id skip SSE fanout entirely (job stream requires a job key).
func TestStorage_LogEnrichmentWithoutJobID(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()

	logLine := "System log without job context\n"
	gzippedLog := gzipData(t, logLine)

	getJobCalled := false
	mock := &mockStore{
		getJobFunc: func(ctx context.Context, id domaintypes.JobID) (store.Job, error) {
			getJobCalled = true
			return store.Job{}, nil
		},
	}

	svc := newTestEventsService(t, mock)

	ctx := context.Background()
	logRow := store.Log{
		ID:        1,
		RunID:     runID,
		JobID:     nil,
		ChunkNo:   1,
		DataSize:  int64(len(gzippedLog)),
		CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}

	if err := svc.CreateAndPublishLog(ctx, logRow, gzippedLog); err != nil {
		t.Fatalf("CreateAndPublishLog failed: %v", err)
	}

	if getJobCalled {
		t.Error("GetJob should not be called when JobID is nil")
	}

	runSnapshot := svc.Hub().Snapshot(runID)
	if len(runSnapshot) != 0 {
		t.Fatalf("expected no events on run stream, got %d", len(runSnapshot))
	}
}

// TestStorage_LogEnrichmentJobLookupFailure verifies that logs are still
// published to the job stream even when job metadata lookup fails.
func TestStorage_LogEnrichmentJobLookupFailure(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()

	logLine := "Log with failing job lookup\n"
	gzippedLog := gzipData(t, logLine)

	mock := &mockStore{
		getJobFunc: func(ctx context.Context, id domaintypes.JobID) (store.Job, error) {
			return store.Job{}, context.DeadlineExceeded
		},
	}

	svc := newTestEventsService(t, mock)

	ctx := context.Background()
	logRow := store.Log{
		ID:        1,
		RunID:     runID,
		JobID:     &jobID,
		ChunkNo:   1,
		DataSize:  int64(len(gzippedLog)),
		CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}

	if err := svc.CreateAndPublishLog(ctx, logRow, gzippedLog); err != nil {
		t.Fatalf("CreateAndPublishLog should succeed despite job lookup failure: %v", err)
	}

	snapshot := svc.Hub().SnapshotJob(jobID)
	if len(snapshot) == 0 {
		t.Fatal("expected log event in job hub snapshot, got none")
	}

	var rec logstream.LogRecord
	if err := json.Unmarshal(snapshot[0].Data, &rec); err != nil {
		t.Fatalf("failed to unmarshal log record: %v", err)
	}

	if !rec.NodeID.IsZero() {
		t.Errorf("node_id should be empty after lookup failure, got %q", rec.NodeID)
	}
}

func TestStorage_LogEnrichmentJobContextCacheEvictsLRU(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	nodeID := domaintypes.NodeID(domaintypes.NewNodeKey())
	jobID1 := domaintypes.NewJobID()
	jobID2 := domaintypes.NewJobID()
	jobID3 := domaintypes.NewJobID()

	var (
		mu       sync.Mutex
		getCalls = make(map[domaintypes.JobID]int)
	)
	mock := &mockStore{
		getJobFunc: func(ctx context.Context, id domaintypes.JobID) (store.Job, error) {
			mu.Lock()
			getCalls[id]++
			mu.Unlock()
			return store.Job{
				ID:      id,
				RunID:   runID,
				Name:    "job",
				JobType: "mig",
				NodeID:  &nodeID,
				Meta:    []byte(`{}`),
			}, nil
		},
	}

	svc := newTestEventsService(t, mock, func(o *EventsOptions) {
		o.JobCacheSize = 2
	})

	gzippedLog := gzipData(t, "cached line\n")
	ctx := context.Background()
	publish := func(jobID domaintypes.JobID) {
		t.Helper()
		logRow := store.Log{
			ID:        1,
			RunID:     runID,
			JobID:     &jobID,
			ChunkNo:   1,
			DataSize:  int64(len(gzippedLog)),
			CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
		}
		if err := svc.CreateAndPublishLog(ctx, logRow, gzippedLog); err != nil {
			t.Fatalf("CreateAndPublishLog failed for job %s: %v", jobID, err)
		}
	}

	publish(jobID1)
	publish(jobID1) // cache hit
	publish(jobID2)
	publish(jobID3) // evicts least-recently-used jobID1
	publish(jobID1) // cache miss after eviction

	mu.Lock()
	defer mu.Unlock()
	if getCalls[jobID1] != 2 {
		t.Fatalf("expected GetJob calls for job1 to be 2 after eviction, got %d", getCalls[jobID1])
	}
	if getCalls[jobID2] != 1 {
		t.Fatalf("expected GetJob calls for job2 to be 1, got %d", getCalls[jobID2])
	}
	if getCalls[jobID3] != 1 {
		t.Fatalf("expected GetJob calls for job3 to be 1, got %d", getCalls[jobID3])
	}
}
