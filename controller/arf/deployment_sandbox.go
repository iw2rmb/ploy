package arf

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	
	"github.com/google/uuid"
)

// DeploymentSandboxManager implements SandboxManager using the deployment system
// This leverages existing lane detection, build pipeline, and deployment infrastructure
type DeploymentSandboxManager struct {
	controllerURL string
	httpClient    *http.Client
	apiKey        string
}

// NewDeploymentSandboxManager creates a new deployment-integrated sandbox manager
func NewDeploymentSandboxManager(controllerURL string) *DeploymentSandboxManager {
	if controllerURL == "" {
		controllerURL = "https://api.dev.ployd.app/v1"
	}
	
	return &DeploymentSandboxManager{
		controllerURL: controllerURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Minute, // Long timeout for builds
		},
	}
}

// CreateSandbox creates a sandbox by deploying the repository as a temporary application
func (d *DeploymentSandboxManager) CreateSandbox(ctx context.Context, config SandboxConfig) (*Sandbox, error) {
	fmt.Printf("=== CreateSandbox Debug Start ===\n")
	fmt.Printf("Repository: %s\n", config.Repository)
	fmt.Printf("LocalPath: %s\n", config.LocalPath)
	fmt.Printf("Language: %s\n", config.Language)
	fmt.Printf("BuildTool: %s\n", config.BuildTool)
	fmt.Printf("Controller URL: %s\n", d.controllerURL)
	
	// Generate unique app name for this sandbox
	sandboxID := uuid.New().String()[:8]
	appName := fmt.Sprintf("arf-benchmark-%s", sandboxID)
	fmt.Printf("Generated App Name: %s\n", appName)
	fmt.Printf("Generated Sandbox ID: %s\n", sandboxID)
	
	// Create the sandbox metadata
	sandbox := &Sandbox{
		ID:         sandboxID,
		JailName:   appName, // Reuse JailName field for app name
		RootPath:   config.Repository,
		WorkingDir: config.Repository,
		CreatedAt:  time.Now(),
		ExpiresAt:  time.Now().Add(config.TTL),
		Status:     SandboxStatusCreating,
		Config:     config,
		Metadata: map[string]string{
			"app_name":     appName,
			"controller":   d.controllerURL,
			"language":     config.Language,
			"build_tool":   config.BuildTool,
			"deploy_type":  "arf_benchmark",
		},
	}
	fmt.Printf("Created sandbox metadata successfully\n")
	
	// Check if LocalPath is provided (transformed code location)
	// If LocalPath is set, it means we have already cloned and transformed the code
	if config.LocalPath != "" {
		fmt.Printf("LocalPath provided: %s\n", config.LocalPath)
		
		// Check if directory exists
		if _, err := os.Stat(config.LocalPath); os.IsNotExist(err) {
			fmt.Printf("ERROR: LocalPath directory does not exist: %s\n", config.LocalPath)
			sandbox.Status = SandboxStatusError
			return sandbox, fmt.Errorf("local path does not exist: %s", config.LocalPath)
		}
		fmt.Printf("LocalPath directory verified to exist\n")
		
		// Create tar from the transformed repository
		fmt.Printf("Creating tar archive from directory: %s\n", config.LocalPath)
		tarData, err := d.createTarFromDirectory(config.LocalPath)
		if err != nil {
			fmt.Printf("ERROR: Failed to create tar from directory: %v\n", err)
			sandbox.Status = SandboxStatusError
			return sandbox, fmt.Errorf("failed to create tar from transformed code: %w", err)
		}
		fmt.Printf("Tar archive created successfully, size: %d bytes\n", len(tarData))
		
		// Deploy the tar archive
		fmt.Printf("About to call deployTarArchive...\n")
		if err := d.deployTarArchive(ctx, appName, tarData, config); err != nil {
			fmt.Printf("ERROR: deployTarArchive failed: %v\n", err)
			sandbox.Status = SandboxStatusError
			return sandbox, fmt.Errorf("failed to deploy sandbox app: %w", err)
		}
		fmt.Printf("deployTarArchive completed successfully\n")
		
		// Wait for deployment to complete
		fmt.Printf("Starting deployment polling...\n")
		if err := d.waitForDeployment(ctx, appName, 5*time.Minute); err != nil {
			fmt.Printf("ERROR: waitForDeployment failed: %v\n", err)
			sandbox.Status = SandboxStatusError
			return sandbox, fmt.Errorf("deployment failed or timed out: %w", err)
		}
		fmt.Printf("Deployment polling completed successfully\n")
		
		// Get app URL - use the configured domain
		appDomain := "ployd.app"
		if envDomain := os.Getenv("PLOY_APPS_DOMAIN"); envDomain != "" {
			appDomain = envDomain
		}
		appURL := fmt.Sprintf("https://%s.%s", appName, appDomain)
		sandbox.Metadata["app_url"] = appURL
		sandbox.Status = SandboxStatusReady
		
		fmt.Printf("Sandbox deployed successfully: %s\n", appURL)
	} else {
		// For backward compatibility: create mock sandbox if no local path
		fmt.Printf("Creating mock sandbox (no transformed code provided)\n")
		sandbox.Status = SandboxStatusReady
		appDomain := "ployd.app"
		if envDomain := os.Getenv("PLOY_APPS_DOMAIN"); envDomain != "" {
			appDomain = envDomain
		}
		sandbox.Metadata["app_url"] = fmt.Sprintf("https://%s.%s", appName, appDomain)
		sandbox.Metadata["mock_deployment"] = "true"
	}
	
	return sandbox, nil
}

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
			"ttl":          config.TTL.String(),
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
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("deploy request failed with status %d: %s", resp.StatusCode, string(body))
	}
	
	return nil
}

// waitForDeployment polls the app status until deployment completes
func (d *DeploymentSandboxManager) waitForDeployment(ctx context.Context, appName string, timeout time.Duration) error {
	statusURL := fmt.Sprintf("%s/status/%s", d.controllerURL, appName)
	
	fmt.Printf("=== ARF Deployment Polling ===\n")
	fmt.Printf("App Name: %s\n", appName)
	fmt.Printf("Status URL: %s\n", statusURL)
	fmt.Printf("Timeout: %v\n", timeout)
	
	ctxTimeout, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	
	attempt := 0
	for {
		select {
		case <-ctxTimeout.Done():
			fmt.Printf("Deployment polling timeout after %v (attempted %d times)\n", timeout, attempt)
			return fmt.Errorf("deployment timeout after %v", timeout)
		case <-ticker.C:
			attempt++
			fmt.Printf("Polling attempt %d: Checking app status...\n", attempt)
			
			status, err := d.getAppStatus(ctxTimeout, statusURL)
			if err != nil {
				fmt.Printf("Status check failed (attempt %d): %v\n", attempt, err)
				// Continue polling on errors
				continue
			}
			
			fmt.Printf("Status response (attempt %d): %s\n", attempt, status)
			
			switch status {
			case "running", "healthy":
				fmt.Printf("Deployment successful! App is %s after %d attempts\n", status, attempt)
				return nil
			case "failed", "stopped":
				fmt.Printf("Deployment failed with final status: %s after %d attempts\n", status, attempt)
				return fmt.Errorf("deployment failed with status: %s", status)
			default:
				// Continue polling for "building", "deploying", etc.
				fmt.Printf("Deployment status: %s\n", status)
			}
		}
	}
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
	defer resp.Body.Close()
	
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
				resp.Body.Close()
				return fmt.Sprintf("https://%s", domains[0]), nil
			}
			resp.Body.Close()
		}
	}
	
	// Fallback to default domain
	appDomain := "ployd.app"
	if envDomain := os.Getenv("PLOY_APPS_DOMAIN"); envDomain != "" {
		appDomain = envDomain
	}
	return fmt.Sprintf("https://%s.%s", appName, appDomain), nil
}

// DestroySandbox destroys the sandbox by deleting the deployed application
func (d *DeploymentSandboxManager) DestroySandbox(ctx context.Context, sandboxID string) error {
	appName := fmt.Sprintf("arf-benchmark-%s", sandboxID)
	
	// Delete the app using lifecycle API
	deleteURL := fmt.Sprintf("%s/apps/%s", d.controllerURL, appName)
	
	req, err := http.NewRequestWithContext(ctx, "DELETE", deleteURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create delete request: %w", err)
	}
	
	if d.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+d.apiKey)
	}
	
	// Add force parameter for immediate cleanup
	q := req.URL.Query()
	q.Add("force", "true")
	req.URL.RawQuery = q.Encode()
	
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("delete request failed: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete failed with status %d: %s", resp.StatusCode, string(body))
	}
	
	return nil
}

// ListSandboxes lists all active ARF benchmark sandboxes
func (d *DeploymentSandboxManager) ListSandboxes(ctx context.Context) ([]SandboxInfo, error) {
	// List all apps and filter for ARF benchmark apps
	appsURL := fmt.Sprintf("%s/apps", d.controllerURL)
	
	req, err := http.NewRequestWithContext(ctx, "GET", appsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create list request: %w", err)
	}
	
	if d.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+d.apiKey)
	}
	
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list request failed: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list failed with status: %d", resp.StatusCode)
	}
	
	var apps []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&apps); err != nil {
		return nil, fmt.Errorf("failed to decode apps list: %w", err)
	}
	
	var sandboxes []SandboxInfo
	for _, app := range apps {
		appName, ok := app["name"].(string)
		if !ok || !strings.HasPrefix(appName, "arf-benchmark-") {
			continue
		}
		
		// Extract sandbox ID from app name
		sandboxID := strings.TrimPrefix(appName, "arf-benchmark-")
		
		status := "unknown"
		if appStatus, ok := app["status"].(string); ok {
			status = d.mapAppStatusToSandboxStatus(appStatus)
		}
		
		var createdAt time.Time
		if createdStr, ok := app["created_at"].(string); ok {
			createdAt, _ = time.Parse(time.RFC3339, createdStr)
		}
		
		sandbox := SandboxInfo{
			ID:         sandboxID,
			JailName:   appName,
			Status:     SandboxStatus(status),
			CreatedAt:  createdAt,
			ExpiresAt:  createdAt.Add(30 * time.Minute), // Default TTL
			Repository: "", // Would need to be stored in app metadata
		}
		
		sandboxes = append(sandboxes, sandbox)
	}
	
	return sandboxes, nil
}

// mapAppStatusToSandboxStatus converts application status to sandbox status
func (d *DeploymentSandboxManager) mapAppStatusToSandboxStatus(appStatus string) string {
	switch appStatus {
	case "building", "deploying":
		return string(SandboxStatusCreating)
	case "running", "healthy":
		return string(SandboxStatusReady)
	case "stopped":
		return string(SandboxStatusStopped)
	case "failed":
		return string(SandboxStatusError)
	default:
		return string(SandboxStatusCreating)
	}
}

// CleanupExpiredSandboxes removes sandboxes that have exceeded their TTL
func (d *DeploymentSandboxManager) CleanupExpiredSandboxes(ctx context.Context) error {
	sandboxes, err := d.ListSandboxes(ctx)
	if err != nil {
		return fmt.Errorf("failed to list sandboxes: %w", err)
	}
	
	now := time.Now()
	var errors []string
	
	for _, sandbox := range sandboxes {
		if now.After(sandbox.ExpiresAt) {
			fmt.Printf("Cleaning up expired sandbox: %s\n", sandbox.ID)
			if err := d.DestroySandbox(ctx, sandbox.ID); err != nil {
				errors = append(errors, fmt.Sprintf("Failed to cleanup %s: %v", sandbox.ID, err))
			}
		}
	}
	
	if len(errors) > 0 {
		return fmt.Errorf("cleanup errors: %s", strings.Join(errors, "; "))
	}
	
	return nil
}

// createTarFromDirectory creates a tar archive from a directory
func (d *DeploymentSandboxManager) createTarFromDirectory(sourceDir string) ([]byte, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	defer tw.Close()
	
	// Walk through the directory and add files to tar
	err := filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		// Skip .git directory
		if strings.Contains(path, ".git") {
			return nil
		}
		
		// Create tar header
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		
		// Update the name to be relative to sourceDir
		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		header.Name = relPath
		
		// Write header
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		
		// If it's a file, write its contents
		if !info.IsDir() {
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()
			
			if _, err := io.Copy(tw, file); err != nil {
				return err
			}
		}
		
		return nil
	})
	
	if err != nil {
		return nil, fmt.Errorf("failed to create tar archive: %w", err)
	}
	
	return buf.Bytes(), nil
}

// deployTarArchive deploys a tar archive through the build endpoint
func (d *DeploymentSandboxManager) deployTarArchive(ctx context.Context, appName string, tarData []byte, config SandboxConfig) error {
	// Let Ploy's lane detection handle the optimal lane selection
	// Don't force a specific lane - let the system auto-detect
	
	// Build the deployment URL with parameters
	// Use auto lane detection by not specifying lane parameter
	sha := fmt.Sprintf("arf-%s", time.Now().Format("20060102-150405"))
	deployURL := fmt.Sprintf("%s/apps/%s/builds?sha=%s", 
		d.controllerURL, appName, sha)
	
	// Create the request with tar data as body
	req, err := http.NewRequestWithContext(ctx, "POST", deployURL, bytes.NewReader(tarData))
	if err != nil {
		return fmt.Errorf("failed to create deploy request: %w", err)
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
	resp, err := d.httpClient.Do(req)
	if err != nil {
		fmt.Printf("HTTP Request Error: %v\n", err)
		return fmt.Errorf("deploy request failed: %w", err)
	}
	defer resp.Body.Close()
	
	// Read response body
	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		fmt.Printf("Error reading response body: %v\n", readErr)
		return fmt.Errorf("failed to read response: %w", readErr)
	}
	
	// Log complete response details
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
				fmt.Printf("Parsed Error Message: %s\n", errMsg)
				return fmt.Errorf("deployment failed: %s", errMsg)
			}
		}
		fmt.Printf("Raw Error Response: %s\n", string(body))
		return fmt.Errorf("deploy request failed with status %d: %s", resp.StatusCode, string(body))
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

// TestSandbox performs a health check on the deployed sandbox app
func (d *DeploymentSandboxManager) TestSandbox(ctx context.Context, sandbox *Sandbox) error {
	appURL, ok := sandbox.Metadata["app_url"]
	if !ok {
		return fmt.Errorf("sandbox missing app_url metadata")
	}
	
	// Test the health endpoint - apps deployed through Ploy respond at root or /health
	// Try multiple endpoints to ensure compatibility
	healthURL := appURL
	
	req, err := http.NewRequestWithContext(ctx, "GET", healthURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create health check request: %w", err)
	}
	
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("health check request failed: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check failed with status: %d", resp.StatusCode)
	}
	
	return nil
}

// GetSandboxLogs retrieves logs from the deployed app
func (d *DeploymentSandboxManager) GetSandboxLogs(ctx context.Context, sandboxID string) (string, error) {
	appName := fmt.Sprintf("arf-benchmark-%s", sandboxID)
	logsURL := fmt.Sprintf("%s/apps/%s/logs", d.controllerURL, appName)
	
	req, err := http.NewRequestWithContext(ctx, "GET", logsURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create logs request: %w", err)
	}
	
	if d.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+d.apiKey)
	}
	
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("logs request failed: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("logs request failed with status: %d", resp.StatusCode)
	}
	
	logs, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read logs: %w", err)
	}
	
	return string(logs), nil
}

// GetSandboxMetrics retrieves performance metrics from the deployed app
func (d *DeploymentSandboxManager) GetSandboxMetrics(ctx context.Context, sandboxID string) (map[string]interface{}, error) {
	appName := fmt.Sprintf("arf-benchmark-%s", sandboxID)
	metricsURL := fmt.Sprintf("%s/apps/%s/metrics", d.controllerURL, appName)
	
	req, err := http.NewRequestWithContext(ctx, "GET", metricsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create metrics request: %w", err)
	}
	
	if d.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+d.apiKey)
	}
	
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("metrics request failed: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("metrics request failed with status: %d", resp.StatusCode)
	}
	
	var metrics map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&metrics); err != nil {
		return nil, fmt.Errorf("failed to decode metrics: %w", err)
	}
	
	return metrics, nil
}