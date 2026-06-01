package batchscheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// mockStore implements store.Store for testing.
// Uses types.WaveID (KSUID-backed) per the typed IDs migration.
type mockStore struct {
	store.Store
	listWavesWithQueuedRunsResult []types.WaveID
	listWavesWithQueuedRunsErr    error
	listWavesWithQueuedRunsCalled bool
}

func (m *mockStore) ListWavesWithQueuedRuns(ctx context.Context) ([]types.WaveID, error) {
	m.listWavesWithQueuedRunsCalled = true
	return m.listWavesWithQueuedRunsResult, m.listWavesWithQueuedRunsErr
}

func (m *mockStore) Close() {}

func (m *mockStore) Pool() *pgxpool.Pool {
	return nil
}

// mockRepoStarter implements RepoStarter for testing.
// Uses types.WaveID (KSUID-backed) per the typed IDs migration.
type mockRepoStarter struct {
	startCalls   []types.WaveID
	startResults map[types.WaveID]StartPendingReposResult
	startErrors  map[types.WaveID]error
}

func newMockRepoStarter() *mockRepoStarter {
	return &mockRepoStarter{
		startCalls:   []types.WaveID{},
		startResults: make(map[types.WaveID]StartPendingReposResult),
		startErrors:  make(map[types.WaveID]error),
	}
}

func (m *mockRepoStarter) StartPendingRepos(ctx context.Context, waveID types.WaveID) (StartPendingReposResult, error) {
	m.startCalls = append(m.startCalls, waveID)
	if err, ok := m.startErrors[waveID]; ok && err != nil {
		return StartPendingReposResult{}, err
	}
	if result, ok := m.startResults[waveID]; ok {
		return result, nil
	}
	return StartPendingReposResult{}, nil
}

// newTestWaveID generates a new types.WaveID (KSUID-backed) for test IDs.
func newTestWaveID() types.WaveID {
	return types.NewWaveID()
}

func TestNew(t *testing.T) {
	customInterval := 10 * time.Second
	tests := []struct {
		name         string
		opts         Options
		wantErr      error
		wantInterval time.Duration
	}{
		{
			name:    "nil store returns error",
			opts:    Options{RepoStarter: newMockRepoStarter()},
			wantErr: ErrNilStore,
		},
		{
			name:    "nil repoStarter returns error",
			opts:    Options{Store: &mockStore{}},
			wantErr: ErrNilRepoStarter,
		},
		{
			name:         "default interval",
			opts:         Options{Store: &mockStore{}, RepoStarter: newMockRepoStarter()},
			wantInterval: 5 * time.Second,
		},
		{
			name:         "custom interval",
			opts:         Options{Store: &mockStore{}, RepoStarter: newMockRepoStarter(), Interval: customInterval},
			wantInterval: customInterval,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sched, err := New(tt.opts)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("New() error=%v, want %v", err, tt.wantErr)
				}
				if sched != nil {
					t.Fatalf("New() scheduler=%v, want nil", sched)
				}
				return
			}
			if err != nil {
				t.Fatalf("New() failed: %v", err)
			}
			if sched.Name() != "batch-scheduler" {
				t.Fatalf("Name()=%q, want batch-scheduler", sched.Name())
			}
			if sched.Interval() != tt.wantInterval {
				t.Fatalf("Interval()=%v, want %v", sched.Interval(), tt.wantInterval)
			}
		})
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
			listWavesWithQueuedRunsResult: []types.WaveID{},
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

		if !mockSt.listWavesWithQueuedRunsCalled {
			t.Error("expected ListWavesWithQueuedRuns to be called")
		}

		if len(mockStarter.startCalls) != 0 {
			t.Errorf("expected no StartPendingRepos calls, got %d", len(mockStarter.startCalls))
		}
	})

	t.Run("starts repos for each batch run", func(t *testing.T) {
		// Use types.WaveID (KSUID-backed) per the typed IDs migration.
		waveID1 := newTestWaveID()
		waveID2 := newTestWaveID()

		mockSt := &mockStore{
			listWavesWithQueuedRunsResult: []types.WaveID{waveID1, waveID2},
		}

		mockStarter := newMockRepoStarter()
		mockStarter.startResults[waveID1] = StartPendingReposResult{Started: 2, AlreadyDone: 0, Pending: 0}
		mockStarter.startResults[waveID2] = StartPendingReposResult{Started: 1, AlreadyDone: 1, Pending: 0}

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
		processedWaves := make(map[types.WaveID]bool)
		for _, waveID := range mockStarter.startCalls {
			processedWaves[waveID] = true
		}

		if !processedWaves[waveID1] {
			t.Errorf("expected waveID1 %s to be processed", waveID1)
		}
		if !processedWaves[waveID2] {
			t.Errorf("expected waveID2 %s to be processed", waveID2)
		}
	})

	t.Run("continues on error", func(t *testing.T) {
		// Use types.WaveID (KSUID-backed) per the typed IDs migration.
		waveID1 := newTestWaveID()
		waveID2 := newTestWaveID()

		mockSt := &mockStore{
			listWavesWithQueuedRunsResult: []types.WaveID{waveID1, waveID2},
		}

		mockStarter := newMockRepoStarter()
		mockStarter.startErrors[waveID1] = errors.New("failed to start repos")
		mockStarter.startResults[waveID2] = StartPendingReposResult{Started: 3, AlreadyDone: 0, Pending: 0}

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
			listWavesWithQueuedRunsErr: errors.New("database error"),
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
		if !mockSt.listWavesWithQueuedRunsCalled {
			t.Error("expected ListWavesWithQueuedRuns to be called")
		}
		if len(mockStarter.startCalls) != 0 {
			t.Errorf("expected no StartPendingRepos calls, got %d", len(mockStarter.startCalls))
		}
	})
}
