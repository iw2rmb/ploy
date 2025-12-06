package batchscheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iw2rmb/ploy/internal/store"
)

// mockStore implements store.Store for testing.
type mockStore struct {
	store.Store
	listRunsWithPendingReposResult []pgtype.UUID
	listRunsWithPendingReposErr    error
	listRunsWithPendingReposCalled bool
}

func (m *mockStore) ListBatchRunsWithPendingRepos(ctx context.Context) ([]pgtype.UUID, error) {
	m.listRunsWithPendingReposCalled = true
	return m.listRunsWithPendingReposResult, m.listRunsWithPendingReposErr
}

func (m *mockStore) Close() {}

func (m *mockStore) Pool() *pgxpool.Pool {
	return nil
}

// mockRepoStarter implements RepoStarter for testing.
type mockRepoStarter struct {
	startCalls   []pgtype.UUID
	startResults map[uuid.UUID]int
	startErrors  map[uuid.UUID]error
}

func newMockRepoStarter() *mockRepoStarter {
	return &mockRepoStarter{
		startCalls:   []pgtype.UUID{},
		startResults: make(map[uuid.UUID]int),
		startErrors:  make(map[uuid.UUID]error),
	}
}

func (m *mockRepoStarter) StartPendingRepos(ctx context.Context, runID pgtype.UUID) (int, error) {
	m.startCalls = append(m.startCalls, runID)
	uid := uuid.UUID(runID.Bytes)
	if err, ok := m.startErrors[uid]; ok && err != nil {
		return 0, err
	}
	if result, ok := m.startResults[uid]; ok {
		return result, nil
	}
	return 0, nil
}

// uuidToPgtype converts uuid.UUID to pgtype.UUID.
func uuidToPgtype(u uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: u, Valid: true}
}

func TestNew(t *testing.T) {
	t.Run("nil store returns nil scheduler", func(t *testing.T) {
		sched, err := New(Options{
			Store:       nil,
			RepoStarter: newMockRepoStarter(),
		})
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if sched != nil {
			t.Errorf("expected nil scheduler, got %v", sched)
		}
	})

	t.Run("nil repoStarter returns nil scheduler", func(t *testing.T) {
		sched, err := New(Options{
			Store:       &mockStore{},
			RepoStarter: nil,
		})
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if sched != nil {
			t.Errorf("expected nil scheduler, got %v", sched)
		}
	})

	t.Run("default interval is 5 seconds", func(t *testing.T) {
		sched, err := New(Options{
			Store:       &mockStore{},
			RepoStarter: newMockRepoStarter(),
		})
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if sched == nil {
			t.Fatal("expected scheduler, got nil")
		}
		expected := 5 * time.Second
		if sched.interval != expected {
			t.Errorf("expected interval %v, got %v", expected, sched.interval)
		}
	})

	t.Run("custom interval", func(t *testing.T) {
		customInterval := 10 * time.Second
		sched, err := New(Options{
			Store:       &mockStore{},
			RepoStarter: newMockRepoStarter(),
			Interval:    customInterval,
		})
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if sched == nil {
			t.Fatal("expected scheduler, got nil")
		}
		if sched.interval != customInterval {
			t.Errorf("expected interval %v, got %v", customInterval, sched.interval)
		}
	})
}

func TestScheduler_Name(t *testing.T) {
	sched, _ := New(Options{
		Store:       &mockStore{},
		RepoStarter: newMockRepoStarter(),
	})
	if sched.Name() != "batch-scheduler" {
		t.Errorf("expected name 'batch-scheduler', got %q", sched.Name())
	}
}

func TestScheduler_Interval(t *testing.T) {
	expected := 15 * time.Second
	sched, _ := New(Options{
		Store:       &mockStore{},
		RepoStarter: newMockRepoStarter(),
		Interval:    expected,
	})
	if sched.Interval() != expected {
		t.Errorf("expected interval %v, got %v", expected, sched.Interval())
	}
}

func TestScheduler_Run(t *testing.T) {
	t.Run("nil scheduler does nothing", func(t *testing.T) {
		var sched *Scheduler
		if err := sched.Run(context.Background()); err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("no runs with pending repos", func(t *testing.T) {
		mockSt := &mockStore{
			listRunsWithPendingReposResult: []pgtype.UUID{},
		}
		mockStarter := newMockRepoStarter()

		sched, err := New(Options{
			Store:       mockSt,
			RepoStarter: mockStarter,
		})
		if err != nil {
			t.Fatalf("failed to create scheduler: %v", err)
		}

		if err := sched.Run(context.Background()); err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if !mockSt.listRunsWithPendingReposCalled {
			t.Error("expected ListBatchRunsWithPendingRepos to be called")
		}

		if len(mockStarter.startCalls) != 0 {
			t.Errorf("expected no StartPendingRepos calls, got %d", len(mockStarter.startCalls))
		}
	})

	t.Run("starts repos for each batch run", func(t *testing.T) {
		runID1 := uuid.New()
		runID2 := uuid.New()

		mockSt := &mockStore{
			listRunsWithPendingReposResult: []pgtype.UUID{
				uuidToPgtype(runID1),
				uuidToPgtype(runID2),
			},
		}

		mockStarter := newMockRepoStarter()
		mockStarter.startResults[runID1] = 2
		mockStarter.startResults[runID2] = 1

		sched, err := New(Options{
			Store:       mockSt,
			RepoStarter: mockStarter,
		})
		if err != nil {
			t.Fatalf("failed to create scheduler: %v", err)
		}

		if err := sched.Run(context.Background()); err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if len(mockStarter.startCalls) != 2 {
			t.Fatalf("expected 2 StartPendingRepos calls, got %d", len(mockStarter.startCalls))
		}

		// Verify both runs were processed.
		processedRuns := make(map[uuid.UUID]bool)
		for _, runID := range mockStarter.startCalls {
			processedRuns[uuid.UUID(runID.Bytes)] = true
		}

		if !processedRuns[runID1] {
			t.Errorf("expected runID1 %s to be processed", runID1)
		}
		if !processedRuns[runID2] {
			t.Errorf("expected runID2 %s to be processed", runID2)
		}
	})

	t.Run("continues on error", func(t *testing.T) {
		runID1 := uuid.New()
		runID2 := uuid.New()

		mockSt := &mockStore{
			listRunsWithPendingReposResult: []pgtype.UUID{
				uuidToPgtype(runID1),
				uuidToPgtype(runID2),
			},
		}

		mockStarter := newMockRepoStarter()
		mockStarter.startErrors[runID1] = errors.New("failed to start repos")
		mockStarter.startResults[runID2] = 3

		sched, err := New(Options{
			Store:       mockSt,
			RepoStarter: mockStarter,
		})
		if err != nil {
			t.Fatalf("failed to create scheduler: %v", err)
		}

		// Run should not return error even if one batch fails.
		if err := sched.Run(context.Background()); err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// Both runs should still be attempted.
		if len(mockStarter.startCalls) != 2 {
			t.Fatalf("expected 2 StartPendingRepos calls, got %d", len(mockStarter.startCalls))
		}
	})

	t.Run("handles list error gracefully", func(t *testing.T) {
		mockSt := &mockStore{
			listRunsWithPendingReposErr: errors.New("database error"),
		}
		mockStarter := newMockRepoStarter()

		sched, err := New(Options{
			Store:       mockSt,
			RepoStarter: mockStarter,
		})
		if err != nil {
			t.Fatalf("failed to create scheduler: %v", err)
		}

		// Run should not return error even if list fails.
		if err := sched.Run(context.Background()); err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// List was called but no starts should happen.
		if !mockSt.listRunsWithPendingReposCalled {
			t.Error("expected ListBatchRunsWithPendingRepos to be called")
		}
		if len(mockStarter.startCalls) != 0 {
			t.Errorf("expected no StartPendingRepos calls, got %d", len(mockStarter.startCalls))
		}
	})
}
