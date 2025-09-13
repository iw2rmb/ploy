package arf

import "github.com/gofiber/fiber/v2"

// Health returns a simple health status for ARF subsystem
func (h *Handler) Health(c *fiber.Ctx) error {
	components := fiber.Map{}
	if h.recipeRegistry != nil {
		components["registry"] = "available"
	} else {
		components["registry"] = "unavailable"
	}
	if h.sandboxMgr != nil {
		components["sandbox"] = "available"
	} else {
		components["sandbox"] = "unavailable"
	}
	if h.recipeValidator != nil {
		components["validator"] = "available"
	} else {
		components["validator"] = "unavailable"
	}

	return c.JSON(fiber.Map{
		"status":     "healthy",
		"components": components,
	})
}
