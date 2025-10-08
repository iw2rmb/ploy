package grid

import (
	"fmt"
	"strings"
	"time"

	workflowsdk "github.com/iw2rmb/grid/sdk/workflowrpc/go"
	helper "github.com/iw2rmb/grid/sdk/workflowrpc/helper"

	"github.com/iw2rmb/ploy/internal/workflow/aster"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
)

func buildSubmitRequest(ticket contracts.WorkflowTicket, stage runner.Stage, workflowID string) (workflowsdk.SubmitRequest, error) {
	builder := helper.NewSubmitBuilder().
		Tenant(strings.TrimSpace(ticket.Tenant)).
		Workflow(strings.TrimSpace(workflowID)).
		Correlation(strings.TrimSpace(ticket.TicketID)).
		Idempotency(idempotencyKey(ticket, stage)).
		Label("stage", stage.Name).
		Label("lane", stage.Lane).
		Label("ticket_id", ticket.TicketID)

	manifest := stage.Constraints.Manifest.Manifest
	if manifest.Name != "" {
		builder.Label("manifest_name", manifest.Name)
	}
	if manifest.Version != "" {
		builder.Label("manifest_version", manifest.Version)
	}

	builder.Job(func(job *helper.JobBuilder) {
		if strings.TrimSpace(stage.Job.Image) != "" {
			job.Image(stage.Job.Image)
		}
		if len(stage.Job.Command) > 0 {
			job.Command(stage.Job.Command...)
		}
		for key, value := range stage.Job.Env {
			job.Env(key, value)
		}
		for key, value := range copyStringMap(stage.Job.Metadata) {
			job.Metadata(key, value)
		}
		if _, ok := stage.Job.Metadata["priority"]; !ok || strings.TrimSpace(stage.Job.Metadata["priority"]) == "" {
			job.Metadata("priority", "standard")
		}
		if stage.Lane != "" {
			job.Metadata("lane", stage.Lane)
		}
		if stage.CacheKey != "" {
			job.Metadata("cache_key", stage.CacheKey)
		}
		if manifest.Name != "" {
			job.Metadata("manifest_name", manifest.Name)
		}
		if manifest.Version != "" {
			job.Metadata("manifest_version", manifest.Version)
		}
		if stage.Job.Resources.CPU != "" {
			job.Metadata("resources.cpu", stage.Job.Resources.CPU)
		}
		if stage.Job.Resources.Memory != "" {
			job.Metadata("resources.memory", stage.Job.Resources.Memory)
		}
		if stage.Job.Resources.Disk != "" {
			job.Metadata("resources.disk", stage.Job.Resources.Disk)
		}
		if stage.Job.Resources.GPU != "" {
			job.Metadata("resources.gpu", stage.Job.Resources.GPU)
		}
		if len(stage.Aster.Toggles) > 0 {
			job.Metadata("aster.toggles", append([]string(nil), stage.Aster.Toggles...))
		}
		if len(stage.Aster.Bundles) > 0 {
			job.Metadata("aster.bundles", append([]aster.Metadata(nil), stage.Aster.Bundles...))
		}
	})

	return builder.Build()
}

func idempotencyKey(ticket contracts.WorkflowTicket, stage runner.Stage) string {
	base := strings.TrimSpace(ticket.TicketID)
	if base == "" {
		base = strings.TrimSpace(ticket.Manifest.Name)
	}
	stageName := strings.TrimSpace(stage.Name)
	if stageName == "" {
		stageName = "stage"
	}
	return fmt.Sprintf("%s:%s", base, stageName)
}

func mapRunStatus(status workflowsdk.RunStatus) runner.StageStatus {
	switch status {
	case workflowsdk.RunStatusSucceeded:
		return runner.StageStatusCompleted
	case workflowsdk.RunStatusFailed, workflowsdk.RunStatusCanceled:
		return runner.StageStatusFailed
	default:
		return runner.StageStatusRunning
	}
}

func isTerminalStatus(status workflowsdk.RunStatus) bool {
	switch status {
	case workflowsdk.RunStatusSucceeded, workflowsdk.RunStatusFailed, workflowsdk.RunStatusCanceled:
		return true
	default:
		return false
	}
}

func stageArchiveFromTerminal(term terminalRun) *runner.StageArchive {
	if len(term.result) == 0 {
		return nil
	}
	id := strings.TrimSpace(stringFromAny(term.result[archiveResultIDKey]))
	class := strings.TrimSpace(stringFromAny(term.result[archiveResultClassKey]))
	queued := strings.TrimSpace(stringFromAny(term.result[archiveResultQueuedAtKey]))
	if id == "" && class == "" && queued == "" {
		return nil
	}
	var queuedAt time.Time
	if queued != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, queued); err == nil {
			queuedAt = parsed
		}
	}
	return &runner.StageArchive{ID: id, Class: class, QueuedAt: queuedAt}
}

func copyStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneAnyMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func stringFromAny(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case time.Time:
		return v.Format(time.RFC3339Nano)
	case fmt.Stringer:
		return v.String()
	case []byte:
		return string(v)
	case bool:
		if v {
			return "true"
		}
		return "false"
	case int:
		return fmt.Sprintf("%d", v)
	case int64:
		return fmt.Sprintf("%d", v)
	case float64:
		return fmt.Sprintf("%f", v)
	default:
		return ""
	}
}
