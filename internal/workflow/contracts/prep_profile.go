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
	PrepRunnerModeSimple         = "simple"
	PrepRunnerModeComplex        = "complex"
	PrepDockerModeNone           = "none"
	PrepDockerModeHostSocket     = "host_socket"
	PrepDockerModeTCP            = "tcp"

	PrepDockerHostEnv       = "DOCKER_HOST"
	PrepDockerAPIVersionEnv = "DOCKER_API_VERSION"

	defaultPrepDockerHostSocket = "unix:///var/run/docker.sock"

	PrepProfileCandidateArtifactPath = "/out/prep-profile-candidate.json"
	PrepProfileCandidateSchemaID     = "prep_profile_v1"
)

type BuildGatePrepPhase string

const (
	BuildGatePrepPhasePre  BuildGatePrepPhase = "pre"
	BuildGatePrepPhasePost BuildGatePrepPhase = "post"
)

type PrepProfile struct {
	SchemaVersion int                      `json:"schema_version"`
	RepoID        string                   `json:"repo_id"`
	RunnerMode    string                   `json:"runner_mode"`
	Stack         PrepProfileStack         `json:"stack"`
	Targets       PrepProfileTargets       `json:"targets"`
	Runtime       *PrepProfileRuntime      `json:"runtime,omitempty"`
	Orchestration PrepProfileOrchestration `json:"orchestration"`
}

type PrepProfileStack struct {
	Language string `json:"language"`
	Tool     string `json:"tool"`
	Release  string `json:"release,omitempty"`
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

type PrepProfileRuntime struct {
	Docker *PrepProfileRuntimeDocker `json:"docker,omitempty"`
}

type PrepProfileRuntimeDocker struct {
	Mode       string `json:"mode"`
	Host       string `json:"host,omitempty"`
	APIVersion string `json:"api_version,omitempty"`
}

type PrepProfileOrchestration struct {
	Pre  []json.RawMessage `json:"pre"`
	Post []json.RawMessage `json:"post"`
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
	if strings.TrimSpace(profile.Stack.Language) == "" {
		return nil, fmt.Errorf("prep_profile.stack.language: required")
	}
	if strings.TrimSpace(profile.Stack.Tool) == "" {
		return nil, fmt.Errorf("prep_profile.stack.tool: required")
	}
	switch profile.RunnerMode {
	case PrepRunnerModeSimple, PrepRunnerModeComplex:
	default:
		return nil, fmt.Errorf("prep_profile.runner_mode: invalid value %q", profile.RunnerMode)
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
	if err := validatePrepProfileRuntime(profile.Runtime); err != nil {
		return nil, err
	}
	if profile.Orchestration.Pre == nil {
		return nil, fmt.Errorf("prep_profile.orchestration.pre: required")
	}
	if profile.Orchestration.Post == nil {
		return nil, fmt.Errorf("prep_profile.orchestration.post: required")
	}
	if profile.RunnerMode == PrepRunnerModeSimple {
		if len(profile.Orchestration.Pre) > 0 || len(profile.Orchestration.Post) > 0 {
			return nil, fmt.Errorf("prep_profile.orchestration: simple mode must not define pre/post steps")
		}
	}

	return &profile, nil
}

func PrepProfileStackMatches(profile *PrepProfile, language, tool, release string) bool {
	if profile == nil {
		return false
	}
	pLang := strings.TrimSpace(strings.ToLower(profile.Stack.Language))
	pTool := strings.TrimSpace(strings.ToLower(profile.Stack.Tool))
	if pLang == "" || pTool == "" {
		return false
	}
	if pLang != strings.TrimSpace(strings.ToLower(language)) {
		return false
	}
	if pTool != strings.TrimSpace(strings.ToLower(tool)) {
		return false
	}
	pRelease := strings.TrimSpace(profile.Stack.Release)
	if pRelease == "" {
		return true
	}
	if strings.TrimSpace(release) == "" {
		return false
	}
	return pRelease == strings.TrimSpace(release)
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
	if override == nil {
		return phase, nil, nil
	}
	override.Stack = &PrepProfileStack{
		Language: profile.Stack.Language,
		Tool:     profile.Stack.Tool,
		Release:  profile.Stack.Release,
	}
	runtimeEnv, err := prepProfileRuntimeGateEnv(profile.Runtime)
	if err != nil {
		return "", nil, err
	}
	override.Env = mergePrepEnvs(override.Env, runtimeEnv)
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

func validatePrepProfileRuntime(runtime *PrepProfileRuntime) error {
	if runtime == nil || runtime.Docker == nil {
		return nil
	}

	mode := strings.TrimSpace(runtime.Docker.Mode)
	if mode == "" {
		return fmt.Errorf("prep_profile.runtime.docker.mode: required")
	}
	host := strings.TrimSpace(runtime.Docker.Host)
	switch mode {
	case PrepDockerModeNone, PrepDockerModeHostSocket:
		if host != "" {
			return fmt.Errorf("prep_profile.runtime.docker.host: forbidden for mode %q", mode)
		}
	case PrepDockerModeTCP:
		if host == "" {
			return fmt.Errorf("prep_profile.runtime.docker.host: required for mode %q", PrepDockerModeTCP)
		}
	default:
		return fmt.Errorf("prep_profile.runtime.docker.mode: invalid value %q", runtime.Docker.Mode)
	}
	if runtime.Docker.APIVersion != "" && strings.TrimSpace(runtime.Docker.APIVersion) == "" {
		return fmt.Errorf("prep_profile.runtime.docker.api_version: must not be blank")
	}
	return nil
}

func prepProfileRuntimeGateEnv(runtime *PrepProfileRuntime) (map[string]string, error) {
	if runtime == nil || runtime.Docker == nil {
		return nil, nil
	}

	mode := strings.TrimSpace(runtime.Docker.Mode)
	apiVersion := strings.TrimSpace(runtime.Docker.APIVersion)
	switch mode {
	case PrepDockerModeNone:
		if apiVersion == "" {
			return nil, nil
		}
		return map[string]string{PrepDockerAPIVersionEnv: apiVersion}, nil
	case PrepDockerModeHostSocket:
		env := map[string]string{
			PrepDockerHostEnv: defaultPrepDockerHostSocket,
		}
		if apiVersion != "" {
			env[PrepDockerAPIVersionEnv] = apiVersion
		}
		return env, nil
	case PrepDockerModeTCP:
		host := strings.TrimSpace(runtime.Docker.Host)
		if host == "" {
			return nil, fmt.Errorf("prep_profile.runtime.docker.host: required for mode %q", PrepDockerModeTCP)
		}
		env := map[string]string{
			PrepDockerHostEnv: host,
		}
		if apiVersion != "" {
			env[PrepDockerAPIVersionEnv] = apiVersion
		}
		return env, nil
	default:
		return nil, fmt.Errorf("prep_profile.runtime.docker.mode: invalid value %q", runtime.Docker.Mode)
	}
}

func mergePrepEnvs(base map[string]string, override map[string]string) map[string]string {
	if len(base) == 0 && len(override) == 0 {
		return nil
	}
	out := copyPrepProfileEnv(base)
	if out == nil {
		out = map[string]string{}
	}
	for k, v := range override {
		out[k] = v
	}
	return out
}

func IsSupportedPrepProfileArtifactSchema(schema string) bool {
	return strings.TrimSpace(schema) == PrepProfileCandidateSchemaID
}

func ValidatePrepProfileArtifactContract(path, schema, prefix string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("%s.path: required", prefix)
	}
	if strings.TrimSpace(schema) == "" {
		return fmt.Errorf("%s.schema: required", prefix)
	}
	if schema == PrepProfileCandidateSchemaID && path != PrepProfileCandidateArtifactPath {
		return fmt.Errorf("%s.path: must be %q when schema=%q", prefix, PrepProfileCandidateArtifactPath, PrepProfileCandidateSchemaID)
	}
	if schema != PrepProfileCandidateSchemaID {
		return fmt.Errorf("%s.schema: unsupported value %q", prefix, schema)
	}
	return nil
}
