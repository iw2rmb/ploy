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
	BuildGate       BuildGateOptions
	HealingSelector *contracts.HealingSpec
	Healing         *HealingConfig
	Router          *ModContainerSpec
	MRWiring        MRWiringOptions
	MRFlagsPresent  MRFlagsPresence
	Execution       ModContainerSpec
	Artifacts       ArtifactOptions
	ServerMetadata  ServerMetadataOptions
	Steps           []StepMod
	StackGate       *contracts.StepGateStackSpec
}

// BuildGateOptions configures pre-mig build gate validation.
type BuildGateOptions struct {
	Enabled bool
	Images  []contracts.BuildGateImageRule
	Pre     *contracts.BuildGatePhaseConfig
	Post    *contracts.BuildGatePhaseConfig
}

// HealingConfig describes the heal → re-gate loop configuration.
type HealingConfig struct {
	Retries int
	Mod     ModContainerSpec
}

func (o RunOptions) HasHealingSelector() bool {
	return o.HealingSelector != nil && len(o.HealingSelector.ByErrorKind) > 0
}

// ModContainerSpec describes a container's image, command, and env.
// Used for healing migs, router, execution options, and step migs.
type ModContainerSpec struct {
	Image   contracts.JobImage
	Command contracts.CommandSpec
	Env     map[string]string
	// TmpBundle references a pre-uploaded bundle to extract under /tmp at execution time.
	// Populated from the spec's tmp_bundle field during spec-to-run-options conversion.
	// The nodeagent downloads and unpacks this bundle before container launch.
	TmpBundle *contracts.TmpBundleRef
	// Amata configures amata-mode execution. When non-nil with a non-empty Spec,
	// the container runs `amata run /in/amata.yaml` instead of the direct codex path.
	Amata *contracts.AmataRunSpec
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

// StepMod describes a single mig step in a multi-step run (steps[] array).
// Each step has its own container spec and optional Stack Gate validation.
type StepMod struct {
	ModContainerSpec
	Stack  *contracts.StackGateSpec
	Always bool
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
		runOpts.BuildGate.Pre = spec.BuildGate.Pre
		runOpts.BuildGate.Post = spec.BuildGate.Post

		if spec.BuildGate.Healing != nil {
			runOpts.HealingSelector = copyHealingSpec(spec.BuildGate.Healing)
			selectedKind := strings.TrimSpace(spec.BuildGate.Healing.SelectedErrorKind)
			if selectedKind != "" {
				if action, ok := spec.BuildGate.Healing.ByErrorKind[selectedKind]; ok {
					healing := &HealingConfig{Retries: action.Retries}
					if healing.Retries <= 0 {
						healing.Retries = 1
					}
					healing.Mod = ModContainerSpec{
						Image:     action.Image,
						Command:   action.Command,
						Env:       copyStringMap(action.Env),
						TmpBundle: action.TmpBundle,
						Amata:     action.Amata,
					}
					runOpts.Healing = healing
				}
			}
		}

		if spec.BuildGate.Router != nil {
			runOpts.Router = &ModContainerSpec{
				Image:     spec.BuildGate.Router.Image,
				Command:   spec.BuildGate.Router.Command,
				Env:       copyStringMap(spec.BuildGate.Router.Env),
				TmpBundle: spec.BuildGate.Router.TmpBundle,
				Amata:     spec.BuildGate.Router.Amata,
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
		runOpts.Execution.TmpBundle = step.TmpBundle
		runOpts.Execution.Amata = step.Amata
	}

	if len(spec.Steps) > 1 {
		runOpts.Steps = make([]StepMod, 0, len(spec.Steps))
		for _, step := range spec.Steps {
			runOpts.Steps = append(runOpts.Steps, StepMod{
				ModContainerSpec: ModContainerSpec{
					Image:     step.Image,
					Command:   step.Command,
					Env:       copyStringMap(step.Env),
					TmpBundle: step.TmpBundle,
					Amata:     step.Amata,
				},
				Stack:  step.Stack,
				Always: step.Always,
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

func copyHealingSpec(in *contracts.HealingSpec) *contracts.HealingSpec {
	if in == nil {
		return nil
	}
	out := &contracts.HealingSpec{
		SelectedErrorKind: in.SelectedErrorKind,
	}
	if len(in.ByErrorKind) > 0 {
		out.ByErrorKind = make(map[string]contracts.HealingActionSpec, len(in.ByErrorKind))
		for k, v := range in.ByErrorKind {
			item := v
			item.Env = copyStringMap(v.Env)
			if v.Expectations != nil {
				exp := *v.Expectations
				if len(v.Expectations.Artifacts) > 0 {
					exp.Artifacts = append([]contracts.RecoveryExpectedArtifact(nil), v.Expectations.Artifacts...)
				}
				item.Expectations = &exp
			}
			out.ByErrorKind[k] = item
		}
	}
	return out
}
