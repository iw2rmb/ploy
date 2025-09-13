package deploy

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	utils "github.com/iw2rmb/ploy/internal/cli/utils"
)

// DeployResult contains app deployment outcome information
type DeployResult struct {
	Success      bool
	Version      string
	DeploymentID string
	URL          string
	Message      string
}

// DeployApp handles deployment for regular applications (ploy)
func DeployApp(appName, lane, mainClass, sha string, blueGreen bool) (*DeployResult, error) {
	// Validate inputs
	if appName == "" {
		return nil, fmt.Errorf("app name is required")
	}

	// Get controller URL (regular apps always use ployd.app domain)
	controllerURL := os.Getenv("PLOY_CONTROLLER")
	if controllerURL == "" {
		controllerURL = "http://localhost:8081/v1"
	}

	// Generate SHA if not provided
	if sha == "" {
		if v := utils.GitSHA(); v != "" {
			sha = v
		} else {
			sha = time.Now().Format("20060102-150405")
		}
	}

	// Create tar archive into a temp file so we can set Content-Length
	ign, _ := utils.ReadGitignore(".")
	tmpf, err := os.CreateTemp("", "ploy-push-*.tar")
	if err != nil {
		return nil, fmt.Errorf("create temp: %w", err)
	}
	tmpPath := tmpf.Name()
	if err := utils.TarDir(".", tmpf, ign); err != nil {
		_ = tmpf.Close()
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("tar dir: %w", err)
	}
	if err := tmpf.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("close temp: %w", err)
	}
	rf, err := os.Open(tmpPath)
	if err != nil {
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("open temp: %w", err)
	}
	stat, _ := rf.Stat()

	// Build app-specific URL
	url := fmt.Sprintf("%s/apps/%s/builds?sha=%s",
		controllerURL, appName, sha)

	if mainClass != "" {
		url += "&main=" + utils.URLQueryEsc(mainClass)
	}

	if lane != "" {
		url += "&lane=" + lane
	}

	if blueGreen {
		url += "&blue_green=true"
	}

	// Create HTTP request with known Content-Length
	req, _ := http.NewRequest("POST", url, rf)
	req.Header.Set("Content-Type", "application/x-tar")
	req.Header.Set("X-Target-Domain", "ployd.app")
	if stat != nil {
		req.ContentLength = stat.Size()
	}

	// Execute request with a generous timeout
	client := &http.Client{Timeout: 3 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("app deployment request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Parse response
	result := &DeployResult{
		Success:      resp.StatusCode == http.StatusOK,
		Version:      sha,
		DeploymentID: resp.Header.Get("X-Deployment-ID"),
		URL:          fmt.Sprintf("https://%s.ployd.app", appName),
		Message:      "App deployment completed",
	}

	// Add error message if not successful
	if !result.Success {
		result.Message = fmt.Sprintf("App deployment failed with status %d", resp.StatusCode)
	}

	// Output response to console and clean up temp
	_, _ = io.Copy(os.Stdout, resp.Body)
	_ = rf.Close()
	_ = os.Remove(tmpPath)

	return result, nil
}
