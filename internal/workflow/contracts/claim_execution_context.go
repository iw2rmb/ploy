package contracts

import (
	"strings"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// MigClaimContext carries the concrete mig step selected for execution.
type MigClaimContext struct {
	StepIndex int                 `json:"step_index"`
	InFrom    []ResolvedInFromRef `json:"in_from,omitempty"`
}

// GateClaimContext carries concrete gate execution routing.
type GateClaimContext struct {
	CycleName string `json:"cycle_name"`
}

// JavaClasspathClaimContext carries java classpath materialization metadata for
// non-SBOM jobs. When Required is true, the node must provide
// /in/java.classpath before execution.
type JavaClasspathClaimContext struct {
	Required         bool                `json:"required"`
	SourceArtifactID string              `json:"source_artifact_id,omitempty"`
	SourceJobID      domaintypes.JobID   `json:"source_job_id,omitempty"`
	SourceJobType    domaintypes.JobType `json:"source_job_type,omitempty"`
}

func (c *GateClaimContext) Normalize() {
	if c == nil {
		return
	}
	c.CycleName = strings.TrimSpace(c.CycleName)
}
