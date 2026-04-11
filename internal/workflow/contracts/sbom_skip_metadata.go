package contracts

import (
	"fmt"
	"regexp"
	"strings"
)

var artifactUUIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// SBOMStepSkipMetadata records claim-time sbom cache-hit reuse decisions.
// When present on a claimed sbom job, node runtime may restore cached sbom
// artifacts from ref_artifact_id instead of running sbom collection.
type SBOMStepSkipMetadata struct {
	Enabled       bool   `json:"enabled"`
	RefArtifactID string `json:"ref_artifact_id,omitempty"`
	RefJobImage   string `json:"ref_job_image,omitempty"`
}

func (m *SBOMStepSkipMetadata) Validate() error {
	if m == nil {
		return nil
	}
	if !m.Enabled {
		return fmt.Errorf("enabled: must be true when sbom skip metadata is present")
	}
	if !artifactUUIDPattern.MatchString(strings.TrimSpace(strings.ToLower(m.RefArtifactID))) {
		return fmt.Errorf("ref_artifact_id: must be canonical UUID")
	}
	if strings.TrimSpace(m.RefJobImage) == "" {
		return fmt.Errorf("ref_job_image: required")
	}
	return nil
}
