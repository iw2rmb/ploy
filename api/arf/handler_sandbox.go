package arf

import (
	"time"

	"github.com/gofiber/fiber/v2"
)

// ListSandboxes handles GET /v1/arf/sandboxes
func (h *Handler) ListSandboxes(c *fiber.Ctx) error {
	if h.sandboxMgr == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "Sandbox manager not available",
		})
	}

	sandboxes, err := h.sandboxMgr.ListSandboxes(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "Failed to list sandboxes",
			"details": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"sandboxes": sandboxes,
		"count":     len(sandboxes),
	})
}

// CreateSandbox handles POST /v1/arf/sandboxes
func (h *Handler) CreateSandbox(c *fiber.Ctx) error {
	if h.sandboxMgr == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "Sandbox manager not available",
		})
	}

	var config SandboxConfig
	if err := c.BodyParser(&config); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Invalid request format",
			"details": err.Error(),
		})
	}

	// Set defaults if not provided
	if config.TTL == 0 {
		config.TTL = 30 * time.Minute
	}
	if config.MemoryLimit == "" {
		config.MemoryLimit = "1G"
	}

	sandbox, err := h.sandboxMgr.CreateSandbox(c.Context(), config)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "Failed to create sandbox",
			"details": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(sandbox)
}

// DestroySandbox handles DELETE /v1/arf/sandboxes/:id
func (h *Handler) DestroySandbox(c *fiber.Ctx) error {
	if h.sandboxMgr == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "Sandbox manager not available",
		})
	}

	sandboxID := c.Params("id")
	if sandboxID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Sandbox ID is required",
		})
	}

	err := h.sandboxMgr.DestroySandbox(c.Context(), sandboxID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "Failed to destroy sandbox",
			"details": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message":    "Sandbox destroyed successfully",
		"sandbox_id": sandboxID,
	})
}
