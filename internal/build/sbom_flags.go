package build

import "github.com/gofiber/fiber/v2"

// sbomFeatureEnabled controls whether SBOM generation runs during build flows.
// In Dev fallback deployments, default to false to avoid breaking builds.
func sbomFeatureEnabled(_ *fiber.Ctx) bool { return false }

// sbomFailOnError determines whether SBOM generation errors are fatal.
// Default to false in Dev to keep deployments moving when optional features fail.
func sbomFailOnError() bool { return false }
