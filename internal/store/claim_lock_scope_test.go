package store

import (
	"errors"
	"testing"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/jackc/pgx/v5/pgconn"
)

// ClaimJob protects the selected job and node without locking joined run or wave rows.
func TestClaimJobLocksJobAndNodeOnly(t *testing.T) {
	ctx, db := newTestStore(t)

	if db.Pool().Config().MaxConns < 2 {
		t.Skipf("pgxpool max_conns=%d; need >=2 to exercise concurrent transactions", db.Pool().Config().MaxConns)
	}

	fx := newV1Fixture(t, ctx, db, "https://github.com/test/lock-scope", "main", []byte(`{"type":"lock-scope"}`))

	jobID := types.NewJobID()
	createdJob, err := db.CreateJob(ctx, CreateJobParams{
		ID:          jobID,
		RunID:       fx.Run.ID,
		RepoID:      fx.MigRepo.RepoID,
		RepoBaseRef: fx.Run.RepoBaseRef,
		Attempt:     fx.Run.Attempt,
		Name:        "job-lock",
		JobType:     "mig",
		JobImage:    "",
		Status:      types.JobStatusQueued,
		NextID:      nil,
		Meta:        []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("CreateJob() failed: %v", err)
	}

	lockID := types.NodeID(types.NewNodeKey())
	node, err := db.CreateNode(ctx, CreateNodeParams{
		ID:          lockID,
		Name:        nodeNameForTest(lockID),
		IpAddress:   nodeAddrForTest(lockID),
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

	for _, tc := range []struct {
		name       string
		query      string
		id         string
		wantLocked bool
	}{
		{"selected job", `SELECT id FROM jobs WHERE id=$1 FOR UPDATE NOWAIT`, createdJob.ID.String(), true},
		{"claiming node", `SELECT id FROM nodes WHERE id=$1 FOR UPDATE NOWAIT`, node.ID.String(), true},
		{"joined run", `SELECT id FROM runs WHERE id=$1 FOR UPDATE NOWAIT`, fx.Run.ID.String(), false},
		{"joined wave", `SELECT id FROM waves WHERE id=$1 FOR UPDATE NOWAIT`, fx.Wave.ID.String(), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// A rejected NOWAIT statement aborts its transaction, so each check needs a new one.
			txCheck, err := db.Pool().Begin(ctx)
			if err != nil {
				t.Fatalf("Begin(txCheck) failed: %v", err)
			}
			defer txCheck.Rollback(ctx)

			var id string
			err = txCheck.QueryRow(ctx, tc.query, tc.id).Scan(&id)
			if tc.wantLocked {
				var pgErr *pgconn.PgError
				if !errors.As(err, &pgErr) || pgErr.Code != "55P03" {
					t.Fatalf("NOWAIT error=%v, want lock_not_available", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("joined row must remain lockable: %v", err)
			}
			if id != tc.id {
				t.Fatalf("locked row %s, want %s", id, tc.id)
			}
		})
	}
}
