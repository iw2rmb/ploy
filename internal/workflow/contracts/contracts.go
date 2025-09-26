package contracts

import (
	"fmt"
	"strings"
)

const SchemaVersion = "2025-09-25"

type SubjectSet struct {
	TicketInbox      string
	CheckpointStream string
	ArtifactStream   string
	StatusStream     string
}

func SubjectsForTenant(tenant, ticketID string) SubjectSet {
	return SubjectSet{
		TicketInbox:      fmt.Sprintf("grid.webhook.%s", tenant),
		CheckpointStream: fmt.Sprintf("ploy.workflow.%s.checkpoints", ticketID),
		ArtifactStream:   fmt.Sprintf("ploy.artifact.%s", ticketID),
		StatusStream:     fmt.Sprintf("grid.status.%s", ticketID),
	}
}

type WorkflowTicket struct {
	SchemaVersion string            `json:"schema_version"`
	TicketID      string            `json:"ticket_id"`
	Tenant        string            `json:"tenant"`
	Manifest      ManifestReference `json:"manifest"`
}

func (t WorkflowTicket) Validate() error {
	if t.SchemaVersion == "" {
		return fmt.Errorf("schema_version is required")
	}
	if t.TicketID == "" {
		return fmt.Errorf("ticket_id is required")
	}
	if t.Tenant == "" {
		return fmt.Errorf("tenant is required")
	}
	if err := t.Manifest.Validate(); err != nil {
		return fmt.Errorf("manifest invalid: %w", err)
	}
	return nil
}

type ManifestReference struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

func (m ManifestReference) Validate() error {
	if strings.TrimSpace(m.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if strings.TrimSpace(m.Version) == "" {
		return fmt.Errorf("version is required")
	}
	return nil
}

type CheckpointStatus string

const (
	CheckpointStatusPending   CheckpointStatus = "pending"
	CheckpointStatusClaimed   CheckpointStatus = "claimed"
	CheckpointStatusRunning   CheckpointStatus = "running"
	CheckpointStatusRetrying  CheckpointStatus = "retrying"
	CheckpointStatusCompleted CheckpointStatus = "completed"
	CheckpointStatusFailed    CheckpointStatus = "failed"
)

type WorkflowCheckpoint struct {
	SchemaVersion string           `json:"schema_version"`
	TicketID      string           `json:"ticket_id"`
	Stage         string           `json:"stage"`
	Status        CheckpointStatus `json:"status"`
}

func (c WorkflowCheckpoint) Validate() error {
	if c.SchemaVersion == "" {
		return fmt.Errorf("schema_version is required")
	}
	if c.TicketID == "" {
		return fmt.Errorf("ticket_id is required")
	}
	if c.Stage == "" {
		return fmt.Errorf("stage is required")
	}
	if c.Status == "" {
		return fmt.Errorf("status is required")
	}
	return nil
}

func (c WorkflowCheckpoint) Subject() string {
	return fmt.Sprintf("ploy.workflow.%s.checkpoints", c.TicketID)
}
