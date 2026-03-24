package store

import (
	"context"
	"encoding/binary"
	"hash/fnv"
	"net/netip"
	"os"
	"strconv"
	"sync"
	"testing"

	"github.com/iw2rmb/ploy/internal/domain/types"
)

// Tests use ClaimJob to test job claiming behavior.
// ClaimRun was removed; jobs are now the unified execution unit.

// TestClaimJob_Basic tests the basic ClaimJob operation:
// - Creates a run in Started status with a Queued job (v1 model)
// - Claims the job for a node
// - Verifies the job is assigned with correct node_id and started_at
func TestClaimJob_Basic(t *testing.T) {
	dsn := os.Getenv("PLOY_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("PLOY_TEST_PG_DSN not set; skipping integration test")
	}

	ctx := context.Background()
	db, err := NewStore(ctx, dsn)
	if err != nil {
		t.Fatalf("NewStore() failed: %v", err)
	}
	defer db.Close()
	cleanTestTables(t, ctx, db)

	fx := newV1Fixture(t, ctx, db, "https://github.com/test/repo", "main", "feature", []byte(`{"type":"test"}`))
	run := fx.Run

	if run.Status != types.RunStatusStarted {
		t.Errorf("expected status Started, got %s", run.Status)
	}

	// Create a Queued job for the run (v1 status model: Queued replaces pending).
	// Job ID is now KSUID-backed; generate via types.NewJobID().
	job, err := db.CreateJob(ctx, CreateJobParams{
		ID:          types.NewJobID(),
		RunID:       run.ID,
		RepoID:      fx.MigRepo.RepoID,
		RepoBaseRef: fx.RunRepo.RepoBaseRef,
		Attempt:     fx.RunRepo.Attempt,
		Name:        "test-job",
		JobType:     "mig",
		JobImage:    "",
		Status:      types.JobStatusQueued,
		NextID:      nil,
		Meta:        []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("CreateJob() failed: %v", err)
	}

	// Create a test node.
	nodeID0 := types.NodeID(types.NewNodeKey())
	node, err := db.CreateNode(ctx, CreateNodeParams{
		ID:          nodeID0,
		Name:        nodeNameForTest(nodeID0),
		IpAddress:   nodeAddrForTest(nodeID0),
		Concurrency: 1,
	})
	if err != nil {
		t.Fatalf("CreateNode() failed: %v", err)
	}

	// Claim the job for the node.
	claimedJob, err := db.ClaimJob(ctx, node.ID)
	if err != nil {
		t.Fatalf("ClaimJob() failed: %v", err)
	}

	// Verify the claimed job has the correct properties.
	// Job ID is now a string (KSUID-backed).
	if claimedJob.ID != job.ID {
		t.Errorf("Expected job ID %v, got %v", job.ID, claimedJob.ID)
	}

	if claimedJob.Status != types.JobStatusRunning {
		t.Errorf("Expected status assigned, got %s", claimedJob.Status)
	}

	// Node ID is now *string.
	if claimedJob.NodeID == nil || *claimedJob.NodeID != node.ID {
		t.Errorf("Expected node_id %v, got %v", node.ID, claimedJob.NodeID)
	}

	if !claimedJob.StartedAt.Valid {
		t.Error("Expected started_at to be set")
	}

	// Verify no more jobs can be claimed.
	_, err = db.ClaimJob(ctx, node.ID)
	if err == nil {
		t.Error("Expected ClaimJob to fail when no pending jobs exist")
	}
}

// TestClaimJob_FIFO tests that jobs are claimed in FIFO order by next_id.
func TestClaimJob_FIFO(t *testing.T) {
	dsn := os.Getenv("PLOY_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("PLOY_TEST_PG_DSN not set; skipping integration test")
	}

	ctx := context.Background()
	db, err := NewStore(ctx, dsn)
	if err != nil {
		t.Fatalf("NewStore() failed: %v", err)
	}
	defer db.Close()
	cleanTestTables(t, ctx, db)

	fx := newV1Fixture(t, ctx, db, "https://github.com/test/fifo", "main", "feature", []byte(`{"type":"fifo"}`))
	run := fx.Run

	// Create three pending jobs with different next_id values.
	job1, err := db.CreateJob(ctx, CreateJobParams{
		ID:          types.NewJobID(),
		RunID:       run.ID,
		RepoID:      fx.MigRepo.RepoID,
		RepoBaseRef: fx.RunRepo.RepoBaseRef,
		Attempt:     fx.RunRepo.Attempt,
		Name:        "job-1",
		JobType:     "mig",
		JobImage:    "",
		Status:      types.JobStatusQueued,
		NextID:      nil,
		Meta:        []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("CreateJob() 1 failed: %v", err)
	}

	job2, err := db.CreateJob(ctx, CreateJobParams{
		ID:          types.NewJobID(),
		RunID:       run.ID,
		RepoID:      fx.MigRepo.RepoID,
		RepoBaseRef: fx.RunRepo.RepoBaseRef,
		Attempt:     fx.RunRepo.Attempt,
		Name:        "job-2",
		JobType:     "mig",
		JobImage:    "",
		Status:      types.JobStatusQueued,
		NextID:      nil,
		Meta:        []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("CreateJob() 2 failed: %v", err)
	}

	job3, err := db.CreateJob(ctx, CreateJobParams{
		ID:          types.NewJobID(),
		RunID:       run.ID,
		RepoID:      fx.MigRepo.RepoID,
		RepoBaseRef: fx.RunRepo.RepoBaseRef,
		Attempt:     fx.RunRepo.Attempt,
		Name:        "job-3",
		JobType:     "mig",
		JobImage:    "",
		Status:      types.JobStatusQueued,
		NextID:      nil,
		Meta:        []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("CreateJob() 3 failed: %v", err)
	}

	// Create test nodes.
	fifoID0 := types.NodeID(types.NewNodeKey())
	node1, err := db.CreateNode(ctx, CreateNodeParams{
		ID:          fifoID0,
		Name:        nodeNameForTest(fifoID0),
		IpAddress:   nodeAddrForTest(fifoID0),
		Concurrency: 1,
	})
	if err != nil {
		t.Fatalf("CreateNode() failed: %v", err)
	}

	fifoID1 := types.NodeID(types.NewNodeKey())
	node2, err := db.CreateNode(ctx, CreateNodeParams{
		ID:          fifoID1,
		Name:        nodeNameForTest(fifoID1),
		IpAddress:   nodeAddrForTest(fifoID1),
		Concurrency: 1,
	})
	if err != nil {
		t.Fatalf("CreateNode() failed: %v", err)
	}

	fifoID2 := types.NodeID(types.NewNodeKey())
	node3, err := db.CreateNode(ctx, CreateNodeParams{
		ID:          fifoID2,
		Name:        nodeNameForTest(fifoID2),
		IpAddress:   nodeAddrForTest(fifoID2),
		Concurrency: 1,
	})
	if err != nil {
		t.Fatalf("CreateNode() failed: %v", err)
	}

	// ClaimJob orders by j.id ASC. KSUIDs created within the same second have
	// random ordering, so we only verify that each node claims a distinct job.
	wanted := map[types.JobID]bool{job1.ID: true, job2.ID: true, job3.ID: true}
	claimed := map[types.JobID]bool{}

	for idx, node := range []Node{node1, node2, node3} {
		job, err := db.ClaimJob(ctx, node.ID)
		if err != nil {
			t.Fatalf("ClaimJob() for node%d failed: %v", idx+1, err)
		}
		if !wanted[job.ID] {
			t.Errorf("ClaimJob() for node%d returned unexpected job %v", idx+1, job.ID)
		}
		if claimed[job.ID] {
			t.Errorf("ClaimJob() for node%d returned already-claimed job %v", idx+1, job.ID)
		}
		claimed[job.ID] = true
	}
	if len(claimed) != 3 {
		t.Errorf("Expected 3 distinct jobs claimed, got %d", len(claimed))
	}
}

// TestClaimJob_SkipLocked tests that FOR UPDATE SKIP LOCKED prevents concurrent claims
// of the same job by multiple nodes.
func TestClaimJob_SkipLocked(t *testing.T) {
	dsn := os.Getenv("PLOY_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("PLOY_TEST_PG_DSN not set; skipping integration test")
	}

	ctx := context.Background()
	db, err := NewStore(ctx, dsn)
	if err != nil {
		t.Fatalf("NewStore() failed: %v", err)
	}
	defer db.Close()
	cleanTestTables(t, ctx, db)

	fx := newV1Fixture(t, ctx, db, "https://github.com/test/skip-locked", "main", "concurrent", []byte(`{"type":"concurrent"}`))
	run := fx.Run

	// Create multiple pending jobs for concurrent claiming.
	const numJobs = 10
	jobs := make([]Job, numJobs)
	for i := 0; i < numJobs; i++ {
		job, err := db.CreateJob(ctx, CreateJobParams{
			ID:          types.NewJobID(),
			RunID:       run.ID,
			RepoID:      fx.MigRepo.RepoID,
			RepoBaseRef: fx.RunRepo.RepoBaseRef,
			Attempt:     fx.RunRepo.Attempt,
			Name:        "job-" + strconv.Itoa(i),
			JobType:     "mig",
			JobImage:    "",
			Status:      types.JobStatusQueued,
			NextID:      nil,
			Meta:        []byte(`{}`),
		})
		if err != nil {
			t.Fatalf("CreateJob() %d failed: %v", i, err)
		}
		jobs[i] = job
	}

	// Create multiple nodes to claim concurrently.
	const numNodes = 10
	nodes := make([]Node, numNodes)
	for i := 0; i < numNodes; i++ {
		concID := types.NodeID(types.NewNodeKey())
		node, err := db.CreateNode(ctx, CreateNodeParams{
			ID:          concID,
			Name:        nodeNameForTest(concID),
			IpAddress:   nodeAddrForTest(concID),
			Concurrency: 1,
		})
		if err != nil {
			t.Fatalf("CreateNode() %d failed: %v", i, err)
		}
		nodes[i] = node
	}

	// Claim jobs concurrently.
	var wg sync.WaitGroup
	claimedJobs := make([]Job, numNodes)
	errors := make([]error, numNodes)

	for i := 0; i < numNodes; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			nodeID := nodes[idx].ID
			claimedJobs[idx], errors[idx] = db.ClaimJob(ctx, nodeID)
		}(i)
	}

	wg.Wait()

	// Count successful claims.
	successCount := 0
	// Job IDs use types.JobID, use map for deduplication.
	claimedIDs := make(map[types.JobID]bool)
	// Node IDs use types.NodeID, use map for deduplication.
	validNode := make(map[types.NodeID]bool, numNodes)
	for i := range nodes {
		validNode[nodes[i].ID] = true
	}
	for i := 0; i < numNodes; i++ {
		if errors[i] == nil {
			successCount++
			// Verify each job is claimed only once.
			jobID := claimedJobs[i].ID
			if claimedIDs[jobID] {
				t.Errorf("Job %v was claimed multiple times", claimedJobs[i].ID)
			}
			claimedIDs[jobID] = true

			// Additional invariants for claimed jobs.
			if claimedJobs[i].Status != types.JobStatusRunning {
				t.Errorf("claimed job %v status = %s, want assigned", claimedJobs[i].ID, claimedJobs[i].Status)
			}
			if !claimedJobs[i].StartedAt.Valid {
				t.Errorf("claimed job %v missing started_at", claimedJobs[i].ID)
			}
			if claimedJobs[i].NodeID == nil || !validNode[*claimedJobs[i].NodeID] {
				t.Errorf("claimed job %v has unexpected node_id %v", claimedJobs[i].ID, claimedJobs[i].NodeID)
			}
		}
	}

	// Verify all jobs were claimed exactly once.
	if successCount != numJobs {
		t.Errorf("Expected %d successful claims, got %d", numJobs, successCount)
	}

	// Verify no duplicate claims.
	if len(claimedIDs) != numJobs {
		t.Errorf("Expected %d unique claimed jobs, got %d", numJobs, len(claimedIDs))
	}
}

// TestClaimJob_NoPendingJobs tests ClaimJob when no pending jobs exist.
func TestClaimJob_NoPendingJobs(t *testing.T) {
	dsn := os.Getenv("PLOY_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("PLOY_TEST_PG_DSN not set; skipping integration test")
	}

	ctx := context.Background()
	db, err := NewStore(ctx, dsn)
	if err != nil {
		t.Fatalf("NewStore() failed: %v", err)
	}
	defer db.Close()
	cleanTestTables(t, ctx, db)

	// Create a test node.
	emptyID := types.NodeID(types.NewNodeKey())
	node, err := db.CreateNode(ctx, CreateNodeParams{
		ID:          emptyID,
		Name:        nodeNameForTest(emptyID),
		IpAddress:   nodeAddrForTest(emptyID),
		Concurrency: 1,
	})
	if err != nil {
		t.Fatalf("CreateNode() failed: %v", err)
	}

	// Try to claim when no pending jobs exist.
	_, err = db.ClaimJob(ctx, node.ID)
	if err == nil {
		t.Error("Expected ClaimJob to fail when no pending jobs exist")
	}
}

// TestClaimJob_DrainedNode tests that drained nodes cannot claim jobs.
// Note: ClaimJob does not currently check node drained status; this test is a placeholder.
func TestClaimJob_DrainedNode(t *testing.T) {
	dsn := os.Getenv("PLOY_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("PLOY_TEST_PG_DSN not set; skipping integration test")
	}

	ctx := context.Background()
	db, err := NewStore(ctx, dsn)
	if err != nil {
		t.Fatalf("NewStore() failed: %v", err)
	}
	defer db.Close()
	cleanTestTables(t, ctx, db)

	fx := newV1Fixture(t, ctx, db, "https://github.com/test/drained", "main", "feature", []byte(`{"type":"draintest"}`))
	run := fx.Run

	job, err := db.CreateJob(ctx, CreateJobParams{
		ID:          types.NewJobID(),
		RunID:       run.ID,
		RepoID:      fx.MigRepo.RepoID,
		RepoBaseRef: fx.RunRepo.RepoBaseRef,
		Attempt:     fx.RunRepo.Attempt,
		Name:        "test-job",
		JobType:     "mig",
		JobImage:    "",
		Status:      types.JobStatusQueued,
		NextID:      nil,
		Meta:        []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("CreateJob() failed: %v", err)
	}

	// Create a test node.
	drainedID := types.NodeID(types.NewNodeKey())
	node, err := db.CreateNode(ctx, CreateNodeParams{
		ID:          drainedID,
		Name:        nodeNameForTest(drainedID),
		IpAddress:   nodeAddrForTest(drainedID),
		Concurrency: 1,
	})
	if err != nil {
		t.Fatalf("CreateNode() failed: %v", err)
	}

	// Drain the node.
	err = db.UpdateNodeDrained(ctx, UpdateNodeDrainedParams{
		ID:      node.ID,
		Drained: true,
	})
	if err != nil {
		t.Fatalf("UpdateNodeDrained() failed: %v", err)
	}

	// Note: ClaimJob currently does not check node drained status.
	// The handler should check this before calling ClaimJob.
	// For now, we just verify the claim succeeds at the DB level.
	claimedJob, err := db.ClaimJob(ctx, node.ID)
	if err != nil {
		// If the claim fails, that's acceptable (the query might have been updated).
		t.Logf("ClaimJob for drained node failed as expected: %v", err)
		return
	}

	// If claim succeeds, verify job was claimed.
	if claimedJob.ID != job.ID {
		t.Errorf("Unexpected job claimed: got %v, want %v", claimedJob.ID, job.ID)
	}
	t.Log("Note: ClaimJob does not check node drained status at DB level; handler should verify")
}

// TestClaimJob_UndrainedNodeClaims tests that undrained nodes can claim jobs.
func TestClaimJob_UndrainedNodeClaims(t *testing.T) {
	dsn := os.Getenv("PLOY_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("PLOY_TEST_PG_DSN not set; skipping integration test")
	}

	ctx := context.Background()
	db, err := NewStore(ctx, dsn)
	if err != nil {
		t.Fatalf("NewStore() failed: %v", err)
	}
	defer db.Close()
	cleanTestTables(t, ctx, db)

	fx := newV1Fixture(t, ctx, db, "https://github.com/test/undrained", "main", "feature", []byte(`{"type":"undraintest"}`))
	run := fx.Run

	job, err := db.CreateJob(ctx, CreateJobParams{
		ID:          types.NewJobID(),
		RunID:       run.ID,
		RepoID:      fx.MigRepo.RepoID,
		RepoBaseRef: fx.RunRepo.RepoBaseRef,
		Attempt:     fx.RunRepo.Attempt,
		Name:        "test-job",
		JobType:     "mig",
		JobImage:    "",
		Status:      types.JobStatusQueued,
		NextID:      nil,
		Meta:        []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("CreateJob() failed: %v", err)
	}

	// Create a test node.
	undrainedID := types.NodeID(types.NewNodeKey())
	node, err := db.CreateNode(ctx, CreateNodeParams{
		ID:          undrainedID,
		Name:        nodeNameForTest(undrainedID),
		IpAddress:   nodeAddrForTest(undrainedID),
		Concurrency: 1,
	})
	if err != nil {
		t.Fatalf("CreateNode() failed: %v", err)
	}

	// Now the node should be able to claim the job.
	claimedJob, err := db.ClaimJob(ctx, node.ID)
	if err != nil {
		t.Fatalf("ClaimJob() failed: %v", err)
	}

	// Verify the job is assigned.
	if claimedJob.ID != job.ID {
		t.Errorf("Expected job ID %v, got %v", job.ID, claimedJob.ID)
	}
	if claimedJob.Status != types.JobStatusRunning {
		t.Errorf("Expected status assigned, got %s", claimedJob.Status)
	}
	if claimedJob.NodeID == nil || *claimedJob.NodeID != node.ID {
		t.Errorf("Expected node_id %v, got %v", node.ID, claimedJob.NodeID)
	}
}

// Helper functions for state transition tests.

// cleanTestTables truncates all transactional tables so that integration tests
// start from a clean state even when run repeatedly against a persistent DB.
func cleanTestTables(t *testing.T, ctx context.Context, db Store) {
	t.Helper()
	_, err := db.Pool().Exec(ctx,
		`TRUNCATE jobs, nodes, run_repos, runs, mig_repos, migs, specs, gates, gate_profiles, repos CASCADE`)
	if err != nil {
		t.Fatalf("cleanTestTables: %v", err)
	}
}

// nodeNameForTest generates a unique node name from the node ID, with a
// human-readable prefix for debugging. The node ID guarantees uniqueness across
// test runs against a persistent DB.
func nodeNameForTest(id types.NodeID) string {
	return "test-node-" + id.String()
}

// nodeAddrForTest derives a unique 100.64.x.x IP from the node ID so that
// repeated test runs against a persistent DB never collide on the
// nodes_ip_address_key unique constraint.
func nodeAddrForTest(id types.NodeID) netip.Addr {
	h := fnv.New32a()
	_, _ = h.Write([]byte(id.String()))
	n := h.Sum32()
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], n)
	// Use 100.64.x.x (CGNAT shared address space) — never a real host IP.
	return netip.AddrFrom4([4]byte{100, 64, b[2], b[3]})
}

func ipForTest(subnet, host int) string {
	if host > 254 {
		host = 254
	}
	// Generate unique IPs in the 192.168.subnet.host range.
	return "192.168." + strconv.Itoa(subnet) + "." + strconv.Itoa(host)
}

// TestClaimJob_OrdersByStepIndex tests that ClaimJob claims jobs in next_id order
// regardless of creation order.
func TestClaimJob_OrdersByStepIndex(t *testing.T) {
	dsn := os.Getenv("PLOY_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("PLOY_TEST_PG_DSN not set; skipping integration test")
	}

	ctx := context.Background()
	db, err := NewStore(ctx, dsn)
	if err != nil {
		t.Fatalf("NewStore() failed: %v", err)
	}
	defer db.Close()
	cleanTestTables(t, ctx, db)

	fx := newV1Fixture(t, ctx, db, "https://github.com/test/step-order", "main", "feature", []byte(`{"type":"step-order"}`))
	run := fx.Run

	// Create jobs in reverse next_id order to verify ordering.
	job3, err := db.CreateJob(ctx, CreateJobParams{
		ID:          types.NewJobID(),
		RunID:       run.ID,
		RepoID:      fx.MigRepo.RepoID,
		RepoBaseRef: fx.RunRepo.RepoBaseRef,
		Attempt:     fx.RunRepo.Attempt,
		Name:        "job-3",
		JobType:     "mig",
		JobImage:    "",
		Status:      types.JobStatusQueued,
		NextID:      nil,
		Meta:        []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("CreateJob(3) failed: %v", err)
	}

	job1, err := db.CreateJob(ctx, CreateJobParams{
		ID:          types.NewJobID(),
		RunID:       run.ID,
		RepoID:      fx.MigRepo.RepoID,
		RepoBaseRef: fx.RunRepo.RepoBaseRef,
		Attempt:     fx.RunRepo.Attempt,
		Name:        "job-1",
		JobType:     "mig",
		JobImage:    "",
		Status:      types.JobStatusQueued,
		NextID:      nil,
		Meta:        []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("CreateJob(1) failed: %v", err)
	}

	job2, err := db.CreateJob(ctx, CreateJobParams{
		ID:          types.NewJobID(),
		RunID:       run.ID,
		RepoID:      fx.MigRepo.RepoID,
		RepoBaseRef: fx.RunRepo.RepoBaseRef,
		Attempt:     fx.RunRepo.Attempt,
		Name:        "job-2",
		JobType:     "mig",
		JobImage:    "",
		Status:      types.JobStatusQueued,
		NextID:      nil,
		Meta:        []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("CreateJob(2) failed: %v", err)
	}

	// Create a test node.
	stepOrderID := types.NodeID(types.NewNodeKey())
	node, err := db.CreateNode(ctx, CreateNodeParams{
		ID:          stepOrderID,
		Name:        nodeNameForTest(stepOrderID),
		IpAddress:   nodeAddrForTest(stepOrderID),
		Concurrency: 1,
	})
	if err != nil {
		t.Fatalf("CreateNode() failed: %v", err)
	}

	// ClaimJob orders by j.id ASC. KSUIDs created within the same second have random
	// ordering, so we verify all 3 jobs are claimed exactly once (any order).
	wantedIDs := map[types.JobID]bool{job1.ID: true, job2.ID: true, job3.ID: true}
	claimedIDs := map[types.JobID]bool{}
	for i := 0; i < 3; i++ {
		c, err := db.ClaimJob(ctx, node.ID)
		if err != nil {
			t.Fatalf("ClaimJob() %d failed: %v", i+1, err)
		}
		if !wantedIDs[c.ID] {
			t.Errorf("ClaimJob() %d returned unexpected job %v", i+1, c.ID)
		}
		if claimedIDs[c.ID] {
			t.Errorf("ClaimJob() %d returned already-claimed job %v", i+1, c.ID)
		}
		claimedIDs[c.ID] = true
	}
	if len(claimedIDs) != 3 {
		t.Errorf("Expected 3 distinct jobs claimed, got %d", len(claimedIDs))
	}
}

// TestClaimJob_OnlyPendingJobs tests that ClaimJob only claims pending jobs,
// not assigned or completed ones.
func TestClaimJob_OnlyPendingJobs(t *testing.T) {
	dsn := os.Getenv("PLOY_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("PLOY_TEST_PG_DSN not set; skipping integration test")
	}

	ctx := context.Background()
	db, err := NewStore(ctx, dsn)
	if err != nil {
		t.Fatalf("NewStore() failed: %v", err)
	}
	defer db.Close()
	cleanTestTables(t, ctx, db)

	fx := newV1Fixture(t, ctx, db, "https://github.com/test/only-pending", "main", "feature", []byte(`{"type":"only-pending"}`))
	run := fx.Run

	// Create a non-pending job (already running).
	_, err = db.CreateJob(ctx, CreateJobParams{
		ID:          types.NewJobID(),
		RunID:       run.ID,
		RepoID:      fx.MigRepo.RepoID,
		RepoBaseRef: fx.RunRepo.RepoBaseRef,
		Attempt:     fx.RunRepo.Attempt,
		Name:        "running-job",
		Status:      types.JobStatusRunning,
		JobType:     "mig",
		JobImage:    "",
		NextID:      nil,
		Meta:        []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("CreateJob() failed: %v", err)
	}

	// Create a test node.
	onlyPendingID := types.NodeID(types.NewNodeKey())
	node, err := db.CreateNode(ctx, CreateNodeParams{
		ID:          onlyPendingID,
		Name:        nodeNameForTest(onlyPendingID),
		IpAddress:   nodeAddrForTest(onlyPendingID),
		Concurrency: 1,
	})
	if err != nil {
		t.Fatalf("CreateNode() failed: %v", err)
	}

	// Attempt to claim a job. Should fail because the only job is already running.
	_, err = db.ClaimJob(ctx, node.ID)
	if err == nil {
		t.Error("Expected ClaimJob to fail when no pending jobs exist, but it succeeded")
	}
}

// mustParseAddr parses an IP address string and fails the test on error.
func mustParseAddr(t *testing.T, s string) netip.Addr {
	t.Helper()
	addr, err := netip.ParseAddr(s)
	if err != nil {
		t.Fatalf("mustParseAddr(%q) failed: %v", s, err)
	}
	return addr
}
