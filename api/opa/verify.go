package opa

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/utils"
)

type ArtifactInput struct {
	Signed      bool   `json:"signed"`
	SBOMPresent bool   `json:"sbom"`
	Env         string `json:"env"`
	SSHEnabled  bool   `json:"ssh_enabled"`
	BreakGlass  bool   `json:"break_glass_approval"`
	App         string `json:"app"`
	Lane        string `json:"lane"`
	Debug       bool   `json:"debug"`
	// Image size information
	ImageSizeMB float64 `json:"image_size_mb,omitempty"`
	ImagePath   string  `json:"image_path,omitempty"`
	DockerImage string  `json:"docker_image,omitempty"`
	// Enhanced environment-specific fields
	VulnScanPassed bool   `json:"vuln_scan_passed,omitempty"`
	SigningMethod  string `json:"signing_method,omitempty"` // key-based, keyless-oidc, development
	BuildTime      int64  `json:"build_time,omitempty"`
	SourceRepo     string `json:"source_repo,omitempty"`
}

// Enforce validates deployment policies for artifacts with environment-specific rules
func Enforce(input ArtifactInput) error {
	log.Printf("OPA Policy Enforcement: app=%s, lane=%s, env=%s, signed=%v, sbom=%v, ssh=%v, debug=%v, size=%.1fMB, vuln_scan=%v, signing_method=%s",
		input.App, input.Lane, input.Env, input.Signed, input.SBOMPresent, input.SSHEnabled, input.Debug, input.ImageSizeMB, input.VulnScanPassed, input.SigningMethod)

	// Environment-specific policy enforcement
	switch normalizeEnvironment(input.Env) {
	case "production":
		return enforceProductionPolicies(input)
	case "staging":
		return enforceStagingPolicies(input)
	case "development":
		return enforceDevelopmentPolicies(input)
	default:
		// Default to development policies for unknown environments
		log.Printf("OPA Policy Enforcement: Unknown environment '%s', applying development policies", input.Env)
		return enforceDevelopmentPolicies(input)
	}
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
			utils.FormatSize(int64(input.ImageSizeMB*1024*1024)), sizeLimit.MaxSizeMB)
	}

	log.Printf("OPA Size Cap Enforcement: PASSED - app %s size %.1fMB within lane %s limit %dMB",
		input.App, input.ImageSizeMB, input.Lane, sizeLimit.MaxSizeMB)
	return nil
}

// GetLaneSizeLimits returns all lane size limits for external use
func GetLaneSizeLimits() []utils.LaneSizeLimit {
	return utils.GetLaneSizeLimits()
}

// normalizeEnvironment standardizes environment names
func normalizeEnvironment(env string) string {
	switch env {
	case "prod", "production", "live":
		return "production"
	case "stage", "staging", "uat":
		return "staging"
	case "dev", "development", "local", "":
		return "development"
	default:
		return env
	}
}

// enforceProductionPolicies implements strict security policies for production
func enforceProductionPolicies(input ArtifactInput) error {
	log.Printf("OPA Policy Enforcement: Applying PRODUCTION policies for app %s", input.App)

	// Strict supply chain security requirements
	if !input.Signed {
		return fmt.Errorf("PRODUCTION POLICY VIOLATION: artifact must be cryptographically signed")
	}

	if !input.SBOMPresent {
		return fmt.Errorf("PRODUCTION POLICY VIOLATION: Software Bill of Materials (SBOM) required")
	}

	// Production requires key-based or OIDC signing (no development signatures)
	if input.SigningMethod == "development" || input.SigningMethod == "dummy" {
		return fmt.Errorf("PRODUCTION POLICY VIOLATION: production deployments require key-based or OIDC signing, not %s", input.SigningMethod)
	}

	// Vulnerability scanning is mandatory in production
	if !input.VulnScanPassed {
		return fmt.Errorf("PRODUCTION POLICY VIOLATION: vulnerability scan must pass before production deployment")
	}

	// SSH access prohibited in production without break-glass
	if input.SSHEnabled && !input.BreakGlass {
		return fmt.Errorf("PRODUCTION POLICY VIOLATION: SSH access not allowed in production without break-glass approval")
	}

	// Debug builds prohibited in production without break-glass
	if input.Debug && !input.BreakGlass {
		return fmt.Errorf("PRODUCTION POLICY VIOLATION: debug builds not allowed in production without break-glass approval")
	}

	// Image size cap enforcement
	if err := enforceSizeCaps(input); err != nil {
		return fmt.Errorf("PRODUCTION POLICY VIOLATION: %w", err)
	}

	// Additional production validations
	if err := enforceProductionArtifactValidation(input); err != nil {
		return fmt.Errorf("PRODUCTION POLICY VIOLATION: %w", err)
	}

	log.Printf("OPA Policy Enforcement: PRODUCTION policies PASSED for app %s", input.App)
	return nil
}

// enforceStagingPolicies implements moderate security policies for staging
func enforceStagingPolicies(input ArtifactInput) error {
	log.Printf("OPA Policy Enforcement: Applying STAGING policies for app %s", input.App)

	// Core supply chain security requirements (same as production)
	if !input.Signed {
		return fmt.Errorf("STAGING POLICY VIOLATION: artifact must be cryptographically signed")
	}

	if !input.SBOMPresent {
		return fmt.Errorf("STAGING POLICY VIOLATION: Software Bill of Materials (SBOM) required")
	}

	// Staging allows development signatures but warns about them
	if input.SigningMethod == "development" || input.SigningMethod == "dummy" {
		log.Printf("OPA Policy Enforcement: WARNING - staging deployment using %s signing method", input.SigningMethod)
	}

	// Vulnerability scanning recommended but not mandatory in staging
	if !input.VulnScanPassed {
		log.Printf("OPA Policy Enforcement: WARNING - vulnerability scan failed or not performed for staging deployment")
	}

	// SSH access allowed in staging but logged
	if input.SSHEnabled {
		log.Printf("OPA Policy Enforcement: NOTICE - SSH access enabled for staging deployment of app %s", input.App)
	}

	// Debug builds allowed in staging but logged
	if input.Debug {
		log.Printf("OPA Policy Enforcement: NOTICE - debug build deployed to staging for app %s", input.App)
	}

	// Image size cap enforcement (same limits as production)
	if err := enforceSizeCaps(input); err != nil {
		return fmt.Errorf("STAGING POLICY VIOLATION: %w", err)
	}

	log.Printf("OPA Policy Enforcement: STAGING policies PASSED for app %s", input.App)
	return nil
}

// enforceDevelopmentPolicies implements relaxed policies for development
func enforceDevelopmentPolicies(input ArtifactInput) error {
	log.Printf("OPA Policy Enforcement: Applying DEVELOPMENT policies for app %s", input.App)

	// Relaxed requirements for development - log warnings instead of blocking
	if !input.Signed {
		log.Printf("OPA Policy Enforcement: WARNING - unsigned artifact in development for app %s", input.App)
	}

	if !input.SBOMPresent {
		log.Printf("OPA Policy Enforcement: WARNING - missing SBOM in development for app %s", input.App)
	}

	// Development signatures are acceptable
	if input.SigningMethod == "development" || input.SigningMethod == "dummy" {
		log.Printf("OPA Policy Enforcement: NOTICE - using development signing for app %s", input.App)
	}

	// Vulnerability scanning not required in development
	if !input.VulnScanPassed {
		log.Printf("OPA Policy Enforcement: NOTICE - vulnerability scan not performed for development deployment")
	}

	// SSH and debug builds fully allowed in development
	if input.SSHEnabled {
		log.Printf("OPA Policy Enforcement: NOTICE - SSH access enabled for development deployment")
	}

	if input.Debug {
		log.Printf("OPA Policy Enforcement: NOTICE - debug build for development deployment")
	}

	// Size caps still enforced but with warnings only for small overages
	if err := enforceSizeCaps(input); err != nil {
		// In development, allow small size overages with warnings
		if input.ImageSizeMB > 0 {
			log.Printf("OPA Policy Enforcement: WARNING in development - %v", err)
		}
	}

	log.Printf("OPA Policy Enforcement: DEVELOPMENT policies PASSED for app %s", input.App)
	return nil
}

// enforceProductionArtifactValidation performs additional validation for production artifacts
func enforceProductionArtifactValidation(input ArtifactInput) error {
	// Validate source repository for production deployments
	if input.SourceRepo != "" {
		// Check for trusted repositories (implement your organization's repo validation)
		if !isValidSourceRepository(input.SourceRepo) {
			return fmt.Errorf("untrusted source repository: %s", input.SourceRepo)
		}
	}

	// Validate build time freshness (artifacts older than 30 days are suspicious)
	if input.BuildTime > 0 {
		buildAge := time.Now().Unix() - input.BuildTime
		maxAge := int64(30 * 24 * 60 * 60) // 30 days in seconds

		if buildAge > maxAge {
			return fmt.Errorf("artifact build time too old: %d days (max 30 days for production)", buildAge/(24*60*60))
		}
	}

	return nil
}

// isValidSourceRepository validates if a source repository is trusted
func isValidSourceRepository(repo string) bool {
	// Implement your organization's repository validation logic
	// For now, allow any github.com repository
	if strings.Contains(repo, "github.com") {
		return true
	}

	// Add your organization's trusted repository patterns here
	trustedPatterns := []string{
		"gitlab.company.com",
		"bitbucket.company.com",
	}

	for _, pattern := range trustedPatterns {
		if strings.Contains(repo, pattern) {
			return true
		}
	}

	return false
}
