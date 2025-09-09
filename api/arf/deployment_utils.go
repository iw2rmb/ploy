package arf

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

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
			Repository: "",                              // Would need to be stored in app metadata
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
