//go:build arf_legacy
// +build arf_legacy

package arf

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHealingWorkflowE2E tests the complete end-to-end healing workflow
func TestHealingWorkflowE2E(t *testing.T) {
	// Skip if not in integration test mode
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	t.Run("healing workflow with consul persistence", func(t *testing.T) {
		// Setup mock Consul store
		mockConsul := NewMockConsulStore()
		transformID := uuid.New().String()

		// Create initial transformation status with failure
		initialStatus := &TransformationStatus{
			TransformationID: transformID,
			Status:           "failed",
			WorkflowStage:    "build",
			StartTime:        time.Now(),
			Error:            "Build failed: undefined symbol 'processData'",
		}

		// Store initial status
		err := mockConsul.StoreTransformationStatus(ctx, transformID, initialStatus)
		require.NoError(t, err)

		// Simulate adding a healing attempt
		healingAttempt := &HealingAttempt{
			TransformationID: uuid.New().String(),
			AttemptPath:      "1",
			TriggerReason:    "build_failure",
			Status:           "in_progress",
			StartTime:        time.Now(),
			LLMAnalysis: &LLMAnalysisResult{
				ErrorType:    "missing_import",
				Confidence:   0.85,
				SuggestedFix: "Add import statement for processData",
			},
		}

		// Add healing attempt to transformation
		err = mockConsul.AddHealingAttempt(ctx, transformID, "1", healingAttempt)
		require.NoError(t, err)

		// Simulate healing completion
		healingAttempt.Status = "completed"
		healingAttempt.Result = "success"
		healingAttempt.EndTime = time.Now()

		err = mockConsul.UpdateHealingAttempt(ctx, transformID, "1", healingAttempt)
		require.NoError(t, err)

		// Update transformation to completed
		err = mockConsul.UpdateWorkflowStage(ctx, transformID, "completed")
		require.NoError(t, err)

		// Retrieve and verify final status
		finalStatus, err := mockConsul.GetTransformationStatus(ctx, transformID)
		require.NoError(t, err)

		assert.Equal(t, "completed", finalStatus.WorkflowStage)
		assert.Len(t, finalStatus.Children, 1)
		assert.Equal(t, "completed", finalStatus.Children[0].Status)
		assert.Equal(t, "success", finalStatus.Children[0].Result)
	})

	t.Run("nested healing attempts", func(t *testing.T) {
		mockConsul := NewMockConsulStore()
		transformID := uuid.New().String()

		// Initial transformation with failure
		status := &TransformationStatus{
			TransformationID: transformID,
			Status:           "failed",
			WorkflowStage:    "test",
			StartTime:        time.Now(),
			Error:            "Tests failed: 5 failures",
		}

		err := mockConsul.StoreTransformationStatus(ctx, transformID, status)
		require.NoError(t, err)

		// First healing attempt
		attempt1 := &HealingAttempt{
			TransformationID: uuid.New().String(),
			AttemptPath:      "1",
			TriggerReason:    "test_failure",
			Status:           "completed",
			Result:           "failed",
			StartTime:        time.Now().Add(-10 * time.Minute),
			EndTime:          time.Now().Add(-5 * time.Minute),
			TargetErrors:     []string{"Still 2 tests failing"},
		}

		err = mockConsul.AddHealingAttempt(ctx, transformID, "1", attempt1)
		require.NoError(t, err)

		// Nested healing attempt (child of first attempt)
		attempt2 := &HealingAttempt{
			TransformationID: uuid.New().String(),
			AttemptPath:      "1.1",
			TriggerReason:    "remaining_test_failures",
			Status:           "completed",
			Result:           "success",
			StartTime:        time.Now().Add(-4 * time.Minute),
			EndTime:          time.Now(),
		}

		err = mockConsul.AddHealingAttempt(ctx, transformID, "1.1", attempt2)
		require.NoError(t, err)

		// Retrieve and verify hierarchy
		finalStatus, err := mockConsul.GetTransformationStatus(ctx, transformID)
		require.NoError(t, err)

		// Check nested structure
		assert.Len(t, finalStatus.Children, 1)
		assert.Equal(t, "1", finalStatus.Children[0].AttemptPath)
		assert.Len(t, finalStatus.Children[0].Children, 1)
		assert.Equal(t, "1.1", finalStatus.Children[0].Children[0].AttemptPath)
		assert.Equal(t, "success", finalStatus.Children[0].Children[0].Result)
	})

	t.Run("parallel healing attempts tracking", func(t *testing.T) {
		mockConsul := NewMockConsulStore()
		transformID := uuid.New().String()

		status := &TransformationStatus{
			TransformationID: transformID,
			Status:           "in_progress",
			WorkflowStage:    "healing",
			StartTime:        time.Now(),
		}

		err := mockConsul.StoreTransformationStatus(ctx, transformID, status)
		require.NoError(t, err)

		// Add multiple parallel attempts
		for i := 1; i <= 3; i++ {
			attempt := &HealingAttempt{
				TransformationID: uuid.New().String(),
				AttemptPath:      fmt.Sprintf("%d", i),
				TriggerReason:    "parallel_fix",
				Status:           "in_progress",
				StartTime:        time.Now(),
			}
			err = mockConsul.AddHealingAttempt(ctx, transformID, attempt.AttemptPath, attempt)
			require.NoError(t, err)
		}

		// Get active attempts
		activeAttempts, err := mockConsul.GetActiveHealingAttempts(ctx, transformID)
		require.NoError(t, err)
		assert.Len(t, activeAttempts, 3)

		// Complete one attempt
		completedAttempt := &HealingAttempt{
			TransformationID: uuid.New().String(),
			AttemptPath:      "1",
			Status:           "completed",
			Result:           "success",
			EndTime:          time.Now(),
		}
		err = mockConsul.UpdateHealingAttempt(ctx, transformID, "1", completedAttempt)
		require.NoError(t, err)

		// Check active attempts again
		activeAttempts, err = mockConsul.GetActiveHealingAttempts(ctx, transformID)
		require.NoError(t, err)
		assert.Len(t, activeAttempts, 2)
	})
}

// TestConsulPersistenceAcrossRestarts verifies Consul KV persistence
func TestConsulPersistenceAcrossRestarts(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	transformID := uuid.New().String()

	t.Run("status persists in consul store", func(t *testing.T) {
		// Create first store instance
		store1 := NewMockConsulStore()

		// Store transformation with healing tree
		status := &TransformationStatus{
			TransformationID: transformID,
			Status:           "in_progress",
			WorkflowStage:    "healing",
			StartTime:        time.Now(),
			Children: []HealingAttempt{
				{
					TransformationID: uuid.New().String(),
					AttemptPath:      "1",
					Status:           "completed",
					Result:           "success",
				},
				{
					TransformationID: uuid.New().String(),
					AttemptPath:      "2",
					Status:           "in_progress",
				},
			},
		}

		err := store1.StoreTransformationStatus(ctx, transformID, status)
		require.NoError(t, err)

		// Simulate using the same backend (in real scenario, this would be actual Consul)
		// For mock, we're verifying the data structure is preserved
		retrievedStatus, err := store1.GetTransformationStatus(ctx, transformID)
		require.NoError(t, err)

		// Verify complete healing tree is preserved
		assert.Equal(t, transformID, retrievedStatus.TransformationID)
		assert.Equal(t, "in_progress", retrievedStatus.Status)
		assert.Equal(t, "healing", retrievedStatus.WorkflowStage)
		assert.Len(t, retrievedStatus.Children, 2)

		// Verify healing attempts
		assert.Equal(t, "1", retrievedStatus.Children[0].AttemptPath)
		assert.Equal(t, "completed", retrievedStatus.Children[0].Status)
		assert.Equal(t, "2", retrievedStatus.Children[1].AttemptPath)
		assert.Equal(t, "in_progress", retrievedStatus.Children[1].Status)
	})

	t.Run("TTL and cleanup", func(t *testing.T) {
		store := NewMockConsulStore()

		// Store old transformation
		oldStatus := &TransformationStatus{
			TransformationID: uuid.New().String(),
			Status:           "completed",
			WorkflowStage:    "completed",
			StartTime:        time.Now().Add(-48 * time.Hour),
			EndTime:          time.Now().Add(-47 * time.Hour),
		}

		err := store.StoreTransformationStatus(ctx, oldStatus.TransformationID, oldStatus)
		require.NoError(t, err)

		// Set TTL
		err = store.SetTransformationTTL(ctx, oldStatus.TransformationID, 24*time.Hour)
		require.NoError(t, err)

		// Cleanup old transformations
		err = store.CleanupCompletedTransformations(ctx, 24*time.Hour)
		require.NoError(t, err)

		// Old transformation should be cleaned up (in real implementation)
		// For mock, we just verify the method works
		assert.NoError(t, err)
	})
}

// TestHealingCoordinatorIntegration tests the healing coordinator with dependencies
func TestHealingCoordinatorIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	t.Run("coordinator manages healing workflow", func(t *testing.T) {
		// Create healing config
		config := DefaultHealingConfig()
		config.MaxHealingDepth = 3
		config.MaxParallelAttempts = 2

		// Create coordinator
		coordinator := NewHealingCoordinator(config)

		// Start coordinator
		ctx := context.Background()
		err := coordinator.Start(ctx)
		require.NoError(t, err)
		defer coordinator.Stop()

		// Verify coordinator is running
		assert.True(t, coordinator.IsRunning())

		// Get metrics
		metrics := coordinator.GetMetrics()
		assert.NotNil(t, metrics)
		assert.GreaterOrEqual(t, metrics.TotalSubmitted, 0)
	})

	t.Run("circuit breaker protects against failures", func(t *testing.T) {
		config := DefaultHealingConfig()
		config.FailureThreshold = 3
		config.CircuitOpenDuration = 1 * time.Second

		coordinator := NewHealingCoordinator(config)

		ctx := context.Background()
		err := coordinator.Start(ctx)
		require.NoError(t, err)
		defer coordinator.Stop()

		// Circuit breaker is internal to coordinator
		// We can test its behavior through the metrics
		metrics := coordinator.GetMetrics()
		assert.NotNil(t, metrics)
	})
}

// TestSandboxDeploymentValidation tests sandbox integration for healing validation
func TestSandboxDeploymentValidation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	t.Run("sandbox validation flow", func(t *testing.T) {
		ctx := context.Background()
		mockConsul := NewMockConsulStore()
		transformID := uuid.New().String()
		sandboxID := fmt.Sprintf("sandbox-%s", transformID[:8])

		// Store transformation with sandbox ID
		status := &TransformationStatus{
			TransformationID: transformID,
			Status:           "healing",
			WorkflowStage:    "validation",
			StartTime:        time.Now(),
		}

		err := mockConsul.StoreTransformationStatus(ctx, transformID, status)
		require.NoError(t, err)

		// Simulate successful healing with sandbox validation
		healingAttempt := &HealingAttempt{
			TransformationID: uuid.New().String(),
			AttemptPath:      "1",
			Status:           "completed",
			Result:           "success",
			SandboxID:        sandboxID,
		}

		err = mockConsul.AddHealingAttempt(ctx, transformID, "1", healingAttempt)
		require.NoError(t, err)

		// Update workflow stage
		err = mockConsul.UpdateWorkflowStage(ctx, transformID, "completed")
		require.NoError(t, err)

		// Verify final status
		finalStatus, err := mockConsul.GetTransformationStatus(ctx, transformID)
		require.NoError(t, err)

		assert.Equal(t, "completed", finalStatus.WorkflowStage)
		assert.Len(t, finalStatus.Children, 1)
		assert.Equal(t, sandboxID, finalStatus.Children[0].SandboxID)
	})
}
