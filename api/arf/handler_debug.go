package arf

import (
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
)

// GetTransformationHierarchy returns a hierarchical view of the transformation and its healing attempts
func (h *Handler) GetTransformationHierarchy(c *fiber.Ctx) error {
	transformID := c.Params("id")
	if transformID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "transformation_id is required",
		})
	}

	// Check if consul store is configured
	if h.consulStore == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Consul store not configured",
		})
	}

	// Get transformation status from Consul
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

	// Check requested format
	format := c.Query("format", "json")

	// Build hierarchy visualization
	viz := buildHierarchyVisualization(status)

	// Return based on format
	switch format {
	case "tree":
		c.Set("Content-Type", "text/plain; charset=utf-8")
		return c.SendString(viz.Visualization)
	case "csv":
		csvData := generateCSVFromHierarchy(viz)
		c.Set("Content-Type", "text/csv")
		c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"hierarchy_%s.csv\"", transformID))
		return c.SendString(csvData)
	default: // json
		return c.JSON(viz)
	}
}

// GetActiveHealingAttempts returns currently active healing attempts for a transformation
func (h *Handler) GetActiveHealingAttempts(c *fiber.Ctx) error {
	transformID := c.Params("id")
	if transformID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "transformation_id is required",
		})
	}

	if h.consulStore == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Consul store not configured",
		})
	}

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

	// Extract active attempts
	activeAttempts := extractActiveAttempts(status.Children)

	response := ActiveAttemptsResponse{
		TransformationID: transformID,
		ActiveAttempts:   activeAttempts,
		TotalActive:      len(activeAttempts),
	}

	// Estimate time remaining based on average duration
	if len(activeAttempts) > 0 {
		avgDuration := calculateAverageDuration(status.Children)
		if avgDuration > 0 {
			response.EstimatedTimeRemaining = avgDuration * time.Duration(len(activeAttempts))
		}
	}

	return c.JSON(response)
}

// GetTransformationTimeline returns a chronological timeline of all events
func (h *Handler) GetTransformationTimeline(c *fiber.Ctx) error {
	transformID := c.Params("id")
	if transformID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "transformation_id is required",
		})
	}

	if h.consulStore == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Consul store not configured",
		})
	}

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

	// Build timeline
	timeline := buildTimeline(status)

	return c.JSON(timeline)
}

// GetTransformationAnalysis provides deep analysis of a transformation
func (h *Handler) GetTransformationAnalysis(c *fiber.Ctx) error {
	transformID := c.Params("id")
	if transformID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "transformation_id is required",
		})
	}

	if h.consulStore == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Consul store not configured",
		})
	}

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

	// Perform deep analysis
	analysis := analyzeTransformation(status)

	return c.JSON(analysis)
}

// GetTransformationReport generates a human-readable markdown report
func (h *Handler) GetTransformationReport(c *fiber.Ctx) error {
	transformID := c.Params("id")
	if transformID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "transformation_id is required",
		})
	}

	// Check if consul store is configured
	if h.consulStore == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Consul store not configured",
		})
	}

	// Get transformation status from Consul
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

	// Generate markdown report with diff from status
	report := generateMarkdownReport(status, status.Diff)
	contentType := "text/markdown; charset=utf-8"

	c.Set("Content-Type", contentType)
	return c.SendString(report)
}

// GetOrphanedTransformations finds transformations with missing parent references
func (h *Handler) GetOrphanedTransformations(c *fiber.Ctx) error {
	if h.consulStore == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Consul store not configured",
		})
	}

	// This would typically require listing all transformations from Consul
	// For now, return a mock implementation
	response := OrphanedTransformationsResponse{
		OrphanedTransformations: []OrphanedTransformation{},
		TotalOrphaned:           0,
		RecommendedAction:       "Review and clean up orphaned transformations",
	}

	return c.JSON(response)
}
