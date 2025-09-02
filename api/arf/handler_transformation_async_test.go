package arf

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockRecipeExecutor for testing
type MockRecipeExecutor struct {
	shouldFail bool
}

func NewMockRecipeExecutor() *MockRecipeExecutor {
	return &MockRecipeExecutor{}
}

func (m *MockRecipeExecutor) ExecuteRecipeByID(ctx context.Context, recipeID string, repoPath string, recipeType string, transformID string) (*TransformationResult, error) {
	if m.shouldFail {
		return nil, fmt.Errorf("mock execution failed")
	}
	return &TransformationResult{
		RecipeID:       recipeID,
		Success:        true,
		ChangesApplied: 5,
		ExecutionTime:  100 * time.Millisecond,
	}, nil
}

// MockConsulHealingStore implements the ConsulHealingStore interface for testing
type MockConsulHealingStore struct {
	MockConsulStore
}

func (m *MockConsulHealingStore) AddHealingAttempt(ctx context.Context, rootID, attemptPath string, attempt *HealingAttempt) error {
	status, exists := m.data[rootID]
	if !exists {
		return fmt.Errorf("transformation %s not found", rootID)
	}
	status.Children = append(status.Children, *attempt)
	status.TotalHealingAttempts++
	if attempt.Status == "in_progress" {
		status.ActiveHealingCount++
	}
	return nil
}

func (m *MockConsulHealingStore) UpdateHealingAttempt(ctx context.Context, rootID, attemptPath string, attempt *HealingAttempt) error {
	return nil
}

func (m *MockConsulHealingStore) GetHealingTree(ctx context.Context, rootID string) (*HealingTree, error) {
	status, exists := m.data[rootID]
	if !exists {
		return nil, nil
	}
	return &HealingTree{
		RootTransformID: rootID,
		Attempts:        status.Children,
		TotalAttempts:   status.TotalHealingAttempts,
	}, nil
}

func (m *MockConsulHealingStore) GetActiveHealingAttempts(ctx context.Context, rootID string) ([]string, error) {
	return []string{}, nil
}

func (m *MockConsulHealingStore) CleanupCompletedTransformations(ctx context.Context, maxAge time.Duration) error {
	return nil
}

func (m *MockConsulHealingStore) SetTransformationTTL(ctx context.Context, id string, ttl time.Duration) error {
	return nil
}

func TestExecuteTransformation_Async(t *testing.T) {
	// Create a mock Consul store
	mockStore := &MockConsulHealingStore{
		MockConsulStore: MockConsulStore{
			data: make(map[string]*TransformationStatus),
		},
	}

	// Create handler with mock store
	handler := &Handler{
		consulStore:    mockStore,
		recipeExecutor: nil, // Will be mocked at the method level
	}

	app := fiber.New()
	app.Post("/v1/arf/transform", handler.ExecuteTransformationAsync)
	app.Get("/v1/arf/transforms/:id/status", handler.GetTransformationStatusAsync)

	t.Run("returns immediately with status link", func(t *testing.T) {
		request := map[string]interface{}{
			"recipe_id": "test-recipe",
			"type":      "openrewrite",
			"codebase": map[string]interface{}{
				"repository": "https://github.com/example/test-repo",
				"branch":     "main",
				"language":   "java",
			},
		}

		body, _ := json.Marshal(request)
		req := httptest.NewRequest("POST", "/v1/arf/transform", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		// Measure response time
		start := time.Now()
		resp, err := app.Test(req, 2000) // 2 second timeout
		elapsed := time.Since(start)

		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		// Should return within 1 second
		assert.Less(t, elapsed, 1*time.Second, "Response should be returned within 1 second")

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		// Check response format
		assert.NotEmpty(t, result["transformation_id"])
		assert.Equal(t, "initiated", result["status"])
		assert.Contains(t, result["status_url"], "/v1/arf/transforms/")
		assert.Contains(t, result["status_url"], "/status")
		assert.Contains(t, result["message"], "Transformation started")
	})

	t.Run("stores initial status in Consul immediately", func(t *testing.T) {
		request := map[string]interface{}{
			"recipe_id": "test-recipe",
			"type":      "openrewrite",
			"codebase": map[string]interface{}{
				"repository": "https://github.com/example/test-repo",
				"branch":     "main",
			},
		}

		body, _ := json.Marshal(request)
		req := httptest.NewRequest("POST", "/v1/arf/transform", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(req)
		require.NoError(t, err)

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		transformID := result["transformation_id"].(string)

		// Check that status was stored immediately
		status, err := mockStore.GetTransformationStatus(context.Background(), transformID)
		assert.NoError(t, err)
		assert.NotNil(t, status)
		assert.Equal(t, transformID, status.TransformationID)
		assert.Equal(t, "initiated", status.Status)
		assert.Equal(t, "openrewrite", status.WorkflowStage)
	})

	t.Run("status endpoint returns current transformation state", func(t *testing.T) {
		// Manually store a transformation status
		transformID := uuid.New().String()
		mockStore.StoreTransformationStatus(context.Background(), transformID, &TransformationStatus{
			TransformationID: transformID,
			Status:           "in_progress",
			WorkflowStage:    "build",
			StartTime:        time.Now(),
			Progress: &TransformationProgress{
				Stage:           "compilation",
				PercentComplete: 65,
			},
		})

		// Query status endpoint
		req := httptest.NewRequest("GET", "/v1/arf/transforms/"+transformID+"/status", nil)
		resp, err := app.Test(req)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		assert.Equal(t, transformID, result["transformation_id"])
		assert.Equal(t, "in_progress", result["status"])
		assert.Equal(t, "build", result["workflow_stage"])

		progress := result["progress"].(map[string]interface{})
		assert.Equal(t, "compilation", progress["stage"])
		assert.Equal(t, float64(65), progress["percent_complete"])
	})

	t.Run("status endpoint returns healing tree when available", func(t *testing.T) {
		// Store transformation with healing attempts
		transformID := uuid.New().String()
		mockStore.StoreTransformationStatus(context.Background(), transformID, &TransformationStatus{
			TransformationID: transformID,
			Status:           "healing",
			WorkflowStage:    "heal",
			StartTime:        time.Now(),
			Children: []HealingAttempt{
				{
					TransformationID: uuid.New().String(),
					AttemptPath:      "1",
					TriggerReason:    "build_failure",
					Status:           "completed",
					Result:           "partial_success",
					Children: []HealingAttempt{
						{
							TransformationID: uuid.New().String(),
							AttemptPath:      "1.1",
							Status:           "in_progress",
							TriggerReason:    "test_failure",
							Children:         []HealingAttempt{},
						},
					},
				},
			},
			ActiveHealingCount:   1,
			TotalHealingAttempts: 2,
		})

		req := httptest.NewRequest("GET", "/v1/arf/transforms/"+transformID+"/status", nil)
		resp, err := app.Test(req)
		require.NoError(t, err)

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)

		assert.Equal(t, "healing", result["status"])
		assert.Equal(t, "heal", result["workflow_stage"])

		children := result["children"].([]interface{})
		assert.Len(t, children, 1)

		firstChild := children[0].(map[string]interface{})
		assert.Equal(t, "1", firstChild["attempt_path"])
		assert.Equal(t, "build_failure", firstChild["trigger_reason"])

		nestedChildren := firstChild["children"].([]interface{})
		assert.Len(t, nestedChildren, 1)
	})

	t.Run("handles missing transformation gracefully", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/arf/transforms/non-existent-id/status", nil)
		resp, err := app.Test(req)
		require.NoError(t, err)
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		assert.Contains(t, result["error"], "not found")
	})
}

func TestBackgroundExecution(t *testing.T) {
	t.Run("updates status throughout transformation lifecycle", func(t *testing.T) {
		mockStore := &MockConsulHealingStore{
			MockConsulStore: MockConsulStore{
				data: make(map[string]*TransformationStatus),
			},
		}

		transformID := uuid.New().String()

		// Simulate background execution updating status
		ctx := context.Background()

		// Initial status
		mockStore.StoreTransformationStatus(ctx, transformID, &TransformationStatus{
			TransformationID: transformID,
			Status:           "initiated",
			WorkflowStage:    "openrewrite",
			StartTime:        time.Now(),
		})

		// OpenRewrite stage
		time.Sleep(10 * time.Millisecond)
		mockStore.UpdateWorkflowStage(ctx, transformID, "openrewrite")
		status, _ := mockStore.GetTransformationStatus(ctx, transformID)
		status.Status = "in_progress"
		mockStore.StoreTransformationStatus(ctx, transformID, status)

		// Build stage
		time.Sleep(10 * time.Millisecond)
		mockStore.UpdateWorkflowStage(ctx, transformID, "build")

		// Test stage
		time.Sleep(10 * time.Millisecond)
		mockStore.UpdateWorkflowStage(ctx, transformID, "test")

		// Completed
		time.Sleep(10 * time.Millisecond)
		status, _ = mockStore.GetTransformationStatus(ctx, transformID)
		status.Status = "completed"
		status.EndTime = time.Now()
		mockStore.StoreTransformationStatus(ctx, transformID, status)

		// Verify final status
		finalStatus, err := mockStore.GetTransformationStatus(ctx, transformID)
		assert.NoError(t, err)
		assert.Equal(t, "completed", finalStatus.Status)
		assert.Equal(t, "test", finalStatus.WorkflowStage)
		assert.False(t, finalStatus.EndTime.IsZero())
	})
}
