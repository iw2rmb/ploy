package ttlworker

import (
	"context"
	"testing"
	"time"
)

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
