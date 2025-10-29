//go:build experiment
// +build experiment

package rolesep_test

import (
    "context"
    "testing"
    "time"

    rolesep "github.com/iw2rmb/ploy/tests/experiments/role_sep/subject"
    "github.com/iw2rmb/ploy/internal/node/logstream"
)

// HT-1: Posting a log entry results in a snapshot containing the log event with exact payload.
func TestHT_1_PostLogThenSnapshot_IncludesExactLogFrame(t *testing.T) {
    hub := logstream.NewHub(logstream.Options{BufferSize: 8, HistorySize: 32})
    ctx := context.Background()
    if err := rolesep.PublishRecord(ctx, hub, "job-exp", "stderr", "oops"); err != nil {
        t.Fatalf("publish: %v", err)
    }
    // Allow async propagation if any
    time.Sleep(5 * time.Millisecond)
    if !rolesep.SnapshotHasRecord(hub, "job-exp", "stderr", "oops") {
        t.Fatalf("snapshot missing expected log frame")
    }
}

// HT-2: Simulated cancel path returns accepted when job is running.
func TestHT_2_CancelAcceptedForRunning(t *testing.T) {
    // For experiment, we simulate by returning true from the subject CancelRunning stub
    if !rolesep.CancelRunning("job-running") {
        t.Fatalf("expected cancel accepted for running job")
    }
}

