// run_options.go defines typed option structs for nodeagent execution.
// These types replace untyped map[string]any lookups with type-safe accessors.
package nodeagent

import (
	"strings"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// RunOptions holds all typed configuration options for a run execution.
type RunOptions struct {
	BuildGate      BuildGateOptions
	Healing        *HealingConfig
	Router         *ModContainerSpec
	MRWiring       MRWiringOptions
	MRFlagsPresent MRFlagsPresence
	Execution      ModContainerSpec
	Artifacts      ArtifactOptions
	ServerMetadata ServerMetadataOptions
	Steps          []StepMod
	StackGate      *contracts.StepGateStackSpec
}

// BuildGateOptions configures pre-mod build gate validation.
type BuildGateOptions struct {
	Enabled   bool
	Images    []contracts.BuildGateImageRule
	PreStack  *contracts.BuildGateStackConfig
	PostStack *contracts.BuildGateStackConfig
}

// HealingConfig describes the heal → re-gate loop configuration.
type HealingConfig struct {
	Retries int
	Mod     ModContainerSpec
}

// ModContainerSpec describes a container's image, command, env, and retention policy.
// Used for healing mods, router, execution options, and step mods.
type ModContainerSpec struct {
	Image           contracts.ModImage
	Command         contracts.CommandSpec
	Env             map[string]string
	RetainContainer bool
}

// MRWiringOptions configures GitLab merge request creation for run outcomes.
type MRWiringOptions struct {
	GitLabPAT    string
	GitLabDomain string
	MROnSuccess  bool
	MROnFail     bool
}

// ArtifactOptions configures artifact collection and upload.
type ArtifactOptions struct {
	Paths []string
	Name  string
}

// MRFlagsPresence tracks whether MR creation flags were explicitly set in the spec.
type MRFlagsPresence struct {
	MROnSuccessSet bool
	MROnFailSet    bool
}

// ServerMetadataOptions holds server-injected metadata for uploads and tracking.
type ServerMetadataOptions struct {
	JobID domaintypes.JobID
}

// StepMod describes a single mod step in a multi-step run (steps[] array).
// Each step has its own container spec and optional Stack Gate validation.
type StepMod struct {
	ModContainerSpec
	Stack *contracts.StackGateSpec
}

// modsSpecToRunOptions converts contracts.ModsSpec directly to RunOptions.
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
			healing.Mod = ModContainerSpec{
				Image:           spec.BuildGate.Healing.Image,
				Command:         spec.BuildGate.Healing.Command,
				Env:             copyStringMap(spec.BuildGate.Healing.Env),
				RetainContainer: spec.BuildGate.Healing.RetainContainer,
			}
			runOpts.Healing = healing
		}

		if spec.BuildGate.Router != nil {
			runOpts.Router = &ModContainerSpec{
				Image:           spec.BuildGate.Router.Image,
				Command:         spec.BuildGate.Router.Command,
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
		runOpts.Execution.Command = step.Command
		runOpts.Execution.RetainContainer = step.RetainContainer
	}

	if len(spec.Steps) > 1 {
		runOpts.Steps = make([]StepMod, 0, len(spec.Steps))
		for _, step := range spec.Steps {
			runOpts.Steps = append(runOpts.Steps, StepMod{
				ModContainerSpec: ModContainerSpec{
					Image:           step.Image,
					Command:         step.Command,
					Env:             copyStringMap(step.Env),
					RetainContainer: step.RetainContainer,
				},
				Stack: step.Stack,
			})
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
