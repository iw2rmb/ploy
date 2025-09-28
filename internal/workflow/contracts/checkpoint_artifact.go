package contracts

import (
	"fmt"
	"strings"
)

// CheckpointArtifact records an artifact manifest emitted by a workflow stage.
type CheckpointArtifact struct {
	Name        string `json:"name"`
	ArtifactCID string `json:"artifact_cid,omitempty"`
	Digest      string `json:"digest,omitempty"`
	MediaType   string `json:"media_type,omitempty"`
}

// Validate ensures the artifact manifest has at least a display name.
func (a CheckpointArtifact) Validate() error {
	if strings.TrimSpace(a.Name) == "" {
		return fmt.Errorf("artifact name is required")
	}
	return nil
}
