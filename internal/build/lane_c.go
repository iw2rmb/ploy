package build

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	clutils "github.com/iw2rmb/ploy/internal/cli/utils"
	"github.com/iw2rmb/ploy/internal/orchestration"
)

// buildLaneC builds an OSv image using the dedicated builder job and returns imagePath.
func buildLaneC(c *fiber.Ctx, deps *BuildDependencies, appName, srcDir, sha, mainClass, detectedJavaVersion, tmpDir string) (string, error) {
	builderTar := filepath.Join(tmpDir, "context.tar")
	if err := func() error {
		f, err := os.Create(builderTar)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		ign, _ := clutils.ReadGitignore(srcDir)
		return clutils.TarDir(srcDir, f, ign)
	}(); err != nil {
		return "", fmt.Errorf("create build context: %w", err)
	}
	ctxKey := fmt.Sprintf("builds/%s/%s/src.tar", appName, sha)
	var ctxURL string
	if deps.Storage != nil {
		ctxUp := context.Context(c.Context())
		if err := uploadFileWithUnifiedStorage(ctxUp, deps.Storage, builderTar, ctxKey, "application/x-tar"); err != nil {
			return "", fmt.Errorf("failed to upload build context: %w", err)
		}
		base := os.Getenv("PLOY_SEAWEEDFS_URL")
		if base == "" {
			base = "http://seaweedfs-filer.service.consul:8888"
		}
		if !strings.HasPrefix(base, "http") {
			base = "http://" + base
		}
		ctxURL = strings.TrimRight(base, "/") + "/" + ctxKey
	} else {
		return "", fmt.Errorf("storage not available for build context upload")
	}
	outPath := fmt.Sprintf("/opt/ploy/artifacts/%s-%s-osv.qemu", appName, sha)
	jobFile, err := orchestration.RenderOSVBuilder(appName, sha, outPath, ctxURL, mainClass, detectedJavaVersion)
	if err != nil {
		return "", err
	}
	if vErr := orchestration.ValidateJob(jobFile); vErr != nil {
		return "", fmt.Errorf("OSv builder job validation failed: %w", vErr)
	}
	if err := orchestration.SubmitAndWaitTerminal(jobFile, 10*time.Minute); err != nil {
		jobName := fmt.Sprintf("%s-c-build-%s", appName, sha)
		snippet := getJobLogsSnippet(jobName, 80)
		be := &BuildError{
			Type:    "lane_c_build",
			Message: fmt.Sprintf("OSv builder failed for job %s", jobName),
			Details: err.Error(),
			Stdout:  snippet,
		}
		formatted := FormatBuildError(be, true, 4000)
		c.Set("X-Deployment-ID", jobName)
		return "", c.Status(500).JSON(fiber.Map{ //nolint:wrapcheck
			"error":   formatted,
			"builder": fiber.Map{"job": jobName, "logs": snippet},
		})
	}
	return outPath, nil
}
