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

// TicketState mirrors Mods ticket lifecycle states exposed over the API.
type TicketState string

const (
	TicketStatePending    TicketState = "pending"
	TicketStateRunning    TicketState = "running"
	TicketStateSucceeded  TicketState = "succeeded"
	TicketStateFailed     TicketState = "failed"
	TicketStateCancelling TicketState = "cancelling"
	TicketStateCancelled  TicketState = "cancelled"
)

// StageDefinition defines a stage within the Mods ticket graph.
type StageDefinition struct {
	ID           domaintypes.StageID   `json:"id"`
	Dependencies []domaintypes.StageID `json:"dependencies,omitempty"`
	Lane         string                `json:"lane,omitempty"`
	Priority     string                `json:"priority,omitempty"`
	MaxAttempts  int                   `json:"max_attempts,omitempty"`
	Metadata     map[string]string     `json:"metadata,omitempty"`
}

// TicketSubmitRequest represents a ticket submission payload.
type TicketSubmitRequest struct {
	TicketID   domaintypes.TicketID `json:"ticket_id,omitempty"`
	Submitter  string               `json:"submitter,omitempty"`
	Repository string               `json:"repository,omitempty"`
	Metadata   map[string]string    `json:"metadata,omitempty"`
	Stages     []StageDefinition    `json:"stages"`
}

// TicketSubmitResponse returns the persisted ticket summary after submission.
type TicketSubmitResponse struct {
	Ticket TicketSummary `json:"ticket"`
}

// TicketStatusResponse returns the current ticket summary.
type TicketStatusResponse struct {
	Ticket TicketSummary `json:"ticket"`
}

// TicketSummary summarises ticket lifecycle state and associated stages.
type TicketSummary struct {
	TicketID   domaintypes.TicketID `json:"ticket_id"`
	State      TicketState          `json:"state"`
	Submitter  string               `json:"submitter,omitempty"`
	Repository string               `json:"repository,omitempty"`
	Metadata   map[string]string    `json:"metadata,omitempty"`
	CreatedAt  time.Time            `json:"created_at"`
	UpdatedAt  time.Time            `json:"updated_at"`
	// Stages is keyed by stage UUID (database id). Use StageStatus.StepIndex
	// when you need an ordered, user-facing step sequence.
	Stages map[string]StageStatus `json:"stages"`
}

// StageStatus summarises the execution state for a ticket stage.
type StageStatus struct {
	StageID     domaintypes.StageID `json:"stage_id"`
	State       StageState          `json:"state"`
	Attempts    int                 `json:"attempts"`
	MaxAttempts int                 `json:"max_attempts"`
	// JobID is a simple string-typed identifier for execution jobs.
	// JSON representation remains a plain string for compatibility.
	CurrentJobID JobID             `json:"current_job_id,omitempty"`
	Artifacts    map[string]string `json:"artifacts,omitempty"`
	LastError    string            `json:"last_error,omitempty"`
	// StepIndex identifies the position of this stage in multi-step Mods runs.
	// For single-step runs, this is 0. For multi-step runs (mods[]), this is
	// the array index (0, 1, 2, ...). The control plane uses this to order
	// stages when rehydrating workspaces with diffs from prior steps. It is
	// populated from stages.meta.step_index and matches run_steps.step_index
	// and diffs.step_index for this stage.
	StepIndex int `json:"step_index,omitempty"`
}

// JobID identifies a job within the Mods execution context.
// Kept as a plain string type; JSON remains a string for compatibility.
type JobID string

// StageMetadata captures step-level metadata stored in stages.meta JSONB.
// This metadata enables the control plane to treat a run as an ordered
// sequence of steps for multi-step Mods runs (mods[] array in spec). It is
// serialized directly into stages.meta JSONB and reloaded via GET /v1/mods/{id}.
type StageMetadata struct {
	// StepIndex is the 0-based position of this stage in the run's step sequence.
	// For single-step runs, this is 0. For multi-step runs, this matches the
	// index in the mods[] array (0, 1, 2, ...).
	StepIndex int `json:"step_index"`
	// StepTotal is the total number of steps in this run.
	// For single-step runs, this is 1. For multi-step runs, this is len(mods[]).
	StepTotal int `json:"step_total"`
	// ModImage is the container image for this step (optional, for diagnostics).
	ModImage string `json:"mod_image,omitempty"`
}
