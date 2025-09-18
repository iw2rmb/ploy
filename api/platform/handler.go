package platform

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	envstore "github.com/iw2rmb/ploy/internal/envstore"
	orchestration "github.com/iw2rmb/ploy/internal/orchestration"
	"github.com/iw2rmb/ploy/internal/storage"
	"github.com/iw2rmb/ploy/internal/utils"
)

// Handler handles platform service deployments
type Handler struct {
	storageClient *storage.StorageClient // Legacy storage client (for backward compatibility)
	storage       storage.Storage        // Unified storage interface (preferred)
	envStore      envstore.EnvStoreInterface
}

// NewHandler creates a new platform handler (legacy - for backward compatibility)
func NewHandler(storageClient *storage.StorageClient, envStore envstore.EnvStoreInterface) *Handler {
	return &Handler{
		storageClient: storageClient,
		envStore:      envStore,
	}
}

// NewHandlerWithStorage creates a new platform handler with unified storage interface
func NewHandlerWithStorage(storage storage.Storage, envStore envstore.EnvStoreInterface) *Handler {
	return &Handler{
		storage:  storage,
		envStore: envStore,
	}
}

// DeployPlatformService handles platform service deployment requests
func (h *Handler) DeployPlatformService(c *fiber.Ctx) error {
	serviceName := c.Params("service")

	// Validate service name (platform services have stricter naming)
	if err := validatePlatformServiceName(serviceName); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error":   "Invalid platform service name",
			"details": err.Error(),
		})
	}

	sha := c.Query("sha", "latest")
	lane := c.Query("lane", "D") // Platform services default to Docker lane
	environment := c.Query("env", "dev")

	// Create temp directory for build
	tmpDir, _ := os.MkdirTemp("", fmt.Sprintf("platform-%s-", serviceName))
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Save uploaded tar
	tarPath := filepath.Join(tmpDir, "src.tar")
	f, _ := os.Create(tarPath)
	defer func() { _ = f.Close() }()
	if _, err := f.Write(c.Body()); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error":   "Failed to read request body",
			"details": err.Error(),
		})
	}

	// Extract source
	srcDir := filepath.Join(tmpDir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "mkdir src"})
	}
	_ = utils.Untar(tarPath, srcDir)

	// Get environment variables for the service
	serviceEnvVars, err := h.envStore.GetAll(fmt.Sprintf("platform-%s", serviceName))
	if err != nil {
		serviceEnvVars = make(map[string]string)
	}

	// Add platform-specific environment variables
	serviceEnvVars["PLOY_PLATFORM_SERVICE"] = "true"
	serviceEnvVars["PLOY_SERVICE_NAME"] = serviceName
	serviceEnvVars["PLOY_ENVIRONMENT"] = environment

	// Build based on lane (platform services now use Docker lane)
	if strings.ToUpper(lane) != "D" {
		return c.Status(400).JSON(fiber.Map{
			"error":   "Invalid lane for platform service",
			"details": fmt.Sprintf("Platform services must use Lane D (Docker), got %s", lane),
		})
	}

	// Use registry domain name instead of localhost
	dockerImage := fmt.Sprintf("registry.dev.ployman.app/platform-%s:%s", serviceName, sha)
	builderID, err := h.buildPlatformDockerImage(c, serviceName, sha, srcDir, dockerImage, serviceEnvVars)
	if err != nil {
		return err
	}

	// Generate build timestamp
	buildTime := time.Now().Unix()

	// Store metadata in storage for tracking
	metadataKey := fmt.Sprintf("platform/%s/%s/metadata.json", serviceName, sha)
	metadataJSON := fmt.Sprintf(`{"service":"%s","sha":"%s","lane":"%s","environment":"%s","docker_image":"%s","platform":true,"build_time":"%d"}`,
		serviceName, sha, lane, environment, dockerImage, buildTime)

	// Prefer unified storage interface if available
	if h.storage != nil {
		// Use unified storage interface with context
		ctx := c.Context()
		reader := strings.NewReader(metadataJSON)
		if err := h.storage.Put(ctx, metadataKey, reader, storage.WithContentType("application/json")); err != nil {
			// Log warning but don't fail deployment
			fmt.Printf("Warning: Failed to store platform metadata: %v\n", err)
		}
	} else if h.storageClient != nil {
		// Fallback to legacy storage client
		bucket := h.storageClient.GetArtifactsBucket()
		if _, err := h.storageClient.PutObject(bucket, metadataKey, strings.NewReader(metadataJSON), "application/json"); err != nil {
			// Log warning but don't fail deployment
			fmt.Printf("Warning: Failed to store platform metadata: %v\n", err)
		}
	}

	// Deploy to Nomad with platform-specific configuration
	if err := h.deployToNomad(serviceName, dockerImage, environment, serviceEnvVars); err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error":   "Failed to deploy platform service to Nomad",
			"details": err.Error(),
		})
	}

	// Return success
	return c.JSON(fiber.Map{
		"success":      true,
		"service":      serviceName,
		"version":      sha,
		"environment":  environment,
		"docker_image": dockerImage,
		"builder_id":   builderID,
		"message":      fmt.Sprintf("Platform service %s deployed successfully to %s", serviceName, environment),
	})
}

// GetPlatformStatus returns the status of a platform service
func (h *Handler) GetPlatformStatus(c *fiber.Ctx) error {
	serviceName := c.Params("service")

	// Query status via orchestration health monitor (SDK under the hood)
	jobName := fmt.Sprintf("platform-%s", serviceName)
	monitor := orchestration.NewHealthMonitor()
	allocs, err := monitor.GetJobAllocations(jobName)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{
			"service": serviceName,
			"status":  "not_found",
			"error":   "Platform service not found",
		})
	}

	// Count running allocations
	runningCount := 0
	for _, alloc := range allocs {
		if alloc.ClientStatus == "running" {
			runningCount++
		}
	}

	status := "stopped"
	if runningCount > 0 {
		status = "running"
	}

	return c.JSON(fiber.Map{
		"service":           serviceName,
		"status":            status,
		"running_instances": runningCount,
		"total_allocations": len(allocs),
		"message":           fmt.Sprintf("Platform service %s status retrieved", serviceName),
	})
}

// validatePlatformServiceName validates platform service naming
func validatePlatformServiceName(name string) error {
	// Platform services have stricter naming rules
	if len(name) < 2 || len(name) > 50 {
		return fmt.Errorf("service name must be between 2 and 50 characters")
	}

	// Must start with letter, contain only letters, numbers, hyphens
	validName := regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	if !validName.MatchString(name) {
		return fmt.Errorf("service name must start with a letter and contain only lowercase letters, numbers, and hyphens")
	}

	return nil
}

// deployToNomad deploys a platform service to Nomad
func (h *Handler) deployToNomad(serviceName, dockerImage, environment string, envVars map[string]string) error {
	// Render HCL for platform service and submit via orchestration (uses wrapper on VPS)
	jobName := fmt.Sprintf("platform-%s", serviceName)
	traefikHost := fmt.Sprintf("%s.%s.ployman.app", serviceName, environment)
	hcl := orchestration.RenderServiceDockerJobHCL(jobName, serviceName, serviceName, dockerImage, envVars, traefikHost, "default-acme", environment)

	tmp, err := os.CreateTemp("", "platform-*.hcl")
	if err != nil {
		return fmt.Errorf("create temp job file: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.WriteString(hcl); err != nil {
		return fmt.Errorf("write job HCL: %w", err)
	}
	_ = tmp.Close()

	if err := orchestration.Submit(tmp.Name()); err != nil {
		return fmt.Errorf("submit job via orchestration: %w", err)
	}
	return nil
}

// generatePlatformNomadJob generates a Nomad job specification for platform services
// generatePlatformNomadJob is no longer used; submission now goes through HCL + orchestration
// Keeping helper functions below for compatibility

// Helper functions for Nomad job generation
func stringPtr(s string) *string { return &s }
func intPtr(i int) *int          { return &i }
func boolPtr(b bool) *bool       { return &b }
func durationPtr(d string) *time.Duration {
	dur, _ := time.ParseDuration(d)
	return &dur
}

// SetupRoutes sets up platform-specific routes
func SetupRoutes(app *fiber.App, handler *Handler) {
	api := app.Group("/v1/platform")

	// Platform service deployment
	api.Post("/:service/deploy", handler.DeployPlatformService)
	api.Get("/:service/status", handler.GetPlatformStatus)

	// Future: Add more platform-specific endpoints
	// api.Post("/:service/rollback", handler.RollbackPlatformService)
	// api.Delete("/:service", handler.RemovePlatformService)
	// api.Get("/:service/logs", handler.GetPlatformLogs)
	// api.Post("/:service/scale", handler.ScalePlatformService)
}

func (h *Handler) buildPlatformDockerImage(c *fiber.Ctx, serviceName, sha, srcDir, dockerImage string, envVars map[string]string) (string, error) {
	builderID := fmt.Sprintf("%s-d-build-%s-%d", serviceName, sha, time.Now().Unix())
	logsKey := fmt.Sprintf("build-logs/%s.log", builderID)
	logBuffer := &bytes.Buffer{}
	logWriter := io.MultiWriter(logBuffer, os.Stdout)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	if err := runDockerCommand(ctx, logWriter, srcDir, []string{"build", "-t", dockerImage, "."}, envVars); err != nil {
		_ = h.persistPlatformBuildLog(logBuffer.Bytes(), logsKey)
		return "", fiber.NewError(fiber.StatusBadGateway, fmt.Sprintf("docker build failed for %s: %v", builderID, err))
	}

	if err := runDockerCommand(ctx, logWriter, srcDir, []string{"push", dockerImage}, envVars); err != nil {
		_ = h.persistPlatformBuildLog(logBuffer.Bytes(), logsKey)
		return "", fiber.NewError(fiber.StatusBadGateway, fmt.Sprintf("docker push failed for %s: %v", builderID, err))
	}

	if err := h.persistPlatformBuildLog(logBuffer.Bytes(), logsKey); err != nil {
		fmt.Printf("[platform] warning: failed to persist build log for %s: %v\n", builderID, err)
	}

	c.Set("X-Deployment-ID", builderID)
	return builderID, nil
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

func (h *Handler) persistPlatformBuildLog(data []byte, key string) error {
	if len(data) == 0 || strings.TrimSpace(key) == "" {
		return nil
	}

	if err := writeLocalPlatformBuildLog(key, data); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	if h.storage != nil {
		reader := bytes.NewReader(data)
		return h.storage.Put(ctx, key, reader, storage.WithContentType("text/plain"))
	}

	if h.storageClient != nil {
		bucket := h.storageClient.GetArtifactsBucket()
		reader := bytes.NewReader(data)
		_, err := h.storageClient.PutObject(bucket, key, reader, "text/plain")
		return err
	}

	return nil
}

func writeLocalPlatformBuildLog(key string, data []byte) error {
	path := filepath.Join("/opt/ploy/build-logs", key)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create build log directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write build log: %w", err)
	}
	return nil
}
