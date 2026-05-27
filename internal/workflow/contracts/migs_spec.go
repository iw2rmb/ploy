// Package contracts defines shared workflow types.
//
// migs_spec.go provides the core typed model for Mig run specifications.
// This eliminates drift between CLI/server/nodeagent spec parsing by providing
// a single source of truth for spec structure.
//
// ## Canonical Spec Shape
//
// The MigSpec type supports a single canonical shape:
//   - All runs use steps[] (even single-step runs).
//   - Global build gate policy lives under build_gate.
//
// ## Related Files
//
// The Mig spec implementation is split across several files:
//   - migs_spec.go: Core types (MigSpec, MigStep) and validation
//   - command_spec.go: Polymorphic command handling (CommandSpec)
//   - build_gate_config.go: Build gate configuration types
//   - migs_spec_parse.go: JSON parsing functions
//
// ## Usage
//
// Parse specs using ParseMigSpecJSON (in migs_spec_parse.go):
//
//	spec, err := contracts.ParseMigSpecJSON(jsonBytes)
//	if err != nil {
//	    return err // structured validation error
//	}
//	// Use typed fields: spec.Steps, spec.BuildGate, etc.
package contracts

import (
	"fmt"
	"strings"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

// MigSpec is the canonical typed representation of a mig run specification.
// All specs use steps[]; multi-step runs have len(steps) > 1.
//
// Wire compatibility: This struct marshals to/from JSON with stable field names
// that match the existing spec schema. The JSON tags are the source of truth
// for wire format compatibility.
//
// Validation: Use Validate() to check structural correctness after parsing.
// ParseMigSpecJSON calls Validate() automatically and returns structured
// errors for invalid input.
type MigSpec struct {
	// --- Server-injected metadata (claim-time) ---
	//
	// These fields may be injected by the server when a job is claimed. They are
	// not required (and typically not present) in CLI-submitted specs.

	// JobID is the claimed job ID injected into the spec at claim time.
	JobID types.JobID `json:"job_id,omitempty" yaml:"job_id,omitempty"`

	// APIVersion is an optional schema version identifier (e.g., "ploy.mig/v1alpha1").
	// Informational only; the control plane forwards specs as opaque JSON.
	APIVersion string `json:"apiVersion,omitempty" yaml:"apiVersion,omitempty"`

	// Kind is an optional schema kind identifier (e.g., "MigRunSpec").
	// Informational only; the control plane forwards specs as opaque JSON.
	Kind string `json:"kind,omitempty" yaml:"kind,omitempty"`

	// --- Steps (required) ---

	// Steps holds the ordered list of mig steps.
	// A spec must contain at least one step.
	Steps []MigStep `json:"steps,omitempty" yaml:"steps,omitempty"`

	// Envs holds environment variables applied to every step (step envs overrides on conflicts).
	Envs map[string]string `json:"envs,omitempty" yaml:"envs,omitempty"`

	// --- Shared configuration (applies to both single-step and multi-step) ---

	// BuildGate configures Build Gate validation policy.
	// Applies globally to all steps.
	BuildGate *BuildGateConfig `json:"build_gate,omitempty" yaml:"build_gate,omitempty"`

	// BundleMap maps content hashes used in In/Out/Home entries to their
	// spec bundle download identifiers (bundle IDs). Populated by the CLI
	// compiler during spec submission. The nodeagent uses this to resolve
	// shortHash → bundleID for resource download during materialization.
	BundleMap map[string]string `json:"bundle_map,omitempty" yaml:"bundle_map,omitempty"`
}

// MigStep describes a single mig step in a run (steps[] array).
// Each step has its own image, command, and environment configuration.
// Steps execute sequentially with shared workspace, each running gate+mig.
type MigStep struct {
	// Name is an optional human-readable name for this step.
	Name string `json:"name,omitempty" yaml:"name,omitempty"`

	// Image is the container image for this step (required).
	// Supports both universal images (string) and stack-specific images (map).
	Image JobImage `json:"image,omitempty" yaml:"image,omitempty"`

	// Command is the container command override for this step (optional).
	// Can be a shell string or an exec array.
	Command CommandSpec `json:"command,omitempty,omitzero" yaml:"command,omitempty"`

	// Envs holds environment variables specific to this step.
	Envs map[string]string `json:"envs,omitempty" yaml:"envs,omitempty"`

	// In lists canonical read-only input entries ("shortHash:/in/dst").
	In []string `json:"in,omitempty" yaml:"in,omitempty"`

	// InFrom lists cross-step /out → /in references in canonical form.
	// Source selector forms:
	//   <type>://out/<path>
	//   <name>@<type>://out/<path>
	// Legacy compatibility alias:
	//   <step-name>://out/<path> (equivalent to <step-name>@mig://...)
	// and targets a destination under /in.
	InFrom []InFromRef `json:"in_from,omitempty" yaml:"in_from,omitempty"`

	// Out lists canonical read-write output entries ("shortHash:/out/dst").
	Out []string `json:"out,omitempty" yaml:"out,omitempty"`

	// Home lists canonical home-relative entries ("shortHash:dst{:ro}").
	Home []string `json:"home,omitempty" yaml:"home,omitempty"`

	// Stack configures Stack Gate validation for this step.
	// Inbound validates pre-mig expectations; Outbound validates post-mig expectations.
	Stack *StackGateSpec `json:"stack,omitempty" yaml:"stack,omitempty"`
}

// Validate checks that the spec is structurally valid.
// Returns nil if valid, or a descriptive error for invalid specs.
//
// Validation rules:
//   - steps must be non-empty and each step must have a non-empty image.
//   - Stack Gate phases must not be disabled with expectations set.
func (s MigSpec) Validate() error {
	// Validate steps.
	if len(s.Steps) == 0 {
		return fmt.Errorf("steps: required")
	}
	for i, mig := range s.Steps {
		if mig.Image.IsEmpty() {
			return fmt.Errorf("steps[%d].image: required", i)
		}
		// Validate Stack Gate configuration.
		if mig.Stack != nil {
			if err := validateStackGateSpec(mig.Stack, fmt.Sprintf("steps[%d].stack", i)); err != nil {
				return err
			}
		}
		if err := validateHydraFields(mig.In, mig.Out, mig.Home, fmt.Sprintf("steps[%d]", i)); err != nil {
			return err
		}
	}
	if err := validateInFromReferences(s.Steps); err != nil {
		return err
	}

	// Validate build gate images.
	if s.BuildGate != nil && len(s.BuildGate.Images) > 0 {
		mapping := BuildGateImageMapping{Images: s.BuildGate.Images}
		if err := mapping.Validate("build_gate.images"); err != nil {
			return err
		}
	}

	// Validate build gate stack configuration (pre/post).
	if s.BuildGate != nil {
		for _, pair := range []struct {
			phase  *BuildGatePhaseConfig
			prefix string
		}{
			{s.BuildGate.Pre, "build_gate.pre"},
			{s.BuildGate.Post, "build_gate.post"},
		} {
			if pair.phase == nil {
				continue
			}
			if err := validateBuildGateStackConfig(pair.phase.Stack, pair.prefix+".stack"); err != nil {
				return err
			}
		}
	}

	return nil
}

func validateBuildGateStackConfig(stack *BuildGateStackConfig, prefix string) error {
	if stack == nil {
		return nil
	}

	mode := BuildGateStackMode(strings.TrimSpace(string(stack.Mode)))
	if mode == "" {
		if strings.TrimSpace(stack.Language) != "" || strings.TrimSpace(stack.Tool) != "" || strings.TrimSpace(stack.Release) != "" {
			return fmt.Errorf("%s.mode: required when language, tool, or release is set", prefix)
		}
		return nil
	}

	switch mode {
	case BuildGateStackModeForced, BuildGateStackModeStrict, BuildGateStackModeFallback:
	default:
		return fmt.Errorf("%s.mode: must be one of forced, strict, fallback", prefix)
	}

	if strings.TrimSpace(stack.Language) == "" {
		return fmt.Errorf("%s.language: required", prefix)
	}
	if strings.TrimSpace(stack.Tool) == "" {
		return fmt.Errorf("%s.tool: required", prefix)
	}
	if strings.TrimSpace(stack.Release) == "" {
		return fmt.Errorf("%s.release: required", prefix)
	}
	return nil
}

func validateStackGateSpec(spec *StackGateSpec, prefix string) error {
	if spec == nil {
		return nil
	}
	if spec.Inbound != nil {
		if err := validateStackGatePhaseSpec(spec.Inbound, prefix+".inbound"); err != nil {
			return err
		}
	}
	if spec.Outbound != nil {
		if err := validateStackGatePhaseSpec(spec.Outbound, prefix+".outbound"); err != nil {
			return err
		}
	}
	return nil
}

// validateStackGatePhaseSpec validates a single phase for ambiguous configuration.
// Rejects:
//   - enabled:false with expect:{...} as contradictory (why set expectations if disabled?).
//   - enabled:true without expect as incomplete (enabled without expectations is meaningless).
func validateStackGatePhaseSpec(phase *StackGatePhaseSpec, prefix string) error {
	if phase == nil {
		return nil
	}
	// Reject enabled:false with non-empty expect as ambiguous.
	if !phase.Enabled && phase.Expect != nil && !phase.Expect.IsEmpty() {
		return fmt.Errorf("%s: enabled=false with expect is ambiguous; remove expect or set enabled=true", prefix)
	}
	// Reject enabled:true without expect as incomplete.
	if phase.Enabled && (phase.Expect == nil || phase.Expect.IsEmpty()) {
		return fmt.Errorf("%s: enabled=true requires expect; add expect or set enabled=false", prefix)
	}
	return nil
}
