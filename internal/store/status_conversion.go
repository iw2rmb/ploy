package store

import (
	"fmt"
)

// ConvertToStageStatus converts various stage status string representations
// to the canonical store.StageStatus type. This helper provides type-safe
// conversion from external API representations (e.g., mods API StageState)
// to the database-authoritative store.StageStatus.
//
// Supported mappings:
//   - "pending" -> StageStatusPending
//   - "queued" -> StageStatusPending (mods API compatibility)
//   - "running" -> StageStatusRunning
//   - "succeeded" -> StageStatusSucceeded
//   - "failed" -> StageStatusFailed
//   - "skipped" -> StageStatusSkipped
//   - "canceled"/"cancelled" -> StageStatusCanceled (US/UK spelling)
//   - "cancelling" -> StageStatusCanceled (mods API in-progress cancellation maps to final state)
func ConvertToStageStatus(status string) (StageStatus, error) {
	switch status {
	case "pending", "queued":
		return StageStatusPending, nil
	case "running":
		return StageStatusRunning, nil
	case "succeeded":
		return StageStatusSucceeded, nil
	case "failed":
		return StageStatusFailed, nil
	case "skipped":
		return StageStatusSkipped, nil
	case "canceled", "cancelled", "cancelling":
		return StageStatusCanceled, nil
	default:
		return "", fmt.Errorf("unknown stage status: %q", status)
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

// ValidateStageStatus validates that a string is a valid StageStatus value.
// Returns the typed status if valid, otherwise returns an error.
func ValidateStageStatus(status string) (StageStatus, error) {
	s := StageStatus(status)
	switch s {
	case StageStatusPending, StageStatusRunning, StageStatusSucceeded,
		StageStatusFailed, StageStatusSkipped, StageStatusCanceled:
		return s, nil
	default:
		return "", fmt.Errorf("invalid stage status: %q (expected: pending, running, succeeded, failed, skipped, canceled)", status)
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
