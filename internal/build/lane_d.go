package build

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/iw2rmb/ploy/internal/config"
	"github.com/iw2rmb/ploy/internal/detect/project"
)

// buildLaneD builds the application using Docker directly on the controller host.
// It returns the fully qualified image reference and a synthetic builder identifier
// used for log retrieval.
func buildLaneD(c *fiber.Ctx, deps *BuildDependencies, buildCtx *BuildContext, appName, srcDir, sha string, facts project.BuildFacts, appEnvVars map[string]string) (dockerImage, builderID string, err error) {
	_ = facts
	registry := config.GetRegistryConfigForAppType(buildCtx.AppType)
	dockerImage = registry.GetDockerImageTag(appName, sha, buildCtx.AppType)
	if strings.TrimSpace(dockerImage) == "" {
		return "", "", fiber.NewError(fiber.StatusInternalServerError, "empty docker image tag")
	}

	// Prepare logging
	nonce := time.Now().Unix()
	builderID = fmt.Sprintf("%s-d-build-%s-%d", appName, sha, nonce)
	logBuffer := &bytes.Buffer{}
	logWriter := io.MultiWriter(logBuffer, os.Stdout)
	logsKey := fmt.Sprintf("build-logs/%s.log", builderID)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	// docker build
	if err := runDockerCommand(ctx, logWriter, srcDir, []string{"build", "-t", dockerImage, "."}, appEnvVars); err != nil {
		_ = persistBuildLog(context.Context(c.Context()), deps, logBuffer.Bytes(), logsKey)
		return "", builderID, fiber.NewError(fiber.StatusBadGateway, fmt.Sprintf("docker build failed: %v", err))
	}

	// docker push
	if err := runDockerCommand(ctx, logWriter, srcDir, []string{"push", dockerImage}, appEnvVars); err != nil {
		_ = persistBuildLog(context.Context(c.Context()), deps, logBuffer.Bytes(), logsKey)
		return "", builderID, fiber.NewError(fiber.StatusBadGateway, fmt.Sprintf("docker push failed: %v", err))
	}

	if err := persistBuildLog(context.Context(c.Context()), deps, logBuffer.Bytes(), logsKey); err != nil {
		fmt.Printf("[Lane D] warning: failed to persist build log: %v\n", err)
	}

	c.Set("X-Deployment-ID", builderID)
	return dockerImage, builderID, nil
}

func runDockerCommand(ctx context.Context, writer io.Writer, dir string, args []string, env map[string]string) error {
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = dir
	cmd.Stdout = writer
	cmd.Stderr = writer
	envList := os.Environ()
	for k, v := range env {
		if strings.TrimSpace(k) == "" {
			continue
		}
		envList = append(envList, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Env = envList
	return cmd.Run()
}

func persistBuildLog(ctx context.Context, deps *BuildDependencies, data []byte, key string) error {
	if len(data) == 0 {
		return nil
	}
	if err := writeLocalBuildLog(key, data); err != nil {
		return err
	}
	if deps != nil && deps.Storage != nil {
		return uploadBytesWithUnifiedStorage(ctx, deps.Storage, data, key, "text/plain")
	}
	return nil
}

func writeLocalBuildLog(key string, data []byte) error {
	if strings.TrimSpace(key) == "" {
		return nil
	}
	path := filepath.Join("/opt/ploy/build-logs", key)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create build log directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write build log: %w", err)
	}
	return nil
}
