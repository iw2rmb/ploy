package contracts

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

const (
	PrepTargetStatusPassed       = "passed"
	PrepTargetStatusFailed       = "failed"
	PrepTargetStatusNotAttempted = "not_attempted"
)

type BuildGatePrepPhase string

const (
	BuildGatePrepPhasePre  BuildGatePrepPhase = "pre"
	BuildGatePrepPhasePost BuildGatePrepPhase = "post"
)

type PrepProfile struct {
	SchemaVersion int                `json:"schema_version"`
	RepoID        string             `json:"repo_id"`
	RunnerMode    string             `json:"runner_mode"`
	Targets       PrepProfileTargets `json:"targets"`
}

type PrepProfileTargets struct {
	Build    *PrepProfileTarget `json:"build"`
	Unit     *PrepProfileTarget `json:"unit"`
	AllTests *PrepProfileTarget `json:"all_tests"`
}

type PrepProfileTarget struct {
	Status      string            `json:"status"`
	Command     string            `json:"command,omitempty"`
	Env         map[string]string `json:"env"`
	FailureCode *string           `json:"failure_code,omitempty"`
}

func ParsePrepProfileJSON(raw []byte) (*PrepProfile, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, fmt.Errorf("prep_profile: required")
	}

	var profile PrepProfile
	if err := json.Unmarshal(raw, &profile); err != nil {
		return nil, fmt.Errorf("prep_profile: parse json: %w", err)
	}
	if profile.SchemaVersion < 1 {
		return nil, fmt.Errorf("prep_profile.schema_version: must be >= 1")
	}
	if strings.TrimSpace(profile.RepoID) == "" {
		return nil, fmt.Errorf("prep_profile.repo_id: required")
	}
	if strings.TrimSpace(profile.RunnerMode) == "" {
		return nil, fmt.Errorf("prep_profile.runner_mode: required")
	}
	if profile.Targets.Build == nil {
		return nil, fmt.Errorf("prep_profile.targets.build: required")
	}
	if profile.Targets.Unit == nil {
		return nil, fmt.Errorf("prep_profile.targets.unit: required")
	}
	if profile.Targets.AllTests == nil {
		return nil, fmt.Errorf("prep_profile.targets.all_tests: required")
	}

	if err := validatePrepProfileTarget(profile.Targets.Build, "prep_profile.targets.build"); err != nil {
		return nil, err
	}
	if err := validatePrepProfileTarget(profile.Targets.Unit, "prep_profile.targets.unit"); err != nil {
		return nil, err
	}
	if err := validatePrepProfileTarget(profile.Targets.AllTests, "prep_profile.targets.all_tests"); err != nil {
		return nil, err
	}

	return &profile, nil
}

func PrepProfileGateOverrideForJobType(
	profile *PrepProfile,
	jobType types.JobType,
) (BuildGatePrepPhase, *BuildGatePrepOverride, error) {
	if profile == nil {
		return "", nil, nil
	}

	var phase BuildGatePrepPhase
	var target *PrepProfileTarget

	switch jobType {
	case types.JobTypePreGate:
		phase = BuildGatePrepPhasePre
		target = profile.Targets.Build
	case types.JobTypePostGate, types.JobTypeReGate:
		phase = BuildGatePrepPhasePost
		target = profile.Targets.Unit
	default:
		return "", nil, nil
	}

	if target == nil {
		return "", nil, fmt.Errorf("prep_profile: missing mapped target for job_type %q", jobType)
	}

	override, err := prepProfileTargetToBuildGateOverride(target)
	if err != nil {
		return "", nil, err
	}
	return phase, override, nil
}

func validatePrepProfileTarget(target *PrepProfileTarget, prefix string) error {
	if target == nil {
		return fmt.Errorf("%s: required", prefix)
	}
	switch target.Status {
	case PrepTargetStatusPassed, PrepTargetStatusFailed, PrepTargetStatusNotAttempted:
	default:
		return fmt.Errorf("%s.status: invalid value %q", prefix, target.Status)
	}

	if target.Status == PrepTargetStatusPassed || target.Status == PrepTargetStatusFailed {
		if strings.TrimSpace(target.Command) == "" {
			return fmt.Errorf("%s.command: required when status=%q", prefix, target.Status)
		}
	}
	return nil
}

func prepProfileTargetToBuildGateOverride(target *PrepProfileTarget) (*BuildGatePrepOverride, error) {
	if target == nil || target.Status != PrepTargetStatusPassed {
		return nil, nil
	}
	cmd := strings.TrimSpace(target.Command)
	if cmd == "" {
		return nil, nil
	}
	return &BuildGatePrepOverride{
		Command: CommandSpec{Shell: cmd},
		Env:     copyPrepProfileEnv(target.Env),
	}, nil
}

func copyPrepProfileEnv(env map[string]string) map[string]string {
	if len(env) == 0 {
		return nil
	}
	out := make(map[string]string, len(env))
	for k, v := range env {
		out[k] = v
	}
	return out
}
