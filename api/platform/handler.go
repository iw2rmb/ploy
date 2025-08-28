package platform

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/hashicorp/nomad/api"
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
		// Use registry domain name instead of localhost
		tag := fmt.Sprintf("registry.dev.ployman.app/platform-%s:%s", serviceName, sha)
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

	// Store build metadata in storage backend
	metadata := map[string]interface{}{
		"service":      serviceName,
		"sha":          sha,
		"lane":         lane,
		"environment":  environment,
		"docker_image": dockerImage,
		"platform":     true,
		"build_time":   fmt.Sprintf("%d", time.Now().Unix()),
	}

	// Store metadata in storage for tracking
	metadataKey := fmt.Sprintf("platform/%s/%s/metadata", serviceName, sha)
	if err := h.storageClient.Store(metadataKey, metadata); err != nil {
		// Log warning but don't fail deployment
		fmt.Printf("Warning: Failed to store platform metadata: %v\n", err)
	}

	// Deploy to Nomad with platform-specific configuration
	if err := h.deployToNomad(serviceName, dockerImage, environment, serviceEnvVars); err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to deploy platform service to Nomad",
			"details": err.Error(),
		})
	}

	// Return success
	return c.JSON(fiber.Map{
		"success": true,
		"service": serviceName,
		"version": sha,
		"environment": environment,
		"docker_image": dockerImage,
		"message": fmt.Sprintf("Platform service %s deployed successfully to %s", serviceName, environment),
	})
}

// GetPlatformStatus returns the status of a platform service
func (h *Handler) GetPlatformStatus(c *fiber.Ctx) error {
	serviceName := c.Params("service")
	
	// Create Nomad client
	nomadConfig := api.DefaultConfig()
	nomadAddr := utils.Getenv("NOMAD_ADDR", "http://127.0.0.1:4646")
	nomadConfig.Address = nomadAddr
	
	nomadClient, err := api.NewClient(nomadConfig)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to connect to Nomad",
			"details": err.Error(),
		})
	}

	// Get job status
	jobName := fmt.Sprintf("platform-%s", serviceName)
	jobs := nomadClient.Jobs()
	job, _, err := jobs.Info(jobName, nil)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{
			"service": serviceName,
			"status": "not_found",
			"error": "Platform service not found",
		})
	}

	// Get allocations
	allocs, _, err := jobs.Allocations(jobName, false, nil)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to get allocation status",
			"details": err.Error(),
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
		"service": serviceName,
		"status": status,
		"job_status": job.Status,
		"running_instances": runningCount,
		"total_allocations": len(allocs),
		"message": fmt.Sprintf("Platform service %s status retrieved", serviceName),
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
	// Create Nomad client
	nomadConfig := api.DefaultConfig()
	nomadAddr := utils.Getenv("NOMAD_ADDR", "http://127.0.0.1:4646")
	nomadConfig.Address = nomadAddr
	
	nomadClient, err := api.NewClient(nomadConfig)
	if err != nil {
		return fmt.Errorf("failed to create Nomad client: %w", err)
	}

	// Generate platform-specific Nomad job
	job := h.generatePlatformNomadJob(serviceName, dockerImage, environment, envVars)
	
	// Submit job to Nomad
	jobs := nomadClient.Jobs()
	resp, _, err := jobs.Register(job, nil)
	if err != nil {
		return fmt.Errorf("failed to register Nomad job: %w", err)
	}

	// Wait for deployment to stabilize
	if resp != nil && resp.EvalID != "" {
		// Basic deployment verification - could be enhanced with proper health checks
		evaluations := nomadClient.Evaluations()
		eval, _, err := evaluations.Info(resp.EvalID, nil)
		if err != nil {
			return fmt.Errorf("failed to get evaluation info: %w", err)
		}
		
		if eval.Status == "failed" {
			return fmt.Errorf("deployment evaluation failed: %s", eval.StatusDescription)
		}
	}

	return nil
}

// generatePlatformNomadJob generates a Nomad job specification for platform services
func (h *Handler) generatePlatformNomadJob(serviceName, dockerImage, environment string, envVars map[string]string) *api.Job {
	jobName := fmt.Sprintf("platform-%s", serviceName)
	
	// Create job
	job := &api.Job{
		ID:          &jobName,
		Name:        &jobName,
		Type:        stringPtr("service"),
		Priority:    intPtr(80),
		Datacenters: []string{"dc1"},
		TaskGroups: []*api.TaskGroup{
			{
				Name:  stringPtr(serviceName),
				Count: intPtr(2), // HA deployment
				RestartPolicy: &api.RestartPolicy{
					Attempts: intPtr(3),
					Interval: durationPtr("10m"),
					Delay:    durationPtr("30s"),
					Mode:     stringPtr("fail"),
				},
				ReschedulePolicy: &api.ReschedulePolicy{
					Attempts: intPtr(3),
					Interval: durationPtr("1h"),
				},
				Update: &api.UpdateStrategy{
					MaxParallel:     intPtr(1),
					MinHealthyTime:  durationPtr("30s"),
					HealthyDeadline: durationPtr("5m"),
					AutoRevert:      boolPtr(true),
					Canary:          intPtr(1),
				},
				Tasks: []*api.Task{
					{
						Name:   serviceName,
						Driver: "docker",
						Config: map[string]interface{}{
							"image": dockerImage,
							"ports": []string{"http"},
							"auth": []map[string]string{
								{
									"server_address": "registry.dev.ployman.app",
								},
							},
						},
						Env: envVars,
						Resources: &api.Resources{
							CPU:      intPtr(500),
							MemoryMB: intPtr(512),
							Networks: []*api.NetworkResource{
								{
									DynamicPorts: []api.Port{
										{
											Label: "http",
										},
									},
								},
							},
						},
						Services: []*api.Service{
							{
								Name: fmt.Sprintf("platform-%s", serviceName),
								Port: "http",
								Tags: []string{
									"platform",
									fmt.Sprintf("platform-%s", serviceName),
									"traefik.enable=true",
									fmt.Sprintf("traefik.http.routers.platform-%s.rule=Host(`%s.%s.ployman.app`)", serviceName, serviceName, environment),
									fmt.Sprintf("traefik.http.routers.platform-%s.tls=true", serviceName),
									fmt.Sprintf("traefik.http.routers.platform-%s.tls.certresolver=dev-wildcard", serviceName),
								},
								Checks: []*api.ServiceCheck{
									{
										Type:     "http",
										Path:     "/health",
										Interval: durationPtr("15s"),
										Timeout:  durationPtr("10s"),
									},
									{
										Name:     "readiness",
										Type:     "http",
										Path:     "/ready",
										Interval: durationPtr("20s"),
										Timeout:  durationPtr("15s"),
									},
								},
							},
						},
					},
				},
			},
		},
	}

	return job
}

// Helper functions for Nomad job generation
func stringPtr(s string) *string { return &s }
func intPtr(i int) *int { return &i }
func boolPtr(b bool) *bool { return &b }
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