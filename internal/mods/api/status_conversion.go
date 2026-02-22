package api

import (
	"github.com/iw2rmb/ploy/internal/store"
)

// StageStatusFromStore converts store.JobStatus to mods API StageState.
func StageStatusFromStore(status store.JobStatus) StageState {
	switch status {
	case store.JobStatusCreated, store.JobStatusQueued:
		return StageStatePending
	case store.JobStatusRunning:
		return StageStateRunning
	case store.JobStatusSuccess:
		return StageStateSucceeded
	case store.JobStatusFail:
		return StageStateFailed
	case store.JobStatusCancelled:
		return StageStateCancelled
	default:
		return StageStatePending
	}
}

// RunStatusFromStore converts store.RunStatus to mods API RunState.
func RunStatusFromStore(status store.RunStatus) RunState {
	switch status {
	case store.RunStatusStarted:
		return RunStateRunning
	case store.RunStatusFinished:
		return RunStateSucceeded
	case store.RunStatusCancelled:
		return RunStateCancelled
	default:
		return RunStateRunning
	}
}

// StageStatusToStore converts mods API StageState to store.JobStatus.
func StageStatusToStore(state StageState) store.JobStatus {
	switch state {
	case StageStatePending, StageStateQueued:
		return store.JobStatusCreated
	case StageStateRunning:
		return store.JobStatusRunning
	case StageStateSucceeded:
		return store.JobStatusSuccess
	case StageStateFailed:
		return store.JobStatusFail
	case StageStateCancelling, StageStateCancelled:
		return store.JobStatusCancelled
	default:
		return store.JobStatusCreated
	}
}

// RunStatusToStore converts mods API RunState to store.RunStatus.
func RunStatusToStore(state RunState) store.RunStatus {
	switch state {
	case RunStatePending, RunStateRunning:
		return store.RunStatusStarted
	case RunStateSucceeded, RunStateFailed:
		return store.RunStatusFinished
	case RunStateCancelling, RunStateCancelled:
		return store.RunStatusCancelled
	default:
		return store.RunStatusStarted
	}
}
