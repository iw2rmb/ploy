package arf

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/iw2rmb/ploy/api/arf/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockSeaweedFS lives in mock_seaweedfs_test.go for reuse across tests

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

func (m *MockConsulHealingStore) GenerateNextAttemptPath(ctx context.Context, transformID string, parentPath string) (string, error) {
	status, exists := m.data[transformID]
	if !exists {
		return "1", nil
	}
	return GenerateAttemptPath(transformID, parentPath, status.Children), nil
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
	app.Post("/v1/arf/transforms", handler.ExecuteTransformationAsync)
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
		req := httptest.NewRequest("POST", "/v1/arf/transforms", bytes.NewReader(body))
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
		assert.Contains(t, []string{"initiated", "in_progress"}, result["status"], "Status should be either 'initiated' or 'in_progress' due to async execution race condition")
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
		req := httptest.NewRequest("POST", "/v1/arf/transforms", bytes.NewReader(body))
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
		assert.Contains(t, []string{"initiated", "in_progress"}, status.Status, "Status should be either 'initiated' or 'in_progress' due to async execution race condition")
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

func TestExecuteTransformation_ValidationSuggestions(t *testing.T) {
	// Setup RecipeRegistry with a couple of recipes
	ctx := context.Background()
	sea := newMockSeaweed()
	reg := NewRecipeRegistry(sea)
	_ = reg.StoreRecipe(ctx, &models.Recipe{
		ID: "org.openrewrite.java.RemoveUnusedImports",
		Metadata: models.RecipeMetadata{
			Name:        "RemoveUnusedImports",
			Description: "Removes unused imports",
			Tags:        []string{"RemoveUnusedImport"}, // allow equality match in mock search
			Languages:   []string{"java"},
		},
	})
	_ = reg.StoreRecipe(ctx, &models.Recipe{
		ID: "org.openrewrite.java.migrate.UpgradeToJava17",
		Metadata: models.RecipeMetadata{
			Name:        "UpgradeToJava17",
			Description: "Upgrades Java version",
			Tags:        []string{"UpgradeToJava"},
			Languages:   []string{"java"},
		},
	})

	// Mock Consul store to satisfy handler
	mockStore := &MockConsulHealingStore{MockConsulStore: MockConsulStore{data: make(map[string]*TransformationStatus)}}

	handler := &Handler{
		consulStore:    mockStore,
		recipeRegistry: reg,
	}

	app := fiber.New()
	app.Post("/v1/arf/transforms", handler.ExecuteTransformationAsync)

	// Submit request with slight typo to trigger suggestions
	request := map[string]interface{}{
		"recipe_id": "org.openrewrite.java.RemoveUnusedImport", // missing 's'
		"type":      "openrewrite",
		"codebase": map[string]interface{}{
			"repository": "https://github.com/example/test-repo",
		},
	}
	body, _ := json.Marshal(request)
	req := httptest.NewRequest("POST", "/v1/arf/transforms", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var result map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, "invalid recipe_id", result["error"])
	assert.Equal(t, "org.openrewrite.java.RemoveUnusedImport", result["recipe_id"])

	// Suggestions should include the correct recipe ID
	if arr, ok := result["suggestions"].([]interface{}); ok {
		found := false
		for _, v := range arr {
			if s, ok := v.(string); ok && s == "org.openrewrite.java.RemoveUnusedImports" {
				found = true
				break
			}
		}
		assert.True(t, found, "expected suggestions to include RemoveUnusedImports")
	} else {
		t.Fatalf("expected suggestions array in response")
	}
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

// TestGetTransformationStatusEnhanced tests the enhanced status endpoint with full healing tree
func TestGetTransformationStatusEnhanced(t *testing.T) {
	mockStore := NewMockConsulStore()

	// Create handler with mock store and sandbox manager
	handler := &Handler{
		consulStore: mockStore,
		sandboxMgr: &MockSandboxManager{
			sandboxes: make(map[string]*Sandbox),
		},
		healingCoordinator: NewHealingCoordinator(DefaultHealingConfig()),
	}

	app := fiber.New()
	app.Get("/v1/arf/transforms/:id/status", handler.GetTransformationStatusAsync)

	t.Run("returns comprehensive status with healing tree", func(t *testing.T) {
		transformID := uuid.New().String()

		// Create a transformation with healing attempts
		status := &TransformationStatus{
			TransformationID:     transformID,
			Status:               "in_progress",
			WorkflowStage:        "heal",
			StartTime:            time.Now().Add(-10 * time.Minute),
			TotalHealingAttempts: 3,
			ActiveHealingCount:   1,
			Children: []HealingAttempt{
				{
					TransformationID: uuid.New().String(),
					AttemptPath:      "1",
					TriggerReason:    "build_failure",
					TargetErrors:     []string{"compilation_error_line_45"},
					Status:           "completed",
					Result:           "partial_success",
					StartTime:        time.Now().Add(-8 * time.Minute),
					EndTime:          time.Now().Add(-5 * time.Minute),
					LLMAnalysis: &LLMAnalysisResult{
						ErrorType:      "compilation_error",
						Confidence:     0.85,
						SuggestedFix:   "Add missing import statements",
						RiskAssessment: "low",
					},
					NewIssuesDiscovered: []string{"test_failure_integration"},
					Children: []HealingAttempt{
						{
							TransformationID: uuid.New().String(),
							AttemptPath:      "1.1",
							TriggerReason:    "test_failure_after_heal",
							TargetErrors:     []string{"test_failure_integration"},
							Status:           "in_progress",
							StartTime:        time.Now().Add(-4 * time.Minute),
							Progress: &TransformationProgress{
								Stage:           "build_validation",
								PercentComplete: 45,
								Message:         "Validating healing transformation",
							},
							SandboxID: "sandbox-heal-1-1",
						},
					},
				},
				{
					TransformationID: uuid.New().String(),
					AttemptPath:      "2",
					TriggerReason:    "build_failure",
					TargetErrors:     []string{"missing_import"},
					Status:           "completed",
					Result:           "success",
					StartTime:        time.Now().Add(-7 * time.Minute),
					EndTime:          time.Now().Add(-6 * time.Minute),
					LLMAnalysis: &LLMAnalysisResult{
						ErrorType:      "import_error",
						Confidence:     0.95,
						SuggestedFix:   "Import java.util.List",
						RiskAssessment: "minimal",
					},
				},
			},
		}

		mockStore.StoreTransformationStatus(context.Background(), transformID, status)

		// Also store active attempts for testing
		mockStore.activeAttempts[transformID] = []string{status.Children[0].Children[0].TransformationID}

		// Query status endpoint
		req := httptest.NewRequest("GET", "/v1/arf/transforms/"+transformID+"/status", nil)
		resp, err := app.Test(req)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result TransformationStatus
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		// Verify basic fields
		assert.Equal(t, transformID, result.TransformationID)
		assert.Equal(t, "in_progress", result.Status)
		assert.Equal(t, "heal", result.WorkflowStage)

		// Verify healing summary
		assert.NotNil(t, result.HealingSummary)
		assert.Equal(t, 3, result.HealingSummary.TotalAttempts)
		assert.Equal(t, 1, result.HealingSummary.ActiveAttempts)
		assert.Equal(t, 1, result.HealingSummary.SuccessfulHeals)
		assert.Equal(t, 0, result.HealingSummary.FailedHeals)
		assert.Equal(t, 2, result.HealingSummary.MaxDepthReached)

		// Verify children structure
		assert.Len(t, result.Children, 2)
		assert.Equal(t, "1", result.Children[0].AttemptPath)
		assert.NotNil(t, result.Children[0].LLMAnalysis)
		assert.Equal(t, 0.85, result.Children[0].LLMAnalysis.Confidence)

		// Verify nested children
		assert.Len(t, result.Children[0].Children, 1)
		assert.Equal(t, "1.1", result.Children[0].Children[0].AttemptPath)
		assert.NotNil(t, result.Children[0].Children[0].Progress)
		assert.Equal(t, 45, result.Children[0].Children[0].Progress.PercentComplete)

		// Verify progress calculation
		assert.NotNil(t, result.Progress)
		assert.Contains(t, result.Progress.Message, "Healing in progress")

		// Verify sandbox info
		assert.NotNil(t, result.SandboxInfo)
		assert.NotNil(t, result.SandboxInfo.PrimarySandbox)
		assert.True(t, len(result.SandboxInfo.HealingSandboxes) > 0)
	})

	t.Run("returns status without healing for simple transformation", func(t *testing.T) {
		transformID := uuid.New().String()

		// Create a simple transformation without healing
		status := &TransformationStatus{
			TransformationID: transformID,
			Status:           "completed",
			WorkflowStage:    "test",
			StartTime:        time.Now().Add(-5 * time.Minute),
			EndTime:          time.Now().Add(-1 * time.Minute),
		}

		mockStore.StoreTransformationStatus(context.Background(), transformID, status)

		// Query status endpoint
		req := httptest.NewRequest("GET", "/v1/arf/transforms/"+transformID+"/status", nil)
		resp, err := app.Test(req)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result TransformationStatus
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		// Verify basic fields
		assert.Equal(t, transformID, result.TransformationID)
		assert.Equal(t, "completed", result.Status)
		assert.Equal(t, "test", result.WorkflowStage)

		// Should not have healing summary for simple transformation
		assert.Nil(t, result.HealingSummary)
		assert.Empty(t, result.Children)
	})

	t.Run("calculates progress correctly for different stages", func(t *testing.T) {
		testCases := []struct {
			stage           string
			expectedPercent int
			expectedMessage string
		}{
			{"openrewrite", 25, "Executing transformation recipe"},
			{"build", 50, "Building transformed code"},
			{"deploy", 60, "Deploying to sandbox environment"},
			{"test", 75, "Running test suites"},
		}

		for _, tc := range testCases {
			t.Run(tc.stage, func(t *testing.T) {
				transformID := uuid.New().String()

				status := &TransformationStatus{
					TransformationID: transformID,
					Status:           "in_progress",
					WorkflowStage:    tc.stage,
					StartTime:        time.Now().Add(-2 * time.Minute),
				}

				mockStore.StoreTransformationStatus(context.Background(), transformID, status)

				req := httptest.NewRequest("GET", "/v1/arf/transforms/"+transformID+"/status", nil)
				resp, err := app.Test(req)
				require.NoError(t, err)

				var result TransformationStatus
				json.NewDecoder(resp.Body).Decode(&result)

				assert.NotNil(t, result.Progress)
				assert.Equal(t, tc.expectedPercent, result.Progress.PercentComplete)
				assert.Equal(t, tc.expectedMessage, result.Progress.Message)
			})
		}
	})

	t.Run("handles transformation not found", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/arf/transforms/non-existent-id/status", nil)
		resp, err := app.Test(req)
		require.NoError(t, err)
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		assert.Equal(t, "Transformation not found", result["error"])
	})
}

// Test that catalog hits and misses are tracked
func TestHandler_TracksCatalogMetrics(t *testing.T) {
	// Create handler with registry and metrics
	sea := newMockSeaweed()
	reg := NewRecipeRegistry(sea)
	mockMetrics := &MockCatalogMetrics{}
	mockStore := NewMockConsulStore()

	handler := &Handler{
		recipeRegistry: reg,
		consulStore:    mockStore,
		recipeExecutor: &RecipeExecutor{}, // Use actual RecipeExecutor type
		metrics:        mockMetrics,
	}

	app := fiber.New()
	app.Post("/v1/arf/transforms", handler.ExecuteTransformationAsync)

	// Add a known recipe to registry
	ctx := context.Background()
	_ = reg.StoreRecipe(ctx, &models.Recipe{
		ID: "org.openrewrite.java.RemoveUnusedImports",
		Metadata: models.RecipeMetadata{
			Name: "Remove Unused Imports",
		},
	})

	t.Run("tracks catalog hit on valid recipe", func(t *testing.T) {
		request := map[string]interface{}{
			"recipe_id": "org.openrewrite.java.RemoveUnusedImports",
			"type":      "openrewrite",
			"codebase": map[string]interface{}{
				"repository": "https://github.com/example/test",
				"branch":     "main",
			},
		}

		body, _ := json.Marshal(request)
		req := httptest.NewRequest("POST", "/v1/arf/transforms", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(req)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		// Verify catalog hit was recorded
		assert.Equal(t, int64(1), mockMetrics.hits.Load())
		assert.Equal(t, int64(0), mockMetrics.misses.Load())
	})

	t.Run("tracks catalog miss on invalid recipe", func(t *testing.T) {
		// Reset metrics
		mockMetrics.Reset()

		request := map[string]interface{}{
			"recipe_id": "org.openrewrite.java.NonExistentRecipe",
			"type":      "openrewrite",
			"codebase": map[string]interface{}{
				"repository": "https://github.com/example/test",
				"branch":     "main",
			},
		}

		body, _ := json.Marshal(request)
		req := httptest.NewRequest("POST", "/v1/arf/transforms", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(req)
		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

		// Verify catalog miss was recorded
		assert.Equal(t, int64(0), mockMetrics.hits.Load())
		assert.Equal(t, int64(1), mockMetrics.misses.Load())

		// Verify validation failure was recorded
		assert.Equal(t, int64(1), mockMetrics.validationFailures.Load())
	})

	t.Run("tracks search performance", func(t *testing.T) {
		// Reset metrics
		mockMetrics.Reset()

		// Simulate multiple searches
		for i := 0; i < 5; i++ {
			request := map[string]interface{}{
				"recipe_id": fmt.Sprintf("org.openrewrite.test.Recipe%d", i),
				"type":      "openrewrite",
				"codebase": map[string]interface{}{
					"repository": "https://github.com/example/test",
					"branch":     "main",
				},
			}

			body, _ := json.Marshal(request)
			req := httptest.NewRequest("POST", "/v1/arf/transforms", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			_, _ = app.Test(req)
		}

		// Verify search count
		assert.Equal(t, int64(5), mockMetrics.searchCount.Load())
	})
}

// MockCatalogMetrics tracks catalog access metrics
type MockCatalogMetrics struct {
	hits               atomic.Int64
	misses             atomic.Int64
	validationFailures atomic.Int64
	searchCount        atomic.Int64
	searchDuration     atomic.Int64
}

func (m *MockCatalogMetrics) RecordHit() {
	m.hits.Add(1)
}

func (m *MockCatalogMetrics) RecordMiss() {
	m.misses.Add(1)
}

func (m *MockCatalogMetrics) RecordValidationFailure() {
	m.validationFailures.Add(1)
}

func (m *MockCatalogMetrics) RecordSearch(duration time.Duration) {
	m.searchCount.Add(1)
	m.searchDuration.Store(int64(duration))
}

func (m *MockCatalogMetrics) Reset() {
	m.hits.Store(0)
	m.misses.Store(0)
	m.validationFailures.Store(0)
	m.searchCount.Store(0)
	m.searchDuration.Store(0)
}
