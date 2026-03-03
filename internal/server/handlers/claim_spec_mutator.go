package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/config"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

type claimSpecMutatorInput struct {
	spec            json.RawMessage
	job             store.Job
	jobType         domaintypes.JobType
	gitLab          config.GitLabConfig
	globalEnv       map[string]GlobalEnvVar
	repoGateProfile []byte
}

// mutateClaimSpec applies all claim-time spec mutators in a fixed order with
// one parse at the beginning and one marshal at the end.
func mutateClaimSpec(input claimSpecMutatorInput) (json.RawMessage, error) {
	specMap, err := parseSpecObjectStrict(input.spec)
	if err != nil {
		return nil, fmt.Errorf("merge job_id into spec: %w", err)
	}

	if err := applyJobIDMutator(specMap, input.job.ID); err != nil {
		return nil, fmt.Errorf("merge job_id into spec: %w", err)
	}
	if err := applyGitLabConfigMutator(specMap, input.gitLab); err != nil {
		return nil, fmt.Errorf("merge gitlab defaults into spec: %w", err)
	}
	if err := applyGlobalEnvMutator(specMap, input.globalEnv, input.jobType); err != nil {
		return nil, fmt.Errorf("merge global env into spec: %w", err)
	}
	if err := applyRecoveryCandidatePrepMutator(specMap, input.job, input.jobType); err != nil {
		return nil, fmt.Errorf("merge recovery candidate prep into spec: %w", err)
	}
	if err := applyRepoGateProfileMutator(specMap, input.repoGateProfile, input.jobType); err != nil {
		return nil, fmt.Errorf("merge repo gate_profile into spec: %w", err)
	}
	if err := applyHealingSelectedKindMutator(specMap, input.job, input.jobType); err != nil {
		return nil, fmt.Errorf("merge healing selected_error_kind into spec: %w", err)
	}
	if err := applyHealingSchemaMutator(specMap, input.job, input.jobType); err != nil {
		return nil, fmt.Errorf("merge healing schema into spec: %w", err)
	}

	return marshalSpecObject(specMap)
}

// mergeJobIDIntoSpec injects job_id into the spec JSONB for downstream execution.
func mergeJobIDIntoSpec(spec []byte, jobID domaintypes.JobID) (json.RawMessage, error) {
	m, err := parseSpecObjectStrict(json.RawMessage(spec))
	if err != nil {
		return nil, err
	}
	if err := applyJobIDMutator(m, jobID); err != nil {
		return nil, err
	}
	return marshalSpecObject(m)
}

func parseSpecObjectStrict(spec json.RawMessage) (map[string]any, error) {
	if len(bytes.TrimSpace(spec)) == 0 {
		return nil, fmt.Errorf("spec: expected JSON object, got empty")
	}

	var m map[string]any
	if err := json.Unmarshal(spec, &m); err != nil {
		return nil, fmt.Errorf("spec: expected JSON object: %w", err)
	}
	if m == nil {
		return nil, fmt.Errorf("spec: expected JSON object, got null")
	}
	return m, nil
}

func marshalSpecObject(m map[string]any) (json.RawMessage, error) {
	b, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("marshal merged spec: %w", err)
	}
	return json.RawMessage(b), nil
}

func applyJobIDMutator(m map[string]any, jobID domaintypes.JobID) error {
	m["job_id"] = jobID.String()
	return nil
}

// mergeGlobalEnvIntoSpec injects global environment variables into the spec's "env" map.
// Global env vars are only merged if their scope matches the job type.
// Per-run env vars (already in spec) take precedence over global env — existing keys
// are not overwritten.
func mergeGlobalEnvIntoSpec(spec json.RawMessage, env map[string]GlobalEnvVar, jobType domaintypes.JobType) (json.RawMessage, error) {
	if len(env) == 0 {
		return spec, nil
	}

	m, err := parseSpecObjectStrict(spec)
	if err != nil {
		return nil, err
	}

	if err := applyGlobalEnvMutator(m, env, jobType); err != nil {
		return nil, err
	}

	return marshalSpecObject(m)
}

func applyGlobalEnvMutator(m map[string]any, env map[string]GlobalEnvVar, jobType domaintypes.JobType) error {
	if len(env) == 0 {
		return nil
	}

	var em map[string]any
	if v, ok := m["env"]; ok && v != nil {
		var ok2 bool
		em, ok2 = v.(map[string]any)
		if !ok2 {
			return fmt.Errorf("spec.env: expected object, got %T", v)
		}
	} else {
		em = map[string]any{}
	}

	for k, v := range env {
		if !v.Scope.MatchesJobType(jobType) {
			continue
		}
		if _, exists := em[k]; exists {
			continue
		}
		em[k] = v.Value
	}
	m["env"] = em
	return nil
}

// mergeGitLabConfigIntoSpec merges GitLab default token and domain into the JSON spec payload.
// Only merges values if they are non-empty and not already present in the spec.
// Per-run values (already in spec) take precedence over server defaults.
func mergeGitLabConfigIntoSpec(spec json.RawMessage, cfg config.GitLabConfig) (json.RawMessage, error) {
	if strings.TrimSpace(cfg.Token) == "" && strings.TrimSpace(cfg.Domain) == "" {
		return spec, nil
	}

	m, err := parseSpecObjectStrict(spec)
	if err != nil {
		return nil, err
	}

	if err := applyGitLabConfigMutator(m, cfg); err != nil {
		return nil, err
	}

	return marshalSpecObject(m)
}

func applyGitLabConfigMutator(m map[string]any, cfg config.GitLabConfig) error {
	if strings.TrimSpace(cfg.Token) == "" && strings.TrimSpace(cfg.Domain) == "" {
		return nil
	}
	if _, hasPerRunPAT := m["gitlab_pat"]; !hasPerRunPAT && cfg.Token != "" {
		m["gitlab_pat"] = cfg.Token
	}
	if _, hasPerRunDomain := m["gitlab_domain"]; !hasPerRunDomain && cfg.Domain != "" {
		m["gitlab_domain"] = cfg.Domain
	}
	return nil
}

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

func ensureObjectField(parent map[string]any, key string, prefix string) (map[string]any, error) {
	if v, ok := parent[key]; ok && v != nil {
		obj, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s.%s: expected object, got %T", prefix, key, v)
		}
		return obj, nil
	}
	obj := map[string]any{}
	parent[key] = obj
	return obj, nil
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

func mergeHealingSelectedKindIntoSpec(spec json.RawMessage, job store.Job, jobType domaintypes.JobType) (json.RawMessage, error) {
	if jobType != domaintypes.JobTypeHeal {
		return spec, nil
	}
	if len(job.Meta) == 0 {
		return spec, nil
	}

	m, err := parseSpecObjectStrict(spec)
	if err != nil {
		return nil, err
	}

	if err := applyHealingSelectedKindMutator(m, job, jobType); err != nil {
		return nil, err
	}

	return marshalSpecObject(m)
}

func applyHealingSelectedKindMutator(m map[string]any, job store.Job, jobType domaintypes.JobType) error {
	if jobType != domaintypes.JobTypeHeal {
		return nil
	}
	if len(job.Meta) == 0 {
		return nil
	}
	jobMeta, err := contracts.UnmarshalJobMeta(job.Meta)
	if err != nil {
		return nil
	}
	if jobMeta.Recovery == nil || strings.TrimSpace(jobMeta.Recovery.ErrorKind) == "" {
		return nil
	}

	buildGate, err := ensureObjectField(m, "build_gate", "spec")
	if err != nil {
		return err
	}
	healing, err := ensureObjectField(buildGate, "healing", "spec.build_gate")
	if err != nil {
		return err
	}
	kind, ok := contracts.ParseRecoveryErrorKind(jobMeta.Recovery.ErrorKind)
	if !ok {
		kind = contracts.DefaultRecoveryErrorKind()
	}
	healing["selected_error_kind"] = kind.String()
	if len(jobMeta.Recovery.Expectations) > 0 {
		var ex struct {
			Artifacts []struct {
				Path string `json:"path"`
			} `json:"artifacts"`
		}
		if err := json.Unmarshal(jobMeta.Recovery.Expectations, &ex); err == nil && len(ex.Artifacts) > 0 {
			existing := map[string]struct{}{}
			var paths []any
			if cur, ok := m["artifact_paths"]; ok && cur != nil {
				switch vv := cur.(type) {
				case []any:
					for _, item := range vv {
						if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
							existing[s] = struct{}{}
							paths = append(paths, s)
						}
					}
				}
			}
			for _, artifact := range ex.Artifacts {
				p := strings.TrimSpace(artifact.Path)
				if p == "" {
					continue
				}
				if _, ok := existing[p]; ok {
					continue
				}
				existing[p] = struct{}{}
				paths = append(paths, p)
			}
			if len(paths) > 0 {
				m["artifact_paths"] = paths
			}
		}
	}
	return nil
}

func mergeHealingSchemaIntoSpec(spec json.RawMessage, job store.Job, jobType domaintypes.JobType) (json.RawMessage, error) {
	if jobType != domaintypes.JobTypeHeal {
		return spec, nil
	}
	if len(job.Meta) == 0 {
		return spec, nil
	}

	m, err := parseSpecObjectStrict(spec)
	if err != nil {
		return nil, err
	}

	if err := applyHealingSchemaMutator(m, job, jobType); err != nil {
		return nil, err
	}

	return marshalSpecObject(m)
}

func applyHealingSchemaMutator(m map[string]any, job store.Job, jobType domaintypes.JobType) error {
	if jobType != domaintypes.JobTypeHeal {
		return nil
	}
	if len(job.Meta) == 0 {
		return nil
	}
	jobMeta, err := contracts.UnmarshalJobMeta(job.Meta)
	if err != nil || jobMeta.Recovery == nil {
		return nil
	}
	kind, ok := contracts.ParseRecoveryErrorKind(jobMeta.Recovery.ErrorKind)
	if !ok || !contracts.IsInfraRecoveryErrorKind(kind) {
		return nil
	}

	schemaRaw, err := contracts.ReadGateProfileSchemaJSON()
	if err != nil {
		return err
	}
	if !json.Valid(schemaRaw) {
		return fmt.Errorf("gate profile schema JSON is invalid")
	}

	env, err := ensureObjectField(m, "env", "spec")
	if err != nil {
		return err
	}
	env[contracts.GateProfileSchemaJSONEnv] = string(schemaRaw)
	return nil
}
