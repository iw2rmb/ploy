package gateprofile

import (
	"fmt"
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
