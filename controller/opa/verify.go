package opa

import (
	"fmt"
	"log"
	
	"github.com/ploy/ploy/internal/utils"
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
	// Image size information
	ImageSizeMB  float64 `json:"image_size_mb,omitempty"`
	ImagePath    string  `json:"image_path,omitempty"`
	DockerImage  string  `json:"docker_image,omitempty"`
}

// Enforce validates deployment policies for artifacts
func Enforce(input ArtifactInput) error {
	log.Printf("OPA Policy Enforcement: app=%s, lane=%s, env=%s, signed=%v, sbom=%v, ssh=%v, debug=%v, size=%.1fMB", 
		input.App, input.Lane, input.Env, input.Signed, input.SBOMPresent, input.SSHEnabled, input.Debug, input.ImageSizeMB)

	// Core supply chain security requirements
	if !input.Signed {
		return fmt.Errorf("deployment blocked: artifact must be cryptographically signed")
	}
	
	if !input.SBOMPresent {
		return fmt.Errorf("deployment blocked: Software Bill of Materials (SBOM) required")
	}

	// Image size cap enforcement per lane
	if err := enforceSizeCaps(input); err != nil {
		return fmt.Errorf("deployment blocked: %w", err)
	}

	// Production environment SSH restrictions
	if input.Env == "prod" && input.SSHEnabled && !input.BreakGlass {
		return fmt.Errorf("deployment blocked: SSH access not allowed in production without break-glass approval")
	}

	// Debug builds in production require additional scrutiny
	if input.Env == "prod" && input.Debug && !input.BreakGlass {
		return fmt.Errorf("deployment blocked: debug builds not allowed in production without break-glass approval")
	}

	log.Printf("OPA Policy Enforcement: PASSED - deployment allowed for app %s (size: %.1fMB)", input.App, input.ImageSizeMB)
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

// enforceSizeCaps validates that the artifact size is within lane-specific limits
func enforceSizeCaps(input ArtifactInput) error {
	// Skip size enforcement if no size information provided
	if input.ImageSizeMB == 0 {
		log.Printf("OPA Size Cap Enforcement: SKIPPED - no size information for app %s", input.App)
		return nil
	}

	// Get size limit for the lane
	sizeLimit, err := utils.GetLaneSizeLimit(input.Lane)
	if err != nil {
		log.Printf("OPA Size Cap Enforcement: WARNING - no size limit defined for lane %s", input.Lane)
		return nil // Don't fail deployment for unknown lanes
	}

	// Check if size exceeds limit
	if input.ImageSizeMB > float64(sizeLimit.MaxSizeMB) {
		// Allow break-glass override for size caps
		if input.BreakGlass {
			log.Printf("OPA Size Cap Enforcement: BYPASSED with break-glass - app %s size %.1fMB exceeds lane %s limit %dMB", 
				input.App, input.ImageSizeMB, input.Lane, sizeLimit.MaxSizeMB)
			return nil
		}
		
		return fmt.Errorf("image size %.1fMB exceeds lane %s limit of %dMB (%s). Actual: %s, Limit: %dMB", 
			input.ImageSizeMB, input.Lane, sizeLimit.MaxSizeMB, sizeLimit.Description, 
			utils.FormatSize(int64(input.ImageSizeMB * 1024 * 1024)), sizeLimit.MaxSizeMB)
	}

	log.Printf("OPA Size Cap Enforcement: PASSED - app %s size %.1fMB within lane %s limit %dMB", 
		input.App, input.ImageSizeMB, input.Lane, sizeLimit.MaxSizeMB)
	return nil
}

// GetLaneSizeLimits returns all lane size limits for external use
func GetLaneSizeLimits() []utils.LaneSizeLimit {
	return utils.GetLaneSizeLimits()
}
