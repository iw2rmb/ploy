// build_gate_config.go defines Build Gate validation and healing configuration types.
//
// These types configure how Build Gate validation runs before/after mig execution
// and how healing operates when the gate fails.
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

	// Healing configures the heal -> re-gate loop selector keyed by router
	// error_kind classification.
	Healing *HealingSpec `json:"healing,omitempty" yaml:"healing,omitempty"`

	// Router configures the router container that runs on gate failure
	// to produce a bug_summary before healing begins.
	Router *RouterSpec `json:"router,omitempty" yaml:"router,omitempty"`

	// Images provides mig-level image mapping overrides for Build Gate image resolution.
	// These rules override the default mapping file.
	Images []BuildGateImageRule `json:"images,omitempty" yaml:"images,omitempty"`
}

// BuildGatePhaseConfig configures a single phase of Build Gate execution.
// This holds optional stack detection configuration and prep overrides.
type BuildGatePhaseConfig struct {
	// Stack configures stack detection behavior for this gate phase.
	Stack *BuildGateStackConfig `json:"stack,omitempty" yaml:"stack,omitempty"`

	// GateProfile configures gate_profile-derived command/env overrides for this phase.
	GateProfile *BuildGateProfileOverride `json:"gate_profile,omitempty" yaml:"gate_profile,omitempty"`

	// Target pins the gate target for this phase (build|unit|all_tests).
	Target string `json:"target,omitempty" yaml:"target,omitempty"`

	// Always forces this phase to run even when an exact prior profile could skip it.
	Always bool `json:"always,omitempty" yaml:"always,omitempty"`
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

// HealingSpec describes recovery action selection keyed by router error_kind.
// The control plane selects the action for a failed gate and injects the
// selected kind as selected_error_kind for heal job claims.
type HealingSpec struct {
	// SelectedErrorKind is a server-injected selector for heal job claims.
	// User specs should not set this field.
	SelectedErrorKind string `json:"selected_error_kind,omitempty" yaml:"selected_error_kind,omitempty"`

	// ByErrorKind defines recovery actions keyed by router classification
	// (infra|code|deps|mixed|unknown).
	ByErrorKind map[string]HealingActionSpec `json:"by_error_kind,omitempty" yaml:"by_error_kind,omitempty"`
}

// HealingActionSpec describes a per-error_kind healing action.
type HealingActionSpec struct {
	// Retries is the maximum number of healing attempts (default: 1).
	// Each retry executes the healing action, then re-runs the gate.
	Retries int `json:"retries,omitempty" yaml:"retries,omitempty"`

	// Image is the container image for the healing action (required for
	// non-terminal kinds).
	Image JobImage `json:"image,omitempty" yaml:"image,omitempty"`

	// Command is the container command override (optional).
	Command CommandSpec `json:"command,omitempty" yaml:"command,omitempty"`

	// Env holds environment variables to inject into the healing container.
	Env map[string]string `json:"env,omitempty" yaml:"env,omitempty"`

	// Expectations defines typed strategy output contracts for downstream
	// validation/promotion boundaries.
	Expectations *RecoveryExpectationsSpec `json:"expectations,omitempty" yaml:"expectations,omitempty"`

	// TmpBundle references a pre-uploaded bundle to extract under /tmp in the healing container.
	TmpBundle *TmpBundleRef `json:"tmp_bundle,omitempty" yaml:"tmp_bundle,omitempty"`

	// LegacyTmpDir captures any legacy tmp_dir JSON payload for explicit rejection.
	LegacyTmpDir json.RawMessage `json:"tmp_dir,omitempty" yaml:"-"`

	// Amata configures amata-mode execution for this healing action container.
	// When non-nil, the container runs `amata run /in/amata.yaml` with optional
	// --set flags; CODEX_PROMPT is not required in this mode.
	// When nil, the container uses the direct codex exec path and CODEX_PROMPT is required.
	Amata *AmataRunSpec `json:"amata,omitempty" yaml:"amata,omitempty"`
}

// RecoveryExpectationsSpec defines structured expectations for recovery output.
type RecoveryExpectationsSpec struct {
	Artifacts []RecoveryExpectedArtifact `json:"artifacts,omitempty" yaml:"artifacts,omitempty"`
}

// RecoveryExpectedArtifact defines one expected artifact path/schema pair.
type RecoveryExpectedArtifact struct {
	Path   string `json:"path,omitempty" yaml:"path,omitempty"`
	Schema string `json:"schema,omitempty" yaml:"schema,omitempty"`
}

// RouterSpec describes the router container that runs on gate failure to produce
// a bug_summary before healing begins. Router is mig-like (Image, Command, Env)
// but has no Retries — it runs exactly once per gate failure.
type RouterSpec struct {
	// Image is the container image for the router (required).
	Image JobImage `json:"image,omitempty" yaml:"image,omitempty"`

	// Command is the container command override (optional).
	Command CommandSpec `json:"command,omitempty" yaml:"command,omitempty"`

	// Env holds environment variables to inject into the router container.
	Env map[string]string `json:"env,omitempty" yaml:"env,omitempty"`

	// TmpBundle references a pre-uploaded bundle to extract under /tmp in the router container.
	TmpBundle *TmpBundleRef `json:"tmp_bundle,omitempty" yaml:"tmp_bundle,omitempty"`

	// LegacyTmpDir captures any legacy tmp_dir JSON payload for explicit rejection.
	LegacyTmpDir json.RawMessage `json:"tmp_dir,omitempty" yaml:"-"`

	// Amata configures amata-mode execution for this router container.
	// When non-nil, the container runs `amata run /in/amata.yaml` with optional
	// --set flags; CODEX_PROMPT is not required in this mode.
	// When nil, the container uses the direct codex exec path and CODEX_PROMPT is required.
	Amata *AmataRunSpec `json:"amata,omitempty" yaml:"amata,omitempty"`
}

// AmataRunSpec describes an amata execution configuration for router or healing containers.
//
// Command mapping rules (deterministic):
//   - When Spec is non-empty: materialize Spec as /in/amata.yaml, then run
//     `amata run /in/amata.yaml` followed by ordered `--set '<param>=<value>'` flags
//     from Set. CODEX_PROMPT is not required in this mode.
//   - When AmataRunSpec is absent (nil pointer on RouterSpec or HealingActionSpec):
//     fall through to the existing direct `codex exec` path; CODEX_PROMPT is required.
type AmataRunSpec struct {
	// Spec is the amata spec content materialized as /in/amata.yaml before execution.
	// Required when AmataRunSpec is present (non-empty).
	Spec string `json:"spec" yaml:"spec"`

	// Set is an ordered list of --set parameters emitted as `--set '<param>=<value>'`
	// flags to `amata run`. Order is preserved for deterministic CLI materialization.
	Set []AmataSetParam `json:"set,omitempty" yaml:"set,omitempty"`
}

// AmataSetParam is a single --set parameter for amata run.
type AmataSetParam struct {
	// Param is the parameter name (required, non-empty).
	Param string `json:"param" yaml:"param"`

	// Value is the parameter value passed verbatim (may be empty string).
	Value string `json:"value" yaml:"value"`
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
	spec.Always = phase.Always
	if phase.Stack != nil && phase.Stack.Enabled {
		spec.StackDetect = phase.Stack
	}
}
