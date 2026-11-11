package lifecycle

import (
	"context"
	"errors"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// JobStatusFromRunError maps a job execution error to the appropriate terminal job status.
// context.Canceled and context.DeadlineExceeded produce Cancelled; all other
// errors produce Error. This is the canonical status-from-error mapping consumed
// across all nodeagent execution paths.
func JobStatusFromRunError(err error) domaintypes.JobStatus {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return domaintypes.JobStatusCancelled
	}
	return domaintypes.JobStatusError
}

// JobStatusFromExitCode maps a completed process exit code to terminal status.
// 0 => Success, 1 => Fail, any other value => Error.
func JobStatusFromExitCode(exitCode int) domaintypes.JobStatus {
	if exitCode == 0 {
		return domaintypes.JobStatusSuccess
	}
	if exitCode == 1 {
		return domaintypes.JobStatusFail
	}
	return domaintypes.JobStatusError
}
