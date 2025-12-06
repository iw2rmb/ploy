package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/store"
)

// TestBatchRepoStarter_StartPendingRepos verifies the BatchRepoStarter helper
// that background schedulers use to start pending repos in batch runs.
func TestBatchRepoStarter_StartPendingRepos(t *testing.T) {
	t.Parallel()

	batchRunID := uuid.New()
	repo1ID := uuid.New()
	childRunID := uuid.New()

	queuedBatch := store.Run{
		ID:        pgtype.UUID{Bytes: batchRunID, Valid: true},
		Status:    store.RunStatusQueued,
		Spec:      []byte(`{"mod":{"image":"test"}}`),
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	pendingRepo := store.RunRepo{
		ID:        pgtype.UUID{Bytes: repo1ID, Valid: true},
		RunID:     pgtype.UUID{Bytes: batchRunID, Valid: true},
		RepoUrl:   "https://github.com/org/repo.git",
		Status:    store.RunRepoStatusPending,
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	childRun := store.Run{
		ID:        pgtype.UUID{Bytes: childRunID, Valid: true},
		Status:    store.RunStatusQueued,
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	t.Run("starts pending repos successfully", func(t *testing.T) {
		t.Parallel()

		m := &mockStore{
			getRunResult:                   queuedBatch,
			listPendingRunReposByRunResult: []store.RunRepo{pendingRepo},
			createRunResult:                childRun,
		}

		starter := NewBatchRepoStarter(m)
		started, err := starter.StartPendingRepos(context.Background(), queuedBatch.ID)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if started != 1 {
			t.Errorf("started = %d, want 1", started)
		}
		if !m.createRunCalled {
			t.Error("expected CreateRun to be called")
		}
		if !m.setRunRepoExecutionRunCalled {
			t.Error("expected SetRunRepoExecutionRun to be called")
		}
		if !m.ackRunStartCalled {
			t.Error("expected AckRunStart to be called")
		}
	})

	t.Run("skips terminal batch runs", func(t *testing.T) {
		t.Parallel()

		canceledBatch := queuedBatch
		canceledBatch.Status = store.RunStatusCanceled

		m := &mockStore{
			getRunResult: canceledBatch,
		}

		starter := NewBatchRepoStarter(m)
		started, err := starter.StartPendingRepos(context.Background(), canceledBatch.ID)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if started != 0 {
			t.Errorf("started = %d, want 0 (terminal batch)", started)
		}
		// Should not try to list pending repos for terminal batch.
		if m.listPendingRunReposByRunCalled {
			t.Error("should not list repos for terminal batch")
		}
	})

	t.Run("returns zero when no pending repos", func(t *testing.T) {
		t.Parallel()

		m := &mockStore{
			getRunResult:                   queuedBatch,
			listPendingRunReposByRunResult: []store.RunRepo{}, // No pending repos.
		}

		starter := NewBatchRepoStarter(m)
		started, err := starter.StartPendingRepos(context.Background(), queuedBatch.ID)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if started != 0 {
			t.Errorf("started = %d, want 0", started)
		}
	})
}
