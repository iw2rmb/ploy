// build_gate_config.go defines Build Gate validation configuration types.
//
// These types configure how Build Gate validation runs before/after mig execution.
package contracts

import (
	"encoding/json"
)

// BuildGateConfig configures Build Gate validation for a mig run.
type BuildGateConfig struct {
	// Enabled controls whether the build gate runs before/after mig execution.
	Enabled bool `json:"enabled,omitempty" yaml:"enabled,omitempty"`

	// Pre configures stack detection policy for the pre-gate phase.
	Pre *BuildGatePhaseConfig `json:"pre,omitempty" yaml:"pre,omitempty"`

	// Post configures stack detection policy for the post-gate phase.
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
}

// BuildGateStackMode controls how a phase stack config interacts with detection.
type BuildGateStackMode string

const (
	BuildGateStackModeForced   BuildGateStackMode = "forced"
	BuildGateStackModeStrict   BuildGateStackMode = "strict"
	BuildGateStackModeFallback BuildGateStackMode = "fallback"
)

// BuildGateStackConfig configures expected stack information for a gate phase.
type BuildGateStackConfig struct {
	Mode     BuildGateStackMode `json:"mode,omitempty" yaml:"mode,omitempty"`
	Language string             `json:"language,omitempty" yaml:"language,omitempty"`
	Tool     string             `json:"tool,omitempty" yaml:"tool,omitempty"`
	Release  string             `json:"release,omitempty" yaml:"release,omitempty"`
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

// ApplyBuildGatePhaseToGateSpec copies the gate execution fields from a
// BuildGatePhaseConfig into the corresponding fields of a StepGateSpec.
// StackDetect is set only when phase.Stack has a non-empty mode.
func ApplyBuildGatePhaseToGateSpec(spec *StepGateSpec, phase *BuildGatePhaseConfig) {
	if spec == nil || phase == nil {
		return
	}
	if phase.Stack != nil && phase.Stack.Mode != "" {
		spec.StackDetect = phase.Stack
	}
}
