package handlers

import (
	modsapi "github.com/iw2rmb/ploy/internal/mods/api"
	"github.com/iw2rmb/ploy/internal/store"
)

// StageStatusFromStore converts store.JobStatus to mods API StageState.
// This provides a type-safe mapping from the database-authoritative status
// to the external API representation.
//
// Status mapping:
//   - store.JobStatusCreated -> StageStatePending (created jobs are pending from API perspective)
//   - store.JobStatusQueued -> StageStatePending (queued jobs are pending from API perspective)
//   - store.JobStatusRunning -> StageStateRunning
//   - store.JobStatusSuccess -> StageStateSucceeded
//   - store.JobStatusFail -> StageStateFailed
//   - store.JobStatusCancelled -> StageStateCancelled
//
// Note: "skipped" status is not used; jobs are never skipped, only cancelled or failed.
func StageStatusFromStore(status store.JobStatus) modsapi.StageState {
	switch status {
	case store.JobStatusCreated, store.JobStatusQueued:
		return modsapi.StageStatePending
	case store.JobStatusRunning:
		return modsapi.StageStateRunning
	case store.JobStatusSuccess:
		return modsapi.StageStateSucceeded
	case store.JobStatusFail:
		return modsapi.StageStateFailed
	case store.JobStatusCancelled:
		return modsapi.StageStateCancelled
	default:
		// Default to pending for unknown states (defensive).
		return modsapi.StageStatePending
	}
}

// RunStatusFromStore converts store.RunStatus to mods API RunState.
// This provides a type-safe mapping from the database-authoritative status
// to the external API representation.
//
// Status mapping:
//   - store.RunStatusStarted -> RunStateRunning (started runs are running from API perspective)
//   - store.RunStatusCancelled -> RunStateCancelled
//   - store.RunStatusFinished -> RunStateSucceeded (finished runs are treated as succeeded by default)
//
// Run statuses are simplified to Started, Finished, and Cancelled.
func RunStatusFromStore(status store.RunStatus) modsapi.RunState {
	switch status {
	case store.RunStatusStarted:
		// v1 Started means the run is active; map to Running for API.
		return modsapi.RunStateRunning
	case store.RunStatusFinished:
		// v1 Finished is terminal; map to Succeeded for API.
		// NOTE: Future work may inspect repo-level outcomes to distinguish success/failure.
		return modsapi.RunStateSucceeded
	case store.RunStatusCancelled:
		return modsapi.RunStateCancelled
	default:
		// Default to running for unknown states (defensive).
		return modsapi.RunStateRunning
	}
}

// StageStatusToStore converts mods API StageState to store.JobStatus.
// This provides a type-safe mapping from the external API representation
// to the database-authoritative status type.
//
// Status mapping:
//   - StageStatePending/StageStateQueued -> store.JobStatusCreated
//   - StageStateRunning -> store.JobStatusRunning
//   - StageStateSucceeded -> store.JobStatusSuccess
//   - StageStateFailed -> store.JobStatusFail
//   - StageStateCancelling/StageStateCancelled -> store.JobStatusCancelled
func StageStatusToStore(state modsapi.StageState) store.JobStatus {
	switch state {
	case modsapi.StageStatePending, modsapi.StageStateQueued:
		return store.JobStatusCreated
	case modsapi.StageStateRunning:
		return store.JobStatusRunning
	case modsapi.StageStateSucceeded:
		return store.JobStatusSuccess
	case modsapi.StageStateFailed:
		return store.JobStatusFail
	case modsapi.StageStateCancelling, modsapi.StageStateCancelled:
		return store.JobStatusCancelled
	default:
		// Default to created for unknown states (defensive).
		return store.JobStatusCreated
	}
}

// RunStatusToStore converts mods API RunState to store.RunStatus.
// This provides a type-safe mapping from the external API representation
// to the database-authoritative status type.
//
// Status mapping:
//   - RunStatePending -> store.RunStatusStarted (no separate pending state in store)
//   - RunStateRunning -> store.RunStatusStarted
//   - RunStateSucceeded -> store.RunStatusFinished
//   - RunStateFailed -> store.RunStatusFinished
//   - RunStateCancelling/RunStateCancelled -> store.RunStatusCancelled
func RunStatusToStore(state modsapi.RunState) store.RunStatus {
	switch state {
	case modsapi.RunStatePending, modsapi.RunStateRunning:
		// v1 has no pending/running distinction; both map to Started.
		return store.RunStatusStarted
	case modsapi.RunStateSucceeded, modsapi.RunStateFailed:
		// v1 Finished is the terminal state for both success and failure.
		return store.RunStatusFinished
	case modsapi.RunStateCancelling, modsapi.RunStateCancelled:
		return store.RunStatusCancelled
	default:
		// Default to started for unknown states (defensive).
		return store.RunStatusStarted
	}
}
