package contracts

import (
	"fmt"
	"strings"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

// CheckpointArtifact records an artifact manifest emitted by a workflow stage.
type CheckpointArtifact struct {
	Name        string             `json:"name"`
	ArtifactCID types.CID          `json:"artifact_cid,omitempty"`
	Digest      types.Sha256Digest `json:"digest,omitempty"`
	MediaType   string             `json:"media_type,omitempty"`
}

// Validate ensures the artifact manifest has at least a display name.
func (a CheckpointArtifact) Validate() error {
	if strings.TrimSpace(a.Name) == "" {
		return fmt.Errorf("artifact name is required")
	}
	return nil
}
