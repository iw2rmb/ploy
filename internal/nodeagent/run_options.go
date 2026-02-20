// run_options.go defines typed option structs for nodeagent execution.
// These types replace untyped map[string]any lookups with type-safe accessors.
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

	// Router configures the router container that runs on gate failure
	// to produce a bug_summary before healing begins.
	Router *RouterConfig

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
// These options control whether the gate runs and mod-level image overrides.
type BuildGateOptions struct {
	// Enabled controls whether the build gate runs before the main mod execution.
	Enabled bool

	// Images holds mod-level image mapping overrides for Stack Gate.
	// These rules override the default mapping file.
	Images []contracts.BuildGateImageRule

	// PreStack configures stack detection fallback for pre-gate.
	PreStack *contracts.BuildGateStackConfig

	// PostStack configures stack detection fallback for post-gate and re-gate.
	PostStack *contracts.BuildGateStackConfig
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

// ContainerSpec describes a container's image, command, env, and retention policy.
// Used for healing mods, router, and execution options.
type ContainerSpec struct {
	Image           contracts.ModImage
	Command         Command
	Env             map[string]string
	RetainContainer bool
}

// HealingMod is a ContainerSpec for healing containers.
type HealingMod = ContainerSpec

// RouterConfig is a ContainerSpec for router containers.
type RouterConfig = ContainerSpec

// Command represents a container command as either a shell string or exec array.
type Command struct {
	// Shell holds the command when specified as a single shell string.
	Shell string

	// Exec holds the command when specified as an exec array.
	Exec []string
}

// IsEmpty returns true if no command is specified.
func (c Command) IsEmpty() bool {
	return c.Shell == "" && len(c.Exec) == 0
}

// ToSlice converts the command to a []string suitable for manifest execution.
// Returns nil if the command is empty.
func (c Command) ToSlice() []string {
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

// ExecutionOptions is a ContainerSpec for mod execution containers.
type ExecutionOptions = ContainerSpec

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
	Command Command

	// Env holds environment variables specific to this step.
	Env map[string]string

	// RetainContainer controls whether this step's container is retained.
	RetainContainer bool

	// Stack configures Stack Gate validation for this step.
	// Inbound validates pre-mod expectations; Outbound validates post-mod expectations.
	Stack *contracts.StackGateSpec
}

// modsSpecToRunOptions converts contracts.ModsSpec directly to RunOptions,
// avoiding the intermediate map[string]any bridge.
func modsSpecToRunOptions(spec *contracts.ModsSpec) RunOptions {
	if spec == nil {
		return RunOptions{}
	}

	runOpts := RunOptions{}

	if spec.BuildGate != nil {
		runOpts.BuildGate.Enabled = spec.BuildGate.Enabled
		runOpts.BuildGate.Images = spec.BuildGate.Images
		if spec.BuildGate.Pre != nil {
			runOpts.BuildGate.PreStack = spec.BuildGate.Pre.Stack
		}
		if spec.BuildGate.Post != nil {
			runOpts.BuildGate.PostStack = spec.BuildGate.Post.Stack
		}

		if spec.BuildGate.Healing != nil {
			healing := &HealingConfig{
				Retries: spec.BuildGate.Healing.Retries,
			}
			if healing.Retries <= 0 {
				healing.Retries = 1
			}

			healing.Mod = HealingMod{
				Image:           spec.BuildGate.Healing.Image,
				Command:         commandSpecToCommand(spec.BuildGate.Healing.Command),
				Env:             copyStringMap(spec.BuildGate.Healing.Env),
				RetainContainer: spec.BuildGate.Healing.RetainContainer,
			}

			runOpts.Healing = healing
		}

		if spec.BuildGate.Router != nil {
			runOpts.Router = &RouterConfig{
				Image:           spec.BuildGate.Router.Image,
				Command:         commandSpecToCommand(spec.BuildGate.Router.Command),
				Env:             copyStringMap(spec.BuildGate.Router.Env),
				RetainContainer: spec.BuildGate.Router.RetainContainer,
			}
		}
	}

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

	// Single-step: extract from steps[0]. Multi-step: populate Steps[].
	if len(spec.Steps) == 1 {
		step := spec.Steps[0]
		runOpts.Execution.Image = step.Image
		runOpts.Execution.Command = commandSpecToCommand(step.Command)
		runOpts.Execution.RetainContainer = step.RetainContainer
	}

	if len(spec.Steps) > 1 {
		runOpts.Steps = make([]StepMod, 0, len(spec.Steps))
		for _, step := range spec.Steps {
			stepMod := StepMod{
				Image:           step.Image,
				Command:         commandSpecToCommand(step.Command),
				Env:             copyStringMap(step.Env),
				RetainContainer: step.RetainContainer,
				Stack:           step.Stack,
			}
			runOpts.Steps = append(runOpts.Steps, stepMod)
		}
	}

	runOpts.Artifacts.Name = spec.ArtifactName
	if len(spec.ArtifactPaths) > 0 {
		runOpts.Artifacts.Paths = make([]string, 0, len(spec.ArtifactPaths))
		for _, p := range spec.ArtifactPaths {
			if trimmed := strings.TrimSpace(p); trimmed != "" {
				runOpts.Artifacts.Paths = append(runOpts.Artifacts.Paths, p)
			}
		}
	}

	if !spec.JobID.IsZero() {
		runOpts.ServerMetadata.JobID = spec.JobID
	}

	return runOpts
}

// commandSpecToCommand converts contracts.CommandSpec to Command.
func commandSpecToCommand(cmd contracts.CommandSpec) Command {
	return Command{
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
