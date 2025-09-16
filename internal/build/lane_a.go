package build

import (
	"github.com/gofiber/fiber/v2"
	ibuilders "github.com/iw2rmb/ploy/internal/builders"
)

// buildLaneAB handles lanes A and B via Unikraft builders and returns imagePath.
func buildLaneAB(c *fiber.Ctx, deps *BuildDependencies, appName, lane, srcDir, sha, tmpDir string, appEnvVars map[string]string) (string, error) {
	img, err := ibuilders.BuildUnikraft(appName, lane, srcDir, sha, tmpDir, appEnvVars)
	if err != nil {
		return "", err
	}
	return img, nil
}
