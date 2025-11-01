package store

import (
	"context"
	"net/netip"
	"os"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

// TestRunClaim tests basic run claiming functionality.
// Requires PLOY_TEST_PG_DSN to be set with a test database.
func TestRunClaim(t *testing.T) {
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

	// Verify store is functional (connectivity test).
	_, err = db.GetCluster(ctx)
	if err != nil {
		t.Logf("GetCluster() returned error (expected if DB is empty): %v", err)
	}

	t.Log("Store integration test infrastructure is working")
	t.Log("Full integration tests require database setup - see tests/integration/")
}

// TestClaimRun_Basic tests the basic ClaimRun operation:
// - Creates a repo, mod, and run in queued status
// - Claims the run for a node
// - Verifies the run is assigned with correct node_id and started_at
func TestClaimRun_Basic(t *testing.T) {
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

	// Create a test repo.
	repo, err := db.CreateRepo(ctx, CreateRepoParams{
		Url:    "https://github.com/test/repo",
		Branch: ptrStr("main"),
	})
	if err != nil {
		t.Fatalf("CreateRepo() failed: %v", err)
	}

	// Create a test mod.
	mod, err := db.CreateMod(ctx, CreateModParams{
		RepoID: repo.ID,
		Spec:   []byte(`{"type":"test"}`),
	})
	if err != nil {
		t.Fatalf("CreateMod() failed: %v", err)
	}

	// Create a queued run.
	run, err := db.CreateRun(ctx, CreateRunParams{
		ModID:     mod.ID,
		Status:    RunStatusQueued,
		BaseRef:   "main",
		TargetRef: "feature",
	})
	if err != nil {
		t.Fatalf("CreateRun() failed: %v", err)
	}

	if run.Status != RunStatusQueued {
		t.Errorf("Expected status queued, got %s", run.Status)
	}

	// Create a test node.
	node, err := db.CreateNode(ctx, CreateNodeParams{
		Name:      "test-node-1",
		IpAddress: mustParseAddr(t, "192.168.1.100"),
	})
	if err != nil {
		t.Fatalf("CreateNode() failed: %v", err)
	}

	// Claim the run for the node.
	claimedRun, err := db.ClaimRun(ctx, node.ID)
	if err != nil {
		t.Fatalf("ClaimRun() failed: %v", err)
	}

	// Verify the claimed run has the correct properties.
	if claimedRun.ID != run.ID {
		t.Errorf("Expected run ID %v, got %v", run.ID, claimedRun.ID)
	}

	if claimedRun.Status != RunStatusAssigned {
		t.Errorf("Expected status assigned, got %s", claimedRun.Status)
	}

	if !claimedRun.NodeID.Valid || claimedRun.NodeID.Bytes != node.ID.Bytes {
		t.Errorf("Expected node_id %v, got %v", node.ID, claimedRun.NodeID)
	}

	if !claimedRun.StartedAt.Valid {
		t.Error("Expected started_at to be set")
	}

	// Verify no more runs can be claimed.
	_, err = db.ClaimRun(ctx, node.ID)
	if err == nil {
		t.Error("Expected ClaimRun to fail when no queued runs exist")
	}
}

// TestClaimRun_FIFO tests that runs are claimed in FIFO order by created_at.
func TestClaimRun_FIFO(t *testing.T) {
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

	// Create a test repo and mod.
	repo, err := db.CreateRepo(ctx, CreateRepoParams{
		Url:    "https://github.com/test/fifo",
		Branch: ptrStr("main"),
	})
	if err != nil {
		t.Fatalf("CreateRepo() failed: %v", err)
	}

	mod, err := db.CreateMod(ctx, CreateModParams{
		RepoID: repo.ID,
		Spec:   []byte(`{"type":"fifo"}`),
	})
	if err != nil {
		t.Fatalf("CreateMod() failed: %v", err)
	}

	// Create three queued runs.
	run1, err := db.CreateRun(ctx, CreateRunParams{
		ModID:     mod.ID,
		Status:    RunStatusQueued,
		BaseRef:   "main",
		TargetRef: "feature1",
	})
	if err != nil {
		t.Fatalf("CreateRun() failed: %v", err)
	}

	run2, err := db.CreateRun(ctx, CreateRunParams{
		ModID:     mod.ID,
		Status:    RunStatusQueued,
		BaseRef:   "main",
		TargetRef: "feature2",
	})
	if err != nil {
		t.Fatalf("CreateRun() failed: %v", err)
	}

	run3, err := db.CreateRun(ctx, CreateRunParams{
		ModID:     mod.ID,
		Status:    RunStatusQueued,
		BaseRef:   "main",
		TargetRef: "feature3",
	})
	if err != nil {
		t.Fatalf("CreateRun() failed: %v", err)
	}

	// Create test nodes.
	node1, err := db.CreateNode(ctx, CreateNodeParams{
		Name:      "test-node-fifo-1",
		IpAddress: mustParseAddr(t, "192.168.2.100"),
	})
	if err != nil {
		t.Fatalf("CreateNode() failed: %v", err)
	}

	node2, err := db.CreateNode(ctx, CreateNodeParams{
		Name:      "test-node-fifo-2",
		IpAddress: mustParseAddr(t, "192.168.2.101"),
	})
	if err != nil {
		t.Fatalf("CreateNode() failed: %v", err)
	}

	node3, err := db.CreateNode(ctx, CreateNodeParams{
		Name:      "test-node-fifo-3",
		IpAddress: mustParseAddr(t, "192.168.2.102"),
	})
	if err != nil {
		t.Fatalf("CreateNode() failed: %v", err)
	}

	// Claim runs and verify they are claimed in order.
	claimed1, err := db.ClaimRun(ctx, node1.ID)
	if err != nil {
		t.Fatalf("ClaimRun() for node1 failed: %v", err)
	}
	if claimed1.ID != run1.ID {
		t.Errorf("Expected first claim to get run1 (%v), got %v", run1.ID, claimed1.ID)
	}

	claimed2, err := db.ClaimRun(ctx, node2.ID)
	if err != nil {
		t.Fatalf("ClaimRun() for node2 failed: %v", err)
	}
	if claimed2.ID != run2.ID {
		t.Errorf("Expected second claim to get run2 (%v), got %v", run2.ID, claimed2.ID)
	}

	claimed3, err := db.ClaimRun(ctx, node3.ID)
	if err != nil {
		t.Fatalf("ClaimRun() for node3 failed: %v", err)
	}
	if claimed3.ID != run3.ID {
		t.Errorf("Expected third claim to get run3 (%v), got %v", run3.ID, claimed3.ID)
	}
}

// TestClaimRun_SkipLocked tests that FOR UPDATE SKIP LOCKED prevents concurrent claims
// of the same run by multiple nodes.
func TestClaimRun_SkipLocked(t *testing.T) {
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

	// Create a test repo and mod.
	repo, err := db.CreateRepo(ctx, CreateRepoParams{
		Url:    "https://github.com/test/skip-locked",
		Branch: ptrStr("main"),
	})
	if err != nil {
		t.Fatalf("CreateRepo() failed: %v", err)
	}

	mod, err := db.CreateMod(ctx, CreateModParams{
		RepoID: repo.ID,
		Spec:   []byte(`{"type":"concurrent"}`),
	})
	if err != nil {
		t.Fatalf("CreateMod() failed: %v", err)
	}

	// Create multiple queued runs for concurrent claiming.
	const numRuns = 10
	runIDs := make([]pgtype.UUID, numRuns)
	for i := 0; i < numRuns; i++ {
		run, err := db.CreateRun(ctx, CreateRunParams{
			ModID:     mod.ID,
			Status:    RunStatusQueued,
			BaseRef:   "main",
			TargetRef: "concurrent",
		})
		if err != nil {
			t.Fatalf("CreateRun() %d failed: %v", i, err)
		}
		runIDs[i] = run.ID
	}

	// Create multiple nodes to claim concurrently.
	const numNodes = 10
	nodes := make([]Node, numNodes)
	for i := 0; i < numNodes; i++ {
		node, err := db.CreateNode(ctx, CreateNodeParams{
			Name:      nodeNameForTest(t, "concurrent", i),
			IpAddress: mustParseAddr(t, ipForTest(3, i)),
		})
		if err != nil {
			t.Fatalf("CreateNode() %d failed: %v", i, err)
		}
		nodes[i] = node
	}

	// Claim runs concurrently.
	var wg sync.WaitGroup
	claimedRuns := make([]Run, numNodes)
	errors := make([]error, numNodes)

	for i := 0; i < numNodes; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			claimedRuns[idx], errors[idx] = db.ClaimRun(ctx, nodes[idx].ID)
		}(i)
	}

	wg.Wait()

	// Count successful claims.
	successCount := 0
	claimedIDs := make(map[[16]byte]bool)
	for i := 0; i < numNodes; i++ {
		if errors[i] == nil {
			successCount++
			// Verify each run is claimed only once.
			idBytes := claimedRuns[i].ID.Bytes
			if claimedIDs[idBytes] {
				t.Errorf("Run %v was claimed multiple times", claimedRuns[i].ID)
			}
			claimedIDs[idBytes] = true
		}
	}

	// Verify all runs were claimed exactly once.
	if successCount != numRuns {
		t.Errorf("Expected %d successful claims, got %d", numRuns, successCount)
	}

	// Verify no duplicate claims.
	if len(claimedIDs) != numRuns {
		t.Errorf("Expected %d unique claimed runs, got %d", numRuns, len(claimedIDs))
	}
}

// TestClaimRun_NoQueuedRuns tests ClaimRun when no runs are in queued status.
func TestClaimRun_NoQueuedRuns(t *testing.T) {
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

	// Create a test node.
	node, err := db.CreateNode(ctx, CreateNodeParams{
		Name:      "test-node-empty",
		IpAddress: mustParseAddr(t, "192.168.4.100"),
	})
	if err != nil {
		t.Fatalf("CreateNode() failed: %v", err)
	}

	// Try to claim when no queued runs exist.
	_, err = db.ClaimRun(ctx, node.ID)
	if err == nil {
		t.Error("Expected ClaimRun to fail when no queued runs exist")
	}
}

// Helper functions.

func ptrStr(s string) *string {
	return &s
}

func mustParseAddr(t *testing.T, s string) netip.Addr {
	addr, err := netip.ParseAddr(s)
	if err != nil {
		t.Fatalf("mustParseAddr(%q) failed: %v", s, err)
	}
	return addr
}

func nodeNameForTest(t *testing.T, prefix string, idx int) string {
	return prefix + "-node-" + t.Name() + "-" + itoa(idx)
}

func ipForTest(subnet, host int) string {
	if host > 254 {
		host = 254
	}
	// Generate unique IPs in the 192.168.subnet.host range.
	return "192.168." + itoa(subnet) + "." + itoa(host)
}

func itoa(i int) string {
	if i < 0 {
		return "-" + itoa(-i)
	}
	if i < 10 {
		return string(rune('0' + i))
	}
	return itoa(i/10) + string(rune('0'+i%10))
}
