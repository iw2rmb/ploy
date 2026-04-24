// build_gate_config.go defines Build Gate validation configuration types.
//
// These types configure how Build Gate validation runs before/after mig execution.
package contracts

import (
	"encoding/json"
	"strings"
)

// BuildGateConfig configures Build Gate validation for a mig run.
type BuildGateConfig struct {
	// Enabled controls whether the build gate runs before/after mig execution.
	Enabled bool `json:"enabled,omitempty" yaml:"enabled,omitempty"`

	// Pre configures stack detection policy for the pre-gate phase.
	Pre *BuildGatePhaseConfig `json:"pre,omitempty" yaml:"pre,omitempty"`

	// Post configures stack detection policy for the post-gate (and re-gate) phase.
	Post *BuildGatePhaseConfig `json:"post,omitempty" yaml:"post,omitempty"`

	// Images provides mig-level image mapping overrides for Build Gate image resolution.
	// These rules override the default mapping file.
	Images []BuildGateImageRule `json:"images,omitempty" yaml:"images,omitempty"`
}

// BuildGatePhaseConfig configures a single phase of Build Gate execution.
// This holds optional stack detection configuration and prep overrides.
type BuildGatePhaseConfig struct {
	// Stack configures stack detection behavior for this gate phase.
	Stack *BuildGateStackConfig `json:"stack,omitempty" yaml:"stack,omitempty"`

	// CA lists canonical CA certificate entries for this gate phase.
	CA []string `json:"ca,omitempty" yaml:"ca,omitempty"`

	// GateProfile configures gate_profile-derived command/env overrides for this phase.
	GateProfile *BuildGateProfileOverride `json:"gate_profile,omitempty" yaml:"gate_profile,omitempty"`

	// Target pins the gate target for this phase (build|unit|all_tests).
	Target string `json:"target,omitempty" yaml:"target,omitempty"`
}

// BuildGateProfileOverride configures a gate_profile-derived command/env override.
//
// Command is required when this object is present. Env is optional and merged
// into gate environment (override wins on key conflicts).
type BuildGateProfileOverride struct {
	Command CommandSpec       `json:"command,omitempty" yaml:"command,omitempty"`
	Env     map[string]string `json:"env,omitempty" yaml:"env,omitempty"`
	Stack   *GateProfileStack `json:"stack,omitempty" yaml:"stack,omitempty"`
	// Target is the source gate profile target (build|unit|all_tests).
	// This field is server-injected for repo/candidate profile overrides.
	Target string `json:"target,omitempty" yaml:"target,omitempty"`
}

// BuildGateStackConfig configures expected stack information for a gate phase.
//
// When Default is true, the gate can fall back to this configuration when stack
// detection cannot determine tool or version.
// When Default is false, stack detection failures cancel execution for the repo.
type BuildGateStackConfig struct {
	Enabled  bool   `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	Language string `json:"language,omitempty" yaml:"language,omitempty"`
	Tool     string `json:"tool,omitempty" yaml:"tool,omitempty"`
	Release  string `json:"release,omitempty" yaml:"release,omitempty"`
	Default  bool   `json:"default,omitempty" yaml:"default,omitempty"`
}

// UnmarshalJSON handles numeric release values (e.g., YAML `release: 11` → JSON number).
func (s *BuildGateStackConfig) UnmarshalJSON(data []byte) error {
	type alias BuildGateStackConfig
	aux := &struct {
		Release json.RawMessage `json:"release,omitempty"`
		*alias
	}{alias: (*alias)(s)}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	if len(aux.Release) > 0 {
		r, err := unmarshalReleaseJSON(aux.Release)
		if err != nil {
			return err
		}
		s.Release = r
	}
	return nil
}

// BuildGateProfileOverrideToSpecMap converts a BuildGateProfileOverride to the
// map[string]any wire format used for spec JSON injection.
// Returns nil when override is nil.
func BuildGateProfileOverrideToSpecMap(override *BuildGateProfileOverride) map[string]any {
	if override == nil {
		return nil
	}
	m := map[string]any{}
	if len(override.Command.Exec) > 0 {
		exec := make([]any, len(override.Command.Exec))
		for i, v := range override.Command.Exec {
			exec[i] = v
		}
		m["command"] = exec
	} else {
		m["command"] = override.Command.Shell
	}
	if len(override.Env) > 0 {
		env := make(map[string]any, len(override.Env))
		for k, v := range override.Env {
			env[k] = v
		}
		m["env"] = env
	}
	if override.Stack != nil {
		stack := map[string]any{
			"language": override.Stack.Language,
			"tool":     override.Stack.Tool,
		}
		if strings.TrimSpace(override.Stack.Release) != "" {
			stack["release"] = override.Stack.Release
		}
		m["stack"] = stack
	}
	if t := strings.TrimSpace(override.Target); t != "" {
		m["target"] = t
	}
	return m
}

// ApplyBuildGatePhaseToGateSpec copies the gate execution fields from a
// BuildGatePhaseConfig into the corresponding fields of a StepGateSpec.
// StackDetect is set only when phase.Stack is non-nil and enabled.
func ApplyBuildGatePhaseToGateSpec(spec *StepGateSpec, phase *BuildGatePhaseConfig) {
	if spec == nil || phase == nil {
		return
	}
	spec.GateProfile = phase.GateProfile
	spec.Target = phase.Target
	if phase.Stack != nil && phase.Stack.Enabled {
		spec.StackDetect = phase.Stack
	}
}
