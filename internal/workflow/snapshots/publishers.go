package snapshots

import (
	"context"
	"crypto/sha256"
	"fmt"
)

type inMemoryArtifactPublisher struct{}

// NewInMemoryArtifactPublisher returns an ArtifactPublisher that retains data in memory.
func NewInMemoryArtifactPublisher() ArtifactPublisher {
	return &inMemoryArtifactPublisher{}
}

// Publish calculates a deterministic CID for the provided artifact payload.
func (p *inMemoryArtifactPublisher) Publish(ctx context.Context, data []byte) (string, error) {
	sum := sha256.Sum256(data)
	return fmt.Sprintf("ipfs:%x", sum[:8]), nil
}

type noopMetadataPublisher struct{}

// NewNoopMetadataPublisher returns a MetadataPublisher that discards metadata writes.
func NewNoopMetadataPublisher() MetadataPublisher {
	return &noopMetadataPublisher{}
}

// Publish accepts metadata without persisting it, enabling tests to run offline.
func (n *noopMetadataPublisher) Publish(ctx context.Context, meta SnapshotMetadata) error {
	return nil
}
