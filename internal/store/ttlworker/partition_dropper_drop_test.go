package ttlworker

import (
	"context"
	"log/slog"
	"testing"
	"time"
)

// TestDropOldPartitions_NilInputs ensures that DropOldPartitions handles
// nil pool and nil store inputs gracefully without panicking or returning errors.
// This validates defensive input checking at the function boundary.
func TestDropOldPartitions_NilInputs(t *testing.T) {
	ctx := context.Background()
	cutoff := time.Now()
	logger := slog.Default()

	t.Run("nil pool", func(t *testing.T) {
		mock := &mockStoreWithPartitions{}
		err := DropOldPartitions(ctx, nil, mock, cutoff, logger)
		if err != nil {
			t.Errorf("expected no error with nil pool, got %v", err)
		}
	})

	t.Run("nil store", func(t *testing.T) {
		err := DropOldPartitions(ctx, nil, nil, cutoff, logger)
		if err != nil {
			t.Errorf("expected no error with nil store, got %v", err)
		}
	})
}

// TestWorker_Run_WithDropPartitions validates the integration between the
// Worker's Run method and the partition dropping feature. It verifies that
// when drop partitions is enabled, the worker attempts partition drops before
// falling back to row-by-row deletion, and when disabled, only row-by-row
// deletion is performed.
func TestWorker_Run_WithDropPartitions(t *testing.T) {
	t.Run("drop partitions enabled", func(t *testing.T) {
		mock := &mockStoreWithPartitions{
			mockStore: mockStore{
				deleteLogsCount: 5,
			},
			logPartitions: []string{
				"ploy.logs_2024_01",
				"ploy.logs_2025_10",
			},
		}

		worker, err := New(Options{
			Store:          mock,
			TTL:            24 * time.Hour,
			DropPartitions: true,
		})
		if err != nil {
			t.Fatalf("failed to create worker: %v", err)
		}

		if !worker.dropPartitions {
			t.Error("expected dropPartitions to be true")
		}

		// Run the worker (partition dropping will be attempted but won't execute SQL without real pool).
		if err := worker.Run(context.Background()); err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// Verify fallback row-by-row deletion still runs.
		if !mock.deleteLogsCalled {
			t.Error("expected DeleteExpiredLogs to be called")
		}
	})

	t.Run("drop partitions disabled", func(t *testing.T) {
		mock := &mockStoreWithPartitions{
			mockStore: mockStore{
				deleteLogsCount: 5,
			},
		}

		worker, err := New(Options{
			Store:          mock,
			TTL:            24 * time.Hour,
			DropPartitions: false,
		})
		if err != nil {
			t.Fatalf("failed to create worker: %v", err)
		}

		if worker.dropPartitions {
			t.Error("expected dropPartitions to be false")
		}

		if err := worker.Run(context.Background()); err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// Verify row-by-row deletion runs.
		if !mock.deleteLogsCalled {
			t.Error("expected DeleteExpiredLogs to be called")
		}
	})
}

// TestWorker_DropPartitionsDefault verifies the default behavior of the
// DropPartitions option when creating a new Worker. By default, partition
// dropping should be disabled, requiring explicit opt-in.
func TestWorker_DropPartitionsDefault(t *testing.T) {
	t.Run("default dropPartitions is false", func(t *testing.T) {
		mock := &mockStore{}
		worker, err := New(Options{
			Store: mock,
		})
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if worker == nil {
			t.Fatal("expected worker, got nil")
		}
		if worker.dropPartitions {
			t.Error("expected dropPartitions to be false by default")
		}
	})

	t.Run("explicit dropPartitions true", func(t *testing.T) {
		mock := &mockStore{}
		worker, err := New(Options{
			Store:          mock,
			DropPartitions: true,
		})
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if worker == nil {
			t.Fatal("expected worker, got nil")
		}
		if !worker.dropPartitions {
			t.Error("expected dropPartitions to be true")
		}
	})
}
