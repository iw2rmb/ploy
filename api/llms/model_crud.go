package llms

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/iw2rmb/ploy/internal/arf/models"
	"github.com/iw2rmb/ploy/internal/storage"
)

// GetModel returns a specific LLM model
func (h *Handler) GetModel(c *fiber.Ctx) error {
	ctx := context.Background()
	modelID := c.Params("id")

	if modelID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "model ID is required",
		})
	}

	// Get model from storage
	key := fmt.Sprintf("llms/models/%s", modelID)
	reader, err := h.storage.Get(ctx, key)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": fmt.Sprintf("model not found: %s", modelID),
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fmt.Sprintf("failed to get model: %v", err),
		})
	}
	defer func() { _ = reader.Close() }()

	var model models.LLMModel
	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(&model); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to parse model data",
		})
	}

	return c.JSON(model)
}

// CreateModel creates a new LLM model
func (h *Handler) CreateModel(c *fiber.Ctx) error {
	ctx := context.Background()

	var model models.LLMModel
	if err := c.BodyParser(&model); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	// Validate the model
	if err := h.validator.ValidateLLMModel(&model); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fmt.Sprintf("validation failed: %v", err),
		})
	}

	// Check if model already exists
	key := fmt.Sprintf("llms/models/%s", model.ID)
	exists, err := h.storage.Exists(ctx, key)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to check model existence",
		})
	}
	if exists {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{
			"error": fmt.Sprintf("model already exists: %s", model.ID),
		})
	}

	// Set system fields
	model.SetSystemFields()

	// Serialize model
	modelData, err := json.Marshal(model)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to serialize model",
		})
	}

	// Store in storage
	reader := strings.NewReader(string(modelData))
	if err := h.storage.Put(ctx, key, reader, storage.WithContentType("application/json")); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fmt.Sprintf("failed to store model: %v", err),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"id":      model.ID,
		"message": "model created successfully",
	})
}

// UpdateModel updates an existing LLM model
func (h *Handler) UpdateModel(c *fiber.Ctx) error {
	ctx := context.Background()
	modelID := c.Params("id")

	if modelID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "model ID is required",
		})
	}

	var updatedModel models.LLMModel
	if err := c.BodyParser(&updatedModel); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	// Ensure the ID in the URL matches the model ID
	if updatedModel.ID != modelID {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "model ID in URL does not match model ID in body",
		})
	}

	// Get existing model
	key := fmt.Sprintf("llms/models/%s", modelID)
	reader, err := h.storage.Get(ctx, key)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": fmt.Sprintf("model not found: %s", modelID),
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fmt.Sprintf("failed to get existing model: %v", err),
		})
	}

	var existingModel models.LLMModel
	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(&existingModel); err != nil {
		_ = reader.Close()
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to parse existing model data",
		})
	}
	_ = reader.Close()

	// Validate update
	if err := h.validator.ValidateModelUpdate(&existingModel, &updatedModel); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fmt.Sprintf("validation failed: %v", err),
		})
	}

	// Preserve creation time
	updatedModel.Created = existingModel.Created
	updatedModel.SetSystemFields()

	// Serialize updated model
	modelData, err := json.Marshal(updatedModel)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to serialize updated model",
		})
	}

	// Update in storage
	updateReader := strings.NewReader(string(modelData))
	if err := h.storage.Put(ctx, key, updateReader, storage.WithContentType("application/json")); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fmt.Sprintf("failed to update model: %v", err),
		})
	}

	return c.JSON(fiber.Map{
		"id":      modelID,
		"message": "model updated successfully",
	})
}

// DeleteModel deletes an LLM model
func (h *Handler) DeleteModel(c *fiber.Ctx) error {
	ctx := context.Background()
	modelID := c.Params("id")

	if modelID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "model ID is required",
		})
	}

	// Check if model exists
	key := fmt.Sprintf("llms/models/%s", modelID)
	exists, err := h.storage.Exists(ctx, key)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to check model existence",
		})
	}
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": fmt.Sprintf("model not found: %s", modelID),
		})
	}

	// Delete from storage
	if err := h.storage.Delete(ctx, key); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fmt.Sprintf("failed to delete model: %v", err),
		})
	}

	return c.JSON(fiber.Map{
		"id":      modelID,
		"message": "model deleted successfully",
	})
}
