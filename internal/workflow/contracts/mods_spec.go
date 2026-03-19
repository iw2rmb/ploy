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
//   - mods_spec_parse.go: JSON parsing functions
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

	// APIVersion is an optional schema version identifier (e.g., "ploy.mig/v1alpha1").
	// Informational only; the control plane forwards specs as opaque JSON.
	APIVersion string `json:"apiVersion,omitempty" yaml:"apiVersion,omitempty"`

	// Kind is an optional schema kind identifier (e.g., "MigRunSpec").
	// Informational only; the control plane forwards specs as opaque JSON.
	Kind string `json:"kind,omitempty" yaml:"kind,omitempty"`

	// --- Steps (required) ---

	// Steps holds the ordered list of mig steps.
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

// TmpFilePayload describes a single file to materialize under /tmp in the container.
// Name is the destination filename (not a path); Content is the raw file bytes.
// Both fields are required: Name must be non-empty and Content must be non-empty.
type TmpFilePayload struct {
	Name    string `json:"name" yaml:"name"`
	Content []byte `json:"content" yaml:"content"`
}

// ModStep describes a single mig step in a run (steps[] array).
// Each step has its own image, command, and environment configuration.
// Steps execute sequentially with shared workspace, each running gate+mig.
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

	// Stack configures Stack Gate validation for this step.
	// Inbound validates pre-mig expectations; Outbound validates post-mig expectations.
	Stack *StackGateSpec `json:"stack,omitempty" yaml:"stack,omitempty"`

	// Always forces this step to run even when a cache hit exists for the same
	// repo_sha_in and canonicalized step operations hash.
	Always bool `json:"always,omitempty" yaml:"always,omitempty"`

	// TmpDir lists files to materialize read-only under /tmp in the container.
	// Each entry must have a unique non-empty name and non-empty content.
	TmpDir []TmpFilePayload `json:"tmp_dir,omitempty" yaml:"tmp_dir,omitempty"`
}

// Validate checks that the spec is structurally valid.
// Returns nil if valid, or a descriptive error for invalid specs.
//
// Validation rules:
//   - steps must be non-empty and each step must have a non-empty image.
//   - build_gate.healing.by_error_kind must configure non-terminal kinds.
//   - build_gate.router must be configured when healing is configured.
//   - Retries must be non-negative.
//   - Stack Gate phases must not be disabled with expectations set.
func (s ModsSpec) Validate() error {
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
		if err := validateTmpDir(mig.TmpDir, fmt.Sprintf("steps[%d].tmp_dir", i)); err != nil {
			return err
		}
	}

	// Validate healing spec.
	if s.BuildGate != nil && s.BuildGate.Healing != nil {
		if len(s.BuildGate.Healing.ByErrorKind) == 0 {
			return fmt.Errorf("build_gate.healing.by_error_kind: required when healing is configured")
		}
		for errorKind, action := range s.BuildGate.Healing.ByErrorKind {
			parsedKind, ok := ParseRecoveryErrorKind(errorKind)
			if !ok {
				return fmt.Errorf("build_gate.healing.by_error_kind.%s: invalid error_kind key", errorKind)
			}
			prefix := fmt.Sprintf("build_gate.healing.by_error_kind.%s", errorKind)
			if action.Retries < 0 {
				return fmt.Errorf("%s.retries: must be non-negative, got %d", prefix, action.Retries)
			}
			if IsTerminalRecoveryErrorKind(parsedKind) {
				return fmt.Errorf("%s: forbidden for terminal error_kind %q", prefix, errorKind)
			}
			if action.Image.IsEmpty() {
				return fmt.Errorf("%s.image: required", prefix)
			}
			if action.Expectations != nil {
				for i, artifact := range action.Expectations.Artifacts {
					if err := ValidateGateProfileArtifactContract(
						artifact.Path,
						artifact.Schema,
						fmt.Sprintf("%s.expectations.artifacts[%d]", prefix, i),
					); err != nil {
						return err
					}
				}
			}
			if err := validateTmpDir(action.TmpDir, prefix+".tmp_dir"); err != nil {
				return err
			}
		}
		// Healing requires a router to be configured (router runs before healing).
		if s.BuildGate.Router == nil || s.BuildGate.Router.Image.IsEmpty() {
			return fmt.Errorf("build_gate.router: required when healing is configured")
		}
		if s.BuildGate.Healing.SelectedErrorKind != "" {
			if _, ok := ParseRecoveryErrorKind(s.BuildGate.Healing.SelectedErrorKind); !ok {
				return fmt.Errorf("build_gate.healing.selected_error_kind: invalid value %q", s.BuildGate.Healing.SelectedErrorKind)
			}
		}
	}

	// Validate router spec.
	if s.BuildGate != nil && s.BuildGate.Router != nil {
		if s.BuildGate.Router.Image.IsEmpty() {
			return fmt.Errorf("build_gate.router.image: required when router is specified")
		}
		if err := validateTmpDir(s.BuildGate.Router.TmpDir, "build_gate.router.tmp_dir"); err != nil {
			return err
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
			if err := validateBuildGateProfileOverride(pair.phase.GateProfile, pair.prefix+".gate_profile"); err != nil {
				return err
			}
			if err := validateBuildGatePhaseTarget(pair.phase.Target, pair.prefix+".target"); err != nil {
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

func validateBuildGateProfileOverride(prep *BuildGateProfileOverride, prefix string) error {
	if prep == nil {
		return nil
	}
	if prep.Command.IsEmpty() {
		return fmt.Errorf("%s.command: required", prefix)
	}
	if err := validateBuildGatePhaseTarget(prep.Target, prefix+".target"); err != nil {
		return err
	}
	if prep.Stack != nil {
		if strings.TrimSpace(prep.Stack.Language) == "" {
			return fmt.Errorf("%s.stack.language: required", prefix)
		}
		if strings.TrimSpace(prep.Stack.Tool) == "" {
			return fmt.Errorf("%s.stack.tool: required", prefix)
		}
	}
	return nil
}

func validateBuildGatePhaseTarget(target string, prefix string) error {
	switch strings.TrimSpace(target) {
	case "", GateProfileTargetBuild, GateProfileTargetUnit, GateProfileTargetAllTests:
		return nil
	default:
		return fmt.Errorf("%s: invalid value %q (expected one of: %s|%s|%s)", prefix, target, GateProfileTargetBuild, GateProfileTargetUnit, GateProfileTargetAllTests)
	}
}

// validateStackGateSpec validates a StackGateSpec for ambiguous configuration.
// Rejects enabled:false with expect:{...} as this is contradictory.
// validateTmpDir checks that each TmpFilePayload has a non-empty name and
// non-empty content, and that no two entries share the same destination name.
func validateTmpDir(entries []TmpFilePayload, prefix string) error {
	seen := make(map[string]struct{}, len(entries))
	for i, e := range entries {
		pos := fmt.Sprintf("%s[%d]", prefix, i)
		if strings.TrimSpace(e.Name) == "" {
			return fmt.Errorf("%s.name: required", pos)
		}
		if len(e.Content) == 0 {
			return fmt.Errorf("%s.content: required", pos)
		}
		if _, dup := seen[e.Name]; dup {
			return fmt.Errorf("%s.name: duplicate %q", pos, e.Name)
		}
		seen[e.Name] = struct{}{}
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
