package handlers

import (
	"bytes"
	"fmt"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
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

func applyRecoveryCandidatePrepMutator(m map[string]any, job store.Job, jobType domaintypes.JobType) error {
	if jobType != domaintypes.JobTypeReGate {
		return nil
	}
	if len(job.Meta) == 0 {
		return nil
	}
	jobMeta, err := contracts.UnmarshalJobMeta(job.Meta)
	if err != nil || jobMeta.RecoveryMetadata == nil {
		return nil
	}
	recovery := jobMeta.RecoveryMetadata
	if recovery.CandidateValidationStatus != contracts.RecoveryCandidateStatusValid {
		return nil
	}
	if len(bytes.TrimSpace(recovery.CandidateGateProfile)) == 0 {
		return nil
	}
	profile, err := contracts.ParseGateProfileJSON(recovery.CandidateGateProfile)
	if err != nil {
		return fmt.Errorf("parse recovery candidate gate_profile: %w", err)
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
