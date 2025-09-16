package mods

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
)

// GetModStatus handles GET /v1/mods/:id/status
func (h *Handler) GetModStatus(c *fiber.Ctx) error {
	modID := c.Params("id")
	if modID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    "missing_id",
				"message": "Execution ID is required",
			},
		})
	}

	// Retrieve status from store
	status, err := h.getStatus(modID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    "not_found",
				"message": fmt.Sprintf("Mod execution %s not found", modID),
			},
		})
	}

	// Enrich statuses: add duration and overdue fields without changing stored state
	if status.Status == "running" {
		elapsed := time.Since(status.StartTime)
		status.Duration = elapsed.String()
		overdueThresh := 30 * time.Minute
		if v := os.Getenv("PLOY_MODS_OVERDUE"); v != "" {
			if d, e := time.ParseDuration(v); e == nil && d > 0 {
				overdueThresh = d
			}
		}
		status.Overdue = elapsed > overdueThresh
	} else if (status.Status == "completed" || status.Status == "failed" || status.Status == "cancelled") && status.EndTime != nil {
		// Compute duration for terminal states if not already set
		if status.Duration == "" {
			status.Duration = status.EndTime.Sub(status.StartTime).String()
		}
	}

	return c.JSON(status)
}

// ListMods handles GET /v1/mods
func (h *Handler) ListMods(c *fiber.Ctx) error {
	// List all mod executions from the status store
	prefix := "mods/status/"
	keys, err := h.statusStore.Keys(prefix, "")
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    "storage_error",
				"message": "Failed to list mod executions",
				"details": err.Error(),
			},
		})
	}

	executions := []ModStatus{}
	for _, key := range keys {
		data, err := h.statusStore.Get(key)
		if err != nil {
			continue
		}

		var status ModStatus
		if err := json.Unmarshal([]byte(data), &status); err != nil {
			continue
		}
		executions = append(executions, status)
	}

	return c.JSON(fiber.Map{
		"executions": executions,
		"count":      len(executions),
	})
}

// CancelMod handles DELETE /v1/mods/:id
func (h *Handler) CancelMod(c *fiber.Ctx) error {
	modID := c.Params("id")
	if modID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    "missing_id",
				"message": "Execution ID is required",
			},
		})
	}

	// Check if execution exists
	status, err := h.getStatus(modID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    "not_found",
				"message": fmt.Sprintf("Mod execution %s not found", modID),
			},
		})
	}

	// Can only cancel running executions
	if status.Status != "running" && status.Status != "initializing" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    "invalid_state",
				"message": fmt.Sprintf("Cannot cancel execution in state: %s", status.Status),
			},
		})
	}

	// Update status to cancelled
	endTime := time.Now()
	status.Status = "cancelled"
	status.EndTime = &endTime
	if err := h.storeStatus(*status); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    "storage_error",
				"message": "Failed to update execution status",
				"details": err.Error(),
			},
		})
	}

	return c.JSON(fiber.Map{
		"message": fmt.Sprintf("Mod execution %s cancelled", modID),
		"status":  status,
	})
}

// storeStatus stores the status in the KV store
func (h *Handler) storeStatus(status ModStatus) error {
	if h.statusStore == nil {
		return nil // Silently skip if no store configured
	}

	key := fmt.Sprintf("mods/status/%s", status.ID)
	data, err := json.Marshal(status)
	if err != nil {
		return err
	}

	return h.statusStore.Put(key, data)
}

// getStatus retrieves the status from the KV store
func (h *Handler) getStatus(modID string) (*ModStatus, error) {
	if h.statusStore == nil {
		return nil, fmt.Errorf("status store not configured")
	}

	key := fmt.Sprintf("mods/status/%s", modID)
	data, err := h.statusStore.Get(key)
	if err != nil {
		return nil, err
	}

	var status ModStatus
	if err := json.Unmarshal([]byte(data), &status); err != nil {
		return nil, err
	}

	return &status, nil
}
