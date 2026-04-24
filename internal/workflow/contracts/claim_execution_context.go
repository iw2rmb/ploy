package contracts

import (
	"strings"
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

// JavaClasspathClaimContext carries java classpath availability metadata for
// non-SBOM jobs. When Required is true, the node must provide
// /share/java.classpath before execution.
type JavaClasspathClaimContext struct {
	Required bool `json:"required"`
}

func (c *GateClaimContext) Normalize() {
	if c == nil {
		return
	}
	c.CycleName = strings.TrimSpace(c.CycleName)
}
