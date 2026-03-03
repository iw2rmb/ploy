package handlers

import (
	"fmt"
	"strings"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

type gatePhasePolicy struct {
	Target            string
	Always            bool
	LookupConstraints GateProfileLookupConstraints
}

func gatePhasePolicyForJobSpec(rawSpec []byte, jobType domaintypes.JobType) (gatePhasePolicy, error) {
	if !shouldResolveGateProfile(jobType) {
		return gatePhasePolicy{}, nil
	}
	spec, err := contracts.ParseModsSpecJSON(rawSpec)
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
		Always:            phase.Always,
		LookupConstraints: lookupConstraints,
	}, nil
}

func resolveGateSkipMetadata(
	payload []byte,
	requiredTarget string,
	sourceProfileID int64,
) (*contracts.BuildGateSkipMetadata, error) {
	profile, err := contracts.ParseGateProfileJSON(payload)
	if err != nil {
		return nil, fmt.Errorf("parse resolved gate profile: %w", err)
	}
	matchedTarget := matchSkipTarget(profile, strings.TrimSpace(requiredTarget))
	if matchedTarget == "" {
		return nil, nil
	}
	return &contracts.BuildGateSkipMetadata{
		Enabled:         true,
		SourceProfileID: sourceProfileID,
		MatchedTarget:   matchedTarget,
	}, nil
}

func matchSkipTarget(profile *contracts.GateProfile, requiredTarget string) string {
	if profile == nil {
		return ""
	}

	if requiredTarget != "" {
		target := profile.Targets.TargetByName(requiredTarget)
		if target != nil && strings.TrimSpace(target.Status) == contracts.PrepTargetStatusPassed {
			return requiredTarget
		}
	}

	checked := map[string]struct{}{}
	candidates := []string{
		strings.TrimSpace(profile.Targets.Active),
		contracts.GateProfileTargetBuild,
		contracts.GateProfileTargetUnit,
		contracts.GateProfileTargetAllTests,
	}
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if _, dup := checked[candidate]; dup {
			continue
		}
		checked[candidate] = struct{}{}
		target := profile.Targets.TargetByName(candidate)
		if target == nil {
			continue
		}
		if strings.TrimSpace(target.Status) == contracts.PrepTargetStatusPassed {
			return candidate
		}
	}
	return ""
}
