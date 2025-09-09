//go:build arf_legacy
// +build arf_legacy

package arf

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/consul/sdk/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestConsulServer(t *testing.T) (*testutil.TestServer, *api.Client) {
	// Create test Consul server
	srv, err := testutil.NewTestServerConfigT(t, nil)
	require.NoError(t, err)

	// Create client
	config := api.DefaultConfig()
	config.Address = srv.HTTPAddr
	client, err := api.NewClient(config)
	require.NoError(t, err)

	return srv, client
}

func TestConsulHealingStore_StoreTransformationStatus(t *testing.T) {
	srv, client := setupTestConsulServer(t)
	defer srv.Stop()

	store := NewConsulHealingStore(client, "ploy/arf/transforms")
	ctx := context.Background()

	t.Run("store initial transformation status", func(t *testing.T) {
		transformID := uuid.New().String()
		status := &TransformationStatus{
			TransformationID: transformID,
			Status:           "initiated",
			WorkflowStage:    "openrewrite",
			StartTime:        time.Now(),
			Children:         []HealingAttempt{},
		}

		err := store.StoreTransformationStatus(ctx, transformID, status)
		assert.NoError(t, err)

		// Verify it was stored
		retrieved, err := store.GetTransformationStatus(ctx, transformID)
		assert.NoError(t, err)
		assert.NotNil(t, retrieved)
		assert.Equal(t, transformID, retrieved.TransformationID)
		assert.Equal(t, "initiated", retrieved.Status)
		assert.Equal(t, "openrewrite", retrieved.WorkflowStage)
	})

	t.Run("update existing transformation status", func(t *testing.T) {
		transformID := uuid.New().String()

		// Store initial status
		initial := &TransformationStatus{
			TransformationID: transformID,
			Status:           "initiated",
			WorkflowStage:    "openrewrite",
			StartTime:        time.Now(),
		}
		err := store.StoreTransformationStatus(ctx, transformID, initial)
		require.NoError(t, err)

		// Update status
		updated := &TransformationStatus{
			TransformationID: transformID,
			Status:           "in_progress",
			WorkflowStage:    "build",
			StartTime:        initial.StartTime,
			Progress: &TransformationProgress{
				Stage:           "compilation",
				PercentComplete: 45,
			},
		}
		err = store.StoreTransformationStatus(ctx, transformID, updated)
		assert.NoError(t, err)

		// Verify update
		retrieved, err := store.GetTransformationStatus(ctx, transformID)
		assert.NoError(t, err)
		assert.Equal(t, "in_progress", retrieved.Status)
		assert.Equal(t, "build", retrieved.WorkflowStage)
		assert.NotNil(t, retrieved.Progress)
		assert.Equal(t, 45, retrieved.Progress.PercentComplete)
	})
}

func TestConsulHealingStore_HealingAttempts(t *testing.T) {
	srv, client := setupTestConsulServer(t)
	defer srv.Stop()

	store := NewConsulHealingStore(client, "ploy/arf/transforms")
	ctx := context.Background()

	t.Run("add healing attempt to transformation", func(t *testing.T) {
		transformID := uuid.New().String()

		// Store initial transformation
		status := &TransformationStatus{
			TransformationID: transformID,
			Status:           "healing",
			WorkflowStage:    "heal",
			StartTime:        time.Now(),
			Children:         []HealingAttempt{},
		}
		err := store.StoreTransformationStatus(ctx, transformID, status)
		require.NoError(t, err)

		// Add healing attempt
		attempt := &HealingAttempt{
			TransformationID: uuid.New().String(),
			AttemptPath:      "1",
			TriggerReason:    "build_failure",
			TargetErrors:     []string{"compilation_error"},
			Status:           "in_progress",
			StartTime:        time.Now(),
			Children:         []HealingAttempt{},
		}

		err = store.AddHealingAttempt(ctx, transformID, "1", attempt)
		assert.NoError(t, err)

		// Verify attempt was added
		retrieved, err := store.GetTransformationStatus(ctx, transformID)
		assert.NoError(t, err)
		assert.Len(t, retrieved.Children, 1)
		assert.Equal(t, "1", retrieved.Children[0].AttemptPath)
		assert.Equal(t, "build_failure", retrieved.Children[0].TriggerReason)
	})

	t.Run("add nested healing attempt", func(t *testing.T) {
		transformID := uuid.New().String()

		// Store transformation with initial healing attempt
		status := &TransformationStatus{
			TransformationID: transformID,
			Status:           "healing",
			WorkflowStage:    "heal",
			StartTime:        time.Now(),
			Children: []HealingAttempt{
				{
					TransformationID:    uuid.New().String(),
					AttemptPath:         "1",
					TriggerReason:       "build_failure",
					Status:              "completed",
					Result:              "partial_success",
					NewIssuesDiscovered: []string{"test_failure"},
					Children:            []HealingAttempt{},
				},
			},
		}
		err := store.StoreTransformationStatus(ctx, transformID, status)
		require.NoError(t, err)

		// Add nested healing attempt
		nestedAttempt := &HealingAttempt{
			TransformationID: uuid.New().String(),
			AttemptPath:      "1.1",
			TriggerReason:    "test_failure_after_heal",
			TargetErrors:     []string{"test_failure_integration"},
			Status:           "in_progress",
			StartTime:        time.Now(),
			ParentAttempt:    "1",
			Children:         []HealingAttempt{},
		}

		err = store.AddHealingAttempt(ctx, transformID, "1.1", nestedAttempt)
		assert.NoError(t, err)

		// Verify nested structure
		retrieved, err := store.GetTransformationStatus(ctx, transformID)
		assert.NoError(t, err)
		assert.Len(t, retrieved.Children, 1)
		assert.Len(t, retrieved.Children[0].Children, 1)
		assert.Equal(t, "1.1", retrieved.Children[0].Children[0].AttemptPath)
	})
}

func TestConsulHealingStore_UpdateWorkflowStage(t *testing.T) {
	srv, client := setupTestConsulServer(t)
	defer srv.Stop()

	store := NewConsulHealingStore(client, "ploy/arf/transforms")
	ctx := context.Background()

	transformID := uuid.New().String()

	// Store initial status
	status := &TransformationStatus{
		TransformationID: transformID,
		Status:           "in_progress",
		WorkflowStage:    "openrewrite",
		StartTime:        time.Now(),
	}
	err := store.StoreTransformationStatus(ctx, transformID, status)
	require.NoError(t, err)

	// Update workflow stage
	err = store.UpdateWorkflowStage(ctx, transformID, "build")
	assert.NoError(t, err)

	// Verify update
	retrieved, err := store.GetTransformationStatus(ctx, transformID)
	assert.NoError(t, err)
	assert.Equal(t, "build", retrieved.WorkflowStage)
}

func TestConsulHealingStore_GetHealingTree(t *testing.T) {
	srv, client := setupTestConsulServer(t)
	defer srv.Stop()

	store := NewConsulHealingStore(client, "ploy/arf/transforms")
	ctx := context.Background()

	transformID := uuid.New().String()

	// Store transformation with complex healing tree
	status := &TransformationStatus{
		TransformationID: transformID,
		Status:           "healing",
		WorkflowStage:    "heal",
		StartTime:        time.Now(),
		Children: []HealingAttempt{
			{
				TransformationID: uuid.New().String(),
				AttemptPath:      "1",
				Status:           "completed",
				Result:           "partial_success",
				Children: []HealingAttempt{
					{
						TransformationID: uuid.New().String(),
						AttemptPath:      "1.1",
						Status:           "in_progress",
						Children:         []HealingAttempt{},
					},
				},
			},
			{
				TransformationID: uuid.New().String(),
				AttemptPath:      "2",
				Status:           "completed",
				Result:           "success",
				Children:         []HealingAttempt{},
			},
		},
	}
	err := store.StoreTransformationStatus(ctx, transformID, status)
	require.NoError(t, err)

	// Get healing tree
	tree, err := store.GetHealingTree(ctx, transformID)
	assert.NoError(t, err)
	assert.NotNil(t, tree)
	assert.Equal(t, transformID, tree.RootTransformID)
	assert.Len(t, tree.Attempts, 2)
	assert.Equal(t, 3, tree.TotalAttempts)
	assert.Equal(t, 1, tree.SuccessfulHeals)
	assert.Equal(t, 2, tree.MaxDepth)
}

func TestConsulHealingStore_GetActiveHealingAttempts(t *testing.T) {
	srv, client := setupTestConsulServer(t)
	defer srv.Stop()

	store := NewConsulHealingStore(client, "ploy/arf/transforms")
	ctx := context.Background()

	transformID := uuid.New().String()

	// Store transformation with mixed status attempts
	status := &TransformationStatus{
		TransformationID: transformID,
		Status:           "healing",
		WorkflowStage:    "heal",
		StartTime:        time.Now(),
		Children: []HealingAttempt{
			{
				TransformationID: uuid.New().String(),
				AttemptPath:      "1",
				Status:           "in_progress",
				Children:         []HealingAttempt{},
			},
			{
				TransformationID: uuid.New().String(),
				AttemptPath:      "2",
				Status:           "completed",
				Children: []HealingAttempt{
					{
						TransformationID: uuid.New().String(),
						AttemptPath:      "2.1",
						Status:           "in_progress",
						Children:         []HealingAttempt{},
					},
				},
			},
		},
	}
	err := store.StoreTransformationStatus(ctx, transformID, status)
	require.NoError(t, err)

	// Get active attempts
	active, err := store.GetActiveHealingAttempts(ctx, transformID)
	assert.NoError(t, err)
	assert.Len(t, active, 2)
	assert.Contains(t, active, "1")
	assert.Contains(t, active, "2.1")
}

func TestConsulHealingStore_CleanupCompletedTransformations(t *testing.T) {
	srv, client := setupTestConsulServer(t)
	defer srv.Stop()

	store := NewConsulHealingStore(client, "ploy/arf/transforms")
	ctx := context.Background()

	// Store old and new transformations
	oldTransformID := uuid.New().String()
	oldStatus := &TransformationStatus{
		TransformationID: oldTransformID,
		Status:           "completed",
		WorkflowStage:    "completed",
		StartTime:        time.Now().Add(-48 * time.Hour),
		EndTime:          time.Now().Add(-47 * time.Hour),
	}
	err := store.StoreTransformationStatus(ctx, oldTransformID, oldStatus)
	require.NoError(t, err)

	newTransformID := uuid.New().String()
	newStatus := &TransformationStatus{
		TransformationID: newTransformID,
		Status:           "completed",
		WorkflowStage:    "completed",
		StartTime:        time.Now().Add(-1 * time.Hour),
		EndTime:          time.Now(),
	}
	err = store.StoreTransformationStatus(ctx, newTransformID, newStatus)
	require.NoError(t, err)

	// Cleanup old transformations (older than 24 hours)
	err = store.CleanupCompletedTransformations(ctx, 24*time.Hour)
	assert.NoError(t, err)

	// Verify old was deleted
	oldRetrieved, err := store.GetTransformationStatus(ctx, oldTransformID)
	assert.NoError(t, err)
	assert.Nil(t, oldRetrieved)

	// Verify new still exists
	newRetrieved, err := store.GetTransformationStatus(ctx, newTransformID)
	assert.NoError(t, err)
	assert.NotNil(t, newRetrieved)
	assert.Equal(t, newTransformID, newRetrieved.TransformationID)
}

func TestConsulHealingStore_SetTransformationTTL(t *testing.T) {
	srv, client := setupTestConsulServer(t)
	defer srv.Stop()

	store := NewConsulHealingStore(client, "ploy/arf/transforms")
	ctx := context.Background()

	transformID := uuid.New().String()

	// Store transformation
	status := &TransformationStatus{
		TransformationID: transformID,
		Status:           "completed",
		WorkflowStage:    "completed",
		StartTime:        time.Now(),
	}
	err := store.StoreTransformationStatus(ctx, transformID, status)
	require.NoError(t, err)

	// Set TTL
	err = store.SetTransformationTTL(ctx, transformID, 5*time.Second)
	assert.NoError(t, err)

	// Verify it exists now
	retrieved, err := store.GetTransformationStatus(ctx, transformID)
	assert.NoError(t, err)
	assert.NotNil(t, retrieved)

	// Wait for TTL to expire
	time.Sleep(6 * time.Second)

	// Verify it's gone
	retrieved, err = store.GetTransformationStatus(ctx, transformID)
	assert.NoError(t, err)
	assert.Nil(t, retrieved)
}
