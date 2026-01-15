// Package contracts defines shared workflow types.
// This file defines JobMeta types for the unified jobs queue, enabling
// gate/build metadata to be stored in jobs.meta JSONB.

package contracts

import (
	"encoding/json"
	"fmt"
)

// JobKind identifies the execution type for a job in the unified queue.
// All execution units (mods, gates, builds, healers) are now stored in
// the jobs table with their kind indicated by this field.
type JobKind string

const (
	// JobKindMod indicates a mod execution job (pre_gate, mod, post_gate, heal, re_gate).
	JobKindMod JobKind = "mod"
	// JobKindGate indicates a build gate validation job.
	JobKindGate JobKind = "gate"
	// JobKindBuild indicates a build tool invocation job (maven, gradle, npm, etc.).
	JobKindBuild JobKind = "build"
)

// Valid returns true if the job kind is a recognized value.
func (k JobKind) Valid() bool {
	switch k {
	case JobKindMod, JobKindGate, JobKindBuild:
		return true
	default:
		return false
	}
}

// JobMeta is the structured metadata stored in jobs.meta JSONB.
// It provides a unified schema for gate, build, and mod metadata,
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
// metadata section (gate/build) is populated. Mod jobs typically
// have kind="mod" with no gate or build metadata.
type JobMeta struct {
	// Kind identifies the job type: "mod", "gate", or "build".
	Kind JobKind `json:"kind"`

	// Gate contains build gate validation metadata when Kind is JobKindGate.
	// This includes static check results, log findings, and digest information.
	Gate *BuildGateStageMetadata `json:"gate,omitempty"`

	// Build contains build tool metadata when Kind is JobKindBuild.
	// This includes tool name, command, status details, and metrics.
	Build *BuildMeta `json:"build,omitempty"`

	// ModsStepName stores the user-defined step name from ModsSpec.Steps[i].Name
	// for mod jobs. Used by the CLI to display a friendly name in --follow mode.
	// Only populated for mod jobs (kind="mod") when a step name is provided.
	ModsStepName string `json:"mods_step_name,omitempty"`
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
	// Gate metadata is only valid for gate jobs.
	if m.Gate != nil && m.Kind != JobKindGate {
		return fmt.Errorf("gate metadata present but kind is %q", m.Kind)
	}
	// Build metadata is only valid for build jobs.
	if m.Build != nil && m.Kind != JobKindBuild {
		return fmt.Errorf("build metadata present but kind is %q", m.Kind)
	}
	// Validate nested gate metadata if present.
	if m.Gate != nil {
		if err := m.Gate.Validate(); err != nil {
			return fmt.Errorf("gate metadata invalid: %w", err)
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
		return nil, fmt.Errorf("job meta is nil; use NewModJobMeta/NewGateJobMeta/NewBuildJobMeta to create valid metadata")
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
// Use NewModJobMeta/NewGateJobMeta/NewBuildJobMeta to create valid metadata.
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

	// Require explicit kind field - no defaulting to mod for legacy payloads.
	if m.Kind == "" {
		return nil, fmt.Errorf("job meta missing required 'kind' field; must be one of: %q, %q, %q",
			JobKindMod, JobKindGate, JobKindBuild)
	}

	// Validate the unmarshaled metadata for structural correctness.
	if err := m.Validate(); err != nil {
		return nil, fmt.Errorf("unmarshal job meta: %w", err)
	}

	return &m, nil
}

// NewModJobMeta creates a JobMeta for mod execution jobs.
// This is a convenience constructor for the common case of mod jobs
// that don't carry gate or build metadata.
func NewModJobMeta() *JobMeta {
	return &JobMeta{Kind: JobKindMod}
}

// NewModJobMetaWithStepName creates a JobMeta for mod execution jobs
// with a user-defined step name. The step name is used by the CLI
// to display a friendly name in --follow mode.
func NewModJobMetaWithStepName(stepName string) *JobMeta {
	return &JobMeta{Kind: JobKindMod, ModsStepName: stepName}
}

// NewGateJobMeta creates a JobMeta for gate validation jobs.
// The gate metadata can be populated later via UpdateGateMeta.
func NewGateJobMeta(gate *BuildGateStageMetadata) *JobMeta {
	return &JobMeta{
		Kind: JobKindGate,
		Gate: gate,
	}
}

// NewBuildJobMeta creates a JobMeta for build tool invocation jobs.
func NewBuildJobMeta(build *BuildMeta) *JobMeta {
	return &JobMeta{
		Kind:  JobKindBuild,
		Build: build,
	}
}
