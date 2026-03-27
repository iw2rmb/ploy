package contracts

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	PrepTargetStatusPassed          = "passed"
	PrepTargetStatusFailed          = "failed"
	PrepTargetStatusNotAttempted    = "not_attempted"
	PrepRunnerModeSimple            = "simple"
	PrepRunnerModeComplex           = "complex"
	GateProfileDockerModeNone       = "none"
	GateProfileDockerModeHostSocket = "host_socket"
	GateProfileDockerModeTCP        = "tcp"
	GateProfileTargetBuild          = "build"
	GateProfileTargetUnit           = "unit"
	GateProfileTargetAllTests       = "all_tests"
	GateProfileTargetUnsupported    = "unsupported"

	GateProfileFailureCodeInfraSupport = "infra_support"

	GateProfileDockerHostEnv       = "DOCKER_HOST"
	GateProfileDockerAPIVersionEnv = "DOCKER_API_VERSION"

	GateProfileCandidateArtifactPath = "/out/gate-profile-candidate.json"
	GateProfileCandidateSchemaID     = "gate_profile_v1"
	GateProfileSchemaJSONEnv         = "PLOY_GATE_PROFILE_SCHEMA_JSON"
)

type BuildGateProfilePhase string

const (
	BuildGateProfilePhasePre  BuildGateProfilePhase = "pre"
	BuildGateProfilePhasePost BuildGateProfilePhase = "post"
)

type GateProfile struct {
	SchemaVersion int                      `json:"schema_version"`
	RepoID        string                   `json:"repo_id"`
	RunnerMode    string                   `json:"runner_mode"`
	Stack         GateProfileStack         `json:"stack"`
	Targets       GateProfileTargets       `json:"targets"`
	Runtime       *GateProfileRuntime      `json:"runtime,omitempty"`
	Orchestration GateProfileOrchestration `json:"orchestration"`
}

type GateProfileStack struct {
	Language string `json:"language"`
	Tool     string `json:"tool"`
	Release  string `json:"release,omitempty"`
}

type GateProfileTargets struct {
	Active   string             `json:"active"`
	Build    *GateProfileTarget `json:"build"`
	Unit     *GateProfileTarget `json:"unit"`
	AllTests *GateProfileTarget `json:"all_tests"`
}

type GateProfileTarget struct {
	Status      string            `json:"status"`
	Command     string            `json:"command,omitempty"`
	Env         map[string]string `json:"env"`
	FailureCode *string           `json:"failure_code,omitempty"`
}

type GateProfileRuntime struct {
	Docker *GateProfileRuntimeDocker `json:"docker,omitempty"`
}

type GateProfileRuntimeDocker struct {
	Mode       string `json:"mode"`
	Host       string `json:"host,omitempty"`
	APIVersion string `json:"api_version,omitempty"`
}

type GateProfileOrchestration struct {
	Pre  []json.RawMessage `json:"pre"`
	Post []json.RawMessage `json:"post"`
}

func ParseGateProfileJSON(raw []byte) (*GateProfile, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, fmt.Errorf("gate_profile: required")
	}

	var profile GateProfile
	if err := json.Unmarshal(raw, &profile); err != nil {
		return nil, fmt.Errorf("gate_profile: parse json: %w", err)
	}
	if profile.SchemaVersion < 1 {
		return nil, fmt.Errorf("gate_profile.schema_version: must be >= 1")
	}
	if strings.TrimSpace(profile.RepoID) == "" {
		return nil, fmt.Errorf("gate_profile.repo_id: required")
	}
	if strings.TrimSpace(profile.RunnerMode) == "" {
		return nil, fmt.Errorf("gate_profile.runner_mode: required")
	}
	if strings.TrimSpace(profile.Stack.Language) == "" {
		return nil, fmt.Errorf("gate_profile.stack.language: required")
	}
	if strings.TrimSpace(profile.Stack.Tool) == "" {
		return nil, fmt.Errorf("gate_profile.stack.tool: required")
	}
	switch profile.RunnerMode {
	case PrepRunnerModeSimple, PrepRunnerModeComplex:
	default:
		return nil, fmt.Errorf("gate_profile.runner_mode: invalid value %q", profile.RunnerMode)
	}
	if profile.Targets.Build == nil {
		return nil, fmt.Errorf("gate_profile.targets.build: required")
	}
	if profile.Targets.Unit == nil {
		return nil, fmt.Errorf("gate_profile.targets.unit: required")
	}
	if profile.Targets.AllTests == nil {
		return nil, fmt.Errorf("gate_profile.targets.all_tests: required")
	}
	profile.Targets.Active = strings.TrimSpace(profile.Targets.Active)
	if profile.Targets.Active == "" {
		return nil, fmt.Errorf("gate_profile.targets.active: required")
	}
	switch profile.Targets.Active {
	case GateProfileTargetBuild, GateProfileTargetUnit, GateProfileTargetAllTests, GateProfileTargetUnsupported:
	default:
		return nil, fmt.Errorf("gate_profile.targets.active: invalid value %q", profile.Targets.Active)
	}

	if err := validateGateProfileTarget(profile.Targets.Build, "gate_profile.targets.build"); err != nil {
		return nil, err
	}
	if err := validateGateProfileTarget(profile.Targets.Unit, "gate_profile.targets.unit"); err != nil {
		return nil, err
	}
	if err := validateGateProfileTarget(profile.Targets.AllTests, "gate_profile.targets.all_tests"); err != nil {
		return nil, err
	}
	if err := validateGateProfileActiveTarget(profile.Targets); err != nil {
		return nil, err
	}
	if err := validateGateProfileRuntime(profile.Runtime); err != nil {
		return nil, err
	}
	if profile.Orchestration.Pre == nil {
		return nil, fmt.Errorf("gate_profile.orchestration.pre: required")
	}
	if profile.Orchestration.Post == nil {
		return nil, fmt.Errorf("gate_profile.orchestration.post: required")
	}
	if profile.RunnerMode == PrepRunnerModeSimple {
		if len(profile.Orchestration.Pre) > 0 || len(profile.Orchestration.Post) > 0 {
			return nil, fmt.Errorf("gate_profile.orchestration: simple mode must not define pre/post steps")
		}
	}

	return &profile, nil
}

func (targets GateProfileTargets) TargetByName(name string) *GateProfileTarget {
	switch strings.TrimSpace(name) {
	case GateProfileTargetBuild:
		return targets.Build
	case GateProfileTargetUnit:
		return targets.Unit
	case GateProfileTargetAllTests:
		return targets.AllTests
	default:
		return nil
	}
}

func validateGateProfileActiveTarget(targets GateProfileTargets) error {
	active := strings.TrimSpace(targets.Active)
	switch active {
	case GateProfileTargetBuild, GateProfileTargetUnit, GateProfileTargetAllTests:
		activeTarget := targets.TargetByName(active)
		if activeTarget == nil {
			return fmt.Errorf("gate_profile.targets.%s: required when targets.active=%q", active, active)
		}
		if strings.TrimSpace(activeTarget.Command) == "" {
			return fmt.Errorf("gate_profile.targets.%s.command: required when targets.active=%q", active, active)
		}
		return nil
	case GateProfileTargetUnsupported:
		if targets.Build == nil {
			return fmt.Errorf("gate_profile.targets.build: required when targets.active=%q", GateProfileTargetUnsupported)
		}
		if targets.Build.Status != PrepTargetStatusFailed {
			return fmt.Errorf(
				"gate_profile.targets.build.status: must be %q when targets.active=%q",
				PrepTargetStatusFailed,
				GateProfileTargetUnsupported,
			)
		}
		if targets.Build.FailureCode == nil || strings.TrimSpace(*targets.Build.FailureCode) != GateProfileFailureCodeInfraSupport {
			return fmt.Errorf(
				"gate_profile.targets.build.failure_code: must be %q when targets.active=%q",
				GateProfileFailureCodeInfraSupport,
				GateProfileTargetUnsupported,
			)
		}
		return nil
	default:
		return fmt.Errorf("gate_profile.targets.active: invalid value %q", active)
	}
}


func validateGateProfileTarget(target *GateProfileTarget, prefix string) error {
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

func validateGateProfileRuntime(runtime *GateProfileRuntime) error {
	if runtime == nil || runtime.Docker == nil {
		return nil
	}

	mode := strings.TrimSpace(runtime.Docker.Mode)
	if mode == "" {
		return fmt.Errorf("gate_profile.runtime.docker.mode: required")
	}
	host := strings.TrimSpace(runtime.Docker.Host)
	switch mode {
	case GateProfileDockerModeNone, GateProfileDockerModeHostSocket:
		if host != "" {
			return fmt.Errorf("gate_profile.runtime.docker.host: forbidden for mode %q", mode)
		}
	case GateProfileDockerModeTCP:
		if host == "" {
			return fmt.Errorf("gate_profile.runtime.docker.host: required for mode %q", GateProfileDockerModeTCP)
		}
	default:
		return fmt.Errorf("gate_profile.runtime.docker.mode: invalid value %q", runtime.Docker.Mode)
	}
	if runtime.Docker.APIVersion != "" && strings.TrimSpace(runtime.Docker.APIVersion) == "" {
		return fmt.Errorf("gate_profile.runtime.docker.api_version: must not be blank")
	}
	return nil
}

func IsSupportedGateProfileArtifactSchema(schema string) bool {
	return strings.TrimSpace(schema) == GateProfileCandidateSchemaID
}

func ValidateGateProfileArtifactContract(path, schema, prefix string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("%s.path: required", prefix)
	}
	if strings.TrimSpace(schema) == "" {
		return fmt.Errorf("%s.schema: required", prefix)
	}
	if schema == GateProfileCandidateSchemaID && path != GateProfileCandidateArtifactPath {
		return fmt.Errorf("%s.path: must be %q when schema=%q", prefix, GateProfileCandidateArtifactPath, GateProfileCandidateSchemaID)
	}
	if schema != GateProfileCandidateSchemaID {
		return fmt.Errorf("%s.schema: unsupported value %q", prefix, schema)
	}
	return nil
}
