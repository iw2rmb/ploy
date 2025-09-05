package cleanup

import (
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
)

// CleanupHandler manages HTTP endpoints for TTL cleanup
type CleanupHandler struct {
	service       *TTLCleanupService
	configManager *ConfigManager
}

// NewCleanupHandler creates a new cleanup handler
func NewCleanupHandler(service *TTLCleanupService, configManager *ConfigManager) *CleanupHandler {
	return &CleanupHandler{
		service:       service,
		configManager: configManager,
	}
}

// GetStatus returns the current status of the cleanup service
func (h *CleanupHandler) GetStatus(c *fiber.Ctx) error {
	stats, err := h.service.GetStats()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": fmt.Sprintf("Failed to get cleanup stats: %v", err),
		})
	}

	status := fiber.Map{
		"service":     "ttl-cleanup",
		"status":      "ok",
		"statistics":  stats,
		"config_path": h.configManager.GetConfigPath(),
	}

	return c.JSON(status)
}

// GetConfig returns the current cleanup configuration
func (h *CleanupHandler) GetConfig(c *fiber.Ctx) error {
	config := h.configManager.GetConfig()

	response := fiber.Map{
		"config":      config,
		"config_path": h.configManager.GetConfigPath(),
		"defaults":    DefaultTTLConfig(),
	}

	return c.JSON(response)
}

// UpdateConfig updates the cleanup configuration
func (h *CleanupHandler) UpdateConfig(c *fiber.Ctx) error {
	var updates map[string]interface{}
	if err := c.BodyParser(&updates); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error":   "Invalid JSON body",
			"details": err.Error(),
		})
	}

	// Update configuration
	if err := h.configManager.UpdateConfig(updates); err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error":   "Failed to update configuration",
			"details": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"status":  "updated",
		"message": "Configuration updated successfully",
		"config":  h.configManager.GetConfig(),
	})
}

// TriggerCleanup manually triggers a cleanup run
func (h *CleanupHandler) TriggerCleanup(c *fiber.Ctx) error {
	dryRun := c.Query("dry_run", "false") == "true"

	log.Printf("Manual cleanup triggered (dry_run: %v)", dryRun)

	// Temporarily set dry run mode if requested
	originalConfig := h.configManager.GetConfig()
	originalDryRun := originalConfig.DryRun

	if dryRun && !originalDryRun {
		originalConfig.DryRun = true
	}

	// Create a temporary service with the updated config
	tempService := NewTTLCleanupService(originalConfig)

	// Run cleanup once
	if err := tempService.runCleanup(); err != nil {
		// Restore original dry run setting
		if dryRun && !originalDryRun {
			originalConfig.DryRun = originalDryRun
		}

		return c.Status(500).JSON(fiber.Map{
			"error":   "Cleanup failed",
			"details": err.Error(),
		})
	}

	// Restore original dry run setting
	if dryRun && !originalDryRun {
		originalConfig.DryRun = originalDryRun
	}

	// Get updated stats
	stats, err := tempService.GetStats()
	if err != nil {
		log.Printf("Failed to get stats after manual cleanup: %v", err)
		stats = map[string]interface{}{"error": "stats unavailable"}
	}

	return c.JSON(fiber.Map{
		"status":     "completed",
		"message":    "Manual cleanup completed",
		"dry_run":    dryRun,
		"statistics": stats,
	})
}

// StartService starts the TTL cleanup service
func (h *CleanupHandler) StartService(c *fiber.Ctx) error {
	if h.service.IsRunning() {
		return c.Status(400).JSON(fiber.Map{
			"error": "TTL cleanup service is already running",
		})
	}

	if err := h.service.Start(); err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error":   "Failed to start TTL cleanup service",
			"details": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"status":  "started",
		"message": "TTL cleanup service started successfully",
	})
}

// StopService stops the TTL cleanup service
func (h *CleanupHandler) StopService(c *fiber.Ctx) error {
	if !h.service.IsRunning() {
		return c.Status(400).JSON(fiber.Map{
			"error": "TTL cleanup service is not running",
		})
	}

	if err := h.service.Stop(); err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error":   "Failed to stop TTL cleanup service",
			"details": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"status":  "stopped",
		"message": "TTL cleanup service stopped successfully",
	})
}

// ListPreviewJobs lists all current preview jobs with their ages
func (h *CleanupHandler) ListPreviewJobs(c *fiber.Ctx) error {
	includeAll := c.Query("include_all", "false") == "true"
	limit := 100 // Default limit

	if limitStr := c.Query("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}

	// Get all Nomad jobs
	jobs, err := h.service.getNomadJobs()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error":   "Failed to get Nomad jobs",
			"details": err.Error(),
		})
	}

	// Identify preview jobs
	previewJobs := h.service.identifyPreviewJobs(jobs)

	// Determine which jobs should be cleaned up
	jobsToClean := h.service.determineJobsToClean(previewJobs)

	// Prepare response
	response := fiber.Map{
		"total_jobs":    len(jobs),
		"preview_jobs":  len(previewJobs),
		"jobs_to_clean": len(jobsToClean),
		"preview_ttl":   h.service.config.PreviewTTL.String(),
		"max_age":       h.service.config.MaxAge.String(),
	}

	// Include job details if requested or if listing jobs to clean
	var jobList []map[string]interface{}

	jobsToList := jobsToClean
	if includeAll {
		// Convert all preview jobs to the same format
		for _, job := range previewJobs {
			found := false
			for _, cleanJob := range jobsToClean {
				if cleanJob.JobName == job.JobName {
					found = true
					break
				}
			}
			if !found {
				job.ShouldClean = false
				job.Reason = "within TTL"
				jobsToList = append(jobsToList, job)
			}
		}
	}

	// Apply limit
	if len(jobsToList) > limit {
		jobsToList = jobsToList[:limit]
		response["limited"] = true
		response["limit"] = limit
	}

	for _, job := range jobsToList {
		jobInfo := map[string]interface{}{
			"job_name":     job.JobName,
			"app":          job.App,
			"sha":          job.SHA,
			"age":          job.Age.Round(time.Minute).String(),
			"age_seconds":  int(job.Age.Seconds()),
			"should_clean": job.ShouldClean,
			"reason":       job.Reason,
		}
		jobList = append(jobList, jobInfo)
	}

	response["jobs"] = jobList

	return c.JSON(response)
}

// ConfigDefaults returns the default configuration values
func (h *CleanupHandler) ConfigDefaults(c *fiber.Ctx) error {
	defaults := DefaultTTLConfig()

	return c.JSON(fiber.Map{
		"defaults": defaults,
		"environment_variables": map[string]string{
			"PLOY_PREVIEW_TTL":      "Duration for preview allocation TTL (e.g., '24h', '2h30m')",
			"PLOY_CLEANUP_INTERVAL": "How often to run cleanup (e.g., '6h', '30m')",
			"PLOY_MAX_PREVIEW_AGE":  "Maximum age for any preview allocation (e.g., '7d', '168h')",
			"PLOY_CLEANUP_DRY_RUN":  "Set to 'true' for dry run mode (default: false)",
			"NOMAD_ADDR":            "Nomad API address (default: http://127.0.0.1:4646)",
		},
		"examples": map[string]interface{}{
			"preview_ttl":      "24h",
			"cleanup_interval": "6h",
			"max_age":          "168h",
			"dry_run":          false,
			"nomad_addr":       "http://127.0.0.1:4646",
		},
	})
}

// SetupRoutes sets up the HTTP routes for cleanup management
func SetupRoutes(app *fiber.App, handler *CleanupHandler) {
	cleanup := app.Group("/v1/cleanup")

	// Status and statistics
	cleanup.Get("/status", handler.GetStatus)
	cleanup.Get("/jobs", handler.ListPreviewJobs)

	// Configuration management
	cleanup.Get("/config", handler.GetConfig)
	cleanup.Put("/config", handler.UpdateConfig)
	cleanup.Get("/config/defaults", handler.ConfigDefaults)

	// Service control
	cleanup.Post("/start", handler.StartService)
	cleanup.Post("/stop", handler.StopService)
	cleanup.Post("/trigger", handler.TriggerCleanup)
}
