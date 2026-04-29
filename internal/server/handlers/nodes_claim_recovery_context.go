package handlers

import (
	"context"

	"github.com/iw2rmb/ploy/internal/blobstore"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func buildRecoveryClaimContext(
	_ context.Context,
	_ store.Store,
	_ blobstore.Store,
	_ domaintypes.RunID,
	_ store.Job,
	_ domaintypes.JobType,
) (*contracts.RecoveryClaimContext, error) {
	// build_gate.heal and rebuild-gate job machinery are removed.
	return nil, nil
}

func isGateJobTypeForClaim(jobType domaintypes.JobType) bool {
	return jobType == domaintypes.JobTypePreGate || jobType == domaintypes.JobTypePostGate
}
