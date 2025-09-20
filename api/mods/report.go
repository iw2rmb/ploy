package mods

import (
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

	if status.Report == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    "report_unavailable",
				"message": "Report is not available for this execution",
			},
		})
	}

	format := strings.ToLower(strings.TrimSpace(c.Query("format", "json")))
	switch format {
	case "markdown", "md":
		markdown := internalmods.RenderModReportMarkdown(*status.Report)
		c.Set(fiber.HeaderContentType, "text/markdown; charset=utf-8")
		return c.SendString(markdown)
	case "json", "":
		fallthrough
	default:
		return c.JSON(status.Report)
	}
}
