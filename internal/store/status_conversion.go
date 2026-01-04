package store

import (
	"fmt"
)

// ConvertToJobStatus converts a job status string to the canonical JobStatus.
// Only v1 canonical values are accepted; non-canonical aliases are rejected.
//
// v1 status model (see roadmap/v1/statuses.md:127):
//   - "Created" -> JobStatusCreated
//   - "Queued" -> JobStatusQueued (replaces v0 "pending")
//   - "Running" -> JobStatusRunning
//   - "Success" -> JobStatusSuccess (replaces v0 "succeeded")
//   - "Fail" -> JobStatusFail (replaces v0 "failed")
//   - "Cancelled" -> JobStatusCancelled (replaces v0 "canceled")
//
// Removed in v1: "skipped" (see roadmap/v1/statuses.md:138).
func ConvertToJobStatus(status string) (JobStatus, error) {
	switch status {
	case "Created":
		return JobStatusCreated, nil
	case "Queued":
		return JobStatusQueued, nil
	case "Running":
		return JobStatusRunning, nil
	case "Success":
		return JobStatusSuccess, nil
	case "Fail":
		return JobStatusFail, nil
	case "Cancelled":
		return JobStatusCancelled, nil
	default:
		return "", fmt.Errorf("unknown job status: %q", status)
	}
}

// ConvertToRunStatus converts a run status string to the canonical RunStatus.
// Only v1 canonical values are accepted; non-canonical aliases are rejected.
//
// v1 status model (see roadmap/v1/statuses.md:113):
//   - "Started" -> RunStatusStarted (replaces v0 "queued"/"assigned"/"running")
//   - "Cancelled" -> RunStatusCancelled (replaces v0 "canceled")
//   - "Finished" -> RunStatusFinished (replaces v0 "succeeded"/"failed")
//
// Removed in v1: "queued", "assigned", "running", "succeeded", "failed" (see roadmap/v1/statuses.md:114).
func ConvertToRunStatus(status string) (RunStatus, error) {
	switch status {
	case "Started":
		return RunStatusStarted, nil
	case "Cancelled":
		return RunStatusCancelled, nil
	case "Finished":
		return RunStatusFinished, nil
	default:
		return "", fmt.Errorf("unknown run status: %q", status)
	}
}

// ValidateJobStatus validates that a string is a valid v1 JobStatus value.
// Returns the typed status if valid, otherwise returns an error.
//
// v1 valid values: Created, Queued, Running, Success, Fail, Cancelled.
// Removed in v1: skipped (see roadmap/v1/statuses.md:138).
func ValidateJobStatus(status string) (JobStatus, error) {
	s := JobStatus(status)
	switch s {
	case JobStatusCreated, JobStatusQueued, JobStatusRunning,
		JobStatusSuccess, JobStatusFail, JobStatusCancelled:
		return s, nil
	default:
		return "", fmt.Errorf("invalid job status: %q (expected: Created, Queued, Running, Success, Fail, Cancelled)", status)
	}
}

// ValidateRunStatus validates that a string is a valid v1 RunStatus value.
// Returns the typed status if valid, otherwise returns an error.
//
// v1 valid values: Started, Cancelled, Finished.
// Removed in v1: queued, assigned, running, succeeded, failed (see roadmap/v1/statuses.md:114).
func ValidateRunStatus(status string) (RunStatus, error) {
	s := RunStatus(status)
	switch s {
	case RunStatusStarted, RunStatusCancelled, RunStatusFinished:
		return s, nil
	default:
		return "", fmt.Errorf("invalid run status: %q (expected: Started, Cancelled, Finished)", status)
	}
}

// ValidateRunRepoStatus validates that a string is a valid v1 RunRepoStatus value.
// Returns the typed status if valid, otherwise returns an error.
// RunRepoStatus tracks per-repo execution state within a batched run.
//
// v1 valid values: Queued, Running, Cancelled, Fail, Success.
// Removed in v1: pending (now Queued), skipped, succeeded (now Success), failed (now Fail).
// See roadmap/v1/statuses.md:116.
func ValidateRunRepoStatus(status string) (RunRepoStatus, error) {
	s := RunRepoStatus(status)
	switch s {
	case RunRepoStatusQueued, RunRepoStatusRunning, RunRepoStatusCancelled,
		RunRepoStatusFail, RunRepoStatusSuccess:
		return s, nil
	default:
		return "", fmt.Errorf("invalid run repo status: %q (expected: Queued, Running, Cancelled, Fail, Success)", status)
	}
}

// ConvertToRunRepoStatus converts a run repo status string to the canonical RunRepoStatus type.
// Only v1 canonical values are accepted; non-canonical aliases are rejected.
//
// v1 status model (see roadmap/v1/statuses.md:116):
//   - "Queued" -> RunRepoStatusQueued (replaces v0 "pending")
//   - "Running" -> RunRepoStatusRunning
//   - "Cancelled" -> RunRepoStatusCancelled
//   - "Fail" -> RunRepoStatusFail (replaces v0 "failed")
//   - "Success" -> RunRepoStatusSuccess (replaces v0 "succeeded")
//
// Removed in v1: "skipped" (see roadmap/v1/statuses.md:40).
func ConvertToRunRepoStatus(status string) (RunRepoStatus, error) {
	switch status {
	case "Queued":
		return RunRepoStatusQueued, nil
	case "Running":
		return RunRepoStatusRunning, nil
	case "Cancelled":
		return RunRepoStatusCancelled, nil
	case "Fail":
		return RunRepoStatusFail, nil
	case "Success":
		return RunRepoStatusSuccess, nil
	default:
		return "", fmt.Errorf("unknown run repo status: %q", status)
	}
}
