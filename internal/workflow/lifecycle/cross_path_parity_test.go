package lifecycle_test

import (
	"context"
	"errors"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/lifecycle"
)

// TestCrossPathParity_StandardJobErrorToChainAction exercises the joint nodeagent→server
// completion path for standard (mig/heal/MR) job error scenarios.
//
// Nodeagent path: lifecycle.JobStatusFromRunError maps execution errors to job statuses.
// Server path: lifecycle.EvaluateCompletionDecision maps (jobType, status, hasNext) to chain actions.
//
// Both paths consume the same canonical lifecycle helpers; this suite locks their combined
// semantics to prevent divergence between the nodeagent and server completion paths.
func TestCrossPathParity_StandardJobErrorToChainAction(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		err        error
		jobType    domaintypes.JobType
		hasNext    bool
		wantStatus domaintypes.JobStatus          // nodeagent emits this status
		wantAction lifecycle.CompletionChainAction // server takes this chain action
	}{
		// Context cancellation: mig → Cancelled → CancelRemainder
		{
			name:       "ctx_canceled/mod/has-next",
			err:        context.Canceled,
			jobType:    domaintypes.JobTypeMod,
			hasNext:    true,
			wantStatus: domaintypes.JobStatusCancelled,
			wantAction: lifecycle.CompletionChainCancelRemainder,
		},
		// Context deadline: heal → Cancelled → CancelRemainder
		{
			name:       "ctx_deadline/heal/has-next",
			err:        context.DeadlineExceeded,
			jobType:    domaintypes.JobTypeHeal,
			hasNext:    true,
			wantStatus: domaintypes.JobStatusCancelled,
			wantAction: lifecycle.CompletionChainCancelRemainder,
		},
		// Context cancellation: MR → Cancelled → NoAction (MR failures do not cascade)
		{
			name:       "ctx_canceled/mr/no-next",
			err:        context.Canceled,
			jobType:    domaintypes.JobTypeMR,
			hasNext:    false,
			wantStatus: domaintypes.JobStatusCancelled,
			wantAction: lifecycle.CompletionChainNoAction,
		},
		// Runtime error: mod → Fail → CancelRemainder
		{
			name:       "runtime_error/mod/has-next",
			err:        errors.New("container exited unexpectedly"),
			jobType:    domaintypes.JobTypeMod,
			hasNext:    true,
			wantStatus: domaintypes.JobStatusFail,
			wantAction: lifecycle.CompletionChainCancelRemainder,
		},
		// Runtime error: heal → Fail → CancelRemainder
		{
			name:       "runtime_error/heal/has-next",
			err:        errors.New("image pull failed"),
			jobType:    domaintypes.JobTypeHeal,
			hasNext:    true,
			wantStatus: domaintypes.JobStatusFail,
			wantAction: lifecycle.CompletionChainCancelRemainder,
		},
		// Runtime error: MR → Fail → NoAction (MR failures do not cascade)
		{
			name:       "runtime_error/mr/no-next",
			err:        errors.New("git push: authentication failed"),
			jobType:    domaintypes.JobTypeMR,
			hasNext:    false,
			wantStatus: domaintypes.JobStatusFail,
			wantAction: lifecycle.CompletionChainNoAction,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Nodeagent path: error → job status via canonical lifecycle mapping.
			gotStatus := lifecycle.JobStatusFromRunError(tc.err)
			if gotStatus != tc.wantStatus {
				t.Fatalf("JobStatusFromRunError(%v) = %v, want %v", tc.err, gotStatus, tc.wantStatus)
			}

			// Server path: job status → chain action via canonical lifecycle mapping.
			gotDecision := lifecycle.EvaluateCompletionDecision(tc.jobType, gotStatus, tc.hasNext)
			if gotDecision.ChainAction != tc.wantAction {
				t.Fatalf("EvaluateCompletionDecision(%v, %v, %v).ChainAction = %v, want %v",
					tc.jobType, gotStatus, tc.hasNext, gotDecision.ChainAction, tc.wantAction)
			}
		})
	}
}

// TestCrossPathParity_GateJobStatusToChainAction exercises the gate-specific nodeagent→server
// completion path. Gate jobs use explicit status assignment (not lifecycle.JobStatusFromRunError):
//   - infra errors always produce Cancelled (preventing healing activation)
//   - test failures produce Fail (enabling healing evaluation via EvaluateGateFailure)
//   - test successes produce Success (advancing the job chain)
//
// This suite locks that EvaluateCompletionDecision handles all three gate statuses correctly,
// so the server correctly reacts to whatever the gate nodeagent path emits.
func TestCrossPathParity_GateJobStatusToChainAction(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		jobType    domaintypes.JobType
		status     domaintypes.JobStatus
		hasNext    bool
		wantAction lifecycle.CompletionChainAction
	}{
		// Gate infra error: nodeagent always emits Cancelled → server produces CancelRemainder (no healing)
		{
			name:       "pre_gate/infra_cancelled/has-next",
			jobType:    domaintypes.JobTypePreGate,
			status:     domaintypes.JobStatusCancelled,
			hasNext:    true,
			wantAction: lifecycle.CompletionChainCancelRemainder,
		},
		{
			name:       "post_gate/infra_cancelled/has-next",
			jobType:    domaintypes.JobTypePostGate,
			status:     domaintypes.JobStatusCancelled,
			hasNext:    true,
			wantAction: lifecycle.CompletionChainCancelRemainder,
		},
		// Gate test failure: nodeagent emits Fail → server produces EvaluateGateFailure (healing eligible)
		{
			name:       "pre_gate/test_fail/has-next",
			jobType:    domaintypes.JobTypePreGate,
			status:     domaintypes.JobStatusFail,
			hasNext:    true,
			wantAction: lifecycle.CompletionChainEvaluateGateFailure,
		},
		{
			name:       "re_gate/test_fail/has-next",
			jobType:    domaintypes.JobTypeReGate,
			status:     domaintypes.JobStatusFail,
			hasNext:    true,
			wantAction: lifecycle.CompletionChainEvaluateGateFailure,
		},
		// Gate success with successor: nodeagent emits Success → server advances chain
		{
			name:       "pre_gate/success/has-next",
			jobType:    domaintypes.JobTypePreGate,
			status:     domaintypes.JobStatusSuccess,
			hasNext:    true,
			wantAction: lifecycle.CompletionChainAdvanceNext,
		},
		// Gate success without successor: NoAction
		{
			name:       "post_gate/success/no-next",
			jobType:    domaintypes.JobTypePostGate,
			status:     domaintypes.JobStatusSuccess,
			hasNext:    false,
			wantAction: lifecycle.CompletionChainNoAction,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Server path: the status emitted by the nodeagent gate path → chain action.
			gotDecision := lifecycle.EvaluateCompletionDecision(tc.jobType, tc.status, tc.hasNext)
			if gotDecision.ChainAction != tc.wantAction {
				t.Fatalf("EvaluateCompletionDecision(%v, %v, %v).ChainAction = %v, want %v",
					tc.jobType, tc.status, tc.hasNext, gotDecision.ChainAction, tc.wantAction)
			}
		})
	}
}
