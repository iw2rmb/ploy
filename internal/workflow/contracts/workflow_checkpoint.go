package contracts

import (
	"fmt"
	"strings"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

// CheckpointStatus enumerates lifecycle states for a workflow stage as it
// progresses through execution. Typical values: pending, claimed, running,
// retrying, completed, failed.
type CheckpointStatus string

const (
	CheckpointStatusPending   CheckpointStatus = "pending"
	CheckpointStatusClaimed   CheckpointStatus = "claimed"
	CheckpointStatusRunning   CheckpointStatus = "running"
	CheckpointStatusRetrying  CheckpointStatus = "retrying"
	CheckpointStatusCompleted CheckpointStatus = "completed"
	CheckpointStatusFailed    CheckpointStatus = "failed"
)

// WorkflowCheckpoint is the envelope published to the per‑ticket checkpoint
// subject for each stage transition. Optional `StageMetadata` can include
// timing and metadata; `Artifacts` may be attached and require metadata.
type WorkflowCheckpoint struct {
	SchemaVersion string               `json:"schema_version"`
	RunID         types.RunID          `json:"ticket_id"`
	Stage         StageName            `json:"stage"`
	Status        CheckpointStatus     `json:"status"`
	CacheKey      string               `json:"cache_key,omitempty"`
	StageMetadata *CheckpointStage     `json:"stage_metadata,omitempty"`
	Artifacts     []CheckpointArtifact `json:"artifacts,omitempty"`
}

// Validate ensures the checkpoint envelope is self‑consistent:
// required fields must be set; `StageMetadata`, when present, must validate
// and its `Name` must match `Stage`; any attached artifacts must validate and
// require metadata to be present.
func (c WorkflowCheckpoint) Validate() error {
	if c.SchemaVersion == "" {
		return fmt.Errorf("schema_version is required")
	}
	if c.RunID.IsZero() {
		return fmt.Errorf("ticket_id is required")
	}
	if strings.TrimSpace(string(c.Stage)) == "" {
		return fmt.Errorf("stage is required")
	}
	if c.Status == "" {
		return fmt.Errorf("status is required")
	}
	if c.StageMetadata != nil {
		if err := c.StageMetadata.Validate(); err != nil {
			return fmt.Errorf("stage metadata invalid: %w", err)
		}
		if strings.TrimSpace(string(c.Stage)) != "" && strings.TrimSpace(c.StageMetadata.Name) != strings.TrimSpace(string(c.Stage)) {
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

// Subject returns the per‑ticket checkpoint subject or an empty string when
// the ticket ID is blank to allow callers to short‑circuit publishing.
func (c WorkflowCheckpoint) Subject() string {
	ticket := strings.TrimSpace(c.RunID.String())
	if ticket == "" {
		return ""
	}
	return fmt.Sprintf(checkpointStreamFormat, ticket)
}
