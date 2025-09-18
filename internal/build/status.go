package build

import (
	"sort"
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
	// Prefer exact lane G (WASM) before container E to avoid misreporting
	lanes := []string{"g", "e", "c", "d", "b", "a", "f"}
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

	// Map Nomad job status to deployment status used by Mods-aware clients
	status := mapNomadStatusToARF(activeJob.Status)

	// Include allocation summaries with recent task events to enable event-driven clients
	allocs, _ := orchestration.NewHealthMonitor().GetJobAllocations(activeJob.Name)
	// Build a compact representation with capped events per task
	const maxTasks = 4
	const maxEventsPerTask = 4
	type taskEvent struct {
		Type           string `json:"type"`
		Time           int64  `json:"time"`
		Message        string `json:"message"`
		DisplayMessage string `json:"display_message"`
	}
	type taskSummary struct {
		Name   string      `json:"name"`
		State  string      `json:"state"`
		Failed bool        `json:"failed"`
		Events []taskEvent `json:"events,omitempty"`
	}
	type allocSummary struct {
		ID            string        `json:"id"`
		ClientStatus  string        `json:"client_status"`
		DesiredStatus string        `json:"desired_status"`
		Healthy       *bool         `json:"healthy,omitempty"`
		Tasks         []taskSummary `json:"tasks,omitempty"`
	}
	summaries := make([]allocSummary, 0, len(allocs))
	for _, a := range allocs {
		s := allocSummary{ID: a.ID, ClientStatus: a.ClientStatus, DesiredStatus: a.DesiredStatus}
		if a.DeploymentStatus != nil {
			s.Healthy = a.DeploymentStatus.Healthy
		}
		// deterministic task ordering
		if len(a.TaskStates) > 0 {
			names := make([]string, 0, len(a.TaskStates))
			for n := range a.TaskStates {
				names = append(names, n)
			}
			sort.Strings(names)
			for i, n := range names {
				if i >= maxTasks {
					break
				}
				ts := a.TaskStates[n]
				tsum := taskSummary{Name: n, State: ts.State, Failed: ts.Failed}
				// last few events, newest last
				if len(ts.Events) > 0 {
					// take last maxEventsPerTask events
					start := 0
					if len(ts.Events) > maxEventsPerTask {
						start = len(ts.Events) - maxEventsPerTask
					}
					for _, ev := range ts.Events[start:] {
						tsum.Events = append(tsum.Events, taskEvent{Type: ev.Type, Time: ev.Time, Message: ev.Message, DisplayMessage: ev.DisplayMessage})
					}
				}
				s.Tasks = append(s.Tasks, tsum)
			}
		}
		summaries = append(summaries, s)
	}

	return c.JSON(fiber.Map{
		"status":        status,
		"job_name":      activeJob.Name,
		"lane":          extractLaneFromJobName(activeJob.Name),
		"deployment_id": activeJob.ID,
		"job_type":      activeJob.Type,
		"stable":        activeJob.Stable,
		"version":       activeJob.Version,
		"allocations":   summaries,
	})
}

// mapNomadStatusToARF converts Nomad job status to a Mods-aligned deployment status.
func mapNomadStatusToARF(nomadStatus string) string {
	switch strings.TrimSpace(strings.ToLower(nomadStatus)) {
	case "pending":
		return "building"
	case "running":
		return "running"
	case "dead", "failed":
		return "failed"
	case "complete", "completed", "successful":
		return "running"
	case "":
		return "unknown"
	default:
		return "unknown"
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
