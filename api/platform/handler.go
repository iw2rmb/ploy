package platform

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/iw2rmb/ploy/api/builders"
	"github.com/iw2rmb/ploy/api/envstore"
	"github.com/iw2rmb/ploy/internal/storage"
	"github.com/iw2rmb/ploy/internal/utils"
)

// Handler handles platform service deployments
type Handler struct {
	storageClient *storage.StorageClient
	envStore      envstore.EnvStoreInterface
}

// NewHandler creates a new platform handler
func NewHandler(storageClient *storage.StorageClient, envStore envstore.EnvStoreInterface) *Handler {
	return &Handler{
		storageClient: storageClient,
		envStore:      envStore,
	}
}

// DeployPlatformService handles platform service deployment requests
func (h *Handler) DeployPlatformService(c *fiber.Ctx) error {
	serviceName := c.Params("service")
	
	// Validate service name (platform services have stricter naming)
	if err := validatePlatformServiceName(serviceName); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid platform service name",
			"details": err.Error(),
		})
	}

	sha := c.Query("sha", "latest")
	lane := c.Query("lane", "E") // Platform services default to containers
	environment := c.Query("env", "dev")

	// Create temp directory for build
	tmpDir, _ := os.MkdirTemp("", fmt.Sprintf("platform-%s-", serviceName))
	defer os.RemoveAll(tmpDir)

	// Save uploaded tar
	tarPath := filepath.Join(tmpDir, "src.tar")
	f, _ := os.Create(tarPath)
	defer f.Close()
	if _, err := f.Write(c.Body()); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Failed to read request body",
			"details": err.Error(),
		})
	}

	// Extract source
	srcDir := filepath.Join(tmpDir, "src")
	os.MkdirAll(srcDir, 0755)
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
		tag := fmt.Sprintf("localhost:5000/platform-%s:%s", serviceName, sha)
		img, err := builders.BuildOCI(serviceName, srcDir, tag, serviceEnvVars)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{
				"error": "Failed to build platform service",
				"details": err.Error(),
			})
		}
		dockerImage = img
	default:
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid lane for platform service",
			"details": fmt.Sprintf("Platform services must use Lane E (containers), got %s", lane),
		})
	}

	// Store build metadata in platform-specific location
	metadata := map[string]interface{}{
		"service":     serviceName,
		"sha":         sha,
		"lane":        lane,
		"environment": environment,
		"docker_image": dockerImage,
		"platform":    true,
	}

	// TODO: Store metadata in storage backend
	_ = metadata

	// TODO: Deploy to Nomad with platform-specific configuration
	// For now, just return success to test the routing

	// Return success
	return c.JSON(fiber.Map{
		"success": true,
		"service": serviceName,
		"version": sha,
		"environment": environment,
		"message": fmt.Sprintf("Platform service %s deployed successfully", serviceName),
	})
}

// GetPlatformStatus returns the status of a platform service
func (h *Handler) GetPlatformStatus(c *fiber.Ctx) error {
	serviceName := c.Params("service")
	
	// TODO: Get actual status from storage/Nomad
	// For now, return mock status to test routing
	
	return c.JSON(fiber.Map{
		"service": serviceName,
		"status": "running",
		"message": "Platform service status endpoint",
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