package handlers

import (
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/lifecycle"
)

func mergeCompletionJobMeta(existingRaw, incomingRaw []byte) ([]byte, error) {
	incoming, err := contracts.UnmarshalJobMeta(incomingRaw)
	if err != nil {
		return nil, err
	}

	existing, err := contracts.UnmarshalJobMeta(existingRaw)
	if err != nil {
		return incomingRaw, nil
	}

	merged := false
	if incoming.RecoveryMetadata == nil && existing.RecoveryMetadata != nil {
		incoming.RecoveryMetadata = lifecycle.CloneRecoveryMetadata(existing.RecoveryMetadata)
		merged = true
	}
	if incoming.GateMetadata != nil && incoming.GateMetadata.Recovery == nil && existing.GateMetadata != nil && existing.GateMetadata.Recovery != nil {
		incoming.GateMetadata.Recovery = lifecycle.CloneRecoveryMetadata(existing.GateMetadata.Recovery)
		merged = true
	}
	if !merged {
		return incomingRaw, nil
	}
	return contracts.MarshalJobMeta(incoming)
}
