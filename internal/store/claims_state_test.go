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

// ---------------------------------------------------------------------------
// Shared test helpers (visible to all *_test.go files in package store)
// ---------------------------------------------------------------------------

// newTestStore opens a store against PLOY_TEST_DB_DSN, skipping if unset,
// truncates all tables, and registers cleanup via t.Cleanup.
func newTestStore(t *testing.T) (context.Context, Store) {
	t.Helper()
	dsn := os.Getenv("PLOY_TEST_DB_DSN")
	if dsn == "" {
		t.Skip("PLOY_TEST_DB_DSN not set; skipping integration test")
	}
	ctx := context.Background()
	db, err := NewStore(ctx, dsn)
	if err != nil {
		t.Fatalf("NewStore() failed: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	cleanTestTables(t, ctx, db)
	return ctx, db
}

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

// createTestJob creates a Queued job attached to the fixture's run/repo.
func createTestJob(t *testing.T, ctx context.Context, db Store, fx v1Fixture, name string) Job {
	t.Helper()
	job, err := db.CreateJob(ctx, CreateJobParams{
		ID:          types.NewJobID(),
		RunID:       fx.Run.ID,
		RepoID:      fx.MigRepo.RepoID,
		RepoBaseRef: fx.RunRepo.RepoBaseRef,
		Attempt:     fx.RunRepo.Attempt,
		Name:        name,
		JobType:     "mig",
		JobImage:    "",
		Status:      types.JobStatusQueued,
		NextID:      nil,
		Meta:        []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("createTestJob(%q) failed: %v", name, err)
	}
	return job
}

// createTestNode creates a node with a unique auto-generated ID, name, and IP.
func createTestNode(t *testing.T, ctx context.Context, db Store) Node {
	t.Helper()
	id := types.NodeID(types.NewNodeKey())
	node, err := db.CreateNode(ctx, CreateNodeParams{
		ID:          id,
		Name:        nodeNameForTest(id),
		IpAddress:   nodeAddrForTest(id),
		Concurrency: 1,
	})
	if err != nil {
		t.Fatalf("createTestNode() failed: %v", err)
	}
	return node
}

// nodeNameForTest generates a unique node name from the node ID.
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
	return netip.AddrFrom4([4]byte{100, 64, b[2], b[3]})
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestClaimJob_Basic verifies the happy path: claim a queued job, check that
// status becomes Running, node_id and started_at are set, and a second claim
// with no remaining jobs returns an error.
func TestClaimJob_Basic(t *testing.T) {
	ctx, db := newTestStore(t)
	fx := newV1Fixture(t, ctx, db, "https://github.com/test/repo", "main", "feature", []byte(`{"type":"test"}`))

	if fx.Run.Status != types.RunStatusStarted {
		t.Errorf("expected status Started, got %s", fx.Run.Status)
	}

	job := createTestJob(t, ctx, db, fx, "test-job")
	node := createTestNode(t, ctx, db)

	claimed, err := db.ClaimJob(ctx, node.ID)
	if err != nil {
		t.Fatalf("ClaimJob() failed: %v", err)
	}
	if claimed.ID != job.ID {
		t.Errorf("expected job ID %v, got %v", job.ID, claimed.ID)
	}
	if claimed.Status != types.JobStatusRunning {
		t.Errorf("expected status Running, got %s", claimed.Status)
	}
	if claimed.NodeID == nil || *claimed.NodeID != node.ID {
		t.Errorf("expected node_id %v, got %v", node.ID, claimed.NodeID)
	}
	if !claimed.StartedAt.Valid {
		t.Error("expected started_at to be set")
	}

	if _, err := db.ClaimJob(ctx, node.ID); err == nil {
		t.Error("expected ClaimJob to fail when no queued jobs remain")
	}
}

// TestClaimJob_AllJobsClaimedOnce verifies that N queued jobs are each claimed
// exactly once, regardless of whether one or many nodes do the claiming.
func TestClaimJob_AllJobsClaimedOnce(t *testing.T) {
	ctx, db := newTestStore(t)

	tests := []struct {
		name     string
		numJobs  int
		numNodes int
	}{
		{"one_node_per_job", 3, 3},
		{"single_node_claims_all", 3, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanTestTables(t, ctx, db)
			fx := newV1Fixture(t, ctx, db, "https://github.com/test/"+tt.name, "main", "feature", []byte(`{}`))

			wanted := make(map[types.JobID]bool, tt.numJobs)
			for i := 0; i < tt.numJobs; i++ {
				j := createTestJob(t, ctx, db, fx, "job-"+strconv.Itoa(i))
				wanted[j.ID] = true
			}

			nodes := make([]Node, tt.numNodes)
			for i := range nodes {
				nodes[i] = createTestNode(t, ctx, db)
			}

			claimed := make(map[types.JobID]bool)
			for i := 0; i < tt.numJobs; i++ {
				c, err := db.ClaimJob(ctx, nodes[i%tt.numNodes].ID)
				if err != nil {
					t.Fatalf("ClaimJob() %d failed: %v", i, err)
				}
				if !wanted[c.ID] {
					t.Errorf("ClaimJob() %d returned unexpected job %v", i, c.ID)
				}
				if claimed[c.ID] {
					t.Errorf("ClaimJob() %d returned already-claimed job %v", i, c.ID)
				}
				claimed[c.ID] = true
			}
			if len(claimed) != tt.numJobs {
				t.Errorf("got %d distinct claims, want %d", len(claimed), tt.numJobs)
			}
		})
	}
}

// TestClaimJob_SkipLocked verifies that FOR UPDATE SKIP LOCKED prevents
// concurrent nodes from claiming the same job.
func TestClaimJob_SkipLocked(t *testing.T) {
	ctx, db := newTestStore(t)
	fx := newV1Fixture(t, ctx, db, "https://github.com/test/skip-locked", "main", "concurrent", []byte(`{}`))

	const n = 10
	for i := 0; i < n; i++ {
		createTestJob(t, ctx, db, fx, "job-"+strconv.Itoa(i))
	}

	nodes := make([]Node, n)
	for i := range nodes {
		nodes[i] = createTestNode(t, ctx, db)
	}

	var wg sync.WaitGroup
	claimedJobs := make([]Job, n)
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			claimedJobs[idx], errs[idx] = db.ClaimJob(ctx, nodes[idx].ID)
		}(i)
	}
	wg.Wait()

	validNode := make(map[types.NodeID]bool, n)
	for i := range nodes {
		validNode[nodes[i].ID] = true
	}

	claimedIDs := make(map[types.JobID]bool)
	successes := 0
	for i := 0; i < n; i++ {
		if errs[i] != nil {
			continue
		}
		successes++
		if claimedIDs[claimedJobs[i].ID] {
			t.Errorf("job %v claimed multiple times", claimedJobs[i].ID)
		}
		claimedIDs[claimedJobs[i].ID] = true

		if claimedJobs[i].Status != types.JobStatusRunning {
			t.Errorf("claimed job %v status = %s, want Running", claimedJobs[i].ID, claimedJobs[i].Status)
		}
		if !claimedJobs[i].StartedAt.Valid {
			t.Errorf("claimed job %v missing started_at", claimedJobs[i].ID)
		}
		if claimedJobs[i].NodeID == nil || !validNode[*claimedJobs[i].NodeID] {
			t.Errorf("claimed job %v has unexpected node_id %v", claimedJobs[i].ID, claimedJobs[i].NodeID)
		}
	}
	if successes != n {
		t.Errorf("expected %d successful claims, got %d", n, successes)
	}
	if len(claimedIDs) != n {
		t.Errorf("expected %d unique claimed jobs, got %d", n, len(claimedIDs))
	}
}

// TestClaimJob_NoPendingJobs verifies ClaimJob returns an error when no queued
// jobs exist.
func TestClaimJob_NoPendingJobs(t *testing.T) {
	ctx, db := newTestStore(t)
	node := createTestNode(t, ctx, db)

	if _, err := db.ClaimJob(ctx, node.ID); err == nil {
		t.Error("expected ClaimJob to fail when no queued jobs exist")
	}
}

// TestClaimJob_OnlyPendingJobs verifies ClaimJob skips non-queued jobs.
func TestClaimJob_OnlyPendingJobs(t *testing.T) {
	ctx, db := newTestStore(t)
	fx := newV1Fixture(t, ctx, db, "https://github.com/test/only-pending", "main", "feature", []byte(`{}`))

	// Create a Running job (not claimable).
	_, err := db.CreateJob(ctx, CreateJobParams{
		ID:          types.NewJobID(),
		RunID:       fx.Run.ID,
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

	node := createTestNode(t, ctx, db)
	if _, err := db.ClaimJob(ctx, node.ID); err == nil {
		t.Error("expected ClaimJob to fail when only non-queued jobs exist")
	}
}
