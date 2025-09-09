package arf

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
)

// DeploymentSandboxManager implements SandboxManager using the deployment system
// This leverages existing lane detection, build pipeline, and deployment infrastructure
type DeploymentSandboxManager struct {
	controllerURL string
	httpClient    *http.Client
	apiKey        string
	logger        func(level, stage, message, details string) // Add logger support
}

// NewDeploymentSandboxManager creates a new deployment-integrated sandbox manager
func NewDeploymentSandboxManager(controllerURL string, logger func(level, stage, message, details string)) *DeploymentSandboxManager {
	if controllerURL == "" {
		controllerURL = "https://api.dev.ployman.app/v1"
	}

	return &DeploymentSandboxManager{
		controllerURL: controllerURL,
		httpClient: &http.Client{
			Timeout: 15 * time.Minute, // Longer timeout for large builds
			Transport: &http.Transport{
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     30 * time.Second,
				DisableKeepAlives:   false,
			},
		},
		logger: logger,
	}
}

// CreateSandbox creates a sandbox by deploying the repository as a temporary application
func (d *DeploymentSandboxManager) CreateSandbox(ctx context.Context, config SandboxConfig) (*Sandbox, error) {
	if d.logger != nil {
		d.logger("DEBUG", "sandbox_creation", "CreateSandbox started", fmt.Sprintf("Repository: %s, LocalPath: %s, Language: %s, BuildTool: %s, Controller: %s", config.Repository, config.LocalPath, config.Language, config.BuildTool, d.controllerURL))
	}

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
			"app_name":    appName,
			"controller":  d.controllerURL,
			"language":    config.Language,
			"build_tool":  config.BuildTool,
			"deploy_type": "arf_benchmark",
		},
	}
	fmt.Printf("Created sandbox metadata successfully\n")

	// Check if LocalPath is provided (transformed code location)
	// If LocalPath is set, it means we have already cloned and transformed the code
	if config.LocalPath != "" {
		fmt.Printf("LocalPath provided: %s\n", config.LocalPath)

		// Check if directory exists
		if _, err := os.Stat(config.LocalPath); os.IsNotExist(err) {
			if d.logger != nil {
				d.logger("ERROR", "sandbox_creation", "LocalPath directory does not exist", fmt.Sprintf("Path: %s", config.LocalPath))
			}
			sandbox.Status = SandboxStatusError
			return sandbox, fmt.Errorf("local path does not exist: %s", config.LocalPath)
		}
		fmt.Printf("LocalPath directory verified to exist\n")

		// Create tar from the transformed repository
		fmt.Printf("Creating tar archive from directory: %s\n", config.LocalPath)
		tarData, err := d.createTarFromDirectory(config.LocalPath)
		if err != nil {
			if d.logger != nil {
				d.logger("ERROR", "sandbox_creation", "Failed to create tar archive", fmt.Sprintf("Directory: %s, Error: %v", config.LocalPath, err))
			}
			sandbox.Status = SandboxStatusError
			return sandbox, fmt.Errorf("failed to create tar from transformed code: %w", err)
		}
		fmt.Printf("Tar archive created successfully, size: %d bytes\n", len(tarData))

		// Deploy the tar archive
		fmt.Printf("About to call deployTarArchive...\n")
		if err := d.deployTarArchive(ctx, appName, tarData, config); err != nil {
			if d.logger != nil {
				d.logger("ERROR", "sandbox_creation", "deployTarArchive failed", fmt.Sprintf("App: %s, Error: %v", appName, err))
			}
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
		body := make([]byte, 1024)
		n, _ := resp.Body.Read(body)
		return fmt.Errorf("delete failed with status %d: %s", resp.StatusCode, string(body[:n]))
	}

	return nil
}
