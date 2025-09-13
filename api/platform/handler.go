package platform

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/iw2rmb/ploy/api/builders"
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
	lane := c.Query("lane", "E") // Platform services default to containers
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

	// Build based on lane (platform services typically use Lane E)
	var dockerImage string
	switch strings.ToUpper(lane) {
	case "E":
		// Use registry domain name instead of localhost
		tag := fmt.Sprintf("registry.dev.ployman.app/platform-%s:%s", serviceName, sha)
		img, err := builders.BuildOCI(serviceName, srcDir, tag, serviceEnvVars)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{
				"error":   "Failed to build platform service",
				"details": err.Error(),
			})
		}
		dockerImage = img
	default:
		return c.Status(400).JSON(fiber.Map{
			"error":   "Invalid lane for platform service",
			"details": fmt.Sprintf("Platform services must use Lane E (containers), got %s", lane),
		})
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
	hcl := orchestration.RenderServiceDockerJobHCL(jobName, serviceName, serviceName, dockerImage, envVars, traefikHost, "dev-wildcard", environment)

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
