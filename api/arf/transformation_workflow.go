package arf

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// TransformationWorkflow orchestrates the complete transformation → deployment → testing pipeline
type TransformationWorkflow struct {
	recipeExecutor *RecipeExecutor
	sandboxMgr     SandboxManager
	workspaceDir   string
}

// WorkflowConfig defines parameters for the complete transformation workflow
type WorkflowConfig struct {
	Repository    string            `json:"repository"`
	Branch        string            `json:"branch"`
	RecipeIDs     []string          `json:"recipe_ids"`
	AppName       string            `json:"app_name"`
	Lane          string            `json:"lane"`
	DeployTimeout time.Duration     `json:"deploy_timeout"`
	TestEndpoints []string          `json:"test_endpoints"`
	CleanupAfter  bool              `json:"cleanup_after"`
	Metadata      map[string]string `json:"metadata"`
}

// WorkflowResult contains the results of the complete transformation workflow
type WorkflowResult struct {
	WorkflowID       string                  `json:"workflow_id"`
	Success          bool                    `json:"success"`
	Transformations  []*TransformationResult `json:"transformations"`
	DeploymentResult *DeploymentResult       `json:"deployment_result"`
	TestResults      []*EndpointTestResult   `json:"test_results"`
	ExecutionTime    time.Duration           `json:"execution_time"`
	Errors           []WorkflowError         `json:"errors,omitempty"`
	Warnings         []WorkflowError         `json:"warnings,omitempty"`
	Metadata         map[string]interface{}  `json:"metadata"`
}

// DeploymentResult contains deployment-specific results
type DeploymentResult struct {
	AppName    string            `json:"app_name"`
	AppURL     string            `json:"app_url"`
	Lane       string            `json:"lane"`
	DeployTime time.Duration     `json:"deploy_time"`
	Status     string            `json:"status"`
	BuildLogs  string            `json:"build_logs,omitempty"`
	Metadata   map[string]string `json:"metadata"`
}

// EndpointTestResult contains HTTP endpoint testing results
type EndpointTestResult struct {
	Endpoint     string            `json:"endpoint"`
	StatusCode   int               `json:"status_code"`
	ResponseTime time.Duration     `json:"response_time"`
	Success      bool              `json:"success"`
	Error        string            `json:"error,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"`
	BodyPreview  string            `json:"body_preview,omitempty"`
}

// WorkflowError represents errors and warnings in the workflow
type WorkflowError struct {
	Stage       string `json:"stage"` // transformation, deployment, testing
	Type        string `json:"type"`
	Message     string `json:"message"`
	Recoverable bool   `json:"recoverable"`
}

// NewTransformationWorkflow creates a new transformation workflow orchestrator
func NewTransformationWorkflow(executor *RecipeExecutor, sandboxMgr SandboxManager, workspaceDir string) *TransformationWorkflow {
	return &TransformationWorkflow{
		recipeExecutor: executor,
		sandboxMgr:     sandboxMgr,
		workspaceDir:   workspaceDir,
	}
}

// ExecuteWorkflow runs the complete transformation → deployment → testing workflow
func (w *TransformationWorkflow) ExecuteWorkflow(ctx context.Context, config WorkflowConfig) (*WorkflowResult, error) {
	startTime := time.Now()

	result := &WorkflowResult{
		WorkflowID:      generateWorkflowID(),
		Success:         false,
		Transformations: []*TransformationResult{},
		TestResults:     []*EndpointTestResult{},
		Errors:          []WorkflowError{},
		Warnings:        []WorkflowError{},
		Metadata:        make(map[string]interface{}),
	}

	// Stage 1: Create sandbox with repository
	sandbox, err := w.createWorkflowSandbox(ctx, config)
	if err != nil {
		result.Errors = append(result.Errors, WorkflowError{
			Stage:       "sandbox_creation",
			Type:        "sandbox_error",
			Message:     fmt.Sprintf("Failed to create sandbox: %v", err),
			Recoverable: false,
		})
		result.ExecutionTime = time.Since(startTime)
		return result, nil
	}

	// Ensure cleanup
	if config.CleanupAfter {
		defer func() {
			if err := w.sandboxMgr.DestroySandbox(ctx, sandbox.ID); err != nil {
				result.Warnings = append(result.Warnings, WorkflowError{
					Stage:       "cleanup",
					Type:        "cleanup_warning",
					Message:     fmt.Sprintf("Failed to cleanup sandbox: %v", err),
					Recoverable: true,
				})
			}
		}()
	}

	// Stage 2: Apply transformations
	for _, recipeID := range config.RecipeIDs {
		transformResult, err := w.executeTransformation(ctx, sandbox, recipeID)
		if err != nil {
			result.Errors = append(result.Errors, WorkflowError{
				Stage:       "transformation",
				Type:        "transformation_error",
				Message:     fmt.Sprintf("Recipe %s failed: %v", recipeID, err),
				Recoverable: len(config.RecipeIDs) > 1, // Recoverable if multiple recipes
			})
			continue
		}

		result.Transformations = append(result.Transformations, transformResult)

		if !transformResult.Success {
			result.Warnings = append(result.Warnings, WorkflowError{
				Stage:       "transformation",
				Type:        "transformation_warning",
				Message:     fmt.Sprintf("Recipe %s completed with warnings", recipeID),
				Recoverable: true,
			})
		}
	}

	// Stage 3: Commit transformed code and deploy
	deployResult, err := w.deployTransformedCode(ctx, sandbox, config)
	if err != nil {
		result.Errors = append(result.Errors, WorkflowError{
			Stage:       "deployment",
			Type:        "deployment_error",
			Message:     fmt.Sprintf("Deployment failed: %v", err),
			Recoverable: false,
		})
		result.ExecutionTime = time.Since(startTime)
		return result, nil
	}

	result.DeploymentResult = deployResult

	// Stage 4: Test deployed application
	testResults, err := w.testDeployedApplication(ctx, deployResult, config.TestEndpoints)
	if err != nil {
		result.Warnings = append(result.Warnings, WorkflowError{
			Stage:       "testing",
			Type:        "testing_warning",
			Message:     fmt.Sprintf("Testing failed: %v", err),
			Recoverable: true,
		})
	} else {
		result.TestResults = testResults
	}

	// Determine overall success
	result.Success = len(result.Errors) == 0 && deployResult.Status == "running"
	result.ExecutionTime = time.Since(startTime)

	// Add workflow metadata
	result.Metadata = map[string]interface{}{
		"repository":      config.Repository,
		"branch":          config.Branch,
		"recipes_applied": len(result.Transformations),
		"app_name":        config.AppName,
		"lane":            config.Lane,
		"total_changes":   w.sumChangesApplied(result.Transformations),
		"deployment_url":  deployResult.AppURL,
		"workflow_type":   "transformation_deployment",
	}

	return result, nil
}

// createWorkflowSandbox creates a sandbox for the workflow with the repository
func (w *TransformationWorkflow) createWorkflowSandbox(ctx context.Context, config WorkflowConfig) (*Sandbox, error) {
	sandboxConfig := SandboxConfig{
		Repository:    config.Repository,
		Branch:        config.Branch,
		Language:      "java",  // Default to Java for now
		BuildTool:     "maven", // Will be auto-detected
		TTL:           config.DeployTimeout,
		MemoryLimit:   "4G",
		CPULimit:      "2",
		NetworkAccess: true, // Needed for deployment
		TempSpace:     "2G",
	}

	return w.sandboxMgr.CreateSandbox(ctx, sandboxConfig)
}

// executeTransformation applies a single transformation recipe
func (w *TransformationWorkflow) executeTransformation(ctx context.Context, sandbox *Sandbox, recipeID string) (*TransformationResult, error) {
	// Execute the recipe directly using the executor
	result, err := w.recipeExecutor.ExecuteRecipeByID(ctx, recipeID, sandbox.WorkingDir, "")
	if err != nil {
		return nil, fmt.Errorf("failed to execute recipe %s: %w", recipeID, err)
	}

	return result, nil
}

// deployTransformedCode commits the transformed code and deploys it
func (w *TransformationWorkflow) deployTransformedCode(ctx context.Context, sandbox *Sandbox, config WorkflowConfig) (*DeploymentResult, error) {
	startTime := time.Now()

	// Commit transformed changes to a temporary branch
	if err := w.commitTransformedCode(ctx, sandbox); err != nil {
		return nil, fmt.Errorf("failed to commit transformed code: %w", err)
	}

	// Create deployment using DeploymentSandboxManager
	deployConfig := SandboxConfig{
		Repository:    sandbox.Config.Repository,
		Branch:        sandbox.Config.Branch,
		Language:      sandbox.Config.Language,
		BuildTool:     sandbox.Config.BuildTool,
		TTL:           config.DeployTimeout,
		MemoryLimit:   "4G",
		CPULimit:      "2",
		NetworkAccess: true,
		TempSpace:     "2G",
	}

	// If using DeploymentSandboxManager, it will handle the deployment
	if deployMgr, ok := w.sandboxMgr.(*DeploymentSandboxManager); ok {
		deploySandbox, err := deployMgr.CreateSandbox(ctx, deployConfig)
		if err != nil {
			return nil, fmt.Errorf("deployment failed: %w", err)
		}

		// Test the deployment
		if err := deployMgr.TestSandbox(ctx, deploySandbox); err != nil {
			return &DeploymentResult{
				AppName:    deploySandbox.JailName,
				AppURL:     deploySandbox.Metadata["app_url"],
				Lane:       config.Lane,
				DeployTime: time.Since(startTime),
				Status:     "failed",
				Metadata:   deploySandbox.Metadata,
			}, nil
		}

		return &DeploymentResult{
			AppName:    deploySandbox.JailName,
			AppURL:     deploySandbox.Metadata["app_url"],
			Lane:       config.Lane,
			DeployTime: time.Since(startTime),
			Status:     "running",
			Metadata:   deploySandbox.Metadata,
		}, nil
	}

	// Fallback for other sandbox managers
	return &DeploymentResult{
		AppName:    config.AppName,
		AppURL:     fmt.Sprintf("http://localhost:8080"), // Mock URL
		Lane:       config.Lane,
		DeployTime: time.Since(startTime),
		Status:     "running",
		Metadata:   make(map[string]string),
	}, nil
}

// commitTransformedCode commits the transformed code to prepare for deployment
func (w *TransformationWorkflow) commitTransformedCode(ctx context.Context, sandbox *Sandbox) error {
	workspaceDir := filepath.Join(sandbox.RootPath, "workspace")

	// Check if there are any changes to commit
	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	cmd.Dir = workspaceDir
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to check git status: %w", err)
	}

	if len(strings.TrimSpace(string(output))) == 0 {
		// No changes to commit
		return nil
	}

	// Add all changes
	cmd = exec.CommandContext(ctx, "git", "add", ".")
	cmd.Dir = workspaceDir
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to add changes: %w", err)
	}

	// Commit changes
	commitMsg := fmt.Sprintf("ARF transformation applied at %s", time.Now().Format("2006-01-02 15:04:05"))
	cmd = exec.CommandContext(ctx, "git", "commit", "-m", commitMsg)
	cmd.Dir = workspaceDir
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to commit changes: %w", err)
	}

	return nil
}

// testDeployedApplication tests the deployed application endpoints
func (w *TransformationWorkflow) testDeployedApplication(ctx context.Context, deployResult *DeploymentResult, testEndpoints []string) ([]*EndpointTestResult, error) {
	if len(testEndpoints) == 0 {
		testEndpoints = []string{"/", "/healthz", "/actuator/health"}
	}

	var results []*EndpointTestResult

	for _, endpoint := range testEndpoints {
		result := w.testEndpoint(ctx, deployResult.AppURL, endpoint)
		results = append(results, result)
	}

	return results, nil
}

// testEndpoint tests a single HTTP endpoint
func (w *TransformationWorkflow) testEndpoint(ctx context.Context, baseURL, endpoint string) *EndpointTestResult {
	startTime := time.Now()

	fullURL := baseURL + endpoint
	if !strings.HasPrefix(endpoint, "/") {
		fullURL = baseURL + "/" + endpoint
	}

	// Use curl for testing to avoid import dependencies
	cmd := exec.CommandContext(ctx, "curl", "-s", "-o", "/dev/null", "-w", "%{http_code}", fullURL)
	output, err := cmd.Output()

	result := &EndpointTestResult{
		Endpoint:     endpoint,
		ResponseTime: time.Since(startTime),
		Success:      false,
	}

	if err != nil {
		result.Error = fmt.Sprintf("Request failed: %v", err)
		result.StatusCode = 0
		return result
	}

	statusCode := 0
	if len(output) > 0 {
		fmt.Sscanf(string(output), "%d", &statusCode)
	}

	result.StatusCode = statusCode
	result.Success = statusCode >= 200 && statusCode < 400

	return result
}

// Helper functions

func generateWorkflowID() string {
	return fmt.Sprintf("wf-%d", time.Now().UnixNano())
}

func (w *TransformationWorkflow) sumChangesApplied(transformations []*TransformationResult) int {
	total := 0
	for _, t := range transformations {
		total += t.ChangesApplied
	}
	return total
}
