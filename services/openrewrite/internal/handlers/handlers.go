package handlers

import (
	"context"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/iw2rmb/ploy/services/openrewrite/internal/executor"
	"github.com/iw2rmb/ploy/services/openrewrite/internal/jobs"
	"github.com/iw2rmb/ploy/services/openrewrite/internal/storage"
)

// Handler handles HTTP requests for the OpenRewrite service
type Handler struct {
	executor   *executor.Executor
	jobManager *jobs.Manager
	storage    *storage.StorageClient
}

// New creates a new HTTP handler
func New(exec *executor.Executor, jobMgr *jobs.Manager, storageClient *storage.StorageClient) *Handler {
	return &Handler{
		executor:   exec,
		jobManager: jobMgr,
		storage:    storageClient,
	}
}

// Health handles health check requests
func (h *Handler) Health(c *fiber.Ctx) error {
	// Check all components
	healthStatus := map[string]interface{}{
		"status":    "healthy",
		"version":   "1.0.0",
		"timestamp": time.Now().Unix(),
	}
	
	// Check storage health
	if err := h.storage.Health(); err != nil {
		healthStatus["status"] = "degraded"
		healthStatus["storage_error"] = err.Error()
		return c.Status(503).JSON(healthStatus)
	}
	
	return c.JSON(healthStatus)
}

// Ready handles readiness check requests
func (h *Handler) Ready(c *fiber.Ctx) error {
	// Check if service is ready to handle requests
	ready := map[string]interface{}{
		"ready":     true,
		"timestamp": time.Now().Unix(),
	}
	
	// Check if essential components are available
	if h.executor == nil || h.jobManager == nil || h.storage == nil {
		ready["ready"] = false
		ready["error"] = "essential components not initialized"
		return c.Status(503).JSON(ready)
	}
	
	return c.JSON(ready)
}

// Transform handles synchronous transformation requests
func (h *Handler) Transform(c *fiber.Ctx) error {
	var request executor.TransformRequest
	if err := c.BodyParser(&request); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request body: " + err.Error(),
		})
	}
	
	// Generate job ID if not provided
	if request.JobID == "" {
		request.JobID = uuid.New().String()
	}
	
	// Validate request
	if request.TarArchive == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "tar_archive is required",
		})
	}
	if request.RecipeConfig.Recipe == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "recipe_config.recipe is required",
		})
	}
	
	log.Printf("[Transform] Starting synchronous transformation for job %s with recipe %s", 
		request.JobID, request.RecipeConfig.Recipe)
	
	// Execute transformation with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	
	result, err := h.executor.ExecuteTransformation(ctx, request)
	if err != nil {
		log.Printf("[Transform] Transformation failed for job %s: %v", request.JobID, err)
		return c.Status(500).JSON(fiber.Map{
			"error":  "Transformation failed: " + err.Error(),
			"job_id": request.JobID,
		})
	}
	
	log.Printf("[Transform] Transformation completed for job %s: success=%t, changes=%d", 
		request.JobID, result.Success, result.ChangesApplied)
	
	return c.JSON(result)
}

// CreateJob handles asynchronous job creation requests
func (h *Handler) CreateJob(c *fiber.Ctx) error {
	var request jobs.CreateJobRequest
	if err := c.BodyParser(&request); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request body: " + err.Error(),
		})
	}
	
	// Validate request
	if request.TarArchive == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "tar_archive is required",
		})
	}
	if request.RecipeConfig.Recipe == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "recipe_config.recipe is required",
		})
	}
	
	// Create job
	jobID, err := h.jobManager.CreateJob(request)
	if err != nil {
		log.Printf("[CreateJob] Failed to create job: %v", err)
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to create job: " + err.Error(),
		})
	}
	
	log.Printf("[CreateJob] Created job %s with recipe %s", jobID, request.RecipeConfig.Recipe)
	
	return c.Status(201).JSON(fiber.Map{
		"job_id": jobID,
	})
}

// GetJob handles job information requests
func (h *Handler) GetJob(c *fiber.Ctx) error {
	jobID := c.Params("id")
	if jobID == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "job_id is required",
		})
	}
	
	job, err := h.storage.GetJob(jobID)
	if err != nil {
		if err.Error() == "job "+jobID+" not found" {
			return c.Status(404).JSON(fiber.Map{
				"error": "Job not found",
			})
		}
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to get job: " + err.Error(),
		})
	}
	
	return c.JSON(job)
}

// GetJobStatus handles job status requests
func (h *Handler) GetJobStatus(c *fiber.Ctx) error {
	jobID := c.Params("id")
	if jobID == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "job_id is required",
		})
	}
	
	job, err := h.storage.GetJob(jobID)
	if err != nil {
		if err.Error() == "job "+jobID+" not found" {
			return c.Status(404).JSON(fiber.Map{
				"error": "Job not found",
			})
		}
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to get job: " + err.Error(),
		})
	}
	
	status := fiber.Map{
		"job_id":     job.JobID,
		"status":     job.Status,
		"progress":   job.Progress,
		"start_time": job.StartTime.Format(time.RFC3339),
	}
	
	if !job.EndTime.IsZero() {
		status["end_time"] = job.EndTime.Format(time.RFC3339)
	}
	
	if job.Error != "" {
		status["error"] = job.Error
	}
	
	return c.JSON(status)
}

// GetJobDiff handles job diff requests
func (h *Handler) GetJobDiff(c *fiber.Ctx) error {
	jobID := c.Params("id")
	if jobID == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "job_id is required",
		})
	}
	
	job, err := h.storage.GetJob(jobID)
	if err != nil {
		if err.Error() == "job "+jobID+" not found" {
			return c.Status(404).JSON(fiber.Map{
				"error": "Job not found",
			})
		}
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to get job: " + err.Error(),
		})
	}
	
	if job.Status != "completed" {
		return c.Status(400).JSON(fiber.Map{
			"error": "Job is not completed yet",
		})
	}
	
	diff, err := h.storage.GetJobDiff(jobID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to get job diff: " + err.Error(),
		})
	}
	
	c.Set("Content-Type", "text/plain")
	return c.Send(diff)
}

// CancelJob handles job cancellation requests
func (h *Handler) CancelJob(c *fiber.Ctx) error {
	jobID := c.Params("id")
	if jobID == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "job_id is required",
		})
	}
	
	err := h.jobManager.CancelJob(jobID)
	if err != nil {
		if err.Error() == "job "+jobID+" not found" {
			return c.Status(404).JSON(fiber.Map{
				"error": "Job not found",
			})
		}
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to cancel job: " + err.Error(),
		})
	}
	
	log.Printf("[CancelJob] Cancelled job %s", jobID)
	
	return c.JSON(fiber.Map{
		"message": "Job cancelled successfully",
		"job_id":  jobID,
	})
}

// Metrics handles metrics requests
func (h *Handler) Metrics(c *fiber.Ctx) error {
	// Get job statistics
	jobs, err := h.storage.ListJobs()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to get metrics: " + err.Error(),
		})
	}
	
	// Count jobs by status
	statusCount := make(map[string]int)
	totalJobs := len(jobs)
	
	for _, jobID := range jobs {
		job, err := h.storage.GetJob(jobID)
		if err != nil {
			continue // Skip jobs that can't be read
		}
		statusCount[job.Status]++
	}
	
	metrics := fiber.Map{
		"total_jobs":   totalJobs,
		"job_status":   statusCount,
		"service_info": fiber.Map{
			"version":   "1.0.0",
			"uptime":    "Service uptime not tracked", // Could add actual uptime tracking
		},
	}
	
	return c.JSON(metrics)
}