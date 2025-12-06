package store

import (
	"fmt"
)

// ConvertToJobStatus converts various job status string representations
// to the canonical store.JobStatus type. This helper provides type-safe
// conversion from external API representations (e.g., mods API StageState)
// to the database-authoritative store.JobStatus.
//
// Supported mappings:
//   - "created" -> JobStatusCreated
//   - "pending" -> JobStatusPending
//   - "queued" -> JobStatusCreated (mods API compatibility)
//   - "running" -> JobStatusRunning
//   - "succeeded" -> JobStatusSucceeded
//   - "failed" -> JobStatusFailed
//   - "skipped" -> JobStatusSkipped
//   - "canceled"/"cancelled" -> JobStatusCanceled (US/UK spelling)
//   - "cancelling" -> JobStatusCanceled (mods API in-progress cancellation maps to final state)
func ConvertToJobStatus(status string) (JobStatus, error) {
	switch status {
	case "created", "queued":
		return JobStatusCreated, nil
	case "pending":
		return JobStatusPending, nil
	case "running":
		return JobStatusRunning, nil
	case "succeeded":
		return JobStatusSucceeded, nil
	case "failed":
		return JobStatusFailed, nil
	case "skipped":
		return JobStatusSkipped, nil
	case "canceled", "cancelled", "cancelling":
		return JobStatusCanceled, nil
	default:
		return "", fmt.Errorf("unknown job status: %q", status)
	}
}

// ConvertToRunStatus converts various run status string representations
// to the canonical store.RunStatus type. This helper provides type-safe
// conversion from external API representations (e.g., mods API TicketState)
// to the database-authoritative store.RunStatus.
//
// Supported mappings:
//   - "pending" -> RunStatusQueued (mods API compatibility)
//   - "queued" -> RunStatusQueued
//   - "assigned" -> RunStatusAssigned
//   - "running" -> RunStatusRunning
//   - "succeeded" -> RunStatusSucceeded
//   - "failed" -> RunStatusFailed
//   - "canceled"/"cancelled" -> RunStatusCanceled (US/UK spelling)
//   - "cancelling" -> RunStatusCanceled (mods API in-progress cancellation maps to final state)
func ConvertToRunStatus(status string) (RunStatus, error) {
	switch status {
	case "pending", "queued":
		return RunStatusQueued, nil
	case "assigned":
		return RunStatusAssigned, nil
	case "running":
		return RunStatusRunning, nil
	case "succeeded":
		return RunStatusSucceeded, nil
	case "failed":
		return RunStatusFailed, nil
	case "canceled", "cancelled", "cancelling":
		return RunStatusCanceled, nil
	default:
		return "", fmt.Errorf("unknown run status: %q", status)
	}
}

// ValidateJobStatus validates that a string is a valid JobStatus value.
// Returns the typed status if valid, otherwise returns an error.
func ValidateJobStatus(status string) (JobStatus, error) {
	s := JobStatus(status)
	switch s {
	case JobStatusCreated, JobStatusPending, JobStatusRunning, JobStatusSucceeded,
		JobStatusFailed, JobStatusSkipped, JobStatusCanceled:
		return s, nil
	default:
		return "", fmt.Errorf("invalid job status: %q (expected: created, pending, running, succeeded, failed, skipped, canceled)", status)
	}
}

// ValidateRunStatus validates that a string is a valid RunStatus value.
// Returns the typed status if valid, otherwise returns an error.
func ValidateRunStatus(status string) (RunStatus, error) {
	s := RunStatus(status)
	switch s {
	case RunStatusQueued, RunStatusAssigned, RunStatusRunning,
		RunStatusSucceeded, RunStatusFailed, RunStatusCanceled:
		return s, nil
	default:
		return "", fmt.Errorf("invalid run status: %q (expected: queued, assigned, running, succeeded, failed, canceled)", status)
	}
}

// ValidateBuildgateJobStatus validates that a string is a valid BuildgateJobStatus value.
// Returns the typed status if valid, otherwise returns an error.
func ValidateBuildgateJobStatus(status string) (BuildgateJobStatus, error) {
	s := BuildgateJobStatus(status)
	switch s {
	case BuildgateJobStatusPending, BuildgateJobStatusClaimed, BuildgateJobStatusRunning,
		BuildgateJobStatusCompleted, BuildgateJobStatusFailed:
		return s, nil
	default:
		return "", fmt.Errorf("invalid buildgate job status: %q (expected: pending, claimed, running, completed, failed)", status)
	}
}

// ValidateRunRepoStatus validates that a string is a valid RunRepoStatus value.
// Returns the typed status if valid, otherwise returns an error.
// RunRepoStatus tracks per-repo execution state within a batched run.
func ValidateRunRepoStatus(status string) (RunRepoStatus, error) {
	s := RunRepoStatus(status)
	switch s {
	case RunRepoStatusPending, RunRepoStatusRunning, RunRepoStatusSucceeded,
		RunRepoStatusFailed, RunRepoStatusSkipped, RunRepoStatusCancelled:
		return s, nil
	default:
		return "", fmt.Errorf("invalid run repo status: %q (expected: pending, running, succeeded, failed, skipped, cancelled)", status)
	}
}

// ConvertToRunRepoStatus converts various run repo status string representations
// to the canonical store.RunRepoStatus type. This helper provides type-safe
// conversion from external API representations to the database-authoritative
// store.RunRepoStatus.
//
// Supported mappings:
//   - "pending" -> RunRepoStatusPending
//   - "running" -> RunRepoStatusRunning
//   - "succeeded" -> RunRepoStatusSucceeded
//   - "failed" -> RunRepoStatusFailed
//   - "skipped" -> RunRepoStatusSkipped
//   - "cancelled" -> RunRepoStatusCancelled
//   - "canceled" -> RunRepoStatusCancelled (US spelling compatibility)
func ConvertToRunRepoStatus(status string) (RunRepoStatus, error) {
	switch status {
	case "pending":
		return RunRepoStatusPending, nil
	case "running":
		return RunRepoStatusRunning, nil
	case "succeeded":
		return RunRepoStatusSucceeded, nil
	case "failed":
		return RunRepoStatusFailed, nil
	case "skipped":
		return RunRepoStatusSkipped, nil
	case "cancelled", "canceled":
		return RunRepoStatusCancelled, nil
	default:
		return "", fmt.Errorf("unknown run repo status: %q", status)
	}
}
