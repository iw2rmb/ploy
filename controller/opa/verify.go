package opa

import "fmt"

type ArtifactInput struct {
	Signed       bool `json:"signed"`
	SBOMPresent  bool `json:"sbom"`
	Env          string `json:"env"`
	SSHEnabled   bool `json:"ssh_enabled"`
	BreakGlass   bool `json:"break_glass_approval"`
}

func Enforce(input ArtifactInput) error {
	if !input.Signed { return fmt.Errorf("artifact not signed") }
	if !input.SBOMPresent { return fmt.Errorf("sbom missing") }
	if input.Env == "prod" && input.SSHEnabled && !input.BreakGlass {
		return fmt.Errorf("ssh enabled in prod without approval")
	}
	return nil
}
