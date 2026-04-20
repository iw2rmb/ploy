package handlers

import (
	"fmt"
	"strings"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

type gatePhasePolicy struct {
	Target            string
	LookupConstraints GateProfileLookupConstraints
}

func gatePhasePolicyForJobSpec(rawSpec []byte, jobType domaintypes.JobType) (gatePhasePolicy, error) {
	if !shouldResolveGateProfile(jobType) {
		return gatePhasePolicy{}, nil
	}
	spec, err := contracts.ParseMigSpecJSON(rawSpec)
	if err != nil {
		return gatePhasePolicy{}, fmt.Errorf("parse build gate phase policy from spec: %w", err)
	}
	if spec.BuildGate == nil {
		return gatePhasePolicy{}, nil
	}

	var phase *contracts.BuildGatePhaseConfig
	switch jobType {
	case domaintypes.JobTypePreGate:
		phase = spec.BuildGate.Pre
	case domaintypes.JobTypePostGate, domaintypes.JobTypeReGate:
		phase = spec.BuildGate.Post
	default:
		return gatePhasePolicy{}, nil
	}
	if phase == nil {
		return gatePhasePolicy{}, nil
	}
	lookupConstraints := GateProfileLookupConstraints{}
	if phase.Stack != nil && phase.Stack.Enabled && !phase.Stack.Default {
		lookupConstraints.StrictStack = &GateProfileLookupStack{
			Language: strings.TrimSpace(phase.Stack.Language),
			Tool:     strings.TrimSpace(phase.Stack.Tool),
			Release:  strings.TrimSpace(phase.Stack.Release),
		}
	}
	return gatePhasePolicy{
		Target:            strings.TrimSpace(phase.Target),
		LookupConstraints: lookupConstraints,
	}, nil
}
