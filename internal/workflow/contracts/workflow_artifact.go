package contracts

import (
	"fmt"
	"strings"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

// WorkflowArtifact represents an artifact envelope published to the artifact
// stream for consumers that hydrate caches without reading checkpoints.
type WorkflowArtifact struct {
	SchemaVersion string             `json:"schema_version"`
	RunID         types.RunID        `json:"ticket_id"`
	Stage         StageName          `json:"stage"`
	CacheKey      string             `json:"cache_key,omitempty"`
	StageMetadata *CheckpointStage   `json:"stage_metadata,omitempty"`
	Artifact      CheckpointArtifact `json:"artifact"`
}

// Validate ensures the artifact envelope is well formed.
func (a WorkflowArtifact) Validate() error {
	if strings.TrimSpace(a.SchemaVersion) == "" {
		return fmt.Errorf("schema_version is required")
	}
	if a.RunID.IsZero() {
		return fmt.Errorf("ticket_id is required")
	}
	if strings.TrimSpace(string(a.Stage)) == "" {
		return fmt.Errorf("stage is required")
	}
	if a.StageMetadata != nil {
		if err := a.StageMetadata.Validate(); err != nil {
			return fmt.Errorf("stage metadata invalid: %w", err)
		}
		if strings.TrimSpace(a.StageMetadata.Name) != strings.TrimSpace(string(a.Stage)) {
			return fmt.Errorf("stage metadata name mismatch")
		}
	}
	if err := a.Artifact.Validate(); err != nil {
		return fmt.Errorf("artifact invalid: %w", err)
	}
	return nil
}

// Subject returns the artifact stream subject for the envelope.
func (a WorkflowArtifact) Subject() string {
	ticket := strings.TrimSpace(a.RunID.String())
	if ticket == "" {
		return ""
	}
	return fmt.Sprintf(artifactStreamFormat, ticket)
}
