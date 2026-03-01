// build_gate_config.go defines Build Gate validation and healing configuration types.
//
// These types configure how Build Gate validation runs before/after mig execution
// and how healing operates when the gate fails.
package contracts

import "encoding/json"

// BuildGateConfig configures Build Gate validation for a Mods run.
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
}

// BuildGateProfileOverride configures a gate_profile-derived command/env override.
//
// Command is required when this object is present. Env is optional and merged
// into gate environment (override wins on key conflicts).
type BuildGateProfileOverride struct {
	Command CommandSpec       `json:"command,omitempty" yaml:"command,omitempty"`
	Env     map[string]string `json:"env,omitempty" yaml:"env,omitempty"`
	Stack   *GateProfileStack `json:"stack,omitempty" yaml:"stack,omitempty"`
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
	// (infra|code|mixed|unknown).
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
}
