package store

import (
	"testing"

	"github.com/iw2rmb/ploy/internal/domain/types"
)

func TestUpsertJobMetric_InsertsAndUpdatesByJobID(t *testing.T) {
	ctx, db := openStoreForCancelBulkTests(t)

	fx := newV1Fixture(t, ctx, db, "https://github.com/test/job-metrics", "main", "feature", []byte(`{"type":"job-metrics"}`))
	job := createJobForStaleRecoveryQueryTest(t, ctx, db, fx.Run.ID, fx.RunRepo.RepoID, fx.RunRepo.RepoBaseRef, 1, "job-metrics", types.JobStatusCreated)

	nodeA := createNodeForStaleRecoveryQueryTest(t, ctx, db)
	nodeB := createNodeForStaleRecoveryQueryTest(t, ctx, db)

	if err := db.UpsertJobMetric(ctx, UpsertJobMetricParams{
		NodeID:            nodeA.ID,
		JobID:             job.ID,
		CpuConsumedNs:     10_000_000,
		DiskConsumedBytes: 1024,
		MemConsumedBytes:  2048,
	}); err != nil {
		t.Fatalf("UpsertJobMetric(insert) failed: %v", err)
	}

	var (
		rowCount      int
		nodeID        string
		cpuConsumed   int64
		diskConsumed  int64
		memoryConsume int64
	)
	if err := db.Pool().QueryRow(
		ctx,
		`SELECT COUNT(*), MIN(node_id)::text, MIN(cpu_consumed_ns), MIN(disk_consumed_bytes), MIN(mem_consumed_bytes)
FROM ploy.job_metrics
WHERE job_id = $1`,
		job.ID,
	).Scan(&rowCount, &nodeID, &cpuConsumed, &diskConsumed, &memoryConsume); err != nil {
		t.Fatalf("query job_metrics after insert failed: %v", err)
	}
	if rowCount != 1 {
		t.Fatalf("job_metrics row count after insert = %d, want 1", rowCount)
	}
	if nodeID != nodeA.ID.String() {
		t.Fatalf("node_id after insert = %q, want %q", nodeID, nodeA.ID)
	}
	if cpuConsumed != 10_000_000 || diskConsumed != 1024 || memoryConsume != 2048 {
		t.Fatalf("unexpected metrics after insert: cpu=%d disk=%d mem=%d", cpuConsumed, diskConsumed, memoryConsume)
	}

	if err := db.UpsertJobMetric(ctx, UpsertJobMetricParams{
		NodeID:            nodeB.ID,
		JobID:             job.ID,
		CpuConsumedNs:     20_000_000,
		DiskConsumedBytes: 4096,
		MemConsumedBytes:  8192,
	}); err != nil {
		t.Fatalf("UpsertJobMetric(update) failed: %v", err)
	}

	if err := db.Pool().QueryRow(
		ctx,
		`SELECT COUNT(*), MIN(node_id)::text, MIN(cpu_consumed_ns), MIN(disk_consumed_bytes), MIN(mem_consumed_bytes)
FROM ploy.job_metrics
WHERE job_id = $1`,
		job.ID,
	).Scan(&rowCount, &nodeID, &cpuConsumed, &diskConsumed, &memoryConsume); err != nil {
		t.Fatalf("query job_metrics after update failed: %v", err)
	}
	if rowCount != 1 {
		t.Fatalf("job_metrics row count after update = %d, want 1", rowCount)
	}
	if nodeID != nodeB.ID.String() {
		t.Fatalf("node_id after update = %q, want %q", nodeID, nodeB.ID)
	}
	if cpuConsumed != 20_000_000 || diskConsumed != 4096 || memoryConsume != 8192 {
		t.Fatalf("unexpected metrics after update: cpu=%d disk=%d mem=%d", cpuConsumed, diskConsumed, memoryConsume)
	}
}
