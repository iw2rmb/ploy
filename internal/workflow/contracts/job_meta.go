// Package contracts defines shared workflow types.
// This file defines JobMeta types for the unified jobs queue, enabling
// gate/build metadata to be stored in jobs.meta JSONB.

package contracts

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// JobKind identifies the execution type for a job in the unified queue.
// All execution units (migs, gates, builds, healers) are now stored in
// the jobs table with their kind indicated by this field.
type JobKind string

const (
	// JobKindMig indicates a mig execution job (pre_gate, mig, post_gate, heal, re_gate).
	JobKindMig JobKind = "mig"
	// JobKindGate indicates a build gate validation job.
	JobKindGate JobKind = "gate"
	// JobKindBuild indicates a build tool invocation job (maven, gradle, npm, etc.).
	JobKindBuild JobKind = "build"
)

// Valid returns true if the job kind is a recognized value.
func (k JobKind) Valid() bool {
	switch k {
	case JobKindMig, JobKindGate, JobKindBuild:
		return true
	default:
		return false
	}
}

// JobMeta is the structured metadata stored in jobs.meta JSONB.
// It provides a unified schema for gate, build, and mig metadata,
// enabling the jobs table to serve as the single execution primitive
// for all workflow stages.
//
// JSON shape example:
//
//	{
//	  "kind": "gate",
//	  "gate": { "log_digest": "...", "static_checks": [...] },
//	  "build": null
//	}
//
// The kind field is always present and determines which optional
// metadata section (gate/build) is populated. Mig jobs typically
// have kind="mig" with no gate or build metadata.
type JobMeta struct {
	// Kind identifies the job type: "mig", "gate", or "build".
	Kind JobKind `json:"kind"`

	// GateMetadata contains build gate validation metadata when Kind is JobKindGate.
	// This includes static check results, log findings, and digest information.
	GateMetadata *BuildGateStageMetadata `json:"gate,omitempty"`

	// Build contains build tool metadata when Kind is JobKindBuild.
	// This includes tool name, command, status details, and metrics.
	Build *BuildMeta `json:"build,omitempty"`

	// MigStepName stores the user-defined step name from MigSpec.Steps[i].Name
	// for mig jobs. Used by the CLI to display a friendly name in --follow mode.
	// Only populated for mig jobs (kind="mig") when a step name is provided.
	MigStepName string `json:"mig_step_name,omitempty"`

	// HookSource stores the fully resolved hook manifest source for hook jobs.
	// Populated for hook jobs (kind="mig") so claim-time runtime does not need to
	// re-resolve source entries that may expand from directories.
	HookSource string `json:"hook_source,omitempty"`

	// ActionSummary is a short one-line description of what the healing mig did,
	// produced by the healing container. Only allowed for mig jobs (kind="mig").
	// Max 200 chars, no newlines.
	ActionSummary string `json:"action_summary,omitempty"`

	// Heal contains structured healing output extracted from /out/heal.json.
	// Only allowed for mig jobs (kind="mig").
	Heal *HealJobMetadata `json:"heal,omitempty"`

	// RecoveryMetadata stores universal recovery loop metadata.
	// Allowed for gate and mig jobs; rejected for build jobs.
	RecoveryMetadata *RecoveryJobMetadata `json:"recovery,omitempty"`

	// SBOM stores cycle context for sbom jobs and related runtime planning.
	// Allowed for mig jobs (sbom/hook job types are mig-kind metadata).
	SBOM *SBOMJobMetadata `json:"sbom,omitempty"`

	// CacheMirror links a replayed target job to its source job.
	// When present, consumers must read content (logs/artifacts/diffs/SBOM rows)
	// through source_job_id so replayed jobs are transparent to orchestration.
	CacheMirror *CacheMirrorMetadata `json:"cache_mirror,omitempty"`
}

// CacheMirrorMetadata captures replay-source linkage for transparent reads.
type CacheMirrorMetadata struct {
	SourceJobID domaintypes.JobID `json:"source_job_id,omitempty"`
}

// RecoveryJobMetadata captures loop context persisted at job level.
type RecoveryJobMetadata = BuildGateRecoveryMetadata

// HealJobMetadata captures structured healing details emitted by the healer.
type HealJobMetadata struct {
	BugSummary    string `json:"bug_summary,omitempty"`
	ActionSummary string `json:"action_summary,omitempty"`
	ErrorKind     string `json:"error_kind,omitempty"`
}

const (
	// SBOMPhasePre identifies pre-gate sbom cycle context.
	SBOMPhasePre = "pre"
	// SBOMPhasePost identifies post/re-gate sbom cycle context.
	SBOMPhasePost = "post"
)

const (
	// SBOMRoleInitial is the initial sbom before pre/post gate.
	SBOMRoleInitial = "initial"
	// SBOMRoleRetry is a retry sbom after sbom-heal recovery.
	SBOMRoleRetry = "retry"
	// SBOMRoleFinal is the final sbom placed after recovered gate flow.
	SBOMRoleFinal = "final"
)

// SBOMJobMetadata captures sbom cycle context so runtime logic does not depend
// on job naming patterns.
type SBOMJobMetadata struct {
	// Phase is one of: pre, post.
	Phase string `json:"phase,omitempty"`
	// CycleName is the concrete gate cycle identifier used for sbom staging.
	// Examples: pre-gate, post-gate, re-gate-1.
	CycleName string `json:"cycle_name,omitempty"`
	// Role is one of: initial, retry, final.
	Role string `json:"role,omitempty"`
	// RootJobID is the stable sbom chain root used for retry accounting.
	RootJobID string `json:"root_job_id,omitempty"`
}

// BuildMeta captures metadata for build tool invocations stored in jobs.meta.
// This consolidates fields previously tracked in the separate builds table.
type BuildMeta struct {
	// Tool identifies the build tool (e.g., "maven", "gradle", "npm", "bazel").
	Tool string `json:"tool,omitempty"`
	// Command is the full command line executed (for diagnostics).
	Command string `json:"command,omitempty"`
	// StatusDetails provides additional context on build outcome.
	StatusDetails string `json:"status_details,omitempty"`
	// Metrics contains arbitrary build metrics (e.g., compilation time, test count).
	Metrics map[string]interface{} `json:"metrics,omitempty"`
}

// Validate ensures JobMeta is well-formed.
func (m JobMeta) Validate() error {
	if !m.Kind.Valid() {
		return fmt.Errorf("invalid job kind: %q", m.Kind)
	}
	// GateMetadata is only valid for gate jobs.
	if m.GateMetadata != nil && m.Kind != JobKindGate {
		return fmt.Errorf("gate metadata present but kind is %q", m.Kind)
	}
	// Build metadata is only valid for build jobs.
	if m.Build != nil && m.Kind != JobKindBuild {
		return fmt.Errorf("build metadata present but kind is %q", m.Kind)
	}
	// Validate nested gate metadata if present.
	if m.GateMetadata != nil {
		if err := m.GateMetadata.Validate(); err != nil {
			return fmt.Errorf("gate metadata invalid: %w", err)
		}
	}
	// RecoveryMetadata is allowed for gate and mig jobs only.
	if m.RecoveryMetadata != nil {
		if m.Kind != JobKindGate && m.Kind != JobKindMig {
			return fmt.Errorf("recovery metadata present but kind is %q (only allowed for %q or %q)", m.Kind, JobKindGate, JobKindMig)
		}
		if err := m.RecoveryMetadata.Validate(); err != nil {
			return fmt.Errorf("recovery metadata invalid: %w", err)
		}
	}
	// ActionSummary is only valid for mig jobs.
	if m.ActionSummary != "" {
		if m.Kind != JobKindMig {
			return fmt.Errorf("action_summary present but kind is %q (only allowed for %q)", m.Kind, JobKindMig)
		}
		if strings.ContainsAny(m.ActionSummary, "\n\r") {
			return fmt.Errorf("action_summary: must be single-line")
		}
		if utf8.RuneCountInString(m.ActionSummary) > 200 {
			return fmt.Errorf("action_summary: must be at most 200 characters, got %d", utf8.RuneCountInString(m.ActionSummary))
		}
	}
	if m.HookSource != "" && m.Kind != JobKindMig {
		return fmt.Errorf("hook_source present but kind is %q (only allowed for %q)", m.Kind, JobKindMig)
	}
	if m.SBOM != nil {
		if m.Kind != JobKindMig {
			return fmt.Errorf("sbom metadata present but kind is %q (only allowed for %q)", m.Kind, JobKindMig)
		}
		if err := m.SBOM.Validate(); err != nil {
			return fmt.Errorf("sbom metadata invalid: %w", err)
		}
	}
	if m.Heal != nil {
		if m.Kind != JobKindMig {
			return fmt.Errorf("heal metadata present but kind is %q (only allowed for %q)", m.Kind, JobKindMig)
		}
		if err := m.Heal.Validate(); err != nil {
			return fmt.Errorf("heal metadata invalid: %w", err)
		}
	}
	if m.CacheMirror != nil {
		if err := m.CacheMirror.Validate(); err != nil {
			return fmt.Errorf("cache_mirror invalid: %w", err)
		}
	}
	return nil
}

// Validate ensures CacheMirrorMetadata is well-formed.
func (m CacheMirrorMetadata) Validate() error {
	if m.SourceJobID.IsZero() {
		return fmt.Errorf("source_job_id: required")
	}
	return nil
}

// Validate ensures SBOMJobMetadata is well-formed.
func (m SBOMJobMetadata) Validate() error {
	if m.Phase != "" && m.Phase != SBOMPhasePre && m.Phase != SBOMPhasePost {
		return fmt.Errorf("phase invalid: %q", m.Phase)
	}
	if strings.ContainsAny(m.CycleName, "\n\r\t") {
		return fmt.Errorf("cycle_name: must not contain control whitespace")
	}
	if strings.TrimSpace(m.CycleName) != m.CycleName {
		return fmt.Errorf("cycle_name: must not have leading or trailing whitespace")
	}
	if m.Role != "" && m.Role != SBOMRoleInitial && m.Role != SBOMRoleRetry && m.Role != SBOMRoleFinal {
		return fmt.Errorf("role invalid: %q", m.Role)
	}
	if strings.ContainsAny(m.RootJobID, "\n\r\t ") {
		return fmt.Errorf("root_job_id: must not contain whitespace")
	}
	return nil
}

// Validate ensures HealJobMetadata is well-formed.
func (m HealJobMetadata) Validate() error {
	validateLine := func(name, value string) error {
		if value == "" {
			return nil
		}
		if strings.ContainsAny(value, "\n\r") {
			return fmt.Errorf("%s: must be single-line", name)
		}
		if utf8.RuneCountInString(value) > 200 {
			return fmt.Errorf("%s: must be at most 200 characters, got %d", name, utf8.RuneCountInString(value))
		}
		return nil
	}

	if err := validateLine("bug_summary", m.BugSummary); err != nil {
		return err
	}
	if err := validateLine("action_summary", m.ActionSummary); err != nil {
		return err
	}
	if err := validateLine("error_kind", m.ErrorKind); err != nil {
		return err
	}
	if m.ErrorKind != "" {
		if _, ok := ParseRecoveryErrorKind(m.ErrorKind); !ok {
			return fmt.Errorf("error_kind invalid: %q", m.ErrorKind)
		}
	}
	return nil
}

// MarshalJobMeta encodes a JobMeta struct to JSON bytes suitable for
// storing in jobs.meta JSONB.
//
// Returns an error if m is nil or if the metadata fails validation.
// Callers must provide a valid JobMeta with a recognized Kind field.
func MarshalJobMeta(m *JobMeta) ([]byte, error) {
	if m == nil {
		return nil, fmt.Errorf("job meta is nil; use NewMigJobMeta/NewGateJobMeta/NewBuildJobMeta to create valid metadata")
	}
	// Validate before marshaling to catch invalid state early.
	if err := m.Validate(); err != nil {
		return nil, fmt.Errorf("marshal job meta: %w", err)
	}
	return json.Marshal(m)
}

// UnmarshalJobMeta decodes JSON bytes from jobs.meta JSONB into a JobMeta struct.
//
// Returns an error for invalid payloads:
//   - Empty bytes, empty JSON objects ("{}"), or JSON null are rejected.
//   - Missing or invalid "kind" field is rejected.
//   - Payloads that fail JobMeta.Validate() are rejected.
//
// All job metadata must now be structured with an explicit kind field.
// Use NewMigJobMeta/NewGateJobMeta/NewBuildJobMeta to create valid metadata.
func UnmarshalJobMeta(data []byte) (*JobMeta, error) {
	// Reject empty/null payloads - structured metadata is now required.
	if len(data) == 0 {
		return nil, fmt.Errorf("job meta is empty; structured metadata with kind field is required")
	}
	s := string(data)
	if s == "{}" || s == "null" {
		return nil, fmt.Errorf("job meta is %s; structured metadata with kind field is required", s)
	}

	var m JobMeta
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("unmarshal job meta: %w", err)
	}

	// Require explicit kind field - no defaulting to mig for legacy payloads.
	if m.Kind == "" {
		return nil, fmt.Errorf("job meta missing required 'kind' field; must be one of: %q, %q, %q",
			JobKindMig, JobKindGate, JobKindBuild)
	}

	// Validate the unmarshaled metadata for structural correctness.
	if err := m.Validate(); err != nil {
		return nil, fmt.Errorf("unmarshal job meta: %w", err)
	}

	return &m, nil
}

// NewMigJobMeta creates a JobMeta for mig execution jobs.
// This is a convenience constructor for the common case of mig jobs
// that don't carry gate or build metadata.
func NewMigJobMeta() *JobMeta {
	return &JobMeta{Kind: JobKindMig}
}

// NewMigJobMetaWithStepName creates a JobMeta for mig execution jobs
// with a user-defined step name. The step name is used by the CLI
// to display a friendly name in --follow mode.
func NewMigJobMetaWithStepName(stepName string) *JobMeta {
	return &JobMeta{Kind: JobKindMig, MigStepName: stepName}
}

// NewGateJobMeta creates a JobMeta for gate validation jobs.
// The gate metadata can be populated later via UpdateGateMeta.
func NewGateJobMeta(gate *BuildGateStageMetadata) *JobMeta {
	return &JobMeta{
		Kind:         JobKindGate,
		GateMetadata: gate,
	}
}

// NewBuildJobMeta creates a JobMeta for build tool invocation jobs.
func NewBuildJobMeta(build *BuildMeta) *JobMeta {
	return &JobMeta{
		Kind:  JobKindBuild,
		Build: build,
	}
}
