package handlers

import (
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestResolveFailedGateRecoveryContext_ReclassifiesToolchainMajorVersionToDeps(t *testing.T) {
	t.Parallel()

	meta, err := contracts.MarshalJobMeta(&contracts.JobMeta{
		Kind: contracts.JobKindGate,
		Gate: &contracts.BuildGateStageMetadata{
			Recovery: &contracts.BuildGateRecoveryMetadata{
				LoopKind:   contracts.RecoveryLoopKindHealing.String(),
				ErrorKind:  contracts.RecoveryErrorKindInfra.String(),
				StrategyID: "infra-default",
				Reason:     "Gradle/Groovy init script failed: Unsupported class file major version 61",
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal meta: %v", err)
	}

	recovery, _, _ := resolveFailedGateRecoveryContext(store.Job{
		ID:      domaintypes.NewJobID(),
		RunID:   domaintypes.NewRunID(),
		JobType: domaintypes.JobTypePostGate.String(),
		Meta:    meta,
	})
	if recovery == nil {
		t.Fatal("expected recovery metadata")
	}
	if got, want := recovery.ErrorKind, contracts.RecoveryErrorKindDeps.String(); got != want {
		t.Fatalf("error_kind = %q, want %q", got, want)
	}
	if got, want := recovery.StrategyID, "deps-default"; got != want {
		t.Fatalf("strategy_id = %q, want %q", got, want)
	}
}

func TestResolveFailedGateRecoveryContext_LeavesInfraWhenReasonDoesNotMatch(t *testing.T) {
	t.Parallel()

	meta, err := contracts.MarshalJobMeta(&contracts.JobMeta{
		Kind: contracts.JobKindGate,
		Gate: &contracts.BuildGateStageMetadata{
			Recovery: &contracts.BuildGateRecoveryMetadata{
				LoopKind:   contracts.RecoveryLoopKindHealing.String(),
				ErrorKind:  contracts.RecoveryErrorKindInfra.String(),
				StrategyID: "infra-default",
				Reason:     "docker socket unavailable",
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal meta: %v", err)
	}

	recovery, _, _ := resolveFailedGateRecoveryContext(store.Job{
		ID:      domaintypes.NewJobID(),
		RunID:   domaintypes.NewRunID(),
		JobType: domaintypes.JobTypePostGate.String(),
		Meta:    meta,
	})
	if recovery == nil {
		t.Fatal("expected recovery metadata")
	}
	if got, want := recovery.ErrorKind, contracts.RecoveryErrorKindInfra.String(); got != want {
		t.Fatalf("error_kind = %q, want %q", got, want)
	}
	if got, want := recovery.StrategyID, "infra-default"; got != want {
		t.Fatalf("strategy_id = %q, want %q", got, want)
	}
}
