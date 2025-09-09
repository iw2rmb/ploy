package arf

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

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

// ExecuteCommand executes a command in a deployment sandbox
func (d *DeploymentSandboxManager) ExecuteCommand(ctx context.Context, sandboxID string, command string, args ...string) (string, error) {
	// Extract app name from sandbox ID
	appName := strings.TrimPrefix(sandboxID, "arf-sandbox-")

	// Build the exec request
	execReq := map[string]interface{}{
		"command": command,
		"args":    args,
	}

	body, err := json.Marshal(execReq)
	if err != nil {
		return "", fmt.Errorf("failed to marshal exec request: %w", err)
	}

	// Send exec request to the deployment
	req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s/apps/%s/exec", d.controllerURL, appName), bytes.NewBuffer(body))
	if err != nil {
		return "", fmt.Errorf("failed to create exec request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if d.apiKey != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", d.apiKey))
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to execute command: %w", err)
	}
	defer resp.Body.Close()

	output, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read exec output: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return string(output), fmt.Errorf("exec failed with status %d: %s", resp.StatusCode, string(output))
	}

	return string(output), nil
}
