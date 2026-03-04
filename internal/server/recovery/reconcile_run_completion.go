package recovery

import (
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	modsapi "github.com/iw2rmb/ploy/internal/migs/api"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/jackc/pgx/v5/pgtype"
)

// RunCompletionEval is the pure evaluation result for run completion.
type RunCompletionEval struct {
	ShouldFinish bool
	RunState     modsapi.RunState
}

// EvaluateRunCompletionFromRepoCounts determines whether a run can be marked Finished.
func EvaluateRunCompletionFromRepoCounts(counts []store.CountRunReposByStatusRow) RunCompletionEval {
	var (
		total        int32
		terminal     int32
		anyFail      bool
		anyCancelled bool
	)
	for _, row := range counts {
		total += row.Count
		switch row.Status {
		case domaintypes.RunRepoStatusSuccess, domaintypes.RunRepoStatusFail, domaintypes.RunRepoStatusCancelled:
			terminal += row.Count
		}
		if row.Status == domaintypes.RunRepoStatusFail && row.Count > 0 {
			anyFail = true
		}
		if row.Status == domaintypes.RunRepoStatusCancelled && row.Count > 0 {
			anyCancelled = true
		}
	}

	if total == 0 || terminal < total {
		return RunCompletionEval{ShouldFinish: false}
	}

	runState := modsapi.RunStateSucceeded
	if anyFail {
		runState = modsapi.RunStateFailed
	} else if anyCancelled {
		runState = modsapi.RunStateCancelled
	}

	return RunCompletionEval{
		ShouldFinish: true,
		RunState:     runState,
	}
}

func timeOrZero(ts pgtype.Timestamptz) time.Time {
	if ts.Valid {
		return ts.Time
	}
	return time.Time{}
}
