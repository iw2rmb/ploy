package store

import (
	"context"
	"net/netip"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/domain/types"
)

// TestClaimJobLocksJobOnly verifies that ClaimJob uses FOR UPDATE OF j (jobs only)
// and does not lock the runs table. When concurrent claims target jobs from
// different runs, they should not block on each other.
//
// This test addresses roadmap/refactor/store.md:
//
//	"ClaimJob uses FOR UPDATE SKIP LOCKED with a join to runs; it may lock runs
//	rows unnecessarily."
//
// The fix uses FOR UPDATE OF j SKIP LOCKED to lock only the jobs table.
//
// Requires PLOY_TEST_PG_DSN to be set with a test database.
func TestClaimJobLocksJobOnly(t *testing.T) {
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

	if db.Pool().Config().MaxConns < 2 {
		t.Skipf("pgxpool max_conns=%d; need >=2 to exercise concurrent claims", db.Pool().Config().MaxConns)
	}

	// Create two separate runs with jobs.
	fx1 := newV1Fixture(t, ctx, db, "https://github.com/test/lock-scope-1", "main", "feature", []byte(`{"type":"lock-scope-1"}`))
	fx2 := newV1Fixture(t, ctx, db, "https://github.com/test/lock-scope-2", "main", "feature", []byte(`{"type":"lock-scope-2"}`))

	// Create jobs in both runs (Queued status).
	job1ID := types.NewJobID()
	_, err = db.CreateJob(ctx, CreateJobParams{
		ID:          job1ID,
		RunID:       fx1.Run.ID,
		RepoID:      fx1.ModRepo.ID,
		RepoBaseRef: fx1.RunRepo.RepoBaseRef,
		Attempt:     fx1.RunRepo.Attempt,
		Name:        "job-lock-1",
		ModType:     "",
		ModImage:    "",
		Status:      JobStatusQueued,
		StepIndex:   types.StepIndex(1000),
		Meta:        []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("CreateJob(run1) failed: %v", err)
	}

	job2ID := types.NewJobID()
	_, err = db.CreateJob(ctx, CreateJobParams{
		ID:          job2ID,
		RunID:       fx2.Run.ID,
		RepoID:      fx2.ModRepo.ID,
		RepoBaseRef: fx2.RunRepo.RepoBaseRef,
		Attempt:     fx2.RunRepo.Attempt,
		Name:        "job-lock-2",
		ModType:     "",
		ModImage:    "",
		Status:      JobStatusQueued,
		StepIndex:   types.StepIndex(1000),
		Meta:        []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("CreateJob(run2) failed: %v", err)
	}

	// Create two nodes for claiming.
	node1, err := db.CreateNode(ctx, CreateNodeParams{
		ID:          types.NodeID(types.NewNodeKey()),
		Name:        "test-node-lock-1",
		IpAddress:   mustParseLockAddr(t, "192.168.100.1"),
		Concurrency: 1,
	})
	if err != nil {
		t.Fatalf("CreateNode(node1) failed: %v", err)
	}

	node2, err := db.CreateNode(ctx, CreateNodeParams{
		ID:          types.NodeID(types.NewNodeKey()),
		Name:        "test-node-lock-2",
		IpAddress:   mustParseLockAddr(t, "192.168.100.2"),
		Concurrency: 1,
	})
	if err != nil {
		t.Fatalf("CreateNode(node2) failed: %v", err)
	}

	// Test: Start concurrent claims from two different runs.
	// If runs table is locked, the second claim would block until the first completes.
	// With FOR UPDATE OF j, both claims should proceed independently.

	var wg sync.WaitGroup
	results := make(chan claimResult, 2)

	wg.Add(2)

	// First goroutine claims from run1
	go func() {
		defer wg.Done()
		start := time.Now()
		job, err := db.ClaimJob(ctx, node1.ID)
		results <- claimResult{
			jobID:    job.ID,
			err:      err,
			duration: time.Since(start),
		}
	}()

	// Second goroutine claims from run2 (should not block on run1's lock)
	go func() {
		defer wg.Done()
		start := time.Now()
		job, err := db.ClaimJob(ctx, node2.ID)
		results <- claimResult{
			jobID:    job.ID,
			err:      err,
			duration: time.Since(start),
		}
	}()

	wg.Wait()
	close(results)

	// Collect results.
	var res []claimResult
	for r := range results {
		res = append(res, r)
	}

	if len(res) != 2 {
		t.Fatalf("expected 2 results, got %d", len(res))
	}

	// Both claims should succeed.
	for i, r := range res {
		if r.err != nil {
			t.Fatalf("ClaimJob(%d) failed: %v", i, r.err)
		}
	}

	// Both claims should return different jobs (one from each run).
	claimedIDs := map[types.JobID]bool{
		res[0].jobID: true,
		res[1].jobID: true,
	}
	if len(claimedIDs) != 2 {
		t.Fatalf("expected 2 different jobs claimed, got duplicates: %v and %v", res[0].jobID, res[1].jobID)
	}

	// Verify one job is from run1 and one from run2.
	if !claimedIDs[job1ID] || !claimedIDs[job2ID] {
		t.Fatalf("expected jobs %s and %s claimed, got %v", job1ID, job2ID, claimedIDs)
	}

	t.Logf("Concurrent claims succeeded without blocking: job1=%s (%v), job2=%s (%v)",
		res[0].jobID, res[0].duration, res[1].jobID, res[1].duration)
}

type claimResult struct {
	jobID    types.JobID
	err      error
	duration time.Duration
}

// mustParseLockAddr parses an IP address string and fails the test on error.
func mustParseLockAddr(t *testing.T, s string) netip.Addr {
	t.Helper()
	addr, err := netip.ParseAddr(s)
	if err != nil {
		t.Fatalf("mustParseLockAddr(%q) failed: %v", s, err)
	}
	return addr
}
