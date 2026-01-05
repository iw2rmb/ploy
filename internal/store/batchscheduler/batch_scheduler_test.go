package batchscheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/segmentio/ksuid"

	"github.com/iw2rmb/ploy/internal/store"
)

// mockStore implements store.Store for testing.
// Uses string IDs (KSUID-backed) per the KSUID migration.
type mockStore struct {
	store.Store
	listRunsWithQueuedReposResult []string
	listRunsWithQueuedReposErr    error
	listRunsWithQueuedReposCalled bool
}

func (m *mockStore) ListRunsWithQueuedRepos(ctx context.Context) ([]string, error) {
	m.listRunsWithQueuedReposCalled = true
	return m.listRunsWithQueuedReposResult, m.listRunsWithQueuedReposErr
}

func (m *mockStore) Close() {}

func (m *mockStore) Pool() *pgxpool.Pool {
	return nil
}

// mockRepoStarter implements RepoStarter for testing.
// Uses string IDs (KSUID-backed) per the KSUID migration.
type mockRepoStarter struct {
	startCalls   []string
	startResults map[string]StartPendingReposResult
	startErrors  map[string]error
}

func newMockRepoStarter() *mockRepoStarter {
	return &mockRepoStarter{
		startCalls:   []string{},
		startResults: make(map[string]StartPendingReposResult),
		startErrors:  make(map[string]error),
	}
}

func (m *mockRepoStarter) StartPendingRepos(ctx context.Context, runID string) (StartPendingReposResult, error) {
	m.startCalls = append(m.startCalls, runID)
	if err, ok := m.startErrors[runID]; ok && err != nil {
		return StartPendingReposResult{}, err
	}
	if result, ok := m.startResults[runID]; ok {
		return result, nil
	}
	return StartPendingReposResult{}, nil
}

// newTestKSUID generates a new KSUID string for test IDs.
func newTestKSUID() string {
	return ksuid.New().String()
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
			listRunsWithQueuedReposResult: []string{},
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

		if !mockSt.listRunsWithQueuedReposCalled {
			t.Error("expected ListRunsWithQueuedRepos to be called")
		}

		if len(mockStarter.startCalls) != 0 {
			t.Errorf("expected no StartPendingRepos calls, got %d", len(mockStarter.startCalls))
		}
	})

	t.Run("starts repos for each batch run", func(t *testing.T) {
		// Use KSUID-backed string IDs per the migration.
		runID1 := newTestKSUID()
		runID2 := newTestKSUID()

		mockSt := &mockStore{
			listRunsWithQueuedReposResult: []string{runID1, runID2},
		}

		mockStarter := newMockRepoStarter()
		mockStarter.startResults[runID1] = StartPendingReposResult{Started: 2, AlreadyDone: 0, Pending: 0}
		mockStarter.startResults[runID2] = StartPendingReposResult{Started: 1, AlreadyDone: 1, Pending: 0}

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
		processedRuns := make(map[string]bool)
		for _, runID := range mockStarter.startCalls {
			processedRuns[runID] = true
		}

		if !processedRuns[runID1] {
			t.Errorf("expected runID1 %s to be processed", runID1)
		}
		if !processedRuns[runID2] {
			t.Errorf("expected runID2 %s to be processed", runID2)
		}
	})

	t.Run("continues on error", func(t *testing.T) {
		// Use KSUID-backed string IDs per the migration.
		runID1 := newTestKSUID()
		runID2 := newTestKSUID()

		mockSt := &mockStore{
			listRunsWithQueuedReposResult: []string{runID1, runID2},
		}

		mockStarter := newMockRepoStarter()
		mockStarter.startErrors[runID1] = errors.New("failed to start repos")
		mockStarter.startResults[runID2] = StartPendingReposResult{Started: 3, AlreadyDone: 0, Pending: 0}

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
			listRunsWithQueuedReposErr: errors.New("database error"),
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
		if !mockSt.listRunsWithQueuedReposCalled {
			t.Error("expected ListRunsWithQueuedRepos to be called")
		}
		if len(mockStarter.startCalls) != 0 {
			t.Errorf("expected no StartPendingRepos calls, got %d", len(mockStarter.startCalls))
		}
	})
}
