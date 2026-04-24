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
	MRWiring       MRWiringOptions
	MRFlagsPresent MRFlagsPresence
	Execution      MigContainerSpec
	Artifacts      ArtifactOptions
	ServerMetadata ServerMetadataOptions
	Steps          []StepMig
	StackGate      *contracts.StepGateStackSpec

	// BundleMap maps content hashes to spec bundle download identifiers.
	// Populated from MigSpec.BundleMap during spec-to-run-options conversion.
	BundleMap map[string]string
}

// BuildGateOptions configures pre-mig build gate validation.
type BuildGateOptions struct {
	Enabled bool
	Images  []contracts.BuildGateImageRule
	Pre     *contracts.BuildGatePhaseConfig
	Post    *contracts.BuildGatePhaseConfig
}

// MigContainerSpec describes a container's image, command, and env.
// Used for healing migs, execution options, and step migs.
type MigContainerSpec struct {
	Image   contracts.JobImage
	Command contracts.CommandSpec
	Env     map[string]string

	// Hydra resource entries for staged materialization and mount planning.
	CA   []string // canonical CA cert entries (shortHash)
	In   []string // canonical read-only input entries (shortHash:/in/dst)
	Out  []string // canonical read-write output entries (shortHash:/out/dst)
	Home []string // canonical home-relative entries (shortHash:dst{:ro})
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

// StepMig describes a single mig step in a multi-step run (steps[] array).
// Each step has its own container spec and optional Stack Gate validation.
type StepMig struct {
	MigContainerSpec
	Stack *contracts.StackGateSpec
}

// migsSpecToRunOptions converts contracts.MigSpec directly to RunOptions.
func migsSpecToRunOptions(spec *contracts.MigSpec) RunOptions {
	if spec == nil {
		return RunOptions{}
	}

	runOpts := RunOptions{}

	if spec.BuildGate != nil {
		runOpts.BuildGate.Enabled = spec.BuildGate.Enabled
		runOpts.BuildGate.Images = spec.BuildGate.Images
		runOpts.BuildGate.Pre = spec.BuildGate.Pre
		runOpts.BuildGate.Post = spec.BuildGate.Post
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
		runOpts.Execution.CA = step.CA
		runOpts.Execution.In = step.In
		runOpts.Execution.Out = step.Out
		runOpts.Execution.Home = step.Home
	}

	if len(spec.Steps) > 1 {
		runOpts.Steps = make([]StepMig, 0, len(spec.Steps))
		for _, step := range spec.Steps {
			runOpts.Steps = append(runOpts.Steps, StepMig{
				MigContainerSpec: MigContainerSpec{
					Image:   step.Image,
					Command: step.Command,
					Env:     copyStringMap(step.Envs),
					CA:      step.CA,
					In:      step.In,
					Out:     step.Out,
					Home:    step.Home,
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

	runOpts.BundleMap = spec.BundleMap

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
