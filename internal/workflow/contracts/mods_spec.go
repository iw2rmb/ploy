// Package contracts defines shared workflow types.
//
// mods_spec.go provides the core typed model for Mods run specifications.
// This eliminates drift between CLI/server/nodeagent spec parsing by providing
// a single source of truth for spec structure.
//
// ## Canonical Spec Shape
//
// The ModsSpec type supports a single canonical shape:
//   - All runs use steps[] (even single-step runs).
//   - Global build gate policy lives under build_gate (including healing).
//
// ## Related Files
//
// The Mods spec implementation is split across several files:
//   - mods_spec.go: Core types (ModsSpec, ModStep) and validation
//   - command_spec.go: Polymorphic command handling (CommandSpec)
//   - build_gate_config.go: Build gate and healing configuration types
//   - mods_spec_parse.go: JSON/map parsing functions
//   - mods_spec_wire.go: Wire serialization (ToMap)
//
// ## Usage
//
// Parse specs using ParseModsSpecJSON (in mods_spec_parse.go):
//
//	spec, err := contracts.ParseModsSpecJSON(jsonBytes)
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
	JobID types.JobID `json:"job_id,omitempty" yaml:"job_id,omitempty"`

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
	Image JobImage `json:"image,omitempty" yaml:"image,omitempty"`

	// Command is the container command override for this step (optional).
	// Can be a shell string or an exec array.
	Command CommandSpec `json:"command,omitempty" yaml:"command,omitempty"`

	// Env holds environment variables specific to this step.
	Env map[string]string `json:"env,omitempty" yaml:"env,omitempty"`

	// RetainContainer controls whether this step's container is retained.
	RetainContainer bool `json:"retain_container,omitempty" yaml:"retain_container,omitempty"`

	// Stack configures Stack Gate validation for this step.
	// Inbound validates pre-mod expectations; Outbound validates post-mod expectations.
	Stack *StackGateSpec `json:"stack,omitempty" yaml:"stack,omitempty"`
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
//   - build_gate.healing.image must be non-empty when healing is configured.
//   - build_gate.router must be configured when healing is configured.
//   - Retries must be non-negative.
//   - Stack Gate phases must not be disabled with expectations set.
func (s ModsSpec) Validate() error {
	// Validate steps.
	if len(s.Steps) == 0 {
		return fmt.Errorf("steps: required")
	}
	for i, mod := range s.Steps {
		if mod.Image.IsEmpty() {
			return fmt.Errorf("steps[%d].image: required", i)
		}
		// Validate Stack Gate configuration.
		if mod.Stack != nil {
			if err := validateStackGateSpec(mod.Stack, fmt.Sprintf("steps[%d].stack", i)); err != nil {
				return err
			}
		}
	}

	// Validate healing spec.
	if s.BuildGate != nil && s.BuildGate.Healing != nil {
		if s.BuildGate.Healing.Retries < 0 {
			return fmt.Errorf("build_gate.healing.retries: must be non-negative, got %d",
				s.BuildGate.Healing.Retries)
		}
		if s.BuildGate.Healing.Image.IsEmpty() {
			return fmt.Errorf("build_gate.healing.image: required when healing is configured")
		}
		// Healing requires a router to be configured (router runs before healing).
		if s.BuildGate.Router == nil || s.BuildGate.Router.Image.IsEmpty() {
			return fmt.Errorf("build_gate.router: required when healing is configured")
		}
	}

	// Validate router spec.
	if s.BuildGate != nil && s.BuildGate.Router != nil {
		if s.BuildGate.Router.Image.IsEmpty() {
			return fmt.Errorf("build_gate.router.image: required when router is specified")
		}
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
		if s.BuildGate.Pre != nil && s.BuildGate.Pre.Stack != nil {
			if err := validateBuildGateStackConfig(s.BuildGate.Pre.Stack, "build_gate.pre.stack"); err != nil {
				return err
			}
		}
		if s.BuildGate.Post != nil && s.BuildGate.Post.Stack != nil {
			if err := validateBuildGateStackConfig(s.BuildGate.Post.Stack, "build_gate.post.stack"); err != nil {
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

	// Reject enabled:false with any configured fields (ambiguous).
	if !stack.Enabled {
		if stack.Language != "" || stack.Tool != "" || stack.Release != "" || stack.Default {
			return fmt.Errorf("%s: enabled=false with stack fields is ambiguous; remove stack fields or set enabled=true", prefix)
		}
		return nil
	}

	// Enabled:true requires at least language and release.
	if strings.TrimSpace(stack.Language) == "" {
		return fmt.Errorf("%s.language: required", prefix)
	}
	if strings.TrimSpace(stack.Release) == "" {
		return fmt.Errorf("%s.release: required", prefix)
	}
	return nil
}

// validateStackGateSpec validates a StackGateSpec for ambiguous configuration.
// Rejects enabled:false with expect:{...} as this is contradictory.
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
