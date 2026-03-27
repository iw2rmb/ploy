package gateprofile

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

const defaultDockerHostSocket = "unix:///var/run/docker.sock"

// ProfilePrecedence represents the precedence tier of a resolved gate profile.
// Higher values take precedence over lower values.
type ProfilePrecedence int

const (
	ProfilePrecedenceDefault ProfilePrecedence = iota + 1 // stack-level default
	ProfilePrecedenceLatest                               // latest repo-level profile
	ProfilePrecedenceExact                                // sha-exact matched profile
)

// ProfileCandidate holds a resolved gate profile record at a given precedence tier.
type ProfileCandidate struct {
	ID         int64
	ObjectKey  string
	Precedence ProfilePrecedence
}

// SelectProfile applies default/exact/latest precedence rules and returns the
// highest-priority non-nil candidate.
//
// Priority (highest first): exact > latest > def.
// Returns nil if all inputs are nil.
func SelectProfile(exact, latest, def *ProfileCandidate) *ProfileCandidate {
	if exact != nil {
		return exact
	}
	if latest != nil {
		return latest
	}
	return def
}

// SelectProfileLazy resolves the highest-priority gate profile candidate using
// lazy fetching. fetchExact is called first; if it returns a non-nil candidate,
// fetchLatest and fetchDefault are not called. If fetchLatest returns a non-nil
// candidate, fetchDefault is not called.
//
// The precedence decision (exact > latest > default) is owned by this function.
// Returns (nil, nil) if all fetch functions return (nil, nil).
func SelectProfileLazy(
	fetchExact func() (*ProfileCandidate, error),
	fetchLatest func() (*ProfileCandidate, error),
	fetchDefault func() (*ProfileCandidate, error),
) (*ProfileCandidate, error) {
	exact, err := fetchExact()
	if err != nil {
		return nil, err
	}
	if exact != nil {
		return exact, nil
	}
	latest, err := fetchLatest()
	if err != nil {
		return nil, err
	}
	if latest != nil {
		return latest, nil
	}
	return fetchDefault()
}

// GateOverrideForJobType derives the gate phase and build gate override from a
// resolved gate profile for the given job type.
//
// Returns ("", nil, nil) for non-gate job types or when profile is nil.
// Returns (phase, nil, nil) when the active target is "unsupported".
func GateOverrideForJobType(
	profile *contracts.GateProfile,
	jobType types.JobType,
) (contracts.BuildGateProfilePhase, *contracts.BuildGateProfileOverride, error) {
	if profile == nil {
		return "", nil, nil
	}

	var phase contracts.BuildGateProfilePhase

	switch jobType {
	case types.JobTypePreGate:
		phase = contracts.BuildGateProfilePhasePre
	case types.JobTypePostGate, types.JobTypeReGate:
		phase = contracts.BuildGateProfilePhasePost
	default:
		return "", nil, nil
	}

	active := strings.TrimSpace(profile.Targets.Active)
	if active == contracts.GateProfileTargetUnsupported {
		return phase, nil, nil
	}
	target := profile.Targets.TargetByName(active)
	if target == nil {
		return "", nil, fmt.Errorf("gate_profile: missing active target %q for job_type %q", active, jobType)
	}

	override, err := targetToOverride(target)
	if err != nil {
		return "", nil, err
	}
	if override == nil {
		return phase, nil, nil
	}
	override.Target = active
	override.Stack = &contracts.GateProfileStack{
		Language: profile.Stack.Language,
		Tool:     profile.Stack.Tool,
		Release:  profile.Stack.Release,
	}
	runtimeEnv, err := runtimeGateEnv(profile.Runtime)
	if err != nil {
		return "", nil, err
	}
	override.Env = contracts.MergeEnv(override.Env, runtimeEnv)
	return phase, override, nil
}

// StackMatches reports whether the gate profile's stack matches the given
// language, tool, and release.
//
// Empty release acts as wildcard (always matches). Non-empty release requires
// an exact match with the profile's release field.
func StackMatches(profile *contracts.GateProfile, language, tool, release string) bool {
	if profile == nil {
		return false
	}
	pLang := strings.TrimSpace(profile.Stack.Language)
	pTool := strings.TrimSpace(profile.Stack.Tool)
	if pLang == "" || pTool == "" {
		return false
	}
	if !contracts.StackFieldsMatch(language, tool, "", pLang, pTool, "") {
		return false
	}
	if strings.TrimSpace(release) == "" {
		return true
	}
	pRelease := strings.TrimSpace(profile.Stack.Release)
	if pRelease == "" {
		return false
	}
	return pRelease == strings.TrimSpace(release)
}

func targetToOverride(target *contracts.GateProfileTarget) (*contracts.BuildGateProfileOverride, error) {
	if target == nil {
		return nil, nil
	}
	cmd := strings.TrimSpace(target.Command)
	if cmd == "" {
		return nil, nil
	}
	return &contracts.BuildGateProfileOverride{
		Command: contracts.CommandSpec{Shell: cmd},
		Env:     contracts.CopyEnv(target.Env),
	}, nil
}

// DeriveProfileSnapshotFromOverride builds a GateProfile snapshot JSON from a
// BuildGateProfileOverride as injected by the server into the gate job spec.
// The snapshot is stored locally by the nodeagent and later hydrated into
// healing jobs as gate_profile.json.
//
// Returns an error when the override is absent/empty, the command form is
// unsupported, the stack cannot be resolved, or the resulting profile fails
// schema validation.
func DeriveProfileSnapshotFromOverride(
	repoID string,
	override *contracts.BuildGateProfileOverride,
	target string,
	jobType types.JobType,
	meta *contracts.BuildGateStageMetadata,
) (json.RawMessage, error) {
	if override == nil || override.Command.IsEmpty() {
		return nil, fmt.Errorf("gate override unavailable")
	}
	command, ok := commandFromOverride(override.Command)
	if !ok {
		return nil, fmt.Errorf("unsupported command form")
	}
	stack := resolveSnapshotStack(override, meta)
	if strings.TrimSpace(stack.Language) == "" || strings.TrimSpace(stack.Tool) == "" {
		return nil, fmt.Errorf("stack metadata unavailable")
	}

	targetPassed := &contracts.GateProfileTarget{
		Status:  contracts.PrepTargetStatusPassed,
		Command: command,
		Env:     snapshotEnvCopy(override.Env),
	}
	targetNotAttempted := func() *contracts.GateProfileTarget {
		return &contracts.GateProfileTarget{
			Status: contracts.PrepTargetStatusNotAttempted,
			Env:    map[string]string{},
		}
	}
	profile := contracts.GateProfile{
		SchemaVersion: 1,
		RepoID:        strings.TrimSpace(repoID),
		RunnerMode:    contracts.PrepRunnerModeSimple,
		Stack:         stack,
		Targets: contracts.GateProfileTargets{
			Active:   resolveSnapshotTarget(target, override.Target),
			Build:    targetNotAttempted(),
			Unit:     targetNotAttempted(),
			AllTests: targetNotAttempted(),
		},
		Orchestration: contracts.GateProfileOrchestration{
			Pre:  []json.RawMessage{},
			Post: []json.RawMessage{},
		},
	}
	switch jobType {
	case types.JobTypePreGate, types.JobTypePostGate, types.JobTypeReGate:
		switch profile.Targets.Active {
		case contracts.GateProfileTargetBuild:
			profile.Targets.Build = targetPassed
		case contracts.GateProfileTargetUnit:
			profile.Targets.Unit = targetPassed
		default:
			profile.Targets.AllTests = targetPassed
		}
	default:
		return nil, fmt.Errorf("unsupported job type %q", jobType)
	}
	raw, err := json.Marshal(profile)
	if err != nil {
		return nil, err
	}
	if _, err := contracts.ParseGateProfileJSON(raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// resolveSnapshotTarget returns the normalized gate target for use in a profile
// snapshot. explicit takes priority over fallback; defaults to GateProfileTargetAllTests.
func resolveSnapshotTarget(explicit, fallback string) string {
	switch strings.TrimSpace(explicit) {
	case contracts.GateProfileTargetBuild, contracts.GateProfileTargetUnit, contracts.GateProfileTargetAllTests:
		return strings.TrimSpace(explicit)
	}
	switch strings.TrimSpace(fallback) {
	case contracts.GateProfileTargetBuild, contracts.GateProfileTargetUnit, contracts.GateProfileTargetAllTests:
		return strings.TrimSpace(fallback)
	}
	return contracts.GateProfileTargetAllTests
}

// resolveSnapshotStack derives the GateProfileStack for a snapshot.
// Precedence: override.Stack > DetectedStackExpectation from meta > ModStack name.
func resolveSnapshotStack(
	override *contracts.BuildGateProfileOverride,
	meta *contracts.BuildGateStageMetadata,
) contracts.GateProfileStack {
	if override != nil && override.Stack != nil {
		return contracts.GateProfileStack{
			Language: strings.TrimSpace(override.Stack.Language),
			Tool:     strings.TrimSpace(override.Stack.Tool),
			Release:  strings.TrimSpace(override.Stack.Release),
		}
	}
	if meta != nil {
		if detected := meta.DetectedStackExpectation(); detected != nil {
			return contracts.GateProfileStack{
				Language: strings.TrimSpace(detected.Language),
				Tool:     strings.TrimSpace(detected.Tool),
				Release:  strings.TrimSpace(detected.Release),
			}
		}
	}
	stack := contracts.ModStackUnknown
	if meta != nil {
		stack = meta.DetectedStack()
	}
	switch stack {
	case contracts.ModStackJavaMaven:
		return contracts.GateProfileStack{Language: "java", Tool: "maven"}
	case contracts.ModStackJavaGradle:
		return contracts.GateProfileStack{Language: "java", Tool: "gradle"}
	case contracts.ModStackJava:
		return contracts.GateProfileStack{Language: "java", Tool: "java"}
	default:
		return contracts.GateProfileStack{}
	}
}

// commandFromOverride extracts a single command string from a CommandSpec.
// Shell form is returned as-is; exec form is joined with quoting for special chars.
func commandFromOverride(command contracts.CommandSpec) (string, bool) {
	if shell := strings.TrimSpace(command.Shell); shell != "" {
		return shell, true
	}
	if len(command.Exec) == 0 {
		return "", false
	}
	parts := make([]string, 0, len(command.Exec))
	for _, part := range command.Exec {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		if strings.ContainsAny(trimmed, " \t\n\r\"'\\$`") {
			parts = append(parts, strconv.Quote(trimmed))
			continue
		}
		parts = append(parts, trimmed)
	}
	if len(parts) == 0 {
		return "", false
	}
	return strings.Join(parts, " "), true
}

// snapshotEnvCopy returns a shallow copy of env for use in GateProfileTarget.Env.
// Returns an empty map (not nil) when input is nil/empty to ensure valid JSON output.
func snapshotEnvCopy(env map[string]string) map[string]string {
	if len(env) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(env))
	for k, v := range env {
		out[k] = v
	}
	return out
}

func runtimeGateEnv(runtime *contracts.GateProfileRuntime) (map[string]string, error) {
	if runtime == nil || runtime.Docker == nil {
		return nil, nil
	}

	mode := strings.TrimSpace(runtime.Docker.Mode)
	apiVersion := strings.TrimSpace(runtime.Docker.APIVersion)
	switch mode {
	case contracts.GateProfileDockerModeNone:
		if apiVersion == "" {
			return nil, nil
		}
		return map[string]string{contracts.GateProfileDockerAPIVersionEnv: apiVersion}, nil
	case contracts.GateProfileDockerModeHostSocket:
		env := map[string]string{
			contracts.GateProfileDockerHostEnv: defaultDockerHostSocket,
		}
		if apiVersion != "" {
			env[contracts.GateProfileDockerAPIVersionEnv] = apiVersion
		}
		return env, nil
	case contracts.GateProfileDockerModeTCP:
		host := strings.TrimSpace(runtime.Docker.Host)
		if host == "" {
			return nil, fmt.Errorf("gate_profile.runtime.docker.host: required for mode %q", contracts.GateProfileDockerModeTCP)
		}
		env := map[string]string{
			contracts.GateProfileDockerHostEnv: host,
		}
		if apiVersion != "" {
			env[contracts.GateProfileDockerAPIVersionEnv] = apiVersion
		}
		return env, nil
	default:
		return nil, fmt.Errorf("gate_profile.runtime.docker.mode: invalid value %q", runtime.Docker.Mode)
	}
}
