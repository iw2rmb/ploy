package mods

import (
	"context"
	"encoding/json"
	"io"
	"strings"

	"github.com/gofiber/fiber/v2"
	internalmods "github.com/iw2rmb/ploy/internal/mods"
)

// GetModReport handles GET /v1/mods/:id/report and returns the saved Mods report in JSON or Markdown.
func (h *Handler) GetModReport(c *fiber.Ctx) error {
	modID := c.Params("id")
	if strings.TrimSpace(modID) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    "missing_id",
				"message": "Execution ID is required",
			},
		})
	}

	status, err := h.getStatus(modID)
	if err != nil || status == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    "not_found",
				"message": "Mod execution not found",
			},
		})
	}

	if h.storage == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    "report_storage_disabled",
				"message": "Report storage is not configured",
			},
		})
	}

	ctx := context.Background()
	key := reportStorageKey(modID)
	exists, err := h.storage.Exists(ctx, key)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    "storage_error",
				"message": "Failed to check report availability",
				"details": err.Error(),
			},
		})
	}
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    "report_unavailable",
				"message": "Report is not available for this execution",
			},
		})
	}

	rc, err := h.storage.Get(ctx, key)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    "storage_error",
				"message": "Failed to load report",
				"details": err.Error(),
			},
		})
	}
	defer func() { _ = rc.Close() }()

	data, err := io.ReadAll(rc)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    "storage_error",
				"message": "Failed to read report",
				"details": err.Error(),
			},
		})
	}

	format := strings.ToLower(strings.TrimSpace(c.Query("format", "json")))
	switch format {
	case "markdown", "md":
		var report internalmods.ModReport
		if err := json.Unmarshal(data, &report); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": fiber.Map{
					"code":    "report_decode_error",
					"message": "Failed to decode stored report",
					"details": err.Error(),
				},
			})
		}
		markdown := internalmods.RenderModReportMarkdown(report)
		c.Set(fiber.HeaderContentType, "text/markdown; charset=utf-8")
		return c.SendString(markdown)
	case "json", "":
		fallthrough
	default:
		c.Set(fiber.HeaderContentType, "application/json")
		return c.Send(data)
	}
}
