package opa

import (
	"fmt"
	"log"
)

type ArtifactInput struct {
	Signed       bool   `json:"signed"`
	SBOMPresent  bool   `json:"sbom"`
	Env          string `json:"env"`
	SSHEnabled   bool   `json:"ssh_enabled"`
	BreakGlass   bool   `json:"break_glass_approval"`
	App          string `json:"app"`
	Lane         string `json:"lane"`
	Debug        bool   `json:"debug"`
}

// Enforce validates deployment policies for artifacts
func Enforce(input ArtifactInput) error {
	log.Printf("OPA Policy Enforcement: app=%s, lane=%s, env=%s, signed=%v, sbom=%v, ssh=%v, debug=%v", 
		input.App, input.Lane, input.Env, input.Signed, input.SBOMPresent, input.SSHEnabled, input.Debug)

	// Core supply chain security requirements
	if !input.Signed {
		return fmt.Errorf("deployment blocked: artifact must be cryptographically signed")
	}
	
	if !input.SBOMPresent {
		return fmt.Errorf("deployment blocked: Software Bill of Materials (SBOM) required")
	}

	// Production environment SSH restrictions
	if input.Env == "prod" && input.SSHEnabled && !input.BreakGlass {
		return fmt.Errorf("deployment blocked: SSH access not allowed in production without break-glass approval")
	}

	// Debug builds in production require additional scrutiny
	if input.Env == "prod" && input.Debug && !input.BreakGlass {
		return fmt.Errorf("deployment blocked: debug builds not allowed in production without break-glass approval")
	}

	log.Printf("OPA Policy Enforcement: PASSED - deployment allowed for app %s", input.App)
	return nil
}

// EnforceWithBypass allows bypassing policies in development environments
func EnforceWithBypass(input ArtifactInput, bypassDev bool) error {
	// Allow bypass in development environment if configured
	if bypassDev && (input.Env == "dev" || input.Env == "development" || input.Env == "") {
		log.Printf("OPA Policy Enforcement: BYPASSED for development environment - app %s", input.App)
		return nil
	}
	
	return Enforce(input)
}
