package build

import (
	"os"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// sbomFeatureEnabled returns whether SBOM generation is enabled for this request.
// Controlled by env PLOY_SBOM_ENABLED (default true). Query param sbom=false disables per-request.
func sbomFeatureEnabled(c *fiber.Ctx) bool {
	// Per-request override
	if v := strings.ToLower(c.Query("sbom", "")); v == "false" || v == "0" || v == "off" {
		return false
	}
	// Global toggle
	env := strings.ToLower(os.Getenv("PLOY_SBOM_ENABLED"))
	if env == "false" || env == "0" || env == "off" {
		return false
	}
	return true
}

// sbomFailOnError returns whether SBOM generation errors should fail the build.
// Controlled by env PLOY_SBOM_FAIL_ON_ERROR (default false).
func sbomFailOnError() bool {
	v := strings.ToLower(os.Getenv("PLOY_SBOM_FAIL_ON_ERROR"))
	return v == "true" || v == "1" || v == "on"
}
