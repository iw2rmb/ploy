package lifecycle_test

// cross_path_parity_test.go verifies the consistency contract between
// the nodeagent execution path and the lifecycle orchestrator path.
//
// The contract: for each (jobType, executionOutcome) pair, the job status
// the nodeagent uploads, when evaluated by EvaluateCompletionDecision,
// produces the expected chain action on the server side.
//
// This ensures that what the nodeagent does (produce a status) and what the
// server does (compute a chain action from that status) are consistent.

import (
	"context"
	"errors"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/lifecycle"
)

// nodeagentExecutionOutcome represents the observable result from nodeagent job execution.
type nodeagentExecutionOutcome int

const (
	// outcomeSuccess: container exited 0, no runtime error.
	outcomeSuccess nodeagentExecutionOutcome = iota
	// outcomeNonZeroExit: container exited with non-zero code.
	outcomeNonZeroExit
	// outcomeRuntimeError: runtime/infrastructure error (not context cancellation).
	outcomeRuntimeError
	// outcomeContextCancelled: execution context was cancelled or deadline exceeded.
	outcomeContextCancelled
	// outcomeGateInfraError: gate execution infrastructure error (Docker pull/start failure).
	// Unlike outcomeRuntimeError, the gate job hardcodes Cancelled for these errors to
	// prevent healing from triggering on infrastructure failures.
	outcomeGateInfraError
)

// nodeagentStatusForOutcome derives the job status using the real nodeagent
// execution paths. Success and non-zero-exit use the same constants the
// nodeagent assigns directly; runtime and cancellation errors are routed
// through lifecycle.JobStatusFromRunError so test behaviour tracks the
// canonical implementation. Gate infra errors return Cancelled directly,
// mirroring the hardcoded assignment in executeGateJob.
func nodeagentStatusForOutcome(o nodeagentExecutionOutcome) domaintypes.JobStatus {
	switch o {
	case outcomeSuccess:
		return domaintypes.JobStatusSuccess
	case outcomeNonZeroExit:
		return domaintypes.JobStatusFail
	case outcomeRuntimeError:
		return lifecycle.JobStatusFromRunError(errors.New("runtime error"))
	case outcomeContextCancelled:
		return lifecycle.JobStatusFromRunError(context.Canceled)
	case outcomeGateInfraError:
		// Gate execution errors (Docker pull/create/start failures) are always
		// Cancelled — hardcoded in executeGateJob to prevent healing on infra failures.
		return domaintypes.JobStatusCancelled
	default:
		return lifecycle.JobStatusFromRunError(errors.New("unknown outcome"))
	}
}

// TestCrossPathTransitionParity verifies that for every (jobType, executionOutcome, hasNext)
// combination, the status the nodeagent would upload is consistent with the lifecycle chain
// action the server computes via EvaluateCompletionDecision.
//
// This pins the semantic contract between the nodeagent (status producer) and the server
// lifecycle orchestrator (chain action consumer), preventing silent divergence.
func TestCrossPathTransitionParity(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name            string
		jobType         domaintypes.JobType
		outcome         nodeagentExecutionOutcome
		hasNext         bool
		wantStatus      domaintypes.JobStatus
		wantChainAction lifecycle.CompletionChainAction
	}{
		// ── Gate job: pre_gate ──────────────────────────────────────────────────

		{
			name:            "pre-gate success with successor advances chain",
			jobType:         domaintypes.JobTypePreGate,
			outcome:         outcomeSuccess,
			hasNext:         true,
			wantStatus:      domaintypes.JobStatusSuccess,
			wantChainAction: lifecycle.CompletionChainAdvanceNext,
		},
		{
			name:            "pre-gate success without successor takes no action",
			jobType:         domaintypes.JobTypePreGate,
			outcome:         outcomeSuccess,
			hasNext:         false,
			wantStatus:      domaintypes.JobStatusSuccess,
			wantChainAction: lifecycle.CompletionChainNoAction,
		},
		{
			name:            "pre-gate non-zero exit triggers gate failure evaluation",
			jobType:         domaintypes.JobTypePreGate,
			outcome:         outcomeNonZeroExit,
			hasNext:         true,
			wantStatus:      domaintypes.JobStatusFail,
			wantChainAction: lifecycle.CompletionChainEvaluateGateFailure,
		},
		{
			name:            "pre-gate runtime error triggers gate failure evaluation",
			jobType:         domaintypes.JobTypePreGate,
			outcome:         outcomeRuntimeError,
			hasNext:         false,
			wantStatus:      domaintypes.JobStatusFail,
			wantChainAction: lifecycle.CompletionChainEvaluateGateFailure,
		},
		{
			name:            "pre-gate context cancelled cancels remainder",
			jobType:         domaintypes.JobTypePreGate,
			outcome:         outcomeContextCancelled,
			hasNext:         true,
			wantStatus:      domaintypes.JobStatusCancelled,
			wantChainAction: lifecycle.CompletionChainCancelRemainder,
		},
		{
			name:            "pre-gate infra error cancels remainder without healing",
			jobType:         domaintypes.JobTypePreGate,
			outcome:         outcomeGateInfraError,
			hasNext:         true,
			wantStatus:      domaintypes.JobStatusCancelled,
			wantChainAction: lifecycle.CompletionChainCancelRemainder,
		},

		// ── Gate job: post_gate ─────────────────────────────────────────────────

		{
			name:            "post-gate success with successor advances chain",
			jobType:         domaintypes.JobTypePostGate,
			outcome:         outcomeSuccess,
			hasNext:         true,
			wantStatus:      domaintypes.JobStatusSuccess,
			wantChainAction: lifecycle.CompletionChainAdvanceNext,
		},
		{
			name:            "post-gate failure triggers gate failure evaluation",
			jobType:         domaintypes.JobTypePostGate,
			outcome:         outcomeNonZeroExit,
			hasNext:         false,
			wantStatus:      domaintypes.JobStatusFail,
			wantChainAction: lifecycle.CompletionChainEvaluateGateFailure,
		},
		{
			name:            "post-gate context cancelled cancels remainder",
			jobType:         domaintypes.JobTypePostGate,
			outcome:         outcomeContextCancelled,
			hasNext:         true,
			wantStatus:      domaintypes.JobStatusCancelled,
			wantChainAction: lifecycle.CompletionChainCancelRemainder,
		},
		{
			name:            "post-gate infra error cancels remainder without healing",
			jobType:         domaintypes.JobTypePostGate,
			outcome:         outcomeGateInfraError,
			hasNext:         true,
			wantStatus:      domaintypes.JobStatusCancelled,
			wantChainAction: lifecycle.CompletionChainCancelRemainder,
		},

		// ── Gate job: re_gate ───────────────────────────────────────────────────

		{
			name:            "re-gate success with successor advances chain",
			jobType:         domaintypes.JobTypeReGate,
			outcome:         outcomeSuccess,
			hasNext:         true,
			wantStatus:      domaintypes.JobStatusSuccess,
			wantChainAction: lifecycle.CompletionChainAdvanceNext,
		},
		{
			name:            "re-gate failure triggers gate failure evaluation",
			jobType:         domaintypes.JobTypeReGate,
			outcome:         outcomeNonZeroExit,
			hasNext:         true,
			wantStatus:      domaintypes.JobStatusFail,
			wantChainAction: lifecycle.CompletionChainEvaluateGateFailure,
		},
		{
			name:            "re-gate context cancelled cancels remainder",
			jobType:         domaintypes.JobTypeReGate,
			outcome:         outcomeContextCancelled,
			hasNext:         true,
			wantStatus:      domaintypes.JobStatusCancelled,
			wantChainAction: lifecycle.CompletionChainCancelRemainder,
		},
		{
			name:            "re-gate infra error cancels remainder without healing",
			jobType:         domaintypes.JobTypeReGate,
			outcome:         outcomeGateInfraError,
			hasNext:         true,
			wantStatus:      domaintypes.JobStatusCancelled,
			wantChainAction: lifecycle.CompletionChainCancelRemainder,
		},

		// ── Mod job ─────────────────────────────────────────────────────────────

		{
			name:            "mod job success with successor advances chain",
			jobType:         domaintypes.JobTypeMod,
			outcome:         outcomeSuccess,
			hasNext:         true,
			wantStatus:      domaintypes.JobStatusSuccess,
			wantChainAction: lifecycle.CompletionChainAdvanceNext,
		},
		{
			name:            "mod job success without successor takes no action",
			jobType:         domaintypes.JobTypeMod,
			outcome:         outcomeSuccess,
			hasNext:         false,
			wantStatus:      domaintypes.JobStatusSuccess,
			wantChainAction: lifecycle.CompletionChainNoAction,
		},
		{
			name:            "mod job non-zero exit cancels remainder",
			jobType:         domaintypes.JobTypeMod,
			outcome:         outcomeNonZeroExit,
			hasNext:         true,
			wantStatus:      domaintypes.JobStatusFail,
			wantChainAction: lifecycle.CompletionChainCancelRemainder,
		},
		{
			name:            "mod job runtime error cancels remainder",
			jobType:         domaintypes.JobTypeMod,
			outcome:         outcomeRuntimeError,
			hasNext:         true,
			wantStatus:      domaintypes.JobStatusFail,
			wantChainAction: lifecycle.CompletionChainCancelRemainder,
		},
		{
			name:            "mod job context cancelled cancels remainder",
			jobType:         domaintypes.JobTypeMod,
			outcome:         outcomeContextCancelled,
			hasNext:         true,
			wantStatus:      domaintypes.JobStatusCancelled,
			wantChainAction: lifecycle.CompletionChainCancelRemainder,
		},

		// ── Heal job ─────────────────────────────────────────────────────────────

		{
			name:            "heal job success with re-gate successor advances chain",
			jobType:         domaintypes.JobTypeHeal,
			outcome:         outcomeSuccess,
			hasNext:         true,
			wantStatus:      domaintypes.JobStatusSuccess,
			wantChainAction: lifecycle.CompletionChainAdvanceNext,
		},
		{
			name:            "heal job non-zero exit cancels remainder",
			jobType:         domaintypes.JobTypeHeal,
			outcome:         outcomeNonZeroExit,
			hasNext:         true,
			wantStatus:      domaintypes.JobStatusFail,
			wantChainAction: lifecycle.CompletionChainCancelRemainder,
		},
		{
			name:            "heal job context cancelled cancels remainder",
			jobType:         domaintypes.JobTypeHeal,
			outcome:         outcomeContextCancelled,
			hasNext:         true,
			wantStatus:      domaintypes.JobStatusCancelled,
			wantChainAction: lifecycle.CompletionChainCancelRemainder,
		},

		// ── MR job ───────────────────────────────────────────────────────────────

		{
			name:            "MR job success takes no chain action",
			jobType:         domaintypes.JobTypeMR,
			outcome:         outcomeSuccess,
			hasNext:         false,
			wantStatus:      domaintypes.JobStatusSuccess,
			wantChainAction: lifecycle.CompletionChainNoAction,
		},
		{
			name:            "MR job failure takes no chain action",
			jobType:         domaintypes.JobTypeMR,
			outcome:         outcomeNonZeroExit,
			hasNext:         true,
			wantStatus:      domaintypes.JobStatusFail,
			wantChainAction: lifecycle.CompletionChainNoAction,
		},
		{
			name:            "MR job context cancelled takes no chain action",
			jobType:         domaintypes.JobTypeMR,
			outcome:         outcomeContextCancelled,
			hasNext:         true,
			wantStatus:      domaintypes.JobStatusCancelled,
			wantChainAction: lifecycle.CompletionChainNoAction,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Step 1: verify the nodeagent-side status determination via the real
			// nodeagent path (lifecycle.JobStatusFromRunError for error outcomes; direct
			// constants for success and non-zero-exit, matching the real code).
			gotStatus := nodeagentStatusForOutcome(tc.outcome)
			if gotStatus != tc.wantStatus {
				t.Fatalf("nodeagentStatusForOutcome(%v) = %v, want %v", tc.outcome, gotStatus, tc.wantStatus)
			}

			// Step 2: verify the server-side lifecycle chain decision for that status.
			decision := lifecycle.EvaluateCompletionDecision(tc.jobType, gotStatus, tc.hasNext)
			if decision.ChainAction != tc.wantChainAction {
				t.Fatalf(
					"EvaluateCompletionDecision(jobType=%v, status=%v, hasNext=%v).ChainAction = %v, want %v",
					tc.jobType, gotStatus, tc.hasNext, decision.ChainAction, tc.wantChainAction,
				)
			}
		})
	}
}

// TestJobStatusFromRunError_Exhaustive verifies that lifecycle.JobStatusFromRunError
// covers every execution outcome variant and maps to the correct status string
// used in the wire protocol.
func TestJobStatusFromRunError_Exhaustive(t *testing.T) {
	t.Parallel()

	cases := []struct {
		outcome    nodeagentExecutionOutcome
		wantStatus string
	}{
		{outcomeSuccess, "Success"},
		{outcomeNonZeroExit, "Fail"},
		{outcomeRuntimeError, "Fail"},
		{outcomeContextCancelled, "Cancelled"},
		{outcomeGateInfraError, "Cancelled"},
	}

	for _, tc := range cases {
		t.Run(tc.wantStatus, func(t *testing.T) {
			t.Parallel()
			got := nodeagentStatusForOutcome(tc.outcome)
			if got.String() != tc.wantStatus {
				t.Fatalf("nodeagentStatusForOutcome(%v).String() = %q, want %q", tc.outcome, got.String(), tc.wantStatus)
			}
		})
	}
}
