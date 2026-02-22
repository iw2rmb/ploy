package handlers

import (
	modsapi "github.com/iw2rmb/ploy/internal/mods/api"
	"github.com/iw2rmb/ploy/internal/store"
)

// StageStatusFromStore converts store.JobStatus to mods API StageState.
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
		return modsapi.StageStatePending
	}
}

// RunStatusFromStore converts store.RunStatus to mods API RunState.
func RunStatusFromStore(status store.RunStatus) modsapi.RunState {
	switch status {
	case store.RunStatusStarted:
		return modsapi.RunStateRunning
	case store.RunStatusFinished:
		return modsapi.RunStateSucceeded
	case store.RunStatusCancelled:
		return modsapi.RunStateCancelled
	default:
		return modsapi.RunStateRunning
	}
}

// StageStatusToStore converts mods API StageState to store.JobStatus.
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
		return store.JobStatusCreated
	}
}

// RunStatusToStore converts mods API RunState to store.RunStatus.
func RunStatusToStore(state modsapi.RunState) store.RunStatus {
	switch state {
	case modsapi.RunStatePending, modsapi.RunStateRunning:
		return store.RunStatusStarted
	case modsapi.RunStateSucceeded, modsapi.RunStateFailed:
		return store.RunStatusFinished
	case modsapi.RunStateCancelling, modsapi.RunStateCancelled:
		return store.RunStatusCancelled
	default:
		return store.RunStatusStarted
	}
}
