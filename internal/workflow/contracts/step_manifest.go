package contracts

import (
	"fmt"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

// StepManifest defines the execution contract for a single Mig step.
type StepManifest struct {
	ID         types.StepID
	Name       string
	Image      string
	Command    []string
	WorkingDir string
	Envs       map[string]string
	Inputs     []StepInput
	Gate       *StepGateSpec
	Resources  StepResourceSpec
	// Options holds arbitrary run-specific options.
	// Read options via OptionString/OptionBool helpers to avoid scattered type
	// assertions in callers. This field is not validated and values are never
	// logged.
	Options map[string]any

	// In lists canonical read-only input entries ("shortHash:/in/dst").
	In []string

	// Out lists canonical read-write output entries ("shortHash:/out/dst").
	Out []string

	// Home lists canonical home-relative entries ("shortHash:dst{:ro}").
	Home []string

	// Tmp lists canonical writable temporary entries ("shortHash:/tmp/dst").
	Tmp []string

	// BundleMap maps content hashes to spec bundle download identifiers.
	// The nodeagent uses this to resolve Hydra entry hashes → bundleIDs
	// for resource download during staged materialization.
	BundleMap map[string]string
}

// StepInputMode describes how the input is mounted into the container.
type StepInputMode string

const (
	// StepInputModeReadOnly mounts the input read-only.
	StepInputModeReadOnly StepInputMode = "ro"
	// StepInputModeReadWrite mounts the input read-write.
	StepInputModeReadWrite StepInputMode = "rw"
)

// StepInput describes repository state presented to the container.
type StepInput struct {
	Name        string
	MountPath   string
	Mode        StepInputMode
	SnapshotCID types.CID
	DiffCID     types.CID
	Hydration   *StepInputHydration
}

// StepGateSpec configures Build Gate validation post step execution.
//
// The RepoURL and Ref fields provide repo metadata for HTTP-based gate execution,
// enabling remote Build Gate workers to clone and validate the repository without
// requiring direct workspace access. These fields are populated from the run's
// StartRunRequest and threaded through manifests.
//
// Ref precedence (set by manifest builders):
//  1. CommitSHA — pinned commit when available (ensures deterministic validation).
//  2. BaseRef — fallback for baseline validations.
type StepGateSpec struct {
	Enabled bool
	Envs    map[string]string

	// ImageOverrides holds mig-level image mapping overrides for gate execution.
	// These rules override the default mapping file.
	ImageOverrides []BuildGateImageRule

	// Stack configures stack detection behavior for this gate.
	// mode=forced uses the configured language/release and detects tool when omitted.
	// mode=strict requires detection to match the configured stack.
	// mode=fallback uses detection when complete, otherwise the configured stack.
	Stack *BuildGateStackConfig

	// RepoURL is the Git repository URL for remote gate execution.
	// Populated from StartRunRequest.RepoURL when building manifests.
	RepoURL types.RepoURL

	// RepoID is the repo identifier for this execution.
	// Used for gate job/runtime correlation.
	RepoID types.RepoID

	// Ref is the Git reference (commit SHA, branch, or tag) for remote gate execution.
	// Derived from CommitSHA > BaseRef precedence when building manifests.
	Ref types.GitRef

	// DiffPatch is an optional gzipped unified diff (base64-encoded) to apply
	// on top of the cloned repo_url+ref baseline, avoiding full archive uploads.
	// The diff captures all changes relative to the initial repo_url+ref clone.
	DiffPatch []byte

	// StackGatePhase holds the Stack Gate configuration for this step.
	// Used for pre/post gate validation of stack expectations.
	StackGatePhase *StackGatePhaseSpec
}

// StepInputHydration describes how to materialise repository state for an input.
type StepInputHydration struct {
	BaseSnapshot StepInputArtifactRef   `json:"base_snapshot,omitempty"`
	Diffs        []StepInputArtifactRef `json:"diffs,omitempty"`
	Repo         *RepoMaterialization   `json:"repo,omitempty"`
}

// StepInputArtifactRef references a snapshot or diff artifact.
type StepInputArtifactRef struct {
	CID    types.CID          `json:"cid"`
	Digest types.Sha256Digest `json:"digest,omitempty"`
	Size   int64              `json:"size,omitempty"`
}

// StepResourceSpec captures runtime resource hints.
type StepResourceSpec struct {
	CPU    types.CPUmilli
	Memory types.Bytes
	Disk   types.Bytes
	GPU    string
}

// ToLimits converts the resource hints into concrete container limit values.
//
// Returns Docker‑compatible quantities:
//   - nanoCPUs: 1e9 per CPU (millis → nanos via CPUmilli.DockerNanoCPUs).
//   - memoryBytes: raw bytes for memory limit (0 means unlimited).
//   - diskBytes: raw bytes for writable layer limit (best‑effort; 0 means unlimited).
//   - storageSizeOpt: string form for Docker storage option "size" when supported
//     by the storage driver (empty when unlimited).
func (s StepResourceSpec) ToLimits() (nanoCPUs int64, memoryBytes int64, diskBytes int64, storageSizeOpt string) {
	if s.CPU > 0 {
		nanoCPUs = s.CPU.DockerNanoCPUs()
	}
	if s.Memory > 0 {
		memoryBytes = s.Memory.DockerMemoryBytes()
	}
	if s.Disk > 0 {
		diskBytes = s.Disk.DockerMemoryBytes()
		// Docker expects a string for the storage size option. Use raw bytes.
		storageSizeOpt = fmt.Sprintf("%d", diskBytes)
	}
	return
}

// OptionString returns the option value for key when it is of type string.
// It returns the string and true on exact type match; otherwise it returns
// an empty string and false. The lookup is safe on a zero-value manifest
// or when Options is nil.
func (m StepManifest) OptionString(key string) (string, bool) {
	if m.Options == nil {
		return "", false
	}
	v, ok := m.Options[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// OptionBool returns the option value for key when it is of type bool.
// It returns the bool and true on exact type match; otherwise it returns
// false and false. The lookup is safe on a zero-value manifest or when
// Options is nil.
func (m StepManifest) OptionBool(key string) (bool, bool) {
	if m.Options == nil {
		return false, false
	}
	v, ok := m.Options[key]
	if !ok {
		return false, false
	}
	b, ok := v.(bool)
	return b, ok
}
