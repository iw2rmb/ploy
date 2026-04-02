package api

import (
	"fmt"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// StageStatusFromDomain converts domain JobStatus to migs API StageState.
func StageStatusFromDomain(status domaintypes.JobStatus) (StageState, error) {
	switch status {
	case domaintypes.JobStatusCreated, domaintypes.JobStatusQueued:
		return StageStatePending, nil
	case domaintypes.JobStatusRunning:
		return StageStateRunning, nil
	case domaintypes.JobStatusSuccess:
		return StageStateSucceeded, nil
	case domaintypes.JobStatusFail, domaintypes.JobStatusError:
		return StageStateFailed, nil
	case domaintypes.JobStatusCancelled:
		return StageStateCancelled, nil
	default:
		return "", fmt.Errorf("unknown domain job status %q", status)
	}
}

// RunStatusFromDomain converts domain RunStatus to migs API RunState.
func RunStatusFromDomain(status domaintypes.RunStatus) (RunState, error) {
	switch status {
	case domaintypes.RunStatusStarted:
		return RunStateRunning, nil
	case domaintypes.RunStatusFinished:
		return RunStateSucceeded, nil
	case domaintypes.RunStatusCancelled:
		return RunStateCancelled, nil
	default:
		return "", fmt.Errorf("unknown domain run status %q", status)
	}
}
