package mods

import "time"

// StageState enumerates per-stage lifecycle states within the Mods orchestrator.
type StageState string

const (
	// StageStatePending indicates the stage is waiting for dependencies.
	StageStatePending StageState = "pending"
	// StageStateQueued indicates the stage has been enqueued as a job.
	StageStateQueued StageState = "queued"
	// StageStateRunning indicates the stage is currently executing.
	StageStateRunning StageState = "running"
	// StageStateSucceeded indicates the stage completed successfully.
	StageStateSucceeded StageState = "succeeded"
	// StageStateFailed indicates the stage exhausted retries and failed.
	StageStateFailed StageState = "failed"
	// StageStateCancelling indicates the stage is in the process of cancelling.
	StageStateCancelling StageState = "cancelling"
	// StageStateCancelled indicates the stage was cancelled before completion.
	StageStateCancelled StageState = "cancelled"
)

// TicketState enumerates aggregate lifecycle states for a Mods ticket.
type TicketState string

const (
	// TicketStatePending indicates the ticket is awaiting execution.
	TicketStatePending TicketState = "pending"
	// TicketStateRunning indicates at least one stage is executing.
	TicketStateRunning TicketState = "running"
	// TicketStateSucceeded indicates all stages have succeeded.
	TicketStateSucceeded TicketState = "succeeded"
	// TicketStateFailed indicates one or more stages failed with no retries left.
	TicketStateFailed TicketState = "failed"
	// TicketStateCancelling indicates a cancellation request is in-flight.
	TicketStateCancelling TicketState = "cancelling"
	// TicketStateCancelled indicates the ticket was cancelled.
	TicketStateCancelled TicketState = "cancelled"
)

// StageDefinition describes a single Mods stage within a ticket stage graph.
type StageDefinition struct {
	ID           string            `json:"id"`
	Dependencies []string          `json:"dependencies,omitempty"`
	Lane         string            `json:"lane,omitempty"`
	Priority     string            `json:"priority,omitempty"`
	MaxAttempts  int               `json:"max_attempts,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// TicketSpec captures submission inputs for creating a Mods ticket.
type TicketSpec struct {
    TicketID   string
    Submitter  string
    Repository string
    Stages     []StageDefinition
    Metadata   map[string]string
}

// TicketStatus summarises ticket lifecycle state for callers.
type TicketStatus struct {
    TicketID   string
    State      TicketState
    Submitter  string
    Repository string
    Metadata   map[string]string
    CreatedAt  time.Time
    UpdatedAt  time.Time
    Stages     map[string]StageStatus
}

// StageStatus surfaces the current execution state for a single stage.
type StageStatus struct {
	StageID      string
	State        StageState
	Attempts     int
	MaxAttempts  int
	CurrentJobID string
	Version      int64
	Artifacts    map[string]string
	LastError    string
}

// ClaimStageRequest scopes a request to claim a queued stage.
type ClaimStageRequest struct {
	TicketID string
	StageID  string
	JobID    string
	NodeID   string
}

// StageJobSpec contains scheduler submission metadata for a stage job.
type StageJobSpec struct {
	JobID        string
	TicketID     string
	StageID      string
	Priority     string
	MaxAttempts  int
	RetryAttempt int
	Metadata     map[string]string
}

// StageJob identifies a scheduler job created for a stage.
type StageJob struct {
	JobID    string
	TicketID string
	StageID  string
}

// JobCompletionState represents a terminal scheduler state surfaced to the orchestrator.
type JobCompletionState string

const (
	// JobCompletionSucceeded signals a stage finished successfully.
	JobCompletionSucceeded JobCompletionState = "succeeded"
	// JobCompletionFailed signals a stage failed during execution.
	JobCompletionFailed JobCompletionState = "failed"
	// JobCompletionCancelled signals a stage was cancelled mid-flight.
	JobCompletionCancelled JobCompletionState = "cancelled"
)

// JobCompletion conveys job terminal state, artifacts, and errors.
type JobCompletion struct {
	TicketID  string
	StageID   string
	JobID     string
	State     JobCompletionState
	Error     string
	Artifacts map[string]string
}
