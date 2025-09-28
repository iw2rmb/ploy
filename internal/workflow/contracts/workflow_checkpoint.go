package contracts

import (
	"fmt"
	"strings"
)

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
	SchemaVersion string               `json:"schema_version"`
	TicketID      string               `json:"ticket_id"`
	Stage         string               `json:"stage"`
	Status        CheckpointStatus     `json:"status"`
	CacheKey      string               `json:"cache_key,omitempty"`
	StageMetadata *CheckpointStage     `json:"stage_metadata,omitempty"`
	Artifacts     []CheckpointArtifact `json:"artifacts,omitempty"`
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
	if c.StageMetadata != nil {
		if err := c.StageMetadata.Validate(); err != nil {
			return fmt.Errorf("stage metadata invalid: %w", err)
		}
		if c.Stage != "" && strings.TrimSpace(c.StageMetadata.Name) != strings.TrimSpace(c.Stage) {
			return fmt.Errorf("stage metadata name mismatch")
		}
	}
	if len(c.Artifacts) > 0 {
		if c.StageMetadata == nil {
			return fmt.Errorf("artifacts require stage metadata")
		}
		for i, artifact := range c.Artifacts {
			if err := artifact.Validate(); err != nil {
				return fmt.Errorf("artifact %d invalid: %w", i, err)
			}
		}
	}
	return nil
}

func (c WorkflowCheckpoint) Subject() string {
	ticket := strings.TrimSpace(c.TicketID)
	if ticket == "" {
		return ""
	}
	return fmt.Sprintf(checkpointStreamFormat, ticket)
}
