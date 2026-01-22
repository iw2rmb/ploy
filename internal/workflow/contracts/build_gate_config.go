// build_gate_config.go defines Build Gate validation and healing configuration types.
//
// These types configure how Build Gate validation runs before/after mod execution
// and how healing operates when the gate fails.
package contracts

// BuildGateConfig configures Build Gate validation for a Mods run.
type BuildGateConfig struct {
	// Enabled controls whether the build gate runs before/after mod execution.
	Enabled bool `json:"enabled,omitempty" yaml:"enabled,omitempty"`

	// Healing configures the heal → re-gate loop when Build Gate fails.
	// This is nested under build_gate to keep gate policy in one place.
	Healing *HealingSpec `json:"healing,omitempty" yaml:"healing,omitempty"`

	// Images provides mod-level image mapping overrides for Build Gate image resolution.
	// These rules override the default mapping file.
	Images []BuildGateImageRule `json:"images,omitempty" yaml:"images,omitempty"`
}

// HealingSpec describes the heal → re-gate loop configuration.
// When the build gate fails, the agent can execute a healing mod then re-run the gate.
type HealingSpec struct {
	// Retries is the maximum number of healing attempts (default: 1).
	// Each retry executes the healing mod, then re-runs the gate.
	Retries int `json:"retries,omitempty" yaml:"retries,omitempty"`

	// Mod is the single healing mod specification for this gate.
	// When the gate fails, this mod runs to attempt workspace fixes.
	Mod *HealingModSpec `json:"mod,omitempty" yaml:"mod,omitempty"`
}

// HealingModSpec describes a single healing mod container specification.
// Healing mods run after gate failure to attempt workspace fixes before re-gate.
type HealingModSpec struct {
	// Image is the container image for the healing mod (required).
	// Supports both universal images (string) and stack-specific images (map).
	Image ModImage `json:"image,omitempty" yaml:"image,omitempty"`

	// Command is the container command override (optional).
	Command CommandSpec `json:"command,omitempty" yaml:"command,omitempty"`

	// Env holds environment variables to inject into the healing container.
	Env map[string]string `json:"env,omitempty" yaml:"env,omitempty"`

	// RetainContainer controls whether the healing container is retained.
	RetainContainer bool `json:"retain_container,omitempty" yaml:"retain_container,omitempty"`
}
