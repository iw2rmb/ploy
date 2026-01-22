// run_options.go defines typed option structs for nodeagent execution.
//
// This file introduces small, focused option structs that clarify which spec/options
// keys are understood by the nodeagent. These types replace untyped map[string]any
// lookups throughout the package while preserving raw JSON where needed for
// wire-level compatibility. The typed options align with the roadmap goal of
// reducing map[string]any casts and improving internal type safety.
//
// ## Hot-Path Optimization
//
// The primary entry point for typed options is modsSpecToRunOptions(), which
// converts directly from contracts.ModsSpec to RunOptions. This eliminates the
// intermediate map[string]any bridge and its associated type conversion hazards:
//   - No float64→int conversion for numeric fields (retries)
//   - No []any→[]string conversion for slices (artifact_paths, command exec arrays)
//   - No map[string]any type assertions at access points
//
// Typed options are constructed at the nodeagent boundary from the canonical
// contracts.ModsSpec (see parseSpec in claimer_spec.go). This removes the
// intermediate map[string]any bridge and its associated type conversion hazards.
//
// ## Stack-Aware Image Selection
//
// The Image field in StepMod, HealingMod, and ExecutionOptions uses the
// contracts.ModImage type to support both universal images (string) and
// stack-specific images (map keyed by stack). Resolution happens at manifest
// build time using the detected stack from Build Gate.
package nodeagent

import (
	"strings"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// RunOptions holds all typed configuration options for a run execution.
// It aggregates build gate configuration, healing policy, MR creation wiring,
// execution shaping parameters, and artifact collection settings.
//
// RunOptions is derived from the run spec at the nodeagent boundary (see parseSpec)
// and is passed through StartRunRequest.TypedOptions into manifest building and
// execution orchestration paths.
//
// This type is the canonical source of truth for run options. Raw map[string]any
// access should not be used; all option keys understood by the nodeagent are
// exposed as typed fields on this struct.
type RunOptions struct {
	// BuildGate configures pre-mod build gate validation.
	BuildGate BuildGateOptions

	// Healing configures the heal → re-gate loop when build gate fails.
	Healing *HealingConfig

	// MRWiring configures GitLab merge request creation.
	MRWiring MRWiringOptions

	// MRFlagsPresent tracks which MR flags were explicitly set in the spec.
	// This enables distinguishing between "not set" and "set to false".
	MRFlagsPresent MRFlagsPresence

	// Execution configures container image, command, and retention.
	Execution ExecutionOptions

	// Artifacts configures artifact collection and upload.
	Artifacts ArtifactOptions

	// ServerMetadata holds server-injected metadata for uploads and tracking.
	ServerMetadata ServerMetadataOptions

	// Steps holds the list of mod steps for multi-step runs (steps[] array).
	// For single-step runs, this slice is empty and Execution options are used.
	// For multi-step runs, this slice contains one entry per mod in steps[].
	Steps []StepMod

	// StackGate holds the effective Stack Gate expectation for gate jobs.
	// Set from steps[stepIndex].stack.{inbound|outbound} based on gate type.
	// Pre-gate uses inbound; post-gate and re-gate use outbound expectations.
	StackGate *contracts.StepGateStackSpec
}

// BuildGateOptions configures pre-mod build gate validation.
// These options control whether the gate runs, which profile to use, and
// environment variables to inject into the gate execution context.
type BuildGateOptions struct {
	// Enabled controls whether the build gate runs before the main mod execution.
	Enabled bool

	// Profile specifies the gate profile name (e.g., "java-auto", "java-maven").
	// The profile determines which static analysis tools and checks to run.
	Profile string

	// Images holds mod-level image mapping overrides for Stack Gate.
	// These rules override default file and cluster/global inline rules
	// when resolving the Build Gate image for Stack Gate mode.
	Images []contracts.BuildGateImageRule
}

// HealingConfig describes the heal → re-gate loop configuration.
// When the build gate fails, the agent can execute a single healing mod per gate.
type HealingConfig struct {
	// Retries is the maximum number of healing attempts (default: 1).
	// Each retry executes a healing mod, then re-runs the gate.
	Retries int

	// Mod is the single healing mod specification for this gate.
	Mod HealingMod
}

// HealingMod describes a single healing mod container specification.
// Healing mods run after gate failure to attempt workspace fixes before re-gate.
type HealingMod struct {
	// Image is the container image for the healing mod (required).
	// Supports both universal images (string) and stack-specific images (map).
	// Resolution to a concrete image happens at manifest build time using the
	// detected stack from Build Gate.
	Image contracts.ModImage

	// Command is the container command override (optional).
	// Can be a single shell string or an array of command + args.
	Command HealingCommand

	// Env holds environment variables to inject into the healing container.
	Env map[string]string

	// RetainContainer controls whether the healing container is retained for debugging.
	RetainContainer bool
}

// HealingCommand represents a healing mod command as either a shell string or exec array.
// This type encapsulates the polymorphic command representation in healing mod specs.
type HealingCommand struct {
	// Shell holds the command when specified as a single shell string.
	// When non-empty, the command is executed via ["/bin/sh", "-c", Shell].
	Shell string

	// Exec holds the command when specified as an exec array.
	// When non-nil, the command is executed directly without a shell.
	Exec []string
}

// IsEmpty returns true if no command is specified.
func (c HealingCommand) IsEmpty() bool {
	return c.Shell == "" && len(c.Exec) == 0
}

// ToSlice converts the command to a []string suitable for manifest execution.
// Returns nil if the command is empty.
func (c HealingCommand) ToSlice() []string {
	if len(c.Exec) > 0 {
		return c.Exec
	}
	if c.Shell != "" {
		return []string{"/bin/sh", "-c", c.Shell}
	}
	return nil
}

// MRWiringOptions configures GitLab merge request creation for run outcomes.
// These options control when MRs are created and how to authenticate with GitLab.
type MRWiringOptions struct {
	// GitLabPAT is the personal access token for GitLab API authentication.
	// This value is never logged and is only passed to the GitLab client.
	GitLabPAT string

	// GitLabDomain is the GitLab instance domain (e.g., "gitlab.com").
	GitLabDomain string

	// MROnSuccess controls whether to create an MR when the run succeeds.
	MROnSuccess bool

	// MROnFail controls whether to create an MR when the run fails.
	MROnFail bool
}

// ExecutionOptions configures container execution parameters.
// These options shape how the main mod container is configured and retained.
type ExecutionOptions struct {
	// Image is the container image to run (default: "ubuntu:latest").
	// Supports both universal images (string) and stack-specific images (map).
	// Resolution to a concrete image happens at manifest build time using the
	// detected stack from Build Gate.
	Image contracts.ModImage

	// Command is the container command override (optional).
	// Can be a shell string or an array of command + args.
	Command ExecutionCommand

	// RetainContainer controls whether the container is retained after run completion.
	RetainContainer bool
}

// ExecutionCommand represents a mod command as either a shell string or exec array.
// This type mirrors HealingCommand but is used for main mod execution.
type ExecutionCommand struct {
	// Shell holds the command when specified as a single shell string.
	Shell string

	// Exec holds the command when specified as an exec array.
	Exec []string
}

// IsEmpty returns true if no command is specified.
func (c ExecutionCommand) IsEmpty() bool {
	return c.Shell == "" && len(c.Exec) == 0
}

// ToSlice converts the command to a []string suitable for manifest execution.
// Returns nil if the command is empty.
func (c ExecutionCommand) ToSlice() []string {
	if len(c.Exec) > 0 {
		return c.Exec
	}
	if c.Shell != "" {
		return []string{"/bin/sh", "-c", c.Shell}
	}
	return nil
}

// ArtifactOptions configures artifact collection and upload.
// These options specify which workspace files/directories to bundle and upload,
// and how to name the uploaded bundle.
type ArtifactOptions struct {
	// Paths is the list of workspace-relative files/directories to upload.
	// Each path is relative to /workspace and can be a file or directory.
	Paths []string

	// Name is the custom name for the uploaded artifact bundle (optional).
	// If empty, the server generates a default name based on run_id and job_id.
	Name string
}

// MRFlagsPresence tracks whether MR creation flags were explicitly set in the spec.
// This enables distinguishing between "not set" and "set to false" for MR wiring options.
type MRFlagsPresence struct {
	// MROnSuccessSet is true when mr_on_success was explicitly specified in the spec.
	MROnSuccessSet bool

	// MROnFailSet is true when mr_on_fail was explicitly specified in the spec.
	MROnFailSet bool
}

// ServerMetadataOptions holds server-injected metadata for uploads and tracking.
// These options are populated by the control plane and used by the nodeagent
// for status reporting, artifact uploads, and run correlation.
type ServerMetadataOptions struct {
	// JobID is the server-provided job identifier for upload correlation.
	// This value is used to associate artifacts and status updates with a job.
	JobID domaintypes.JobID
}

// StepMod describes a single mod step in a multi-step run (steps[] array).
// Each step has its own image, command, and environment configuration.
// Steps execute sequentially with shared workspace, each running gate+mod.
type StepMod struct {
	// Image is the container image for this step (required).
	// Supports both universal images (string) and stack-specific images (map).
	// Resolution to a concrete image happens at manifest build time using the
	// detected stack from Build Gate.
	Image contracts.ModImage

	// Command is the container command override for this step (optional).
	Command ExecutionCommand

	// Env holds environment variables specific to this step.
	Env map[string]string

	// RetainContainer controls whether this step's container is retained.
	RetainContainer bool

	// Stack configures Stack Gate validation for this step.
	// Inbound validates pre-mod expectations; Outbound validates post-mod expectations.
	Stack *contracts.StackGateSpec
}

// modsSpecToRunOptions converts a canonical contracts.ModsSpec directly to RunOptions.
// This is the preferred hot-path conversion that avoids the intermediate map[string]any
// bridge and its associated type conversion hazards (float64/int, []any/[]string).
//
// Env merge semantics are handled alongside spec parsing (see modsSpecToEnv in
// claimer_spec.go). This conversion focuses on producing typed option structs.
//
// ## Why Direct Conversion?
//
// The previous map-bridge pipeline introduced
// type hazards on the hot path:
//   - JSON numbers unmarshaled as float64 required int conversion at access points
//   - String slices passed through []any required element-by-element conversion
//   - Map values required type assertions at every access point
//
// Direct conversion from the typed ModsSpec struct eliminates these hazards by
// reading strongly-typed fields directly (e.g., spec.BuildGate.Healing.Retries as int).
func modsSpecToRunOptions(spec *contracts.ModsSpec) RunOptions {
	if spec == nil {
		return RunOptions{}
	}

	runOpts := RunOptions{}

	// --- Build Gate Options ---
	// Extract enabled/profile from the typed BuildGate struct.
	if spec.BuildGate != nil {
		runOpts.BuildGate.Enabled = spec.BuildGate.Enabled
		runOpts.BuildGate.Profile = spec.BuildGate.Profile
		runOpts.BuildGate.Images = spec.BuildGate.Images

		// --- Healing Configuration ---
		// Convert contracts.HealingSpec to nodeagent.HealingConfig directly,
		// avoiding the map[string]any bridge that required float64→int conversion.
		if spec.BuildGate.Healing != nil {
			healing := &HealingConfig{
				Retries: spec.BuildGate.Healing.Retries,
			}
			// Default retries to 1 if not specified (spec parser sets default of 1).
			if healing.Retries <= 0 {
				healing.Retries = 1
			}

			// Convert healing mod specification.
			if spec.BuildGate.Healing.Mod != nil {
				healing.Mod = healingModSpecToHealingMod(spec.BuildGate.Healing.Mod)
			}

			runOpts.Healing = healing
		}
	}

	// --- MR Wiring Options ---
	// Direct field assignment from typed spec fields.
	runOpts.MRWiring.GitLabPAT = spec.GitLabPAT
	runOpts.MRWiring.GitLabDomain = spec.GitLabDomain
	if spec.MROnSuccess != nil {
		runOpts.MRWiring.MROnSuccess = *spec.MROnSuccess
		runOpts.MRFlagsPresent.MROnSuccessSet = true
	}
	if spec.MROnFail != nil {
		runOpts.MRWiring.MROnFail = *spec.MROnFail
		runOpts.MRFlagsPresent.MROnFailSet = true
	}

	// --- Execution Options (Single-Step) ---
	// For single-step specs, extract image/command/retain_container from steps[0].
	// Multi-step specs populate Steps[] instead.
	if len(spec.Steps) == 1 {
		step := spec.Steps[0]
		runOpts.Execution.Image = step.Image
		runOpts.Execution.Command = commandSpecToExecutionCommand(step.Command)
		runOpts.Execution.RetainContainer = step.RetainContainer
	}

	// --- Multi-Step Steps Array ---
	// For multi-step specs (len > 1), populate Steps[] with typed StepMod entries.
	// Single-step specs use Execution options instead (Steps remains empty).
	if len(spec.Steps) > 1 {
		runOpts.Steps = make([]StepMod, 0, len(spec.Steps))
		for _, step := range spec.Steps {
			stepMod := StepMod{
				Image:           step.Image,
				Command:         commandSpecToExecutionCommand(step.Command),
				Env:             copyStringMap(step.Env),
				RetainContainer: step.RetainContainer,
				Stack:           step.Stack,
			}
			runOpts.Steps = append(runOpts.Steps, stepMod)
		}
	}

	// --- Artifact Options ---
	// Direct slice copy without []any→[]string conversion.
	runOpts.Artifacts.Name = spec.ArtifactName
	if len(spec.ArtifactPaths) > 0 {
		runOpts.Artifacts.Paths = make([]string, 0, len(spec.ArtifactPaths))
		for _, p := range spec.ArtifactPaths {
			if trimmed := strings.TrimSpace(p); trimmed != "" {
				runOpts.Artifacts.Paths = append(runOpts.Artifacts.Paths, p)
			}
		}
	}

	// --- Server Metadata ---
	// Direct assignment from typed spec fields.
	if !spec.JobID.IsZero() {
		runOpts.ServerMetadata.JobID = spec.JobID
	}

	return runOpts
}

// healingModSpecToHealingMod converts contracts.HealingModSpec to nodeagent.HealingMod.
// This direct conversion avoids the map[string]any bridge for healing mod parsing.
func healingModSpecToHealingMod(spec *contracts.HealingModSpec) HealingMod {
	if spec == nil {
		return HealingMod{}
	}

	return HealingMod{
		Image:           spec.Image,
		Command:         commandSpecToHealingCommand(spec.Command),
		Env:             copyStringMap(spec.Env),
		RetainContainer: spec.RetainContainer,
	}
}

// commandSpecToExecutionCommand converts contracts.CommandSpec to ExecutionCommand.
// Direct field mapping without polymorphic type switching.
func commandSpecToExecutionCommand(cmd contracts.CommandSpec) ExecutionCommand {
	return ExecutionCommand{
		Shell: cmd.Shell,
		Exec:  cmd.Exec,
	}
}

// commandSpecToHealingCommand converts contracts.CommandSpec to HealingCommand.
// Direct field mapping without polymorphic type switching.
func commandSpecToHealingCommand(cmd contracts.CommandSpec) HealingCommand {
	return HealingCommand{
		Shell: cmd.Shell,
		Exec:  cmd.Exec,
	}
}

// copyStringMap creates a shallow copy of a string map.
// Returns nil if the input is nil or empty.
func copyStringMap(m map[string]string) map[string]string {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
