package llms

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/gofiber/fiber/v2"
	models "github.com/iw2rmb/ploy/internal/llms/models"
	"github.com/iw2rmb/ploy/internal/storage"
)

// ListModels returns a list of LLM models
func (h *Handler) ListModels(c *fiber.Ctx) error {
	ctx := context.Background()

	// Parse query parameters
	provider := c.Query("provider", "")
	capability := c.Query("capability", "")
	// Default: no limit unless provided
	limit, _ := strconv.Atoi(c.Query("limit", "0"))
	offset, _ := strconv.Atoi(c.Query("offset", "0"))

	// Create filter options
	filterOptions := storage.ListOptions{Prefix: "llms/models/"}
	if limit > 0 {
		filterOptions.MaxKeys = limit
	}

	// List objects from storage
	objects, err := h.storage.List(ctx, filterOptions)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fmt.Sprintf("failed to list models: %v", err),
		})
	}

	// Load and filter models
	modelsList := make([]*models.LLMModel, 0)
	for i, obj := range objects {
		// Apply offset
		if i < offset {
			continue
		}

		// Apply limit when specified
		if limit > 0 && len(modelsList) >= limit {
			break
		}

		// Get model data
		reader, err := h.storage.Get(ctx, obj.Key)
		if err != nil {
			continue // Skip invalid models
		}

		var model models.LLMModel
		decoder := json.NewDecoder(reader)
		if err := decoder.Decode(&model); err != nil {
			_ = reader.Close()
			continue // Skip invalid models
		}
		_ = reader.Close()

		// Apply filters
		if provider != "" && model.Provider != provider {
			continue
		}

		if capability != "" && !model.HasCapability(capability) {
			continue
		}

		modelsList = append(modelsList, &model)
	}

	return c.JSON(fiber.Map{
		"models": modelsList,
		"count":  len(modelsList),
		"total":  len(objects), // Total before filtering
	})
}
