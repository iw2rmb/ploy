package grid

import (
    "fmt"
    "strings"
    "time"

    workflowsdk "github.com/iw2rmb/grid/sdk/workflowrpc/go"

    "github.com/iw2rmb/ploy/internal/workflow/contracts"
    "github.com/iw2rmb/ploy/internal/workflow/runner"
)

func buildSubmitRequest(ticket contracts.WorkflowTicket, stage runner.Stage, workflowID string) (workflowsdk.SubmitRequest, error) {
    // Construct request directly to avoid legacy tenant validation in helper.
    req := workflowsdk.SubmitRequest{
        Tenant:         "", // tenant concept removed
        WorkflowID:     strings.TrimSpace(workflowID),
        IdempotencyKey: idempotencyKey(ticket, stage),
        RunMetadata: workflowsdk.RunMetadata{
            CorrelationID: strings.TrimSpace(ticket.TicketID),
            Labels:        map[string]string{"stage": stage.Name, "lane": stage.Lane, "ticket_id": ticket.TicketID},
        },
    }

    manifest := stage.Constraints.Manifest.Manifest
    if manifest.Name != "" {
        if req.RunMetadata.Labels == nil { req.RunMetadata.Labels = make(map[string]string) }
        req.RunMetadata.Labels["manifest_name"] = manifest.Name
    }
    if manifest.Version != "" {
        if req.RunMetadata.Labels == nil { req.RunMetadata.Labels = make(map[string]string) }
        req.RunMetadata.Labels["manifest_version"] = manifest.Version
    }

    // Job
    // Job metadata uses map[string]any in the SDK; upgrade values accordingly.
    jobMeta := make(map[string]any, len(stage.Job.Metadata))
    for k, v := range stage.Job.Metadata { jobMeta[k] = v }
    if v, ok := jobMeta["priority"]; !ok || strings.TrimSpace(fmt.Sprint(v)) == "" {
        jobMeta["priority"] = "standard"
    }
    if stage.Lane != "" {
        jobMeta["lane"] = stage.Lane
    }
    if stage.CacheKey != "" {
        jobMeta["cache_key"] = stage.CacheKey
    }
    if manifest.Name != "" {
        jobMeta["manifest_name"] = manifest.Name
    }
    if manifest.Version != "" {
        jobMeta["manifest_version"] = manifest.Version
    }
    if stage.Job.Resources.CPU != "" {
        jobMeta["resources.cpu"] = stage.Job.Resources.CPU
    }
    if stage.Job.Resources.Memory != "" {
        jobMeta["resources.memory"] = stage.Job.Resources.Memory
    }
    if stage.Job.Resources.Disk != "" {
        jobMeta["resources.disk"] = stage.Job.Resources.Disk
    }
    if stage.Job.Resources.GPU != "" {
        jobMeta["resources.gpu"] = stage.Job.Resources.GPU
    }
    if len(stage.Aster.Toggles) > 0 {
        jobMeta["aster.toggles"] = append([]string(nil), stage.Aster.Toggles...)
    }
    if len(stage.Aster.Bundles) > 0 {
        // Preserve bundle metadata shape for downstream consumers.
        // Use []map[string]any to avoid extra types.
        bundles := make([]map[string]any, 0, len(stage.Aster.Bundles))
        for _, b := range stage.Aster.Bundles {
            bundles = append(bundles, map[string]any{
                "bundle_id":   b.BundleID,
                "stage":       b.Stage,
                "toggle":      b.Toggle,
                "artifact_cid": b.ArtifactCID,
                "digest":      b.Digest,
            })
        }
        jobMeta["aster.bundles"] = bundles
    }

    req.Job = workflowsdk.JobSpec{
        Image:    strings.TrimSpace(stage.Job.Image),
        Command:  append([]string(nil), stage.Job.Command...),
        Env:      copyStringMap(stage.Job.Env),
        Metadata: jobMeta,
        Runtime:  strings.TrimSpace(stage.Job.Runtime),
    }

    return req, nil
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
