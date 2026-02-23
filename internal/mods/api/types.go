package api

import (
	"encoding/json"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// Package api defines the Mods run and stage types shared by the CLI,
// control plane (/v1/runs + legacy /v1/mods/{id}*) and SSE hub. JSON tags mirror the wire shape.

// StageState mirrors Mods stage lifecycle states exposed over the API.
type StageState string

const (
	StageStatePending    StageState = "pending"
	StageStateQueued     StageState = "queued"
	StageStateRunning    StageState = "running"
	StageStateSucceeded  StageState = "succeeded"
	StageStateFailed     StageState = "failed"
	StageStateCancelling StageState = "cancelling"
	StageStateCancelled  StageState = "cancelled"
)

// RunState mirrors Mods run lifecycle states exposed over the API.
type RunState string

const (
	RunStatePending    RunState = "pending"
	RunStateRunning    RunState = "running"
	RunStateSucceeded  RunState = "succeeded"
	RunStateFailed     RunState = "failed"
	RunStateCancelling RunState = "cancelling"
	RunStateCancelled  RunState = "cancelled"
)

// RunSubmitRequest represents the submit payload for POST /v1/runs.
//
// This mirrors docs/api/components/schemas/controlplane.yaml#/RunSubmitRequest:
//   - repo_url: Git repository URL (https/ssh/file)
//   - base_ref: base Git ref (branch or tag)
//   - target_ref: target Git ref (branch or tag)
//   - spec: arbitrary JSON spec (YAML/JSON from CLI after normalisation)
//   - created_by: optional submitter identifier (email, CI job, etc.)
type RunSubmitRequest struct {
	RepoURL   domaintypes.RepoURL `json:"repo_url"`
	BaseRef   domaintypes.GitRef  `json:"base_ref"`
	TargetRef domaintypes.GitRef  `json:"target_ref"`
	Spec      json.RawMessage     `json:"spec"`
	CreatedBy string              `json:"created_by,omitempty"`
}

// RunSummary is the canonical response type for GET /v1/runs/{id}/status (status)
// and SSE `event: run` payloads. No wrapper types are used — the JSON shape is
// RunSummary directly (run_id, state, stages, etc.).
//
// RunSummary summarises run lifecycle state and associated stages.
// The RunID field uses domaintypes.RunID and marshals as "run_id" in JSON.
type RunSummary struct {
	RunID      domaintypes.RunID `json:"run_id"`
	State      RunState          `json:"state"`
	Submitter  string            `json:"submitter,omitempty"`
	Repository string            `json:"repository,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	CreatedAt  time.Time         `json:"created_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
	// Stages is keyed by job ID (KSUID string from jobs.id). Each entry represents
	// a row from the `jobs` table. Use StageStatus.NextID to follow successor links
	// in the execution chain.
	Stages map[domaintypes.JobID]StageStatus `json:"stages"`
}

// StageStatus summarises the execution state for a job.
// Each StageStatus maps to a row in the `jobs` table.
type StageStatus struct {
	State       StageState `json:"state"`
	Attempts    int        `json:"attempts"`
	MaxAttempts int        `json:"max_attempts"`
	// CurrentJobID is the job identifier for execution jobs.
	CurrentJobID domaintypes.JobID `json:"current_job_id,omitempty"`
	Artifacts    map[string]string `json:"artifacts,omitempty"`
	LastError    string            `json:"last_error,omitempty"`
	// NextID points to the successor job in the run chain. Nil means this stage
	// is currently a chain tail.
	NextID *domaintypes.JobID `json:"next_id,omitempty"`
}

// StageMetadata captures job-level metadata for Mods runs.
// It mirrors information exposed via GET /v1/runs/{id}/status and is derived from
// jobs and related artifacts rather than being tied to a specific storage
// layout for jobs.meta JSONB.
//
// Each execution unit (pre_gate, mod, post_gate, heal, re_gate) has a jobs row
// with job_type identifying the phase type.
type StageMetadata struct {
	// JobType identifies the job phase: "pre_gate", "mod", "post_gate", "heal", or "re_gate".
	JobType domaintypes.ModType `json:"job_type,omitempty"`
	// JobImage is the container image for this job (optional, for diagnostics).
	JobImage string `json:"job_image,omitempty"`
}
