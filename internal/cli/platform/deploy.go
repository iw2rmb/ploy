package platform

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	utils "github.com/iw2rmb/ploy/internal/cli/utils"
)

// DeployResult contains platform deployment outcome information
type DeployResult struct {
	Success      bool
	Version      string
	DeploymentID string
	URL          string
	Message      string
}

// DeployPlatformService handles deployment for platform services (ployman)
func DeployPlatformService(serviceName, environment, sha, lane string) (*DeployResult, error) {
	// Validate inputs
	if serviceName == "" {
		return nil, fmt.Errorf("service name is required")
	}

	// Get controller URL
	controllerURL := os.Getenv("PLOY_CONTROLLER")
	if controllerURL == "" {
		if environment == "prod" {
			controllerURL = "https://api.ployman.app/v1"
		} else {
			controllerURL = "https://api.dev.ployman.app/v1"
		}
	}

	// Generate SHA if not provided
	if sha == "" {
		if v := utils.GitSHA(); v != "" {
			sha = v
		} else {
			sha = time.Now().Format("20060102-150405")
		}
	}

	// Default lane for platform services
	if lane == "" {
		lane = "E" // Platform services typically use containers
	}

	// Create tar archive
	ign, _ := utils.ReadGitignore(".")
	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		_ = utils.TarDir(".", pw, ign)
	}()

	// Build platform-specific URL
	url := fmt.Sprintf("%s/platform/%s/deploy?sha=%s&lane=%s&env=%s",
		controllerURL, serviceName, sha, lane, environment)

	// Create HTTP request
	req, _ := http.NewRequest("POST", url, pr)
	req.Header.Set("Content-Type", "application/x-tar")
	req.Header.Set("X-Platform-Service", "true")
	req.Header.Set("X-Target-Domain", "ployman.app")
	req.Header.Set("X-Environment", environment)

	// Execute request
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("platform deployment request failed: %w", err)
	}
	defer resp.Body.Close()

	// Parse response
	result := &DeployResult{
		Success:      resp.StatusCode == http.StatusOK,
		Version:      sha,
		DeploymentID: resp.Header.Get("X-Deployment-ID"),
		Message:      "Platform service deployment completed",
	}

	// Build URL based on environment
	if environment == "prod" {
		result.URL = fmt.Sprintf("https://%s.ployman.app", serviceName)
	} else {
		result.URL = fmt.Sprintf("https://%s.%s.ployman.app", serviceName, environment)
	}

	// Add error message if not successful
	if !result.Success {
		result.Message = fmt.Sprintf("Platform deployment failed with status %d", resp.StatusCode)
	}

	// Output response to console
	io.Copy(os.Stdout, resp.Body)

	return result, nil
}
