package step

import "context"

// ArtifactPublisher uploads step artifacts.
type ArtifactPublisher interface {
	Publish(ctx context.Context, req ArtifactRequest) (PublishedArtifact, error)
}

// ArtifactRequest describes an artifact to publish.
type ArtifactRequest struct {
	Kind   ArtifactKind
	Path   string
	Buffer []byte
}

// ArtifactKind enumerates artifact types.
type ArtifactKind string

const (
	// ArtifactKindDiff identifies diff bundles.
	ArtifactKindDiff ArtifactKind = "diff"
	// ArtifactKindLogs identifies log bundles.
	ArtifactKindLogs ArtifactKind = "logs"
	// ArtifactKindShiftReport identifies SHIFT execution reports.
	ArtifactKindShiftReport ArtifactKind = "shift_report"
	// ArtifactKindSnapshot identifies repository snapshot archives.
	ArtifactKindSnapshot ArtifactKind = "snapshot"
)

// PublishedArtifact references a stored artifact.
type PublishedArtifact struct {
	CID    string
	Kind   ArtifactKind
	Digest string
	Size   int64
}
