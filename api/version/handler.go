package version

import (
	"github.com/gofiber/fiber/v2"
	"github.com/iw2rmb/ploy/internal/version"
)

// RegisterRoutes registers version endpoints
func RegisterRoutes(app *fiber.App) {
	// Version endpoints (no /v1 prefix for version)
	app.Get("/version", GetVersion)
	app.Get("/version/detailed", GetDetailedVersion)

	// Also register under /v1 for consistency
	v1 := app.Group("/v1")
	v1.Get("/version", GetVersion)
	v1.Get("/version/detailed", GetDetailedVersion)
}

// GetVersion returns the version string
func GetVersion(c *fiber.Ctx) error {
	info := version.Get()
	return c.JSON(fiber.Map{
		"version":    version.Short(),
		"git_commit": info.GitCommit,
	})
}

// GetDetailedVersion returns detailed version information
func GetDetailedVersion(c *fiber.Ctx) error {
	return c.JSON(version.Get())
}
