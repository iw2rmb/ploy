package contracts

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	types "github.com/iw2rmb/ploy/internal/domain/types"
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

	GateProfileDockerHostEnv       = "DOCKER_HOST"
	GateProfileDockerAPIVersionEnv = "DOCKER_API_VERSION"

	defaultGateProfileDockerHostSocket = "unix:///var/run/docker.sock"

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

	if err := validateGateProfileTarget(profile.Targets.Build, "gate_profile.targets.build"); err != nil {
		return nil, err
	}
	if err := validateGateProfileTarget(profile.Targets.Unit, "gate_profile.targets.unit"); err != nil {
		return nil, err
	}
	if err := validateGateProfileTarget(profile.Targets.AllTests, "gate_profile.targets.all_tests"); err != nil {
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

func GateProfileStackMatches(profile *GateProfile, language, tool, release string) bool {
	if profile == nil {
		return false
	}
	pLang := strings.TrimSpace(profile.Stack.Language)
	pTool := strings.TrimSpace(profile.Stack.Tool)
	if pLang == "" || pTool == "" {
		return false
	}
	if !StackFieldsMatch(language, tool, "", pLang, pTool, "") {
		return false
	}
	// Release has special semantics here: empty input release always matches,
	// but a non-empty input release requires the profile to have a release.
	if strings.TrimSpace(release) == "" {
		return true
	}
	pRelease := strings.TrimSpace(profile.Stack.Release)
	if pRelease == "" {
		return false
	}
	return pRelease == strings.TrimSpace(release)
}

func GateProfileGateOverrideForJobType(
	profile *GateProfile,
	jobType types.JobType,
) (BuildGateProfilePhase, *BuildGateProfileOverride, error) {
	if profile == nil {
		return "", nil, nil
	}

	var phase BuildGateProfilePhase
	var target *GateProfileTarget

	switch jobType {
	case types.JobTypePreGate:
		phase = BuildGateProfilePhasePre
		target = profile.Targets.Build
	case types.JobTypePostGate, types.JobTypeReGate:
		phase = BuildGateProfilePhasePost
		target = profile.Targets.Unit
	default:
		return "", nil, nil
	}

	if target == nil {
		return "", nil, fmt.Errorf("gate_profile: missing mapped target for job_type %q", jobType)
	}

	override, err := gateProfileTargetToBuildGateOverride(target)
	if err != nil {
		return "", nil, err
	}
	if override == nil {
		return phase, nil, nil
	}
	override.Stack = &GateProfileStack{
		Language: profile.Stack.Language,
		Tool:     profile.Stack.Tool,
		Release:  profile.Stack.Release,
	}
	runtimeEnv, err := gateProfileRuntimeGateEnv(profile.Runtime)
	if err != nil {
		return "", nil, err
	}
	override.Env = MergeEnv(override.Env, runtimeEnv)
	return phase, override, nil
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

func gateProfileTargetToBuildGateOverride(target *GateProfileTarget) (*BuildGateProfileOverride, error) {
	if target == nil || target.Status != PrepTargetStatusPassed {
		return nil, nil
	}
	cmd := strings.TrimSpace(target.Command)
	if cmd == "" {
		return nil, nil
	}
	return &BuildGateProfileOverride{
		Command: CommandSpec{Shell: cmd},
		Env:     CopyEnv(target.Env),
	}, nil
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

func gateProfileRuntimeGateEnv(runtime *GateProfileRuntime) (map[string]string, error) {
	if runtime == nil || runtime.Docker == nil {
		return nil, nil
	}

	mode := strings.TrimSpace(runtime.Docker.Mode)
	apiVersion := strings.TrimSpace(runtime.Docker.APIVersion)
	switch mode {
	case GateProfileDockerModeNone:
		if apiVersion == "" {
			return nil, nil
		}
		return map[string]string{GateProfileDockerAPIVersionEnv: apiVersion}, nil
	case GateProfileDockerModeHostSocket:
		env := map[string]string{
			GateProfileDockerHostEnv: defaultGateProfileDockerHostSocket,
		}
		if apiVersion != "" {
			env[GateProfileDockerAPIVersionEnv] = apiVersion
		}
		return env, nil
	case GateProfileDockerModeTCP:
		host := strings.TrimSpace(runtime.Docker.Host)
		if host == "" {
			return nil, fmt.Errorf("gate_profile.runtime.docker.host: required for mode %q", GateProfileDockerModeTCP)
		}
		env := map[string]string{
			GateProfileDockerHostEnv: host,
		}
		if apiVersion != "" {
			env[GateProfileDockerAPIVersionEnv] = apiVersion
		}
		return env, nil
	default:
		return nil, fmt.Errorf("gate_profile.runtime.docker.mode: invalid value %q", runtime.Docker.Mode)
	}
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
