package lifecycle_test

import (
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/lifecycle"
)

func TestJobStatusFromExitCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		exitCode int
		want     domaintypes.JobStatus
	}{
		{name: "zero is success", exitCode: 0, want: domaintypes.JobStatusSuccess},
		{name: "one is fail", exitCode: 1, want: domaintypes.JobStatusFail},
		{name: "exit above one is error", exitCode: 2, want: domaintypes.JobStatusError},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := lifecycle.JobStatusFromExitCode(tt.exitCode); got != tt.want {
				t.Fatalf("JobStatusFromExitCode(%d) = %q, want %q", tt.exitCode, got, tt.want)
			}
		})
	}
}

func TestEvaluateCompletionDecision(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		status     domaintypes.JobStatus
		hasNext    bool
		wantAction lifecycle.CompletionChainAction
	}{
		{name: "success advances to next", status: domaintypes.JobStatusSuccess, hasNext: true, wantAction: lifecycle.CompletionChainAdvanceNext},
		{name: "success terminal no action", status: domaintypes.JobStatusSuccess, hasNext: false, wantAction: lifecycle.CompletionChainNoAction},
		{name: "failed gate cancels remainder", status: domaintypes.JobStatusFail, hasNext: true, wantAction: lifecycle.CompletionChainCancelRemainder},
		{name: "errored job cancels remainder", status: domaintypes.JobStatusError, hasNext: true, wantAction: lifecycle.CompletionChainCancelRemainder},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := lifecycle.EvaluateCompletionDecision(tt.status, tt.hasNext)
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
