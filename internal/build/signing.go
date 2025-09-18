package build

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/iw2rmb/ploy/internal/utils"
)

// determineSigningMethod analyzes the signing method used for the artifact
func determineSigningMethod(imagePath, dockerImage, env string) string {
	// Check for certificate files indicating OIDC keyless signing
	if imagePath != "" {
		certPath := imagePath + ".cert"
		if utils.FileExists(certPath) {
			return "keyless-oidc"
		}
	}

	// Check for key-based signing indicators
	if imagePath != "" && utils.FileExists(imagePath+".sig") {
		// Read signature file to determine if it's key-based or development
		if data, err := os.ReadFile(imagePath + ".sig"); err == nil {
			if strings.Contains(string(data), "development") || strings.Contains(string(data), "dummy") {
				return "development"
			}
			return "key-based"
		}
	}

	// For Docker images, assume keyless OIDC in production/staging, development otherwise
	if dockerImage != "" {
		if env == "prod" || env == "production" || env == "staging" {
			return "keyless-oidc"
		}
		return "development"
	}

	// Default to development signing
	return "development"
}

// performVulnerabilityScanning runs Grype vulnerability scanning if available
func performVulnerabilityScanning(imagePath, dockerImage, env string) bool {
	// Skip vulnerability scanning in development environment for performance
	if env == "dev" || env == "development" || env == "" {
		return false
	}
	if skip := strings.ToLower(os.Getenv("PLOY_SKIP_VULN_SCAN")); skip == "1" || skip == "true" || skip == "yes" {
		return false
	}

	// Check if Grype is available
	if _, err := exec.LookPath("grype"); err != nil {
		fmt.Printf("Warning: Grype not available for vulnerability scanning: %v\n", err)
		return false
	}

	var target string
	if imagePath != "" {
		target = imagePath
	} else if dockerImage != "" {
		target = dockerImage
	} else {
		return false
	}

	// Run Grype vulnerability scan
	cmd := exec.Command("grype", target, "--fail-on", "medium", "--output", "json")
	if err := cmd.Run(); err != nil {
		fmt.Printf("Vulnerability scan failed for %s: %v\n", target, err)
		return false
	}

	fmt.Printf("Vulnerability scan passed for %s\n", target)
	return true
}
