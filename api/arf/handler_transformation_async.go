package arf

import (
	"context"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// ExecuteTransformationAsync handles POST /v1/arf/transform with async execution
func (h *Handler) ExecuteTransformationAsync(c *fiber.Ctx) error {
	var req TransformRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Invalid request format",
			"details": err.Error(),
		})
	}

	// Validate required fields
	if req.RecipeID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "recipe_id is required",
		})
	}

	if req.Codebase.Repository == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "codebase.repository is required",
		})
	}

	// Set default branch if not specified
	if req.Codebase.Branch == "" {
		req.Codebase.Branch = "main"
	}

	// Require explicit type specification
	if req.Type == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "recipe type is required - specify 'openrewrite' or other valid type",
		})
	}

	// Generate transformation ID
	transformID := uuid.New().String()

	// Check if we have Consul store
	if h.consulStore == nil {
		// Fallback to synchronous execution if no Consul store
		return h.ExecuteTransformation(c)
	}

	// Store initial status in Consul immediately
	initialStatus := &TransformationStatus{
		TransformationID: transformID,
		Status:           "initiated",
		WorkflowStage:    req.Type, // Start with recipe type as initial stage
		StartTime:        time.Now(),
		Children:         []HealingAttempt{},
	}

	if err := h.consulStore.StoreTransformationStatus(c.Context(), transformID, initialStatus); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "Failed to initialize transformation",
			"details": err.Error(),
		})
	}

	// Start background execution
	go h.executeTransformationBackground(transformID, &req)

	// Return immediately with status link
	return c.JSON(fiber.Map{
		"transformation_id": transformID,
		"status":            "initiated",
		"status_url":        fmt.Sprintf("/v1/arf/transforms/%s/status", transformID),
		"message":           "Transformation started, use status_url to monitor progress",
	})
}

// executeTransformationBackground executes the transformation in the background
func (h *Handler) executeTransformationBackground(transformID string, req *TransformRequest) {
	ctx := context.Background()

	// Update status to in_progress
	status, _ := h.consulStore.GetTransformationStatus(ctx, transformID)
	if status != nil {
		status.Status = "in_progress"
		status.WorkflowStage = "openrewrite"
		h.consulStore.StoreTransformationStatus(ctx, transformID, status)
	}

	// Execute transformation using existing internal method
	result, err := h.executeTransformationInternal(ctx, transformID, req)

	// Update final status
	if status != nil {
		if err != nil {
			status.Status = "failed"
			status.Error = err.Error()
		} else {
			status.Status = "completed"
			if result != nil {
				// Store result details in status
				status.EndTime = time.Now()
				// Could also store result in a separate key if needed
			}
		}
		status.EndTime = time.Now()
		h.consulStore.StoreTransformationStatus(ctx, transformID, status)
	}

	// Store result in global store for backward compatibility
	if result != nil {
		globalTransformStore.store(transformID, result)
	}
}

// GetTransformationStatusAsync handles GET /v1/arf/transforms/:id/status with Consul
func (h *Handler) GetTransformationStatusAsync(c *fiber.Ctx) error {
	transformID := c.Params("id")
	if transformID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "transformation_id is required",
		})
	}

	// Check if we have Consul store
	if h.consulStore == nil {
		// Fallback to existing method
		return h.GetTransformationStatus(c)
	}

	// Get status from Consul
	status, err := h.consulStore.GetTransformationStatus(c.Context(), transformID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "Failed to retrieve transformation status",
			"details": err.Error(),
		})
	}

	if status == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Transformation not found",
		})
	}

	// Build comprehensive response
	response := fiber.Map{
		"transformation_id": status.TransformationID,
		"workflow_stage":    status.WorkflowStage,
		"status":            status.Status,
		"start_time":        status.StartTime,
	}

	if !status.EndTime.IsZero() {
		response["end_time"] = status.EndTime
	}

	if status.Progress != nil {
		response["progress"] = status.Progress
	}

	if status.Error != "" {
		response["error"] = status.Error
	}

	// Add healing summary if applicable
	if len(status.Children) > 0 || status.WorkflowStage == "heal" {
		healingSummary := fiber.Map{
			"total_attempts":   status.TotalHealingAttempts,
			"active_attempts":  status.ActiveHealingCount,
			"successful_heals": countSuccessfulHeals(status.Children),
			"failed_heals":     countFailedHeals(status.Children),
			"max_depth":        calculateMaxDepth(status.Children, 1),
		}
		response["healing_summary"] = healingSummary
		response["children"] = status.Children
	}

	// Add active attempts if any
	if status.ActiveHealingCount > 0 {
		activeAttempts, _ := h.consulStore.GetActiveHealingAttempts(c.Context(), transformID)
		response["active_attempts"] = activeAttempts
	}

	return c.JSON(response)
}

// Helper functions for healing metrics
func countSuccessfulHeals(attempts []HealingAttempt) int {
	count := 0
	for _, attempt := range attempts {
		if attempt.Status == "completed" && attempt.Result == "success" {
			count++
		}
		count += countSuccessfulHeals(attempt.Children)
	}
	return count
}

func countFailedHeals(attempts []HealingAttempt) int {
	count := 0
	for _, attempt := range attempts {
		if attempt.Status == "completed" && attempt.Result == "failed" {
			count++
		}
		count += countFailedHeals(attempt.Children)
	}
	return count
}

func calculateMaxDepth(attempts []HealingAttempt, currentDepth int) int {
	if len(attempts) == 0 {
		return currentDepth - 1
	}

	maxDepth := currentDepth
	for _, attempt := range attempts {
		childDepth := calculateMaxDepth(attempt.Children, currentDepth+1)
		if childDepth > maxDepth {
			maxDepth = childDepth
		}
	}
	return maxDepth
}
