package build

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	orchestration "github.com/iw2rmb/ploy/internal/orchestration"
	"github.com/iw2rmb/ploy/internal/validation"
)

// Status retrieves the deployment status of an application
func Status(c *fiber.Ctx) error {
	appName := c.Params("app")

	// Validate app name
	if err := validation.ValidateAppName(appName); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error":   "Invalid app name",
			"details": err.Error(),
		})
	}

	// Get status from Nomad for all lanes that might be running
	lanes := []string{"a", "b", "c", "d", "e", "f", "g"}
	var activeJob *orchestration.JobStatus
	var lastError error

	monitor := orchestration.NewHealthMonitor()

	for _, lane := range lanes {
		jobName := appName + "-lane-" + lane
		if job, err := monitor.GetJobStatus(jobName); err == nil && job != nil {
			activeJob = job
			break
		} else if err != nil {
			lastError = err
		}
	}

	if activeJob == nil {
		// No active job found, check if it's a known app
		if lastError != nil {
			return c.Status(404).JSON(fiber.Map{
				"status":  "not_found",
				"message": "App not found or not deployed",
				"details": lastError.Error(),
			})
		}
		return c.Status(404).JSON(fiber.Map{
			"status":  "not_found",
			"message": "App not found or not deployed",
		})
	}

	// Map Nomad job status to ARF-compatible status
	status := mapNomadStatusToARF(activeJob.Status)

	return c.JSON(fiber.Map{
		"status":        status,
		"job_name":      activeJob.Name,
		"lane":          extractLaneFromJobName(activeJob.Name),
		"deployment_id": activeJob.ID,
		"job_type":      activeJob.Type,
		"stable":        activeJob.Stable,
		"version":       activeJob.Version,
	})
}

// mapNomadStatusToARF converts Nomad job status to ARF-compatible status
func mapNomadStatusToARF(nomadStatus string) string {
	switch strings.ToLower(nomadStatus) {
	case "pending":
		return "building"
	case "running":
		return "deploying"
	case "dead":
		return "stopped"
	default:
		// For running allocations, we need to check health
		return "running"
	}
}

// extractLaneFromJobName extracts lane from job name format: appname-lane-x
func extractLaneFromJobName(jobName string) string {
	parts := strings.Split(jobName, "-lane-")
	if len(parts) == 2 {
		return strings.ToUpper(parts[1])
	}
	return "unknown"
}
