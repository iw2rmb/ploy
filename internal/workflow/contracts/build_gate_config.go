// build_gate_config.go defines Build Gate validation and healing configuration types.
//
// These types configure how Build Gate validation runs before/after mod execution
// and how healing operates when the gate fails.
package contracts

// BuildGateConfig configures Build Gate validation for a Mods run.
type BuildGateConfig struct {
	// Enabled controls whether the build gate runs before/after mod execution.
	Enabled bool `json:"enabled,omitempty" yaml:"enabled,omitempty"`

	// Pre configures stack detection policy for the pre-gate phase.
	Pre *BuildGatePhaseConfig `json:"pre,omitempty" yaml:"pre,omitempty"`

	// Post configures stack detection policy for the post-gate (and re-gate) phase.
	Post *BuildGatePhaseConfig `json:"post,omitempty" yaml:"post,omitempty"`

	// Healing configures the heal → re-gate loop when Build Gate fails.
	// This is nested under build_gate to keep gate policy in one place.
	Healing *HealingSpec `json:"healing,omitempty" yaml:"healing,omitempty"`

	// Router configures the router container that runs on gate failure
	// to produce a bug_summary before healing begins.
	Router *RouterSpec `json:"router,omitempty" yaml:"router,omitempty"`

	// Images provides mod-level image mapping overrides for Build Gate image resolution.
	// These rules override the default mapping file.
	Images []BuildGateImageRule `json:"images,omitempty" yaml:"images,omitempty"`
}

// BuildGatePhaseConfig configures a single phase of Build Gate execution.
// Currently this holds optional stack detection configuration.
type BuildGatePhaseConfig struct {
	// Stack configures stack detection behavior for this gate phase.
	Stack *BuildGateStackConfig `json:"stack,omitempty" yaml:"stack,omitempty"`
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

// HealingSpec describes the heal → re-gate loop configuration.
// When the build gate fails, the agent can execute a healing mod then re-run the gate.
//
// HealingSpec is itself mod-like: it carries Image, Command, Env, and RetainContainer
// directly (no nested Mod object). Retries controls the healing retry count.
type HealingSpec struct {
	// Retries is the maximum number of healing attempts (default: 1).
	// Each retry executes the healing mod, then re-runs the gate.
	Retries int `json:"retries,omitempty" yaml:"retries,omitempty"`

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

// RouterSpec describes the router container that runs on gate failure to produce
// a bug_summary before healing begins. Router is mod-like (Image, Command, Env,
// RetainContainer) but has no Retries — it runs exactly once per gate failure.
type RouterSpec struct {
	// Image is the container image for the router (required).
	Image ModImage `json:"image,omitempty" yaml:"image,omitempty"`

	// Command is the container command override (optional).
	Command CommandSpec `json:"command,omitempty" yaml:"command,omitempty"`

	// Env holds environment variables to inject into the router container.
	Env map[string]string `json:"env,omitempty" yaml:"env,omitempty"`

	// RetainContainer controls whether the router container is retained.
	RetainContainer bool `json:"retain_container,omitempty" yaml:"retain_container,omitempty"`
}
