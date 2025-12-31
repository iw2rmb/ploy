package handlers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// TestBatchRepoStarter_StartPendingRepos verifies the BatchRepoStarter helper
// that background schedulers use to start pending repos in batch runs.
// This tests the unified implementation that is shared between the HTTP handler
// (startRunHandler) and the background scheduler (batchscheduler.Scheduler).
func TestBatchRepoStarter_StartPendingRepos(t *testing.T) {
	t.Parallel()

	batchRunID := uuid.New()
	repo1ID := uuid.New()
	repo2ID := uuid.New()
	childRunID := uuid.New()

	queuedBatch := store.Run{
		ID:        batchRunID.String(),
		Status:    store.RunStatusQueued,
		Spec:      []byte(`{"image":"test"}`),
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	pendingRepo := store.RunRepo{
		ID:        repo1ID.String(),
		RunID:     domaintypes.RunID(batchRunID.String()),
		RepoUrl:   "https://github.com/org/repo.git",
		Status:    store.RunRepoStatusPending,
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	succeededRepo := store.RunRepo{
		ID:        repo2ID.String(),
		RunID:     domaintypes.RunID(batchRunID.String()),
		RepoUrl:   "https://github.com/org/repo2.git",
		Status:    store.RunRepoStatusSucceeded,
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	childRun := store.Run{
		ID:        childRunID.String(),
		Status:    store.RunStatusQueued,
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	t.Run("starts pending repos successfully and returns correct counts", func(t *testing.T) {
		t.Parallel()

		m := &mockStore{
			getRunResult:                   queuedBatch,
			listRunReposByRunResult:        []store.RunRepo{pendingRepo, succeededRepo},
			listPendingRunReposByRunResult: []store.RunRepo{pendingRepo},
			createRunResult:                childRun,
		}

		starter := NewBatchRepoStarter(m)
		result, err := starter.StartPendingRepos(context.Background(), queuedBatch.ID)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify Started count.
		if result.Started != 1 {
			t.Errorf("Started = %d, want 1", result.Started)
		}

		// Verify AlreadyDone count (succeededRepo is terminal).
		if result.AlreadyDone != 1 {
			t.Errorf("AlreadyDone = %d, want 1", result.AlreadyDone)
		}

		// Verify Pending count (should be 0 after starting the one pending repo).
		if result.Pending != 0 {
			t.Errorf("Pending = %d, want 0", result.Pending)
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
		result, err := starter.StartPendingRepos(context.Background(), canceledBatch.ID)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Started != 0 {
			t.Errorf("Started = %d, want 0 (terminal batch)", result.Started)
		}
		// Should not try to list repos for terminal batch.
		if m.listRunReposByRunCalled {
			t.Error("should not list repos for terminal batch")
		}
		if m.listPendingRunReposByRunCalled {
			t.Error("should not list pending repos for terminal batch")
		}
	})

	t.Run("returns correct counts when no pending repos", func(t *testing.T) {
		t.Parallel()

		m := &mockStore{
			getRunResult:                   queuedBatch,
			listRunReposByRunResult:        []store.RunRepo{succeededRepo}, // Only completed repos.
			listPendingRunReposByRunResult: []store.RunRepo{},              // No pending repos.
		}

		starter := NewBatchRepoStarter(m)
		result, err := starter.StartPendingRepos(context.Background(), queuedBatch.ID)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Started != 0 {
			t.Errorf("Started = %d, want 0", result.Started)
		}
		if result.AlreadyDone != 1 {
			t.Errorf("AlreadyDone = %d, want 1", result.AlreadyDone)
		}
		if result.Pending != 0 {
			t.Errorf("Pending = %d, want 0", result.Pending)
		}
	})

	t.Run("returns pending count when a repo fails to start", func(t *testing.T) {
		t.Parallel()

		// Create a scenario where we have 2 pending repos but the CreateRun call fails for one.
		pendingRepo2 := store.RunRepo{
			ID:        repo2ID.String(),
			RunID:     domaintypes.RunID(batchRunID.String()),
			RepoUrl:   "https://github.com/org/repo2.git",
			Status:    store.RunRepoStatusPending,
			CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		}

		m := &mockStore{
			getRunResult:                   queuedBatch,
			listRunReposByRunResult:        []store.RunRepo{pendingRepo, pendingRepo2},
			listPendingRunReposByRunResult: []store.RunRepo{pendingRepo, pendingRepo2},
			createRunResult:                childRun,
			createRunErrs:                  []error{nil, errors.New("create run failed")},
		}

		starter := NewBatchRepoStarter(m)
		result, err := starter.StartPendingRepos(context.Background(), queuedBatch.ID)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.Started != 1 {
			t.Errorf("Started = %d, want 1", result.Started)
		}
		if result.AlreadyDone != 0 {
			t.Errorf("AlreadyDone = %d, want 0", result.AlreadyDone)
		}
		if result.Pending != 1 {
			t.Errorf("Pending = %d, want 1", result.Pending)
		}

		if !m.createRunCalled {
			t.Error("expected CreateRun to be called")
		}
		if len(m.setRunRepoExecutionRunParams) != 1 {
			t.Errorf("SetRunRepoExecutionRun calls = %d, want 1", len(m.setRunRepoExecutionRunParams))
		}
		if !m.ackRunStartCalled {
			t.Error("expected AckRunStart to be called")
		}
	})
}
