package handlers

import (
	"bytes"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/gateprofile"
)

func applyRepoGateProfileMutator(m map[string]any, gateProfile []byte, jobType domaintypes.JobType) error {
	if len(bytes.TrimSpace(gateProfile)) == 0 {
		return nil
	}

	profile, err := contracts.ParseGateProfileJSON(gateProfile)
	if err != nil {
		return err
	}
	phase, override, err := gateprofile.GateOverrideForJobType(profile, jobType)
	if err != nil {
		return err
	}
	if override == nil {
		return nil
	}
	return applyGatePrepOverrideMutator(m, phase, override)
}

func applyRecoveryCandidatePrepMutator(_ map[string]any, _ domaintypes.JobType) error {
	return nil
}

func applyGatePrepOverrideMutator(
	m map[string]any,
	phase contracts.BuildGateProfilePhase,
	override *contracts.BuildGateProfileOverride,
) error {
	phaseKey := ""
	switch phase {
	case contracts.BuildGateProfilePhasePre:
		phaseKey = "pre"
	case contracts.BuildGateProfilePhasePost:
		phaseKey = "post"
	default:
		return nil
	}

	buildGate, err := ensureObjectField(m, "build_gate", "spec")
	if err != nil {
		return err
	}
	phaseCfg, err := ensureObjectField(buildGate, phaseKey, "spec.build_gate")
	if err != nil {
		return err
	}
	if existing, exists := phaseCfg["gate_profile"]; exists && existing != nil {
		return nil
	}
	phaseCfg["gate_profile"] = contracts.BuildGateProfileOverrideToSpecMap(override)
	return nil
}
