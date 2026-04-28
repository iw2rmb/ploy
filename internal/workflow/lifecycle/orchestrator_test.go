package lifecycle_test

import (
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/lifecycle"
)

func TestJobStatusFromExitCodeForJobType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		jobType  domaintypes.JobType
		exitCode int
		want     domaintypes.JobStatus
	}{
		{name: "pre_gate non-zero is fail", jobType: domaintypes.JobTypePreGate, exitCode: 1, want: domaintypes.JobStatusFail},
		{name: "mig non-zero is fail", jobType: domaintypes.JobTypeMig, exitCode: 1, want: domaintypes.JobStatusFail},
		{name: "gate non-zero is fail", jobType: domaintypes.JobTypePostGate, exitCode: 1, want: domaintypes.JobStatusFail},
		{name: "exit above one is error", jobType: domaintypes.JobTypeMig, exitCode: 2, want: domaintypes.JobStatusError},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := lifecycle.JobStatusFromExitCodeForJobType(tt.jobType, tt.exitCode); got != tt.want {
				t.Fatalf("JobStatusFromExitCodeForJobType(%q, %d) = %q, want %q", tt.jobType, tt.exitCode, got, tt.want)
			}
		})
	}
}

func TestEvaluateCompletionDecision(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		jobType    domaintypes.JobType
		status     domaintypes.JobStatus
		hasNext    bool
		wantAction lifecycle.CompletionChainAction
	}{
		{name: "success advances to next", jobType: domaintypes.JobTypeMig, status: domaintypes.JobStatusSuccess, hasNext: true, wantAction: lifecycle.CompletionChainAdvanceNext},
		{name: "success terminal no action", jobType: domaintypes.JobTypeMig, status: domaintypes.JobStatusSuccess, hasNext: false, wantAction: lifecycle.CompletionChainNoAction},
		{name: "failed gate cancels remainder", jobType: domaintypes.JobTypePostGate, status: domaintypes.JobStatusFail, hasNext: true, wantAction: lifecycle.CompletionChainCancelRemainder},
		{name: "errored job cancels remainder", jobType: domaintypes.JobTypeMig, status: domaintypes.JobStatusError, hasNext: true, wantAction: lifecycle.CompletionChainCancelRemainder},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := lifecycle.EvaluateCompletionDecision(tt.jobType, tt.status, tt.hasNext)
			if got.ChainAction != tt.wantAction {
				t.Fatalf("ChainAction = %v, want %v", got.ChainAction, tt.wantAction)
			}
		})
	}
}

func TestIsGateJobType(t *testing.T) {
	t.Parallel()

	if !lifecycle.IsGateJobType(domaintypes.JobTypePreGate) {
		t.Fatal("pre_gate must be treated as gate job type")
	}
	if !lifecycle.IsGateJobType(domaintypes.JobTypePostGate) {
		t.Fatal("post_gate must be treated as gate job type")
	}
	if lifecycle.IsGateJobType(domaintypes.JobTypeMig) {
		t.Fatal("mig must not be treated as gate job type")
	}
}
