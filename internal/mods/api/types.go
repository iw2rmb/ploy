package api

import "time"

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
	ID           string            `json:"id"`
	Dependencies []string          `json:"dependencies,omitempty"`
	Lane         string            `json:"lane,omitempty"`
	Priority     string            `json:"priority,omitempty"`
	MaxAttempts  int               `json:"max_attempts,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// TicketSubmitRequest represents a ticket submission payload.
type TicketSubmitRequest struct {
	TicketID   string            `json:"ticket_id"`
	Submitter  string            `json:"submitter,omitempty"`
	Repository string            `json:"repository,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	Stages     []StageDefinition `json:"stages"`
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
	TicketID   string                 `json:"ticket_id"`
	State      TicketState            `json:"state"`
	Submitter  string                 `json:"submitter,omitempty"`
	Repository string                 `json:"repository,omitempty"`
	Metadata   map[string]string      `json:"metadata,omitempty"`
	CreatedAt  time.Time              `json:"created_at"`
	UpdatedAt  time.Time              `json:"updated_at"`
	Stages     map[string]StageStatus `json:"stages"`
}

// StageStatus summarises the execution state for a ticket stage.
type StageStatus struct {
	StageID      string            `json:"stage_id"`
	State        StageState        `json:"state"`
	Attempts     int               `json:"attempts"`
	MaxAttempts  int               `json:"max_attempts"`
	CurrentJobID string            `json:"current_job_id,omitempty"`
	Artifacts    map[string]string `json:"artifacts,omitempty"`
	LastError    string            `json:"last_error,omitempty"`
}
