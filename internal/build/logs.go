package build

import (
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/iw2rmb/ploy/controller/nomad"
	"github.com/iw2rmb/ploy/internal/validation"
)

// HealthMonitorInterface defines the interface for health monitoring
type HealthMonitorInterface interface {
	GetJobStatus(jobID string) (*nomad.JobStatus, error)
	GetJobAllocations(jobID string) ([]*nomad.AllocationStatus, error)
}

// GetLogs retrieves logs from the deployed application
func GetLogs(c *fiber.Ctx) error {
	return getLogsWithMonitor(c, nomad.NewHealthMonitor())
}

// getLogsWithMonitor is a testable version of GetLogs that accepts a health monitor
func getLogsWithMonitor(c *fiber.Ctx, monitor HealthMonitorInterface) error {
	appName := c.Params("app")
	
	// Validate app name
	if err := validation.ValidateAppName(appName); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid app name",
			"details": err.Error(),
		})
	}
	
	// Optional query parameters
	lines := c.Query("lines", "100")
	_ = c.Query("follow", "false") == "true" // follow parameter for future streaming implementation
	
	// Find the active job for this app across all lanes
	lanes := []string{"a", "b", "c", "d", "e", "f", "g"}
	var activeJobName string
	
	for _, lane := range lanes {
		jobName := appName + "-lane-" + lane
		if job, err := monitor.GetJobStatus(jobName); err == nil && job != nil {
			activeJobName = jobName
			break
		}
	}
	
	if activeJobName == "" {
		return c.Status(404).JSON(fiber.Map{
			"error": "App not found or not deployed",
		})
	}
	
	// Get allocations for the job
	allocations, err := monitor.GetJobAllocations(activeJobName)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to retrieve allocations",
			"details": err.Error(),
		})
	}
	
	if len(allocations) == 0 {
		return c.JSON(fiber.Map{
			"app_name": appName,
			"job_name": activeJobName,
			"logs": "No running allocations found",
			"lines_requested": lines,
			"timestamp": time.Now().UTC(),
		})
	}
	
	// For now, return allocation status info as logs
	// In a real implementation, we would use Nomad API to get actual logs
	logsText := fmt.Sprintf("Job: %s\nAllocations found: %d\n", activeJobName, len(allocations))
	for i, alloc := range allocations {
		logsText += fmt.Sprintf("Allocation %d: %s (%s)\n", i+1, alloc.ID, alloc.ClientStatus)
	}
	
	return c.JSON(fiber.Map{
		"app_name": appName,
		"job_name": activeJobName,
		"logs": logsText,
		"lines_requested": lines,
		"timestamp": time.Now().UTC(),
	})
}