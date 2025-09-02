package arf

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealingCoordinator_SimpleTest(t *testing.T) {
	coordinator := NewHealingCoordinator(&HealingConfig{
		MaxParallelAttempts: 2,
		MaxTotalAttempts:    10,
		MaxHealingDepth:     5,
		QueueSize:           3, // Small queue for testing
	})

	ctx := context.Background()
	err := coordinator.Start(ctx)
	require.NoError(t, err)
	defer coordinator.Stop()

	var completed int32

	// Submit a task that takes time
	task1 := &HealingTask{
		TransformID: "task1",
		AttemptPath: "1",
		ExecuteFn: func(ctx context.Context) error {
			time.Sleep(100 * time.Millisecond)
			atomic.AddInt32(&completed, 1)
			return nil
		},
	}
	err = coordinator.SubmitTask(ctx, task1)
	require.NoError(t, err)

	// Verify basic functionality
	time.Sleep(50 * time.Millisecond)
	metrics := coordinator.GetMetrics()
	assert.GreaterOrEqual(t, metrics.TotalSubmitted, 1)

	// Wait for completion
	require.Eventually(t, func() bool {
		return atomic.LoadInt32(&completed) == 1
	}, 2*time.Second, 10*time.Millisecond)

	finalMetrics := coordinator.GetMetrics()
	assert.Equal(t, 1, finalMetrics.CompletedTasks)
}
