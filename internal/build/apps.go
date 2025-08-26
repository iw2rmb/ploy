package build

import "github.com/gofiber/fiber/v2"

// ListApps returns a list of deployed applications
func ListApps(c *fiber.Ctx) error {
	// TODO: Implement actual app listing from Nomad
	return c.JSON(fiber.Map{"apps": []string{}})
}