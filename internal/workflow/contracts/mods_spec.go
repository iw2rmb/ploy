// Package contracts defines shared workflow types.
//
// mods_spec.go provides a canonical typed model for Mods run specifications.
// This eliminates drift between CLI/server/nodeagent spec parsing by providing
// a single source of truth for spec structure, validation, and serialization.
//
// ## Canonical Spec Shape
//
// The ModsSpec type supports a single canonical shape:
//   - All runs use steps[] (even single-step runs).
//   - Global build gate policy lives under build_gate (including healing).
//
// The parser validates that specs conform to this shape and rejects malformed
// input with actionable error messages.
//
// ## Usage
//
// Parse specs using ParseModsSpecJSON:
//
//	spec, err := contracts.ParseModsSpecJSON(jsonBytes)
//	if err != nil {
//	    return err // structured validation error
//	}
//	// Use typed fields: spec.Steps, spec.BuildGate, etc.
//
// ## YAML Support
//
// YAML files are accepted at the CLI boundary by loading into map[string]any,
// marshaling to JSON, and validating via ParseModsSpecJSON. There is no
// separate YAML parser in this package; this design keeps validation
// centralized in a single parser and simplifies maintenance.
//
// ## Migration Path
//
// Existing code that uses map[string]any for spec parsing should migrate to
// these typed parsers. The typed spec provides:
//   - Compile-time type safety for field access
//   - Centralized validation with consistent error messages
//   - Stable round-trip serialization for wire compatibility
package contracts

import (
	"encoding/json"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// ModsSpec is the canonical typed representation of a Mods run specification.
// All specs use steps[]; multi-step runs have len(steps) > 1.
//
// Wire compatibility: This struct marshals to/from JSON with stable field names
// that match the existing spec schema. The JSON tags are the source of truth
// for wire format compatibility.
//
// Validation: Use Validate() to check structural correctness after parsing.
// ParseModsSpecJSON calls Validate() automatically and returns structured
// errors for invalid input.
type ModsSpec struct {
	// --- Server-injected metadata (claim-time) ---
	//
	// These fields may be injected by the server when a job is claimed. They are
	// not required (and typically not present) in CLI-submitted specs.

	// JobID is the claimed job ID injected into the spec at claim time.
	JobID string `json:"job_id,omitempty" yaml:"job_id,omitempty"`

	// APIVersion is an optional schema version identifier (e.g., "ploy.mod/v1alpha1").
	// Informational only; the control plane forwards specs as opaque JSON.
	APIVersion string `json:"apiVersion,omitempty" yaml:"apiVersion,omitempty"`

	// Kind is an optional schema kind identifier (e.g., "ModRunSpec").
	// Informational only; the control plane forwards specs as opaque JSON.
	Kind string `json:"kind,omitempty" yaml:"kind,omitempty"`

	// --- Steps (required) ---

	// Steps holds the ordered list of mod steps.
	// A spec must contain at least one step.
	Steps []ModStep `json:"steps,omitempty" yaml:"steps,omitempty"`

	// Env holds environment variables applied to every step (step env overrides on conflicts).
	Env map[string]string `json:"env,omitempty" yaml:"env,omitempty"`

	// --- Shared configuration (applies to both single-step and multi-step) ---

	// BuildGate configures Build Gate validation and healing policy.
	// Applies globally to all steps.
	BuildGate *BuildGateConfig `json:"build_gate,omitempty" yaml:"build_gate,omitempty"`

	// --- GitLab MR integration ---

	// GitLabPAT is the Personal Access Token for GitLab API authentication.
	// This value is never logged and is only passed to the GitLab client.
	GitLabPAT string `json:"gitlab_pat,omitempty" yaml:"gitlab_pat,omitempty"`

	// GitLabDomain is the GitLab instance domain (e.g., "gitlab.com").
	// Defaults to "gitlab.com" when GitLabPAT is provided but domain is empty.
	GitLabDomain string `json:"gitlab_domain,omitempty" yaml:"gitlab_domain,omitempty"`

	// MROnSuccess controls whether to create an MR when the run succeeds.
	// Pointer form preserves presence (absent vs explicitly false).
	MROnSuccess *bool `json:"mr_on_success,omitempty" yaml:"mr_on_success,omitempty"`

	// MROnFail controls whether to create an MR when the run fails.
	// Pointer form preserves presence (absent vs explicitly false).
	MROnFail *bool `json:"mr_on_fail,omitempty" yaml:"mr_on_fail,omitempty"`

	// --- Artifact configuration ---

	// ArtifactPaths lists workspace-relative paths to upload as artifacts.
	ArtifactPaths []string `json:"artifact_paths,omitempty" yaml:"artifact_paths,omitempty"`

	// ArtifactName is an optional custom name for the uploaded artifact bundle.
	ArtifactName string `json:"artifact_name,omitempty" yaml:"artifact_name,omitempty"`
}

// ModStep describes a single mod step in a run (steps[] array).
// Each step has its own image, command, and environment configuration.
// Steps execute sequentially with shared workspace, each running gate+mod.
type ModStep struct {
	// Name is an optional human-readable name for this step.
	Name string `json:"name,omitempty" yaml:"name,omitempty"`

	// Image is the container image for this step (required).
	// Supports both universal images (string) and stack-specific images (map).
	Image ModImage `json:"image,omitempty" yaml:"image,omitempty"`

	// Command is the container command override for this step (optional).
	// Can be a shell string or an exec array.
	Command CommandSpec `json:"command,omitempty" yaml:"command,omitempty"`

	// Env holds environment variables specific to this step.
	Env map[string]string `json:"env,omitempty" yaml:"env,omitempty"`

	// RetainContainer controls whether this step's container is retained.
	RetainContainer bool `json:"retain_container,omitempty" yaml:"retain_container,omitempty"`
}

// CommandSpec represents a container command as either a shell string or exec array.
// This type encapsulates the polymorphic command representation in mod specs.
//
// JSON/YAML Examples:
//
//	# Shell string form (executed via /bin/sh -c):
//	command: "echo hello && ls -la"
//
//	# Exec array form (executed directly):
//	command: ["/bin/sh", "-c", "echo hello"]
type CommandSpec struct {
	// Shell holds the command when specified as a single shell string.
	// When non-empty, the command is executed via ["/bin/sh", "-c", Shell].
	Shell string

	// Exec holds the command when specified as an exec array.
	// When non-nil, the command is executed directly without a shell wrapper.
	Exec []string
}

// IsEmpty returns true if no command is specified.
func (c CommandSpec) IsEmpty() bool {
	return c.Shell == "" && len(c.Exec) == 0
}

// ToSlice converts the command to a []string suitable for container execution.
// Returns nil if the command is empty.
//
// Conversion rules:
//   - Exec array: returned as-is
//   - Shell string: wrapped as ["/bin/sh", "-c", Shell]
//   - Empty: returns nil
func (c CommandSpec) ToSlice() []string {
	if len(c.Exec) > 0 {
		return c.Exec
	}
	if c.Shell != "" {
		return []string{"/bin/sh", "-c", c.Shell}
	}
	return nil
}

// MarshalJSON implements json.Marshaler for CommandSpec.
// Serializes as a string when Shell is set, or as an array when Exec is set.
func (c CommandSpec) MarshalJSON() ([]byte, error) {
	if len(c.Exec) > 0 {
		return json.Marshal(c.Exec)
	}
	if c.Shell != "" {
		return json.Marshal(c.Shell)
	}
	// Empty command serializes as null/omitted (via omitempty on parent).
	return json.Marshal(nil)
}

// UnmarshalJSON implements json.Unmarshaler for CommandSpec.
// Accepts both string and array forms from JSON.
func (c *CommandSpec) UnmarshalJSON(data []byte) error {
	// Handle null
	if string(data) == "null" {
		return nil
	}

	// Try string first (shell form).
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		c.Shell = strings.TrimSpace(s)
		return nil
	}

	// Try array (exec form).
	var arr []string
	if err := json.Unmarshal(data, &arr); err == nil {
		c.Exec = arr
		return nil
	}

	return fmt.Errorf("command: expected string or array, got %s", string(data))
}

// MarshalYAML implements yaml.Marshaler for CommandSpec.
func (c CommandSpec) MarshalYAML() (interface{}, error) {
	if len(c.Exec) > 0 {
		return c.Exec, nil
	}
	if c.Shell != "" {
		return c.Shell, nil
	}
	return nil, nil
}

// UnmarshalYAML implements yaml.Unmarshaler for CommandSpec.
func (c *CommandSpec) UnmarshalYAML(node *yaml.Node) error {
	// Handle scalar (string form).
	if node.Kind == yaml.ScalarNode {
		c.Shell = strings.TrimSpace(node.Value)
		return nil
	}

	// Handle sequence (exec array form).
	if node.Kind == yaml.SequenceNode {
		var arr []string
		if err := node.Decode(&arr); err != nil {
			return fmt.Errorf("command array: %w", err)
		}
		c.Exec = arr
		return nil
	}

	return fmt.Errorf("command: expected string or array, got %s", node.Tag)
}

// BuildGateConfig configures Build Gate validation for a Mods run.
type BuildGateConfig struct {
	// Enabled controls whether the build gate runs before/after mod execution.
	Enabled bool `json:"enabled,omitempty" yaml:"enabled,omitempty"`

	// Profile specifies the gate profile name (e.g., "auto", "java-maven").
	// The profile determines which static analysis tools and checks to run.
	Profile string `json:"profile,omitempty" yaml:"profile,omitempty"`

	// Healing configures the heal → re-gate loop when Build Gate fails.
	// This is nested under build_gate to keep gate policy in one place.
	Healing *HealingSpec `json:"healing,omitempty" yaml:"healing,omitempty"`
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

// IsMultiStep returns true if this spec defines more than one step.
func (s ModsSpec) IsMultiStep() bool {
	return len(s.Steps) > 1
}

// IsSingleStep returns true if this spec defines exactly one step.
func (s ModsSpec) IsSingleStep() bool {
	return len(s.Steps) == 1
}

// Validate checks that the spec is structurally valid.
// Returns nil if valid, or a descriptive error for invalid specs.
//
// Validation rules:
//   - steps must be non-empty and each step must have a non-empty image.
//   - HealingSpec.Mod must have a non-empty image when present.
//   - Retries must be non-negative.
func (s ModsSpec) Validate() error {
	// Validate steps.
	if len(s.Steps) == 0 {
		return fmt.Errorf("steps: required")
	}
	for i, mod := range s.Steps {
		if mod.Image.IsEmpty() {
			return fmt.Errorf("steps[%d].image: required", i)
		}
	}

	// Validate healing spec.
	if s.BuildGate != nil && s.BuildGate.Healing != nil {
		if s.BuildGate.Healing.Retries < 0 {
			return fmt.Errorf("build_gate.healing.retries: must be non-negative, got %d",
				s.BuildGate.Healing.Retries)
		}
		if s.BuildGate.Healing.Mod != nil && s.BuildGate.Healing.Mod.Image.IsEmpty() {
			return fmt.Errorf("build_gate.healing.mod.image: required when healing mod is specified")
		}
	}

	return nil
}

// ParseModsSpecJSON parses a Mods specification from JSON bytes.
// Returns a validated ModsSpec or an error for invalid/malformed input.
//
// Errors are structured with field paths for actionable debugging:
//   - "steps[2].image: required" — missing required field in step
//   - "build_gate.healing.retries: must be non-negative" — invalid value
func ParseModsSpecJSON(data []byte) (*ModsSpec, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("steps: required")
	}

	// Unmarshal into intermediate map to handle polymorphic fields.
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse mods spec json: %w", err)
	}

	return parseModsSpecFromMap(raw)
}

// parseModsSpecFromMap converts a raw map to a typed ModsSpec.
// This shared implementation handles polymorphic field parsing (image, command)
// that requires special handling beyond standard JSON/YAML unmarshaling.
func parseModsSpecFromMap(raw map[string]any) (*ModsSpec, error) {
	spec := &ModsSpec{}

	// Parse server-injected fields.
	if v, ok := raw["job_id"]; ok && v != nil {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("job_id: expected string, got %T", v)
		}
		spec.JobID = strings.TrimSpace(s)
	}
	if _, ok := raw["mod_index"]; ok {
		return nil, fmt.Errorf("mod_index: forbidden (derived internally from step_index; must not be provided)")
	}

	// Parse optional metadata fields.
	if v, ok := raw["apiVersion"]; ok && v != nil {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("apiVersion: expected string, got %T", v)
		}
		spec.APIVersion = strings.TrimSpace(s)
	}
	if v, ok := raw["kind"]; ok && v != nil {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("kind: expected string, got %T", v)
		}
		spec.Kind = strings.TrimSpace(s)
	}

	// Parse top-level env.
	if v, ok := raw["env"]; ok && v != nil {
		env, err := parseEnvMap(v, "env")
		if err != nil {
			return nil, err
		}
		spec.Env = env
	}

	// Parse steps[] array (required).
	v, ok := raw["steps"]
	if !ok || v == nil {
		return nil, fmt.Errorf("steps: required")
	}
	stepsRaw, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("steps: expected array, got %T", v)
	}
	if len(stepsRaw) == 0 {
		return nil, fmt.Errorf("steps: required")
	}
	spec.Steps = make([]ModStep, 0, len(stepsRaw))
	for i, stepRaw := range stepsRaw {
		stepMap, ok := stepRaw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("steps[%d]: expected object, got %T", i, stepRaw)
		}
		step, err := parseModStep(stepMap, i)
		if err != nil {
			return nil, err
		}
		spec.Steps = append(spec.Steps, step)
	}

	// Parse build_gate.
	if v, ok := raw["build_gate"]; ok && v != nil {
		bgRaw, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("build_gate: expected object, got %T", v)
		}
		bg := &BuildGateConfig{}
		if vv, ok := bgRaw["enabled"]; ok && vv != nil {
			b, ok := vv.(bool)
			if !ok {
				return nil, fmt.Errorf("build_gate.enabled: expected bool, got %T", vv)
			}
			bg.Enabled = b
		}
		if vv, ok := bgRaw["profile"]; ok && vv != nil {
			s, ok := vv.(string)
			if !ok {
				return nil, fmt.Errorf("build_gate.profile: expected string, got %T", vv)
			}
			bg.Profile = strings.TrimSpace(s)
		}
		if vv, ok := bgRaw["healing"]; ok && vv != nil {
			healRaw, ok := vv.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("build_gate.healing: expected object, got %T", vv)
			}
			heal, err := parseHealingSpec(healRaw, "build_gate.healing")
			if err != nil {
				return nil, err
			}
			bg.Healing = heal
		}
		spec.BuildGate = bg
	}

	// Parse GitLab integration.
	if v, ok := raw["gitlab_pat"]; ok && v != nil {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("gitlab_pat: expected string, got %T", v)
		}
		spec.GitLabPAT = s
	}
	if v, ok := raw["gitlab_domain"]; ok && v != nil {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("gitlab_domain: expected string, got %T", v)
		}
		spec.GitLabDomain = strings.TrimSpace(s)
	}
	if v, ok := raw["mr_on_success"]; ok && v != nil {
		b, ok := v.(bool)
		if !ok {
			return nil, fmt.Errorf("mr_on_success: expected bool, got %T", v)
		}
		spec.MROnSuccess = &b
	}
	if v, ok := raw["mr_on_fail"]; ok && v != nil {
		b, ok := v.(bool)
		if !ok {
			return nil, fmt.Errorf("mr_on_fail: expected bool, got %T", v)
		}
		spec.MROnFail = &b
	}

	// Parse artifact configuration.
	if v, ok := raw["artifact_name"]; ok && v != nil {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("artifact_name: expected string, got %T", v)
		}
		spec.ArtifactName = strings.TrimSpace(s)
	}
	if pathsRaw, ok := raw["artifact_paths"]; ok && pathsRaw != nil {
		paths, err := parseStringSlice(pathsRaw, "artifact_paths")
		if err != nil {
			return nil, err
		}
		spec.ArtifactPaths = paths
	}

	// Normalize defaults.
	if strings.TrimSpace(spec.GitLabPAT) != "" && strings.TrimSpace(spec.GitLabDomain) == "" {
		spec.GitLabDomain = "gitlab.com"
	}

	// Validate the parsed spec.
	if err := spec.Validate(); err != nil {
		return nil, err
	}

	return spec, nil
}

// parseModStep parses a single mod step entry from the steps[] array.
func parseModStep(raw map[string]any, index int) (ModStep, error) {
	step := ModStep{}
	prefix := fmt.Sprintf("steps[%d]", index)

	// Parse optional name.
	if v, ok := raw["name"]; ok && v != nil {
		s, ok := v.(string)
		if !ok {
			return step, fmt.Errorf("%s.name: expected string, got %T", prefix, v)
		}
		step.Name = strings.TrimSpace(s)
	}

	// Parse image.
	if v, ok := raw["image"]; ok && v != nil {
		img, err := ParseModImage(v)
		if err != nil {
			return step, fmt.Errorf("%s.image: %w", prefix, err)
		}
		step.Image = img
	}

	// Parse command.
	if v, ok := raw["command"]; ok && v != nil {
		cmd, err := parseCommandSpec(v)
		if err != nil {
			return step, fmt.Errorf("%s.command: %w", prefix, err)
		}
		step.Command = cmd
	}

	// Parse env.
	if v, ok := raw["env"]; ok && v != nil {
		env, err := parseEnvMap(v, prefix+".env")
		if err != nil {
			return step, err
		}
		step.Env = env
	}

	// Parse retain_container.
	if v, ok := raw["retain_container"]; ok && v != nil {
		b, ok := v.(bool)
		if !ok {
			return step, fmt.Errorf("%s.retain_container: expected bool, got %T", prefix, v)
		}
		step.RetainContainer = b
	}

	return step, nil
}

func parseHealingSpec(raw map[string]any, prefix string) (*HealingSpec, error) {
	heal := &HealingSpec{
		Retries: 1, // Default to 1 retry.
	}

	// Parse retries (handle both int and float64 from JSON).
	if v, ok := raw["retries"]; ok && v != nil {
		if r, ok := v.(int); ok {
			heal.Retries = r
		} else if rf, ok := v.(float64); ok {
			heal.Retries = int(rf)
		} else {
			return nil, fmt.Errorf("%s.retries: expected number, got %T", prefix, v)
		}
	}

	// Parse healing mod.
	if v, ok := raw["mod"]; ok && v != nil {
		modRaw, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s.mod: expected object, got %T", prefix, v)
		}
		mod, err := parseHealingModSpec(modRaw, prefix+".mod")
		if err != nil {
			return nil, err
		}
		heal.Mod = mod
	}

	return heal, nil
}

func parseHealingModSpec(raw map[string]any, prefix string) (*HealingModSpec, error) {
	mod := &HealingModSpec{}

	// Parse image.
	if v, ok := raw["image"]; ok && v != nil {
		img, err := ParseModImage(v)
		if err != nil {
			return nil, fmt.Errorf("%s.image: %w", prefix, err)
		}
		mod.Image = img
	}

	// Parse command.
	if v, ok := raw["command"]; ok && v != nil {
		cmd, err := parseCommandSpec(v)
		if err != nil {
			return nil, fmt.Errorf("%s.command: %w", prefix, err)
		}
		mod.Command = cmd
	}

	// Parse env.
	if v, ok := raw["env"]; ok && v != nil {
		env, err := parseEnvMap(v, prefix+".env")
		if err != nil {
			return nil, err
		}
		mod.Env = env
	}

	// Parse retain_container.
	if v, ok := raw["retain_container"]; ok && v != nil {
		b, ok := v.(bool)
		if !ok {
			return nil, fmt.Errorf("%s.retain_container: expected bool, got %T", prefix, v)
		}
		mod.RetainContainer = b
	}

	return mod, nil
}

// parseCommandSpec parses a command from polymorphic input (string or array).
func parseCommandSpec(v any) (CommandSpec, error) {
	switch cmd := v.(type) {
	case string:
		return CommandSpec{Shell: strings.TrimSpace(cmd)}, nil
	case []any:
		exec := make([]string, 0, len(cmd))
		for _, elem := range cmd {
			s, ok := elem.(string)
			if !ok {
				return CommandSpec{}, fmt.Errorf("expected string array element, got %T", elem)
			}
			exec = append(exec, s)
		}
		return CommandSpec{Exec: exec}, nil
	case []string:
		return CommandSpec{Exec: cmd}, nil
	default:
		return CommandSpec{}, fmt.Errorf("expected string or array, got %T", v)
	}
}

// parseEnvMap parses an environment variable map from untyped input.
func parseEnvMap(v any, field string) (map[string]string, error) {
	switch e := v.(type) {
	case map[string]any:
		env := make(map[string]string, len(e))
		for k, val := range e {
			s, ok := val.(string)
			if !ok {
				return nil, fmt.Errorf("%s[%s]: expected string value, got %T", field, k, val)
			}
			env[k] = s
		}
		return env, nil
	case map[string]string:
		return e, nil
	default:
		return nil, fmt.Errorf("%s: expected object, got %T", field, v)
	}
}

// parseStringSlice parses a string slice from untyped input.
func parseStringSlice(v any, field string) ([]string, error) {
	switch s := v.(type) {
	case []any:
		result := make([]string, 0, len(s))
		for i, elem := range s {
			str, ok := elem.(string)
			if !ok {
				return nil, fmt.Errorf("%s[%d]: expected string, got %T", field, i, elem)
			}
			if trimmed := strings.TrimSpace(str); trimmed != "" {
				result = append(result, trimmed)
			}
		}
		return result, nil
	case []string:
		result := make([]string, 0, len(s))
		for _, str := range s {
			if trimmed := strings.TrimSpace(str); trimmed != "" {
				result = append(result, trimmed)
			}
		}
		return result, nil
	default:
		return nil, fmt.Errorf("%s: expected array, got %T", field, v)
	}
}

// ToMap converts the ModsSpec to a map[string]any for wire serialization.
// This is useful when the spec needs to be passed through systems that
// expect untyped map representations.
//
// The result can be serialized to JSON for control plane submission.
func (s ModsSpec) ToMap() map[string]any {
	result := make(map[string]any)

	// Server-injected metadata.
	if strings.TrimSpace(s.JobID) != "" {
		result["job_id"] = strings.TrimSpace(s.JobID)
	}

	// Metadata.
	if s.APIVersion != "" {
		result["apiVersion"] = s.APIVersion
	}
	if s.Kind != "" {
		result["kind"] = s.Kind
	}

	if len(s.Env) > 0 {
		result["env"] = s.Env
	}

	// Steps.
	if len(s.Steps) > 0 {
		steps := make([]map[string]any, 0, len(s.Steps))
		for _, step := range s.Steps {
			steps = append(steps, modStepToMap(step))
		}
		result["steps"] = steps
	}

	// Build gate.
	if s.BuildGate != nil {
		bg := make(map[string]any)
		if s.BuildGate.Enabled {
			bg["enabled"] = true
		}
		if s.BuildGate.Profile != "" {
			bg["profile"] = s.BuildGate.Profile
		}
		if s.BuildGate.Healing != nil {
			heal := make(map[string]any)
			if s.BuildGate.Healing.Retries > 0 {
				heal["retries"] = s.BuildGate.Healing.Retries
			}
			if s.BuildGate.Healing.Mod != nil {
				heal["mod"] = healingModToMap(s.BuildGate.Healing.Mod)
			}
			if len(heal) > 0 {
				bg["healing"] = heal
			}
		}
		if len(bg) > 0 {
			result["build_gate"] = bg
		}
	}

	// GitLab.
	if s.GitLabPAT != "" {
		result["gitlab_pat"] = s.GitLabPAT
	}
	if s.GitLabDomain != "" {
		result["gitlab_domain"] = s.GitLabDomain
	}
	if s.MROnSuccess != nil {
		result["mr_on_success"] = *s.MROnSuccess
	}
	if s.MROnFail != nil {
		result["mr_on_fail"] = *s.MROnFail
	}

	// Artifacts.
	if s.ArtifactName != "" {
		result["artifact_name"] = s.ArtifactName
	}
	if len(s.ArtifactPaths) > 0 {
		result["artifact_paths"] = s.ArtifactPaths
	}

	return result
}

// modImageToAny converts ModImage to its wire representation.
func modImageToAny(img ModImage) any {
	if img.Universal != "" {
		return img.Universal
	}
	if len(img.ByStack) > 0 {
		result := make(map[string]string, len(img.ByStack))
		for k, v := range img.ByStack {
			result[string(k)] = v
		}
		return result
	}
	return nil
}

// commandSpecToAny converts CommandSpec to its wire representation.
func commandSpecToAny(cmd CommandSpec) any {
	if len(cmd.Exec) > 0 {
		return cmd.Exec
	}
	if cmd.Shell != "" {
		return cmd.Shell
	}
	return nil
}

// modStepToMap converts ModStep to a map for wire serialization.
func modStepToMap(mod ModStep) map[string]any {
	result := make(map[string]any)
	if mod.Name != "" {
		result["name"] = mod.Name
	}
	if !mod.Image.IsEmpty() {
		result["image"] = modImageToAny(mod.Image)
	}
	if !mod.Command.IsEmpty() {
		result["command"] = commandSpecToAny(mod.Command)
	}
	if len(mod.Env) > 0 {
		result["env"] = mod.Env
	}
	if mod.RetainContainer {
		result["retain_container"] = true
	}
	return result
}

// healingModToMap converts HealingModSpec to a map for wire serialization.
func healingModToMap(mod *HealingModSpec) map[string]any {
	result := make(map[string]any)
	if !mod.Image.IsEmpty() {
		result["image"] = modImageToAny(mod.Image)
	}
	if !mod.Command.IsEmpty() {
		result["command"] = commandSpecToAny(mod.Command)
	}
	if len(mod.Env) > 0 {
		result["env"] = mod.Env
	}
	if mod.RetainContainer {
		result["retain_container"] = true
	}
	return result
}
