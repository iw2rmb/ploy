package api

import (
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// Package api defines the Mods ticket and stage types shared by the CLI,
// control plane (/v1/mods*) and SSE hub. JSON tags mirror the wire shape.

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

// RunState mirrors Mods ticket lifecycle states exposed over the API.
type RunState string

const (
	RunStatePending    RunState = "pending"
	RunStateRunning    RunState = "running"
	RunStateSucceeded  RunState = "succeeded"
	RunStateFailed     RunState = "failed"
	RunStateCancelling RunState = "cancelling"
	RunStateCancelled  RunState = "cancelled"
)

// StageDefinition defines a stage within the Mods ticket graph.
type StageDefinition struct {
	ID           string            `json:"id"`
	Dependencies []string          `json:"dependencies,omitempty"`
	Lane         string            `json:"lane,omitempty"`
	Priority     string            `json:"priority,omitempty"`
	MaxAttempts  int               `json:"max_attempts,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// RunSubmitRequest represents a ticket submission payload.
type RunSubmitRequest struct {
	TicketID   domaintypes.TicketID `json:"run_id,omitempty"`
	Submitter  string               `json:"submitter,omitempty"`
	Repository string               `json:"repository,omitempty"`
	Metadata   map[string]string    `json:"metadata,omitempty"`
	Stages     []StageDefinition    `json:"stages"`
}

// RunSubmitResponse returns the persisted ticket summary after submission.
type RunSubmitResponse struct {
	Ticket RunSummary `json:"ticket"`
}

// RunStatusResponse returns the current ticket summary.
type RunStatusResponse struct {
	Ticket RunSummary `json:"ticket"`
}

// RunSummary summarises ticket lifecycle state and associated stages.
type RunSummary struct {
	TicketID   domaintypes.TicketID `json:"run_id"`
	State      RunState             `json:"state"`
	Submitter  string               `json:"submitter,omitempty"`
	Repository string               `json:"repository,omitempty"`
	Metadata   map[string]string    `json:"metadata,omitempty"`
	CreatedAt  time.Time            `json:"created_at"`
	UpdatedAt  time.Time            `json:"updated_at"`
	// Stages is keyed by job ID (KSUID string from jobs.id). Each entry represents
	// a row from the `jobs` table. The field name "stages" is retained for API
	// backward compatibility. Use StageStatus.StepIndex (mirrors jobs.step_index)
	// for ordered step sequencing.
	Stages map[string]StageStatus `json:"stages"`
}

// StageStatus summarises the execution state for a job (called "stage" for API
// backward compatibility). Each StageStatus maps to a row in the `jobs` table.
type StageStatus struct {
	State       StageState `json:"state"`
	Attempts    int        `json:"attempts"`
	MaxAttempts int        `json:"max_attempts"`
	// CurrentJobID is the job identifier for execution jobs.
	CurrentJobID domaintypes.JobID `json:"current_job_id,omitempty"`
	Artifacts    map[string]string `json:"artifacts,omitempty"`
	LastError    string            `json:"last_error,omitempty"`
	// StepIndex mirrors jobs.step_index and is used to order jobs in multi-step
	// Mods runs. Float values (1000, 2000, 3000) allow dynamic insertion of
	// healing jobs at midpoints (e.g., 1500, 1750). The control plane uses this
	// to order jobs when rehydrating workspaces with diffs from prior steps.
	// Diffs are fetched ordered by jobs.step_index for correct rehydration.
	StepIndex int `json:"step_index,omitempty"`
}

// StageMetadata captures job-level metadata stored in jobs.meta JSONB.
// This metadata enables the control plane to treat a run as an ordered
// sequence of jobs for multi-step Mods runs (mods[] array in spec). It is
// serialized directly into jobs.meta JSONB and exposed via GET /v1/mods/{id}.
//
// Each execution unit (pre_gate, mod, post_gate, heal, re_gate) has a jobs row
// with mod_type identifying the phase type. Float step_index on the jobs table
// provides ordering for execution; metadata does not duplicate step index or
// total step count.
type StageMetadata struct {
	// ModType identifies the job phase: "pre_gate", "mod", "post_gate", "heal", or "re_gate".
	ModType string `json:"mod_type,omitempty"`
	// ModImage is the container image for this job (optional, for diagnostics).
	ModImage string `json:"mod_image,omitempty"`
}
