package lifecycle_test

import (
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/lifecycle"
)

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
