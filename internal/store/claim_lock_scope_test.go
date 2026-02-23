package store

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/jackc/pgx/v5/pgconn"
)

// TestClaimJobLocksJobOnly verifies that ClaimJob does not lock rows in `runs`.
// The ClaimJob query joins `runs` for eligibility checks, but it must lock only
// the selected `jobs` row via `FOR UPDATE OF j SKIP LOCKED`.
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
		t.Skipf("pgxpool max_conns=%d; need >=2 to exercise concurrent transactions", db.Pool().Config().MaxConns)
	}

	fx := newV1Fixture(t, ctx, db, "https://github.com/test/lock-scope", "main", "feature", []byte(`{"type":"lock-scope"}`))

	jobID := types.NewJobID()
	createdJob, err := db.CreateJob(ctx, CreateJobParams{
		ID:          jobID,
		RunID:       fx.Run.ID,
		RepoID:      fx.ModRepo.ID,
		RepoBaseRef: fx.RunRepo.RepoBaseRef,
		Attempt:     fx.RunRepo.Attempt,
		Name:        "job-lock",
		JobType:     "",
		JobImage:    "",
		Status:      JobStatusQueued,
		NextID:      nil,
		Meta:        []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("CreateJob() failed: %v", err)
	}

	node, err := db.CreateNode(ctx, CreateNodeParams{
		ID:          types.NodeID(types.NewNodeKey()),
		Name:        "test-node-lock",
		IpAddress:   mustParseAddr(t, "192.168.100.1"),
		Concurrency: 1,
	})
	if err != nil {
		t.Fatalf("CreateNode() failed: %v", err)
	}

	txClaim, err := db.Pool().Begin(ctx)
	if err != nil {
		t.Fatalf("Begin(txClaim) failed: %v", err)
	}
	defer func() {
		_ = txClaim.Rollback(ctx)
	}()

	claimedJob, err := New(txClaim).ClaimJob(ctx, node.ID)
	if err != nil {
		t.Fatalf("ClaimJob(tx) failed: %v", err)
	}
	if claimedJob.ID != createdJob.ID {
		t.Fatalf("ClaimJob(tx) claimed %s, want %s", claimedJob.ID, createdJob.ID)
	}

	// If ClaimJob incorrectly locks `runs` rows (i.e. plain `FOR UPDATE SKIP LOCKED`
	// on a join), this NOWAIT lock attempt will fail with 55P03 (lock_not_available).
	txCheck, err := db.Pool().Begin(ctx)
	if err != nil {
		t.Fatalf("Begin(txCheck) failed: %v", err)
	}
	defer func() {
		_ = txCheck.Rollback(ctx)
	}()

	var runID types.RunID
	err = txCheck.QueryRow(ctx, `SELECT id FROM runs WHERE id = $1 FOR UPDATE NOWAIT`, fx.Run.ID).Scan(&runID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "55P03" {
			t.Fatalf("runs row was locked by ClaimJob; expected ClaimJob to lock jobs only (use `FOR UPDATE OF j SKIP LOCKED`): %v", err)
		}
		t.Fatalf("failed to lock run row NOWAIT: %v", err)
	}
	if runID != fx.Run.ID {
		t.Fatalf("locked run %s, want %s", runID, fx.Run.ID)
	}
}
