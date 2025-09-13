package arf

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// deployRepository submits the repository to the build system
func (d *DeploymentSandboxManager) deployRepository(ctx context.Context, appName string, config SandboxConfig) error {
	// If it's a Git repository, we need to clone it first and create a tar
	// For now, let's create a simple deployment request

	deployURL := fmt.Sprintf("%s/build/%s", d.controllerURL, appName)

	// Create deployment request body
	deployData := map[string]interface{}{
		"repository": config.Repository,
		"branch":     config.Branch,
		"language":   config.Language,
		"build_tool": config.BuildTool,
		"metadata": map[string]string{
			"arf_benchmark": "true",
			"ttl":           config.TTL.String(),
		},
	}

	jsonData, err := json.Marshal(deployData)
	if err != nil {
		return fmt.Errorf("failed to marshal deploy data: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", deployURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create deploy request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if d.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+d.apiKey)
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("deploy request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("deploy request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// getAppStatus retrieves the current status of a deployed app
func (d *DeploymentSandboxManager) getAppStatus(ctx context.Context, statusURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", statusURL, nil)
	if err != nil {
		return "", err
	}

	if d.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+d.apiKey)
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status request failed with code: %d", resp.StatusCode)
	}

	var statusData map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&statusData); err != nil {
		return "", err
	}

	status, ok := statusData["status"].(string)
	if !ok {
		return "unknown", nil
	}

	return status, nil
}

// getAppURL retrieves the URL for accessing the deployed app
func (d *DeploymentSandboxManager) getAppURL(ctx context.Context, appName string) (string, error) {
	// Try to get custom domain first
	domainsURL := fmt.Sprintf("%s/apps/%s/domains", d.controllerURL, appName)

	req, err := http.NewRequestWithContext(ctx, "GET", domainsURL, nil)
	if err == nil {
		if d.apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+d.apiKey)
		}

		if resp, err := d.httpClient.Do(req); err == nil && resp.StatusCode == http.StatusOK {
			var domains []string
			if json.NewDecoder(resp.Body).Decode(&domains) == nil && len(domains) > 0 {
				_ = resp.Body.Close()
				return fmt.Sprintf("https://%s", domains[0]), nil
			}
			_ = resp.Body.Close()
		}
	}

	// Fallback to default domain
	appDomain := "ployd.app"
	if envDomain := os.Getenv("PLOY_APPS_DOMAIN"); envDomain != "" {
		appDomain = envDomain
	}
	return fmt.Sprintf("https://%s.%s", appName, appDomain), nil
}

// deployTarArchive deploys a tar archive through the build endpoint
func (d *DeploymentSandboxManager) deployTarArchive(ctx context.Context, appName string, tarData []byte, config SandboxConfig) error {
	if d.logger != nil {
		d.logger("DEBUG", "deployment", "deployTarArchive started", fmt.Sprintf("App: %s, Tar size: %d bytes, Controller: %s", appName, len(tarData), d.controllerURL))
	}

	// Let Ploy's lane detection handle the optimal lane selection
	// Don't force a specific lane - let the system auto-detect

	// Build the deployment URL with parameters
	// Use auto lane detection by not specifying lane parameter
	sha := fmt.Sprintf("arf-%s", time.Now().Format("20060102-150405"))
	deployURL := fmt.Sprintf("%s/apps/%s/builds?sha=%s",
		d.controllerURL, appName, sha)

	if d.logger != nil {
		d.logger("DEBUG", "deployment", "Built deployment URL", fmt.Sprintf("URL: %s, SHA: %s", deployURL, sha))
	}

	// Create the request with tar data as body
	req, err := http.NewRequestWithContext(ctx, "POST", deployURL, bytes.NewReader(tarData))
	if err != nil {
		if d.logger != nil {
			d.logger("ERROR", "deployment", "Failed to create HTTP request", fmt.Sprintf("URL: %s, Error: %v", deployURL, err))
		}
		return fmt.Errorf("failed to create deploy request: %w", err)
	}

	if d.logger != nil {
		d.logger("DEBUG", "deployment", "HTTP request created successfully", fmt.Sprintf("Method: POST, URL: %s", deployURL))
	}

	// Set content type for tar
	req.Header.Set("Content-Type", "application/x-tar")
	req.Header.Set("Content-Length", fmt.Sprintf("%d", len(tarData)))

	if d.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+d.apiKey)
	}

	// Log comprehensive deployment request details
	fmt.Printf("=== ARF Deployment Debug ===\n")
	fmt.Printf("URL: %s\n", deployURL)
	fmt.Printf("App Name: %s\n", appName)
	fmt.Printf("SHA: %s\n", sha)
	fmt.Printf("Tar Size: %d bytes\n", len(tarData))
	fmt.Printf("Headers: %+v\n", req.Header)
	fmt.Printf("Controller URL: %s\n", d.controllerURL)

	// Send the request
	if d.logger != nil {
		d.logger("DEBUG", "deployment", "Sending HTTP request", fmt.Sprintf("URL: %s, Content-Type: %s", deployURL, req.Header.Get("Content-Type")))
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		if d.logger != nil {
			d.logger("ERROR", "deployment", "HTTP request failed", fmt.Sprintf("URL: %s, Error: %v", deployURL, err))
		}
		fmt.Printf("HTTP Request Error: %v\n", err)

		// Check if it's a timeout error
		if os.IsTimeout(err) {
			return fmt.Errorf("deployment request timed out (check controller availability): %w", err)
		}

		// Check if it's a connection error
		if strings.Contains(err.Error(), "connection refused") {
			return fmt.Errorf("deployment request failed - controller not reachable at %s: %w", d.controllerURL, err)
		}

		return fmt.Errorf("deploy request failed: %w", err)
	}

	if d.logger != nil {
		d.logger("DEBUG", "deployment", "HTTP request completed", fmt.Sprintf("Status: %d %s", resp.StatusCode, resp.Status))
	}
	defer func() { _ = resp.Body.Close() }()

	// Read response body
	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		if d.logger != nil {
			d.logger("ERROR", "deployment", "Failed to read response body", fmt.Sprintf("Error: %v", readErr))
		}
		fmt.Printf("Error reading response body: %v\n", readErr)
		return fmt.Errorf("failed to read response: %w", readErr)
	}

	// Log complete response details
	if d.logger != nil {
		d.logger("DEBUG", "deployment", fmt.Sprintf("HTTP response: %d %s, Body: %d bytes", resp.StatusCode, resp.Status, len(body)), "")
		if len(body) > 0 && len(body) < 1000 { // Only log short response bodies
			d.logger("DEBUG", "deployment", fmt.Sprintf("Response content: %s", string(body)), "")
		}
	}
	fmt.Printf("=== ARF Deployment Response ===\n")
	fmt.Printf("Status: %d %s\n", resp.StatusCode, resp.Status)
	fmt.Printf("Response Headers: %+v\n", resp.Header)
	fmt.Printf("Response Body: %s\n", string(body))
	fmt.Printf("Response Size: %d bytes\n", len(body))

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		// Parse error response for better debugging
		var errorResp map[string]interface{}
		if err := json.Unmarshal(body, &errorResp); err == nil {
			if errMsg, ok := errorResp["error"].(string); ok {
				if d.logger != nil {
					d.logger("ERROR", "deployment", fmt.Sprintf("Deploy failed - Status: %d, Error: %s", resp.StatusCode, errMsg), "")
				}
				fmt.Printf("Parsed Error Message: %s\n", errMsg)
				return fmt.Errorf("deployment failed: %s", errMsg)
			}
		}
		if d.logger != nil {
			d.logger("ERROR", "deployment", fmt.Sprintf("Deploy failed - Status: %d, Raw response: %s", resp.StatusCode, string(body)), "")
		}
		fmt.Printf("Raw Error Response: %s\n", string(body))
		return fmt.Errorf("deploy request failed with status %d: %s", resp.StatusCode, string(body))
	}

	if d.logger != nil {
		d.logger("DEBUG", "deployment", "Deployment request successful", fmt.Sprintf("Status: %d", resp.StatusCode))
	}
	fmt.Printf("Deployment request successful! Status: %d\n", resp.StatusCode)

	// Parse successful response
	var deployResp map[string]interface{}
	if err := json.Unmarshal(body, &deployResp); err == nil {
		if buildID, ok := deployResp["build_id"].(string); ok {
			fmt.Printf("Build initiated with ID: %s\n", buildID)
		}
	}

	fmt.Printf("Deployment initiated for %s (status: %d)\n", appName, resp.StatusCode)
	return nil
}
