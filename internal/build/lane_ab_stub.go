package build

import (
    "github.com/gofiber/fiber/v2"
)

// buildLaneAB is a minimal stub for Dev deployments. It returns a placeholder image path.
// Lane A/B (Unikraft/MicroVM) builders are not required for API runtime; this avoids compile errors.
func buildLaneAB(_ *fiber.Ctx, _ *BuildDependencies, appName, lane, _ string, sha string, _ string, _ map[string]string) (string, error) {
    // Return a deterministic placeholder path under /opt/ploy/artifacts to satisfy later steps.
    return "/opt/ploy/artifacts/" + appName + "-" + lane + "-" + sha + ".img", nil
}

