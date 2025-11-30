package api

import (
	"github.com/iw2rmb/ploy/internal/store"
)

// StageStatusFromStore converts store.JobStatus to mods API StageState.
// This provides a type-safe mapping from the database-authoritative status
// to the external API representation.
//
// Mapping:
//   - store.JobStatusCreated -> StageStatePending (created jobs are pending from API perspective)
//   - store.JobStatusScheduled -> StageStatePending (scheduled jobs are pending from API perspective)
//   - store.JobStatusRunning -> StageStateRunning
//   - store.JobStatusSucceeded -> StageStateSucceeded
//   - store.JobStatusFailed -> StageStateFailed
//   - store.JobStatusSkipped -> StageStateFailed (skipped jobs are represented as failed in mods API)
//   - store.JobStatusCanceled -> StageStateCancelled (UK spelling for mods API)
func StageStatusFromStore(status store.JobStatus) StageState {
	switch status {
	case store.JobStatusCreated, store.JobStatusScheduled:
		return StageStatePending
	case store.JobStatusRunning:
		return StageStateRunning
	case store.JobStatusSucceeded:
		return StageStateSucceeded
	case store.JobStatusFailed:
		return StageStateFailed
	case store.JobStatusSkipped:
		// Skipped jobs don't have a direct mods API equivalent;
		// map to failed for API consistency.
		return StageStateFailed
	case store.JobStatusCanceled:
		return StageStateCancelled
	default:
		// Default to pending for unknown states (defensive).
		return StageStatePending
	}
}

// TicketStatusFromStore converts store.RunStatus to mods API TicketState.
// This provides a type-safe mapping from the database-authoritative status
// to the external API representation.
//
// Mapping:
//   - store.RunStatusQueued -> TicketStatePending
//   - store.RunStatusAssigned -> TicketStatePending (assigned runs are still pending from API perspective)
//   - store.RunStatusRunning -> TicketStateRunning
//   - store.RunStatusSucceeded -> TicketStateSucceeded
//   - store.RunStatusFailed -> TicketStateFailed
//   - store.RunStatusCanceled -> TicketStateCancelled (UK spelling for mods API)
func TicketStatusFromStore(status store.RunStatus) TicketState {
	switch status {
	case store.RunStatusQueued, store.RunStatusAssigned:
		// Both queued and assigned map to pending in the mods API
		// since assignment is an internal scheduler state.
		return TicketStatePending
	case store.RunStatusRunning:
		return TicketStateRunning
	case store.RunStatusSucceeded:
		return TicketStateSucceeded
	case store.RunStatusFailed:
		return TicketStateFailed
	case store.RunStatusCanceled:
		return TicketStateCancelled
	default:
		// Default to pending for unknown states (defensive).
		return TicketStatePending
	}
}

// StageStatusToStore converts mods API StageState to store.JobStatus.
// This provides a type-safe mapping from the external API representation
// to the database-authoritative status type.
//
// Mapping:
//   - StageStatePending/StageStateQueued -> store.JobStatusCreated
//   - StageStateRunning -> store.JobStatusRunning
//   - StageStateSucceeded -> store.JobStatusSucceeded
//   - StageStateFailed -> store.JobStatusFailed
//   - StageStateCancelling/StageStateCancelled -> store.JobStatusCanceled
func StageStatusToStore(state StageState) store.JobStatus {
	switch state {
	case StageStatePending, StageStateQueued:
		return store.JobStatusCreated
	case StageStateRunning:
		return store.JobStatusRunning
	case StageStateSucceeded:
		return store.JobStatusSucceeded
	case StageStateFailed:
		return store.JobStatusFailed
	case StageStateCancelling, StageStateCancelled:
		return store.JobStatusCanceled
	default:
		// Default to created for unknown states (defensive).
		return store.JobStatusCreated
	}
}

// TicketStatusToStore converts mods API TicketState to store.RunStatus.
// This provides a type-safe mapping from the external API representation
// to the database-authoritative status type.
//
// Mapping:
//   - TicketStatePending -> store.RunStatusQueued
//   - TicketStateRunning -> store.RunStatusRunning
//   - TicketStateSucceeded -> store.RunStatusSucceeded
//   - TicketStateFailed -> store.RunStatusFailed
//   - TicketStateCancelling/TicketStateCancelled -> store.RunStatusCanceled
func TicketStatusToStore(state TicketState) store.RunStatus {
	switch state {
	case TicketStatePending:
		return store.RunStatusQueued
	case TicketStateRunning:
		return store.RunStatusRunning
	case TicketStateSucceeded:
		return store.RunStatusSucceeded
	case TicketStateFailed:
		return store.RunStatusFailed
	case TicketStateCancelling, TicketStateCancelled:
		return store.RunStatusCanceled
	default:
		// Default to queued for unknown states (defensive).
		return store.RunStatusQueued
	}
}
