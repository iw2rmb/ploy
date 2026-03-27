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
	if incoming.Recovery == nil && existing.Recovery != nil {
		incoming.Recovery = lifecycle.CloneRecoveryMetadata(existing.Recovery)
		merged = true
	}
	if incoming.Gate != nil && incoming.Gate.Recovery == nil && existing.Gate != nil && existing.Gate.Recovery != nil {
		incoming.Gate.Recovery = lifecycle.CloneRecoveryMetadata(existing.Gate.Recovery)
		merged = true
	}
	if !merged {
		return incomingRaw, nil
	}
	return contracts.MarshalJobMeta(incoming)
}
