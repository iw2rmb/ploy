package lifecycle

import (
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	migsapi "github.com/iw2rmb/ploy/internal/migs/api"
	"github.com/iw2rmb/ploy/internal/store"
)

// Derived batch status constants representing the batch-level state computed from repo statuses.
const (
	// DerivedStatusPending indicates no repos have started (all queued or no repos).
	DerivedStatusPending = "pending"
	// DerivedStatusRunning indicates at least one repo is currently running.
	DerivedStatusRunning = "running"
	// DerivedStatusCompleted indicates all repos finished with no failures.
	DerivedStatusCompleted = "completed"
	// DerivedStatusFailed indicates at least one repo failed (and none running).
	DerivedStatusFailed = "failed"
	// DerivedStatusCancelled indicates the batch was stopped and repos were cancelled.
	DerivedStatusCancelled = "cancelled"
)

// IsTerminalRunStatus reports whether the run status is terminal.
func IsTerminalRunStatus(status domaintypes.RunStatus) bool {
	switch status {
	case domaintypes.RunStatusSuccess, domaintypes.RunStatusFail, domaintypes.RunStatusCancelled:
		return true
	default:
		return false
	}
}

// IsTerminalWaveStatus reports whether the wave status is terminal.
func IsTerminalWaveStatus(status domaintypes.WaveStatus) bool {
	switch status {
	case domaintypes.WaveStatusFinished, domaintypes.WaveStatusCancelled:
		return true
	default:
		return false
	}
}

// DeriveBatchStatus computes a single batch-level status from repo counts.
// Precedence order: cancelled > running > failed > completed > pending.
func DeriveBatchStatus(counts *domaintypes.RunRepoCounts) string {
	if counts.Total == 0 {
		return DerivedStatusPending
	}
	if counts.Cancelled > 0 {
		return DerivedStatusCancelled
	}
	if counts.Running > 0 {
		return DerivedStatusRunning
	}
	if counts.Fail > 0 {
		return DerivedStatusFailed
	}
	terminalCount := counts.Success + counts.Fail + counts.Cancelled
	if terminalCount == counts.Total {
		return DerivedStatusCompleted
	}
	return DerivedStatusPending
}

// RunCompletionEval is the pure evaluation result for run completion.
type RunCompletionEval struct {
	ShouldFinish bool
	RunState     migsapi.RunState
}

// EvaluateRunCompletionFromRepoCounts determines whether a run can be marked Finished.
// Returns ShouldFinish=true when all repos are in terminal states, along with the
// derived run state (succeeded, failed, or cancelled).
func EvaluateWaveCompletionFromRunCounts(counts []store.CountRunsByWaveStatusRow) RunCompletionEval {
	var (
		total        int32
		terminal     int32
		anyFail      bool
		anyCancelled bool
	)
	for _, row := range counts {
		total += row.Count
		switch row.Status {
		case domaintypes.RunStatusSuccess, domaintypes.RunStatusFail, domaintypes.RunStatusCancelled:
			terminal += row.Count
		}
		if row.Status == domaintypes.RunStatusFail && row.Count > 0 {
			anyFail = true
		}
		if row.Status == domaintypes.RunStatusCancelled && row.Count > 0 {
			anyCancelled = true
		}
	}

	if total == 0 || terminal < total {
		return RunCompletionEval{ShouldFinish: false}
	}

	runState := migsapi.RunStateSucceeded
	if anyFail {
		runState = migsapi.RunStateFailed
	} else if anyCancelled {
		runState = migsapi.RunStateCancelled
	}

	return RunCompletionEval{
		ShouldFinish: true,
		RunState:     runState,
	}
}
