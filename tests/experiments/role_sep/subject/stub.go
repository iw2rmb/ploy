//go:build experiment && experiment_stub
// +build experiment,experiment_stub

package rolesep

import (
    "context"
    "errors"

    "github.com/iw2rmb/ploy/internal/node/logstream"
)

// PublishRecord is a stub that drops the record (used to force RED phase).
func PublishRecord(ctx context.Context, hub *logstream.Hub, jobID, stream, line string) error {
    if hub == nil || jobID == "" { return errors.New("hub/job required") }
    // Intentionally do not publish to simulate missing implementation.
    return nil
}

// SnapshotHasRecord always returns false in the stub.
func SnapshotHasRecord(hub *logstream.Hub, jobID, stream, line string) bool {
    return false
}

// CancelRunning simulates no running job present.
func CancelRunning(jobID string) bool { return false }

