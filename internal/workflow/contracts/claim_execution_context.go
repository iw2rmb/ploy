package contracts

import "strings"

// MigClaimContext carries the concrete mig step selected for execution.
type MigClaimContext struct {
	StepIndex int `json:"step_index"`
}

// HookClaimContext carries concrete hook execution routing.
type HookClaimContext struct {
	CycleName              string `json:"cycle_name"`
	Source                 string `json:"source"`
	Index                  int    `json:"index"`
	UpstreamSBOMArtifactID string `json:"upstream_sbom_artifact_id,omitempty"`
}

// GateClaimContext carries concrete gate execution routing.
type GateClaimContext struct {
	CycleName string `json:"cycle_name"`
}

func (c *HookClaimContext) Normalize() {
	if c == nil {
		return
	}
	c.CycleName = strings.TrimSpace(c.CycleName)
	c.Source = strings.TrimSpace(c.Source)
	c.UpstreamSBOMArtifactID = strings.TrimSpace(c.UpstreamSBOMArtifactID)
}

func (c *GateClaimContext) Normalize() {
	if c == nil {
		return
	}
	c.CycleName = strings.TrimSpace(c.CycleName)
}
