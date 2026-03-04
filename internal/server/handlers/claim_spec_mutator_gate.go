package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func mergeRepoGateProfileIntoSpec(spec json.RawMessage, gateProfile []byte, jobType domaintypes.JobType) (json.RawMessage, error) {
	if len(bytes.TrimSpace(gateProfile)) == 0 {
		return spec, nil
	}

	m, err := parseSpecObjectStrict(spec)
	if err != nil {
		return nil, err
	}

	if err := applyRepoGateProfileMutator(m, gateProfile, jobType); err != nil {
		return nil, err
	}

	return marshalSpecObject(m)
}

func applyRepoGateProfileMutator(m map[string]any, gateProfile []byte, jobType domaintypes.JobType) error {
	if len(bytes.TrimSpace(gateProfile)) == 0 {
		return nil
	}

	profile, err := contracts.ParseGateProfileJSON(gateProfile)
	if err != nil {
		return err
	}
	phase, override, err := contracts.GateProfileGateOverrideForJobType(profile, jobType)
	if err != nil {
		return err
	}
	if override == nil {
		return nil
	}
	return applyGatePrepOverrideMutator(m, phase, override)
}

func mergeRecoveryCandidatePrepIntoSpec(spec json.RawMessage, job store.Job, jobType domaintypes.JobType) (json.RawMessage, error) {
	if jobType != domaintypes.JobTypeReGate {
		return spec, nil
	}
	if len(job.Meta) == 0 {
		return spec, nil
	}

	m, err := parseSpecObjectStrict(spec)
	if err != nil {
		return nil, err
	}

	if err := applyRecoveryCandidatePrepMutator(m, job, jobType); err != nil {
		return nil, err
	}

	return marshalSpecObject(m)
}

func applyRecoveryCandidatePrepMutator(m map[string]any, job store.Job, jobType domaintypes.JobType) error {
	if jobType != domaintypes.JobTypeReGate {
		return nil
	}
	if len(job.Meta) == 0 {
		return nil
	}
	jobMeta, err := contracts.UnmarshalJobMeta(job.Meta)
	if err != nil || jobMeta.Recovery == nil {
		return nil
	}
	recovery := jobMeta.Recovery
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
	phase, override, err := contracts.GateProfileGateOverrideForJobType(profile, jobType)
	if err != nil {
		return err
	}
	if override == nil {
		return nil
	}
	return applyGatePrepOverrideMutator(m, phase, override)
}

func mergeGatePrepOverrideIntoSpec(
	spec json.RawMessage,
	phase contracts.BuildGateProfilePhase,
	override *contracts.BuildGateProfileOverride,
) (json.RawMessage, error) {
	m, err := parseSpecObjectStrict(spec)
	if err != nil {
		return nil, err
	}

	if err := applyGatePrepOverrideMutator(m, phase, override); err != nil {
		return nil, err
	}

	return marshalSpecObject(m)
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
	phaseCfg["gate_profile"] = buildGatePrepOverrideToMap(override)
	return nil
}

func buildGatePrepOverrideToMap(override *contracts.BuildGateProfileOverride) map[string]any {
	if override == nil {
		return nil
	}

	prep := map[string]any{
		"command": commandSpecToWireValue(override.Command),
	}
	if len(override.Env) > 0 {
		env := make(map[string]any, len(override.Env))
		for k, v := range override.Env {
			env[k] = v
		}
		prep["env"] = env
	}
	if override.Stack != nil {
		stack := map[string]any{
			"language": override.Stack.Language,
			"tool":     override.Stack.Tool,
		}
		if strings.TrimSpace(override.Stack.Release) != "" {
			stack["release"] = override.Stack.Release
		}
		prep["stack"] = stack
	}
	if target := strings.TrimSpace(override.Target); target != "" {
		prep["target"] = target
	}
	return prep
}

func commandSpecToWireValue(command contracts.CommandSpec) any {
	if len(command.Exec) > 0 {
		out := make([]any, 0, len(command.Exec))
		for _, v := range command.Exec {
			out = append(out, v)
		}
		return out
	}
	return command.Shell
}
