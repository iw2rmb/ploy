package arf

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestGetTransformationHierarchy(t *testing.T) {
	mockStore := NewMockConsulStore()
	handler := &Handler{
		consulStore: mockStore,
	}

	app := fiber.New()
	app.Get("/v1/arf/transforms/:id/hierarchy", handler.GetTransformationHierarchy)

	t.Run("returns hierarchy visualization for transformation with healing attempts", func(t *testing.T) {
		transformID := uuid.New().String()

		// Create a transformation with nested healing attempts
		status := &TransformationStatus{
			TransformationID: transformID,
			Status:           "in_progress",
			WorkflowStage:    "healing",
			StartTime:        time.Now().Add(-45 * time.Minute),
			Children: []HealingAttempt{
				{
					TransformationID: uuid.New().String(),
					AttemptPath:      "1",
					TriggerReason:    "build_failure",
					Status:           "completed",
					Result:           "success",
					StartTime:        time.Now().Add(-40 * time.Minute),
					EndTime:          time.Now().Add(-37 * time.Minute),
				},
				{
					TransformationID: uuid.New().String(),
					AttemptPath:      "2",
					TriggerReason:    "test_failure",
					Status:           "completed",
					Result:           "failed",
					StartTime:        time.Now().Add(-35 * time.Minute),
					EndTime:          time.Now().Add(-30 * time.Minute),
					Children: []HealingAttempt{
						{
							TransformationID: uuid.New().String(),
							AttemptPath:      "2.1",
							TriggerReason:    "unit_test_fix",
							Status:           "completed",
							Result:           "success",
							StartTime:        time.Now().Add(-28 * time.Minute),
							EndTime:          time.Now().Add(-26 * time.Minute),
						},
						{
							TransformationID: uuid.New().String(),
							AttemptPath:      "2.2",
							TriggerReason:    "integration_test_fix",
							Status:           "in_progress",
							StartTime:        time.Now().Add(-25 * time.Minute),
						},
					},
				},
				{
					TransformationID: uuid.New().String(),
					AttemptPath:      "3",
					TriggerReason:    "runtime_error",
					Status:           "in_progress",
					StartTime:        time.Now().Add(-12 * time.Minute),
					Children: []HealingAttempt{
						{
							TransformationID: uuid.New().String(),
							AttemptPath:      "3.1",
							TriggerReason:    "memory_fix",
							Status:           "pending",
						},
					},
				},
			},
		}

		mockStore.StoreTransformationStatus(context.Background(), transformID, status)

		req := httptest.NewRequest("GET", fmt.Sprintf("/v1/arf/transforms/%s/hierarchy", transformID), nil)
		resp, err := app.Test(req)

		assert.NoError(t, err)
		assert.Equal(t, fiber.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		assert.NoError(t, err)

		// Verify response structure
		assert.Equal(t, transformID, result["transformation_id"])
		assert.Equal(t, "in_progress", result["status"])
		assert.Contains(t, result, "hierarchy_visualization")
		assert.Contains(t, result, "metrics")

		// Verify metrics
		metrics := result["metrics"].(map[string]interface{})
		assert.Equal(t, float64(6), metrics["total_attempts"])
		assert.Equal(t, float64(2), metrics["max_depth"]) // Max depth is 2 (root -> attempt -> child attempt)
		assert.Equal(t, float64(2), metrics["successful_heals"])
		assert.Equal(t, float64(1), metrics["failed_heals"])

		// Verify ASCII tree visualization is present
		visualization := result["hierarchy_visualization"].(string)
		assert.Contains(t, visualization, "[ROOT]")
		assert.Contains(t, visualization, "[1]")
		assert.Contains(t, visualization, "[2.1]")
		assert.Contains(t, visualization, "✓") // Success marker
		assert.Contains(t, visualization, "✗") // Failure marker
		assert.Contains(t, visualization, "⟳") // In-progress marker
	})

	t.Run("returns error when transformation not found", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/arf/transforms/non-existent/hierarchy", nil)
		resp, err := app.Test(req)

		assert.NoError(t, err)
		assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)
	})

	t.Run("returns error when consul store not configured", func(t *testing.T) {
		handlerNoConsul := &Handler{
			consulStore: nil,
		}

		appNoConsul := fiber.New()
		appNoConsul.Get("/v1/arf/transforms/:id/hierarchy", handlerNoConsul.GetTransformationHierarchy)

		req := httptest.NewRequest("GET", "/v1/arf/transforms/some-id/hierarchy", nil)
		resp, err := appNoConsul.Test(req)

		assert.NoError(t, err)
		assert.Equal(t, fiber.StatusInternalServerError, resp.StatusCode)
	})
}

func TestGetActiveHealingAttempts(t *testing.T) {
	mockStore := NewMockConsulStore()
	handler := &Handler{
		consulStore: mockStore,
	}

	app := fiber.New()
	app.Get("/v1/arf/transforms/:id/active", handler.GetActiveHealingAttempts)

	t.Run("returns list of active healing attempts", func(t *testing.T) {
		transformID := uuid.New().String()

		// Create transformation with active attempts
		status := &TransformationStatus{
			TransformationID: transformID,
			Status:           "in_progress",
			WorkflowStage:    "healing",
			Children: []HealingAttempt{
				{
					AttemptPath: "1",
					Status:      "completed",
					Result:      "success",
				},
				{
					AttemptPath:   "2",
					Status:        "in_progress",
					TriggerReason: "test_failure",
					StartTime:     time.Now().Add(-5 * time.Minute),
				},
				{
					AttemptPath:   "3",
					Status:        "in_progress",
					TriggerReason: "build_failure",
					StartTime:     time.Now().Add(-3 * time.Minute),
				},
				{
					AttemptPath: "4",
					Status:      "pending",
				},
			},
		}

		mockStore.StoreTransformationStatus(context.Background(), transformID, status)

		req := httptest.NewRequest("GET", fmt.Sprintf("/v1/arf/transforms/%s/active", transformID), nil)
		resp, err := app.Test(req)

		assert.NoError(t, err)
		assert.Equal(t, fiber.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		assert.NoError(t, err)

		// Verify active attempts
		activeAttempts := result["active_attempts"].([]interface{})
		assert.Len(t, activeAttempts, 2) // Only in_progress attempts

		// Verify attempt details
		for _, attempt := range activeAttempts {
			a := attempt.(map[string]interface{})
			assert.Equal(t, "in_progress", a["status"])
			assert.Contains(t, []string{"2", "3"}, a["attempt_path"])
		}
	})
}

func TestGetTransformationTimeline(t *testing.T) {
	mockStore := NewMockConsulStore()
	handler := &Handler{
		consulStore: mockStore,
	}

	app := fiber.New()
	app.Get("/v1/arf/transforms/:id/timeline", handler.GetTransformationTimeline)

	t.Run("returns chronological timeline of all events", func(t *testing.T) {
		transformID := uuid.New().String()
		now := time.Now()

		status := &TransformationStatus{
			TransformationID: transformID,
			Status:           "completed",
			WorkflowStage:    "completed",
			StartTime:        now.Add(-1 * time.Hour),
			EndTime:          now.Add(-5 * time.Minute),
			Children: []HealingAttempt{
				{
					AttemptPath:   "1",
					Status:        "completed",
					StartTime:     now.Add(-50 * time.Minute),
					EndTime:       now.Add(-45 * time.Minute),
					TriggerReason: "build_failure",
				},
				{
					AttemptPath:   "2",
					Status:        "completed",
					StartTime:     now.Add(-40 * time.Minute),
					EndTime:       now.Add(-30 * time.Minute),
					TriggerReason: "test_failure",
				},
			},
		}

		mockStore.StoreTransformationStatus(context.Background(), transformID, status)

		req := httptest.NewRequest("GET", fmt.Sprintf("/v1/arf/transforms/%s/timeline", transformID), nil)
		resp, err := app.Test(req)

		assert.NoError(t, err)
		assert.Equal(t, fiber.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		assert.NoError(t, err)

		// Verify timeline entries
		timeline := result["timeline"].([]interface{})
		assert.GreaterOrEqual(t, len(timeline), 6) // Start, 2x attempt start/end, transformation end

		// Verify chronological order
		var lastTime time.Time
		for _, entry := range timeline {
			e := entry.(map[string]interface{})
			timestamp, _ := time.Parse(time.RFC3339, e["timestamp"].(string))
			assert.True(t, timestamp.After(lastTime) || timestamp.Equal(lastTime))
			lastTime = timestamp
		}
	})
}

func TestGetTransformationAnalysis(t *testing.T) {
	mockStore := NewMockConsulStore()
	handler := &Handler{
		consulStore: mockStore,
	}

	app := fiber.New()
	app.Get("/v1/arf/transforms/:id/analysis", handler.GetTransformationAnalysis)

	t.Run("returns deep analysis with costs and metrics", func(t *testing.T) {
		transformID := uuid.New().String()

		status := &TransformationStatus{
			TransformationID: transformID,
			Status:           "completed",
			WorkflowStage:    "completed",
			StartTime:        time.Now().Add(-2 * time.Hour),
			EndTime:          time.Now().Add(-30 * time.Minute),
			Children: []HealingAttempt{
				{
					AttemptPath:   "1",
					Status:        "completed",
					Result:        "success",
					TriggerReason: "compilation_error",
					LLMAnalysis: &LLMAnalysisResult{
						ErrorType:  "compilation_error",
						Confidence: 0.85,
					},
				},
				{
					AttemptPath:   "2",
					Status:        "completed",
					Result:        "failed",
					TriggerReason: "test_failure",
					LLMAnalysis: &LLMAnalysisResult{
						ErrorType:  "test_failure",
						Confidence: 0.72,
					},
				},
			},
		}

		mockStore.StoreTransformationStatus(context.Background(), transformID, status)

		req := httptest.NewRequest("GET", fmt.Sprintf("/v1/arf/transforms/%s/analysis", transformID), nil)
		resp, err := app.Test(req)

		assert.NoError(t, err)
		assert.Equal(t, fiber.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		assert.NoError(t, err)

		// Verify analysis sections
		assert.Contains(t, result, "summary")
		assert.Contains(t, result, "cost_analysis")
		assert.Contains(t, result, "error_patterns")
		assert.Contains(t, result, "performance_metrics")
		assert.Contains(t, result, "recommendations")

		// Verify summary
		summary := result["summary"].(map[string]interface{})
		assert.Equal(t, transformID, summary["transformation_id"])
		assert.Equal(t, "completed", summary["status"])
		assert.Equal(t, float64(1.5), summary["total_duration_hours"])

		// Verify error patterns (should have patterns from our test data)
		patterns := result["error_patterns"].([]interface{})
		assert.GreaterOrEqual(t, len(patterns), 2) // Should have compilation_error and test_failure patterns
	})
}

func TestGetOrphanedTransformations(t *testing.T) {
	mockStore := NewMockConsulStore()
	handler := &Handler{
		consulStore: mockStore,
	}

	app := fiber.New()
	app.Get("/v1/arf/transforms/orphaned", handler.GetOrphanedTransformations)

	t.Run("returns list of orphaned transformations", func(t *testing.T) {
		// For now, this test just verifies the endpoint returns properly
		// since GetOrphanedTransformations currently returns a mock implementation

		req := httptest.NewRequest("GET", "/v1/arf/transforms/orphaned", nil)
		resp, err := app.Test(req)

		assert.NoError(t, err)
		assert.Equal(t, fiber.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		assert.NoError(t, err)

		// Verify response structure
		assert.Contains(t, result, "orphaned_transformations")
		assert.Contains(t, result, "total_orphaned")
		assert.Contains(t, result, "recommended_action")

		// Since it's a mock, should return empty list
		orphaned := result["orphaned_transformations"].([]interface{})
		assert.Len(t, orphaned, 0)
		assert.Equal(t, float64(0), result["total_orphaned"])
	})
}

func TestHierarchyVisualizationFormats(t *testing.T) {
	mockStore := NewMockConsulStore()
	handler := &Handler{
		consulStore: mockStore,
	}

	app := fiber.New()
	app.Get("/v1/arf/transforms/:id/hierarchy", handler.GetTransformationHierarchy)

	transformID := uuid.New().String()
	status := &TransformationStatus{
		TransformationID: transformID,
		Status:           "completed",
		Children: []HealingAttempt{
			{
				AttemptPath: "1",
				Status:      "completed",
				Result:      "success",
			},
		},
	}
	mockStore.StoreTransformationStatus(context.Background(), transformID, status)

	t.Run("returns JSON format by default", func(t *testing.T) {
		req := httptest.NewRequest("GET", fmt.Sprintf("/v1/arf/transforms/%s/hierarchy", transformID), nil)
		resp, err := app.Test(req)

		assert.NoError(t, err)
		assert.Equal(t, fiber.StatusOK, resp.StatusCode)
		assert.Contains(t, resp.Header.Get("Content-Type"), "application/json")
	})

	t.Run("returns ASCII tree format when requested", func(t *testing.T) {
		req := httptest.NewRequest("GET", fmt.Sprintf("/v1/arf/transforms/%s/hierarchy?format=tree", transformID), nil)
		resp, err := app.Test(req)

		assert.NoError(t, err)
		assert.Equal(t, fiber.StatusOK, resp.StatusCode)

		body := make([]byte, resp.ContentLength)
		resp.Body.Read(body)
		bodyStr := string(body)

		// Should contain tree characters
		assert.True(t, strings.Contains(bodyStr, "└") || strings.Contains(bodyStr, "├"))
	})

	t.Run("returns CSV format when requested", func(t *testing.T) {
		req := httptest.NewRequest("GET", fmt.Sprintf("/v1/arf/transforms/%s/hierarchy?format=csv", transformID), nil)
		resp, err := app.Test(req)

		assert.NoError(t, err)
		assert.Equal(t, fiber.StatusOK, resp.StatusCode)
		assert.Contains(t, resp.Header.Get("Content-Type"), "text/csv")
	})
}

func TestDebugEndpointsPerformance(t *testing.T) {
	mockStore := NewMockConsulStore()
	handler := &Handler{
		consulStore: mockStore,
	}

	app := fiber.New()
	app.Get("/v1/arf/transforms/:id/hierarchy", handler.GetTransformationHierarchy)

	t.Run("handles large hierarchies efficiently", func(t *testing.T) {
		transformID := uuid.New().String()

		// Create a large hierarchy (100 attempts)
		status := &TransformationStatus{
			TransformationID: transformID,
			Status:           "completed",
			Children:         make([]HealingAttempt, 0, 100),
		}

		for i := 0; i < 100; i++ {
			status.Children = append(status.Children, HealingAttempt{
				AttemptPath:   fmt.Sprintf("%d", i+1),
				Status:        "completed",
				Result:        "success",
				TriggerReason: "test",
			})
		}

		mockStore.StoreTransformationStatus(context.Background(), transformID, status)

		start := time.Now()
		req := httptest.NewRequest("GET", fmt.Sprintf("/v1/arf/transforms/%s/hierarchy", transformID), nil)
		resp, err := app.Test(req, 200) // 200ms timeout
		duration := time.Since(start)

		assert.NoError(t, err)
		assert.Equal(t, fiber.StatusOK, resp.StatusCode)
		assert.Less(t, duration, 100*time.Millisecond, "Response should be under 100ms for 100 nodes")
	})
}
