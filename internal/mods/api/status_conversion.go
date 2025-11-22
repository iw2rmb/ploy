package api

import (
	"github.com/iw2rmb/ploy/internal/store"
)

// StageStatusFromStore converts store.StageStatus to mods API StageState.
// This provides a type-safe mapping from the database-authoritative status
// to the external API representation.
//
// Mapping:
//   - store.StageStatusPending -> StageStatePending
//   - store.StageStatusRunning -> StageStateRunning
//   - store.StageStatusSucceeded -> StageStateSucceeded
//   - store.StageStatusFailed -> StageStateFailed
//   - store.StageStatusSkipped -> StageStateFailed (skipped stages are represented as failed in mods API)
//   - store.StageStatusCanceled -> StageStateCancelled (UK spelling for mods API)
func StageStatusFromStore(status store.StageStatus) StageState {
	switch status {
	case store.StageStatusPending:
		return StageStatePending
	case store.StageStatusRunning:
		return StageStateRunning
	case store.StageStatusSucceeded:
		return StageStateSucceeded
	case store.StageStatusFailed:
		return StageStateFailed
	case store.StageStatusSkipped:
		// Skipped stages don't have a direct mods API equivalent;
		// map to failed for API consistency.
		return StageStateFailed
	case store.StageStatusCanceled:
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

// StageStatusToStore converts mods API StageState to store.StageStatus.
// This provides a type-safe mapping from the external API representation
// to the database-authoritative status type.
//
// Mapping:
//   - StageStatePending/StageStateQueued -> store.StageStatusPending
//   - StageStateRunning -> store.StageStatusRunning
//   - StageStateSucceeded -> store.StageStatusSucceeded
//   - StageStateFailed -> store.StageStatusFailed
//   - StageStateCancelling/StageStateCancelled -> store.StageStatusCanceled
func StageStatusToStore(state StageState) store.StageStatus {
	switch state {
	case StageStatePending, StageStateQueued:
		return store.StageStatusPending
	case StageStateRunning:
		return store.StageStatusRunning
	case StageStateSucceeded:
		return store.StageStatusSucceeded
	case StageStateFailed:
		return store.StageStatusFailed
	case StageStateCancelling, StageStateCancelled:
		return store.StageStatusCanceled
	default:
		// Default to pending for unknown states (defensive).
		return store.StageStatusPending
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
