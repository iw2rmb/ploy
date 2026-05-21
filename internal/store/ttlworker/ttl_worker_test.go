package ttlworker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iw2rmb/ploy/internal/store"
)

// mockStore implements store.Store for testing.
type mockStore struct {
	store.Store
	deleteLogsCount       int64
	deleteEventsCount     int64
	deleteDiffsCount      int64
	deleteArtifactsCount  int64
	deleteLogsCalled      bool
	deleteEventsCalled    bool
	deleteDiffsCalled     bool
	deleteArtifactsCalled bool
	deleteLogsErr         error
	deleteEventsErr       error
	deleteDiffsErr        error
	deleteArtifactsErr    error
	lastLogsArg           pgtype.Timestamptz
	lastEventsArg         pgtype.Timestamptz
	lastDiffsArg          pgtype.Timestamptz
	lastArtifactsArg      pgtype.Timestamptz
}

func (m *mockStore) DeleteExpiredLogs(ctx context.Context, createdAt pgtype.Timestamptz) (int64, error) {
	m.deleteLogsCalled = true
	m.lastLogsArg = createdAt
	return m.deleteLogsCount, m.deleteLogsErr
}

func (m *mockStore) DeleteExpiredEvents(ctx context.Context, time pgtype.Timestamptz) (int64, error) {
	m.deleteEventsCalled = true
	m.lastEventsArg = time
	return m.deleteEventsCount, m.deleteEventsErr
}

func (m *mockStore) DeleteExpiredDiffs(ctx context.Context, createdAt pgtype.Timestamptz) (int64, error) {
	m.deleteDiffsCalled = true
	m.lastDiffsArg = createdAt
	return m.deleteDiffsCount, m.deleteDiffsErr
}

func (m *mockStore) DeleteExpiredArtifactBundles(ctx context.Context, createdAt pgtype.Timestamptz) (int64, error) {
	m.deleteArtifactsCalled = true
	m.lastArtifactsArg = createdAt
	return m.deleteArtifactsCount, m.deleteArtifactsErr
}

func (m *mockStore) Close() {}

func (m *mockStore) Pool() *pgxpool.Pool {
	return nil
}

func TestNew(t *testing.T) {
	customTTL := 7 * 24 * time.Hour
	customInterval := 30 * time.Minute
	tests := []struct {
		name         string
		opts         Options
		wantErr      error
		wantTTL      time.Duration
		wantInterval time.Duration
		wantDrop     bool
	}{
		{
			name:    "nil store returns error",
			opts:    Options{},
			wantErr: ErrNilStore,
		},
		{
			name:         "defaults",
			opts:         Options{Store: &mockStore{}},
			wantTTL:      30 * 24 * time.Hour,
			wantInterval: time.Hour,
		},
		{
			name:         "custom TTL and interval",
			opts:         Options{Store: &mockStore{}, TTL: customTTL, Interval: customInterval},
			wantTTL:      customTTL,
			wantInterval: customInterval,
		},
		{
			name:         "drop partitions enabled",
			opts:         Options{Store: &mockStore{}, DropPartitions: true},
			wantTTL:      30 * 24 * time.Hour,
			wantInterval: time.Hour,
			wantDrop:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			worker, err := New(tt.opts)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("New() error=%v, want %v", err, tt.wantErr)
				}
				if worker != nil {
					t.Fatalf("New() worker=%v, want nil", worker)
				}
				return
			}
			if err != nil {
				t.Fatalf("New() failed: %v", err)
			}
			if worker.Name() != "ttl-worker" {
				t.Fatalf("Name()=%q, want ttl-worker", worker.Name())
			}
			if worker.ttl != tt.wantTTL {
				t.Fatalf("ttl=%v, want %v", worker.ttl, tt.wantTTL)
			}
			if worker.Interval() != tt.wantInterval {
				t.Fatalf("Interval()=%v, want %v", worker.Interval(), tt.wantInterval)
			}
			if worker.dropPartitions != tt.wantDrop {
				t.Fatalf("dropPartitions=%v, want %v", worker.dropPartitions, tt.wantDrop)
			}
		})
	}
}

func TestWorker_Run(t *testing.T) {
	t.Run("nil worker does nothing", func(t *testing.T) {
		var worker *Worker
		if err := worker.Run(context.Background()); err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("successful cleanup", func(t *testing.T) {
		mock := &mockStore{
			deleteLogsCount:      10,
			deleteEventsCount:    20,
			deleteDiffsCount:     5,
			deleteArtifactsCount: 3,
		}
		worker, err := New(Options{
			Store: mock,
			TTL:   24 * time.Hour,
		})
		if err != nil {
			t.Fatalf("failed to create worker: %v", err)
		}

		// Capture window to validate cutoff timestamp passed to store methods.
		before := time.Now()
		if err := worker.Run(context.Background()); err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		after := time.Now()

		if !mock.deleteLogsCalled {
			t.Error("expected DeleteExpiredLogs to be called")
		}
		if !mock.deleteEventsCalled {
			t.Error("expected DeleteExpiredEvents to be called")
		}
		if !mock.deleteDiffsCalled {
			t.Error("expected DeleteExpiredDiffs to be called")
		}
		if !mock.deleteArtifactsCalled {
			t.Error("expected DeleteExpiredArtifactBundles to be called")
		}

		// Validate that cutoff timestamps were passed and are within [before-ttl, after-ttl].
		lower := before.Add(-worker.ttl)
		upper := after.Add(-worker.ttl)
		if !mock.lastLogsArg.Valid || mock.lastLogsArg.Time.Before(lower) || mock.lastLogsArg.Time.After(upper) {
			t.Errorf("logs cutoff outside expected window: %v not in [%v, %v]",
				mock.lastLogsArg.Time, lower, upper)
		}
		if !mock.lastEventsArg.Valid || mock.lastEventsArg.Time.Before(lower) || mock.lastEventsArg.Time.After(upper) {
			t.Errorf("events cutoff outside expected window: %v not in [%v, %v]",
				mock.lastEventsArg.Time, lower, upper)
		}
		if !mock.lastDiffsArg.Valid || mock.lastDiffsArg.Time.Before(lower) || mock.lastDiffsArg.Time.After(upper) {
			t.Errorf("diffs cutoff outside expected window: %v not in [%v, %v]",
				mock.lastDiffsArg.Time, lower, upper)
		}
		if !mock.lastArtifactsArg.Valid || mock.lastArtifactsArg.Time.Before(lower) || mock.lastArtifactsArg.Time.After(upper) {
			t.Errorf("artifacts cutoff outside expected window: %v not in [%v, %v]",
				mock.lastArtifactsArg.Time, lower, upper)
		}
	})

	t.Run("continues on error and returns aggregated errors", func(t *testing.T) {
		mock := &mockStore{
			deleteLogsErr:      errors.New("logs delete failed"),
			deleteEventsErr:    errors.New("events delete failed"),
			deleteDiffsErr:     errors.New("diffs delete failed"),
			deleteArtifactsErr: errors.New("artifacts delete failed"),
		}
		worker, err := New(Options{
			Store: mock,
		})
		if err != nil {
			t.Fatalf("failed to create worker: %v", err)
		}

		// Should return aggregated errors.
		err = worker.Run(context.Background())
		if err == nil {
			t.Error("expected aggregated error, got nil")
		}

		// All operations should still be attempted.
		if !mock.deleteLogsCalled {
			t.Error("expected DeleteExpiredLogs to be called")
		}
		if !mock.deleteEventsCalled {
			t.Error("expected DeleteExpiredEvents to be called")
		}
		if !mock.deleteDiffsCalled {
			t.Error("expected DeleteExpiredDiffs to be called")
		}
		if !mock.deleteArtifactsCalled {
			t.Error("expected DeleteExpiredArtifactBundles to be called")
		}
	})

}
