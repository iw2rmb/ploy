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
	t.Run("nil store returns nil worker", func(t *testing.T) {
		worker, err := New(Options{Store: nil})
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if worker != nil {
			t.Errorf("expected nil worker, got %v", worker)
		}
	})

	t.Run("default TTL is 30 days", func(t *testing.T) {
		mock := &mockStore{}
		worker, err := New(Options{Store: mock})
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if worker == nil {
			t.Fatal("expected worker, got nil")
		}
		expected := 30 * 24 * time.Hour
		if worker.ttl != expected {
			t.Errorf("expected TTL %v, got %v", expected, worker.ttl)
		}
	})

	t.Run("default interval is 1 hour", func(t *testing.T) {
		mock := &mockStore{}
		worker, err := New(Options{Store: mock})
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if worker == nil {
			t.Fatal("expected worker, got nil")
		}
		expected := time.Hour
		if worker.interval != expected {
			t.Errorf("expected interval %v, got %v", expected, worker.interval)
		}
	})

	t.Run("custom TTL and interval", func(t *testing.T) {
		mock := &mockStore{}
		customTTL := 7 * 24 * time.Hour
		customInterval := 30 * time.Minute
		worker, err := New(Options{
			Store:    mock,
			TTL:      customTTL,
			Interval: customInterval,
		})
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if worker == nil {
			t.Fatal("expected worker, got nil")
		}
		if worker.ttl != customTTL {
			t.Errorf("expected TTL %v, got %v", customTTL, worker.ttl)
		}
		if worker.interval != customInterval {
			t.Errorf("expected interval %v, got %v", customInterval, worker.interval)
		}
	})
}

func TestWorker_Name(t *testing.T) {
	mock := &mockStore{}
	worker, _ := New(Options{Store: mock})
	if worker.Name() != "ttl-worker" {
		t.Errorf("expected name 'ttl-worker', got %q", worker.Name())
	}
}

func TestWorker_Interval(t *testing.T) {
	mock := &mockStore{}
	expected := 2 * time.Hour
	worker, _ := New(Options{Store: mock, Interval: expected})
	if worker.Interval() != expected {
		t.Errorf("expected interval %v, got %v", expected, worker.Interval())
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

	t.Run("continues on error", func(t *testing.T) {
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

		// Should not return error even if individual operations fail.
		if err := worker.Run(context.Background()); err != nil {
			t.Errorf("expected no error, got %v", err)
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

	t.Run("deletes rows older than horizon", func(t *testing.T) {
		mock := &mockStore{
			deleteLogsCount:      15,
			deleteEventsCount:    25,
			deleteDiffsCount:     8,
			deleteArtifactsCount: 4,
		}

		// Use a specific TTL for predictable cutoff calculation.
		ttl := 7 * 24 * time.Hour // 7 days
		worker, err := New(Options{
			Store: mock,
			TTL:   ttl,
		})
		if err != nil {
			t.Fatalf("failed to create worker: %v", err)
		}

		// Capture the execution window to verify cutoff.
		before := time.Now().Add(-ttl)
		if err := worker.Run(context.Background()); err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		after := time.Now().Add(-ttl)

		// Verify all delete operations were called.
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

		// Verify cutoff timestamps are valid and within expected range.
		// The cutoff should be approximately (now - TTL).
		if !mock.lastLogsArg.Valid {
			t.Error("logs cutoff timestamp is invalid")
		} else if mock.lastLogsArg.Time.Before(before) || mock.lastLogsArg.Time.After(after) {
			t.Errorf("logs cutoff %v outside expected window [%v, %v]",
				mock.lastLogsArg.Time, before, after)
		}

		if !mock.lastEventsArg.Valid {
			t.Error("events cutoff timestamp is invalid")
		} else if mock.lastEventsArg.Time.Before(before) || mock.lastEventsArg.Time.After(after) {
			t.Errorf("events cutoff %v outside expected window [%v, %v]",
				mock.lastEventsArg.Time, before, after)
		}

		if !mock.lastDiffsArg.Valid {
			t.Error("diffs cutoff timestamp is invalid")
		} else if mock.lastDiffsArg.Time.Before(before) || mock.lastDiffsArg.Time.After(after) {
			t.Errorf("diffs cutoff %v outside expected window [%v, %v]",
				mock.lastDiffsArg.Time, before, after)
		}

		if !mock.lastArtifactsArg.Valid {
			t.Error("artifacts cutoff timestamp is invalid")
		} else if mock.lastArtifactsArg.Time.Before(before) || mock.lastArtifactsArg.Time.After(after) {
			t.Errorf("artifacts cutoff %v outside expected window [%v, %v]",
				mock.lastArtifactsArg.Time, before, after)
		}

		// Verify that the mock return values are being processed correctly.
		// The actual deletion counts are logged but not returned by Run(),
		// so we just verify the operations completed.
	})
}
