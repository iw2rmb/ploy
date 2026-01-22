package contracts

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

var (
	stepIDPattern   = regexp.MustCompile(`^[a-z0-9][a-z0-9\-]{2,63}$`)
	imagePattern    = regexp.MustCompile(`^[\w][\w./:@+-]{2,}$`)
	envKeyPattern   = regexp.MustCompile(`^[A-Z0-9_]+$`)
	stepInputNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9\-]{2,63}$`)
)

// StepManifest defines the execution contract for a single Mod step.
type StepManifest struct {
	ID         types.StepID
	Name       string
	Image      string
	Command    []string
	Args       []string
	WorkingDir string
	Env        map[string]string
	Inputs     []StepInput
	Outputs    []StepOutput
	Artifacts  []StepArtifact
	Gate       *StepGateSpec
	Resources  StepResourceSpec
	Retention  StepRetentionSpec
	// Options holds arbitrary run-specific options (e.g., GitLab PAT, MR flags).
	// Read options via OptionString/OptionBool helpers to avoid scattered type
	// assertions in callers. This field is not validated and values are never
	// logged.
	Options map[string]any
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

// StepOutput describes expected paths produced by the container.
type StepOutput struct {
	Name string
	Path string
	Type string
}

// StepArtifact describes an artifact emitted after execution.
type StepArtifact struct {
	Name string
	Type string
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
//  2. TargetRef — branch/tag for feature branches and PR flows.
//  3. BaseRef — fallback for baseline validations.
type StepGateSpec struct {
	Enabled bool
	Env     map[string]string

	// ImageOverrides holds mod-level image mapping overrides for gate execution.
	// These rules override the default mapping file.
	ImageOverrides []BuildGateImageRule

	// RepoURL is the Git repository URL for remote gate execution.
	// Populated from StartRunRequest.RepoURL when building manifests.
	RepoURL types.RepoURL

	// Ref is the Git reference (commit SHA, branch, or tag) for remote gate execution.
	// Derived from CommitSHA > TargetRef > BaseRef precedence when building manifests.
	Ref types.GitRef

	// DiffPatch is an optional gzipped unified diff (base64-encoded) to apply
	// on top of the cloned repo_url+ref baseline. Used by healing re-gates to
	// verify accumulated workspace changes without shipping full archives.
	//
	// Set by runGateWithHealing when executing re-gates after healing mods.
	// The diff captures all changes relative to the initial repo_url+ref clone.
	DiffPatch []byte

	// StackGate holds the Stack Gate configuration for this step.
	// Used for pre/post gate validation of stack expectations.
	StackGate *StepGateStackSpec
}

// StepGateStackSpec holds the effective Stack Gate configuration for a gate phase.
// This is threaded into manifests from the step's StackGateSpec.
type StepGateStackSpec struct {
	// Enabled controls whether Stack Gate validation is active for this phase.
	Enabled bool

	// Expect holds the stack expectations to validate.
	// Only validated when Enabled is true.
	Expect *StackExpectation
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

// StepRetentionSpec controls container and workspace retention.
type StepRetentionSpec struct {
	RetainContainer bool
	TTL             types.Duration
}

// Validate ensures the manifest is well-formed.
func (m StepManifest) Validate() error {
	if err := m.validateIdentity(); err != nil {
		return err
	}
	if err := m.validateEnv(); err != nil {
		return err
	}
	if err := m.validateInputs(); err != nil {
		return err
	}
	if err := m.validateGate(); err != nil {
		return err
	}
	if err := m.validateResources(); err != nil {
		return err
	}
	return m.validateRetention()
}

func (m StepManifest) validateIdentity() error {
	if m.ID.IsZero() {
		return errors.New("step manifest id required")
	}
	if !stepIDPattern.MatchString(m.ID.String()) {
		return fmt.Errorf("step manifest id invalid: %q", m.ID)
	}
	if strings.TrimSpace(m.Name) == "" {
		return errors.New("step manifest name required")
	}
	if strings.TrimSpace(m.Image) == "" {
		return errors.New("step manifest image required")
	}
	if !imagePattern.MatchString(m.Image) {
		return fmt.Errorf("step manifest image invalid: %q", m.Image)
	}
	if m.WorkingDir != "" && !filepath.IsAbs(m.WorkingDir) {
		return fmt.Errorf("step manifest working dir must be absolute: %q", m.WorkingDir)
	}
	return nil
}

func (m StepManifest) validateEnv() error {
	if len(m.Env) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m.Env))
	for key := range m.Env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if strings.TrimSpace(key) == "" {
			return errors.New("step manifest environment key required")
		}
		if !envKeyPattern.MatchString(key) {
			return fmt.Errorf("step manifest environment key invalid: %q", key)
		}
	}
	return nil
}

func (m StepManifest) validateInputs() error {
	if len(m.Inputs) == 0 {
		return errors.New("step manifest inputs required")
	}
	seenInputs := make(map[string]struct{}, len(m.Inputs))
	for idx, input := range m.Inputs {
		position := fmt.Sprintf("inputs[%d]", idx)
		if strings.TrimSpace(input.Name) == "" {
			return fmt.Errorf("%s name required", position)
		}
		if !stepInputNameRe.MatchString(input.Name) {
			return fmt.Errorf("%s name invalid: %q", position, input.Name)
		}
		if _, exists := seenInputs[input.Name]; exists {
			return fmt.Errorf("%s duplicate name %q", position, input.Name)
		}
		seenInputs[input.Name] = struct{}{}
		if strings.TrimSpace(input.MountPath) == "" {
			return fmt.Errorf("%s mount path required", position)
		}
		if !filepath.IsAbs(input.MountPath) {
			return fmt.Errorf("%s mount path must be absolute: %q", position, input.MountPath)
		}
		switch input.Mode {
		case StepInputModeReadOnly, StepInputModeReadWrite:
			// ok
		default:
			return fmt.Errorf("%s mount mode invalid: %q", position, input.Mode)
		}

		hasSnapshot := strings.TrimSpace(string(input.SnapshotCID)) != ""
		hasDiff := strings.TrimSpace(string(input.DiffCID)) != ""
		if input.Hydration != nil {
			if err := input.Hydration.validate(fmt.Sprintf("%s hydration", position)); err != nil {
				return err
			}
			if !hasSnapshot && !hasDiff && !input.Hydration.hasSource() {
				return fmt.Errorf("%s hydration requires base snapshot, diff, or repo metadata", position)
			}
		} else if hasSnapshot == hasDiff {
			// No hydration: must have exactly one of snapshot or diff.
			return fmt.Errorf("%s must reference exactly one source (snapshot or diff)", position)
		}
	}
	return nil
}

func (m StepManifest) validateGate() error {
	if m.Gate == nil {
		return nil
	}
	if !m.Gate.Enabled {
		return nil
	}
	for key := range m.Gate.Env {
		if !envKeyPattern.MatchString(key) {
			return fmt.Errorf("gate environment key invalid: %q", key)
		}
	}
	return nil
}

func (m StepManifest) validateResources() error {
	if err := m.Resources.CPU.Validate(); err != nil {
		return fmt.Errorf("resources cpu invalid: %w", err)
	}
	if err := m.Resources.Memory.Validate(); err != nil {
		return fmt.Errorf("resources memory invalid: %w", err)
	}
	if err := m.Resources.Disk.Validate(); err != nil {
		return fmt.Errorf("resources disk invalid: %w", err)
	}
	return nil
}

func (m StepManifest) validateRetention() error {
	if m.Retention.RetainContainer {
		if timeDur := int64(m.Retention.TTL); timeDur <= 0 {
			return errors.New("retention ttl required when retaining container")
		}
	}
	return nil
}

func (h StepInputHydration) hasSource() bool {
	if strings.TrimSpace(string(h.BaseSnapshot.CID)) != "" {
		return true
	}
	if len(h.Diffs) > 0 {
		return true
	}
	if h.Repo != nil && strings.TrimSpace(string(h.Repo.URL)) != "" {
		return true
	}
	return false
}

func (h StepInputHydration) validate(position string) error {
	hasBase := strings.TrimSpace(string(h.BaseSnapshot.CID)) != ""
	for idx, diff := range h.Diffs {
		if strings.TrimSpace(string(diff.CID)) == "" {
			return fmt.Errorf("%s diff[%d] cid required", position, idx)
		}
		if strings.TrimSpace(string(diff.Digest)) != "" {
			if err := diff.Digest.Validate(); err != nil {
				return fmt.Errorf("%s diff[%d] digest invalid: %v", position, idx, err)
			}
		}
	}
	if hasBase {
		if strings.TrimSpace(string(h.BaseSnapshot.Digest)) != "" {
			if err := h.BaseSnapshot.Digest.Validate(); err != nil {
				return fmt.Errorf("%s base snapshot digest invalid: %v", position, err)
			}
		}
	} else if len(h.Diffs) > 0 && (h.Repo == nil || strings.TrimSpace(string(h.Repo.URL)) == "") {
		return fmt.Errorf("%s base snapshot cid required when diffs are provided", position)
	}
	if h.Repo != nil {
		if err := h.Repo.Validate(); err != nil {
			return fmt.Errorf("%s repo invalid: %w", position, err)
		}
	}
	if !hasBase && len(h.Diffs) == 0 && (h.Repo == nil || strings.TrimSpace(string(h.Repo.URL)) == "") {
		return fmt.Errorf("%s requires base snapshot, diffs, or repo metadata", position)
	}
	return nil
}
