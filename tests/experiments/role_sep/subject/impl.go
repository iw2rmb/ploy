//go:build experiment && experiment_impl
// +build experiment,experiment_impl

package rolesep

import (
    "context"
    "strings"

    "github.com/iw2rmb/ploy/internal/node/logstream"
)

// PublishRecord publishes a structured log record to the hub.
func PublishRecord(ctx context.Context, hub *logstream.Hub, jobID, stream, line string) error {
    if hub == nil || strings.TrimSpace(jobID) == "" { return nil }
    hub.Ensure(jobID)
    return hub.PublishLog(ctx, jobID, logstream.LogRecord{Timestamp: "T", Stream: strings.TrimSpace(stream), Line: line})
}

// SnapshotHasRecord checks hub snapshot for a matching log record.
func SnapshotHasRecord(hub *logstream.Hub, jobID, stream, line string) bool {
    if hub == nil || strings.TrimSpace(jobID) == "" { return false }
    events := hub.Snapshot(jobID)
    for _, e := range events {
        if e.Type != "log" { continue }
        var rec logstream.LogRecord
        // quick decode since tests only
        // lightweight JSON parse without stdlib to keep it simple in experiment
        data := string(e.Data)
        if strings.Contains(data, "\"stream\":\""+stream+"\"") && strings.Contains(data, "\"line\":\""+line+"\"") {
            _ = rec // placeholder to appease linters; actual match via string contains
            return true
        }
    }
    return false
}

// CancelRunning simulates accepted cancel for a running job.
func CancelRunning(jobID string) bool { return strings.TrimSpace(jobID) != "" }

