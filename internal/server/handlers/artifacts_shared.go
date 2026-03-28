package handlers

import (
	"github.com/google/uuid"

	"github.com/iw2rmb/ploy/internal/store"
)

// artifactSummary is the JSON shape returned when listing artifact bundles
// (by CID or by run-repo). It intentionally omits heavy fields like RunID,
// JobID, and CreatedAt — use artifactDetail for the single-artifact endpoint.
type artifactSummary struct {
	ID     string  `json:"id"`
	CID    string  `json:"cid"`
	Digest string  `json:"digest"`
	Name   *string `json:"name,omitempty"`
	Size   int64   `json:"size"`
}

// bundleToSummary extracts the common summary fields from an ArtifactBundle.
func bundleToSummary(bundle store.ArtifactBundle) artifactSummary {
	id := ""
	if bundle.ID.Valid {
		id = uuid.UUID(bundle.ID.Bytes).String()
	}
	s := artifactSummary{
		ID:   id,
		Size: bundle.BundleSize,
	}
	if bundle.Cid != nil {
		s.CID = *bundle.Cid
	}
	if bundle.Digest != nil {
		s.Digest = *bundle.Digest
	}
	if bundle.Name != nil {
		s.Name = bundle.Name
	}
	return s
}
