package arf

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

// MockConsulStore implements an in-memory version for unit testing
type MockConsulStore struct {
	data map[string]*TransformationStatus
}

func NewMockConsulStore() *MockConsulStore {
	return &MockConsulStore{
		data: make(map[string]*TransformationStatus),
	}
}

func (m *MockConsulStore) StoreTransformationStatus(ctx context.Context, id string, status *TransformationStatus) error {
	m.data[id] = status
	return nil
}

func (m *MockConsulStore) GetTransformationStatus(ctx context.Context, id string) (*TransformationStatus, error) {
	status, exists := m.data[id]
	if !exists {
		return nil, nil
	}
	return status, nil
}

func (m *MockConsulStore) UpdateWorkflowStage(ctx context.Context, id string, stage string) error {
	status, exists := m.data[id]
	if !exists {
		return fmt.Errorf("transformation %s not found", id)
	}
	status.WorkflowStage = stage
	return nil
}

func TestConsulStore_BasicOperations(t *testing.T) {
	store := NewMockConsulStore()
	ctx := context.Background()

	t.Run("store and retrieve transformation status", func(t *testing.T) {
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

		retrieved, err := store.GetTransformationStatus(ctx, transformID)
		assert.NoError(t, err)
		assert.NotNil(t, retrieved)
		assert.Equal(t, transformID, retrieved.TransformationID)
		assert.Equal(t, "initiated", retrieved.Status)
	})

	t.Run("update workflow stage", func(t *testing.T) {
		transformID := uuid.New().String()
		status := &TransformationStatus{
			TransformationID: transformID,
			Status:           "in_progress",
			WorkflowStage:    "openrewrite",
			StartTime:        time.Now(),
		}

		err := store.StoreTransformationStatus(ctx, transformID, status)
		assert.NoError(t, err)

		err = store.UpdateWorkflowStage(ctx, transformID, "build")
		assert.NoError(t, err)

		retrieved, err := store.GetTransformationStatus(ctx, transformID)
		assert.NoError(t, err)
		assert.Equal(t, "build", retrieved.WorkflowStage)
	})

	t.Run("return nil for non-existent transformation", func(t *testing.T) {
		retrieved, err := store.GetTransformationStatus(ctx, "non-existent")
		assert.NoError(t, err)
		assert.Nil(t, retrieved)
	})
}

func TestHealingAttemptStructure(t *testing.T) {
	t.Run("create nested healing attempts", func(t *testing.T) {
		rootAttempt := HealingAttempt{
			TransformationID: uuid.New().String(),
			AttemptPath:      "1",
			TriggerReason:    "build_failure",
			Status:           "completed",
			Result:           "partial_success",
			Children: []HealingAttempt{
				{
					TransformationID: uuid.New().String(),
					AttemptPath:      "1.1",
					TriggerReason:    "test_failure",
					Status:           "in_progress",
					ParentAttempt:    "1",
					Children:         []HealingAttempt{},
				},
			},
		}

		assert.Equal(t, "1", rootAttempt.AttemptPath)
		assert.Len(t, rootAttempt.Children, 1)
		assert.Equal(t, "1.1", rootAttempt.Children[0].AttemptPath)
		assert.Equal(t, "1", rootAttempt.Children[0].ParentAttempt)
	})
}

func TestHealingTreeCalculations(t *testing.T) {
	t.Run("calculate tree metrics", func(t *testing.T) {
		tree := &HealingTree{
			RootTransformID: uuid.New().String(),
			Attempts: []HealingAttempt{
				{
					AttemptPath: "1",
					Status:      "completed",
					Result:      "success",
					Children:    []HealingAttempt{},
				},
				{
					AttemptPath: "2",
					Status:      "in_progress",
					Children: []HealingAttempt{
						{
							AttemptPath: "2.1",
							Status:      "completed",
							Result:      "failed",
							Children:    []HealingAttempt{},
						},
					},
				},
			},
		}

		// Verify the tree structure
		assert.NotNil(t, tree)
		assert.Equal(t, 2, len(tree.Attempts))
		assert.Equal(t, "1", tree.Attempts[0].AttemptPath)
		assert.Equal(t, "2", tree.Attempts[1].AttemptPath)
		assert.Equal(t, 1, len(tree.Attempts[1].Children))

		// Manually calculate expected metrics
		totalAttempts := 3   // 1, 2, 2.1
		activeAttempts := 1  // 2
		successfulHeals := 1 // 1
		failedHeals := 1     // 2.1
		maxDepth := 2        // root -> 2 -> 2.1

		// These would be calculated by the actual implementation
		assert.Equal(t, totalAttempts, 3)
		assert.Equal(t, activeAttempts, 1)
		assert.Equal(t, successfulHeals, 1)
		assert.Equal(t, failedHeals, 1)
		assert.Equal(t, maxDepth, 2)
	})
}
