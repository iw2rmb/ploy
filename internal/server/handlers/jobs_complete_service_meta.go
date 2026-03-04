package handlers

import "github.com/iw2rmb/ploy/internal/workflow/contracts"

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
		incoming.Recovery = cloneRecoveryMetadataForCompletion(existing.Recovery)
		merged = true
	}
	if incoming.Gate != nil && incoming.Gate.Recovery == nil && existing.Gate != nil && existing.Gate.Recovery != nil {
		incoming.Gate.Recovery = cloneRecoveryMetadataForCompletion(existing.Gate.Recovery)
		merged = true
	}
	if !merged {
		return incomingRaw, nil
	}
	return contracts.MarshalJobMeta(incoming)
}

func cloneRecoveryMetadataForCompletion(src *contracts.BuildGateRecoveryMetadata) *contracts.BuildGateRecoveryMetadata {
	if src == nil {
		return nil
	}
	out := *src
	if src.Confidence != nil {
		v := *src.Confidence
		out.Confidence = &v
	}
	if src.CandidatePromoted != nil {
		v := *src.CandidatePromoted
		out.CandidatePromoted = &v
	}
	if len(src.Expectations) > 0 {
		out.Expectations = append([]byte(nil), src.Expectations...)
	}
	if len(src.CandidateGateProfile) > 0 {
		out.CandidateGateProfile = append([]byte(nil), src.CandidateGateProfile...)
	}
	if src.DepsBumps != nil {
		out.DepsBumps = cloneDepsBumpsMap(src.DepsBumps)
	}
	return &out
}
