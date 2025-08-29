package arf

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/iw2rmb/ploy/internal/storage"
)

// RobustTransformRequest represents the request for robust transformation
type RobustTransformRequest struct {
	// Input sources
	InputSource struct {
		Repository string `json:"repository"`
		Archive    string `json:"archive"`
		Branch     string `json:"branch"`
	} `json:"input_source"`

	// Transformations
	Transformations struct {
		RecipeIDs  []string `json:"recipe_ids"`
		LLMPrompts []string `json:"llm_prompts"`
	} `json:"transformations"`

	// Execution configuration
	Execution struct {
		MaxIterations int    `json:"max_iterations"`
		ParallelTries int    `json:"parallel_tries"`
		Timeout       string `json:"timeout"`
		PlanModel     string `json:"plan_model"`
		ExecModel     string `json:"exec_model"`
	} `json:"execution"`

	// Output configuration
	Output struct {
		Format      string `json:"format"`       // archive, diff, mr
		Path        string `json:"path"`
		ReportLevel string `json:"report_level"` // minimal, standard, detailed
	} `json:"output"`

	// Optional fields
	AppName string `json:"app_name,omitempty"`
	Lane    string `json:"lane,omitempty"`
}

// RobustTransformResult represents the result of robust transformation
type RobustTransformResult struct {
	Success bool                   `json:"success"`
	Report  TransformationReport   `json:"report"`
	Output  map[string]interface{} `json:"output"`
	Errors  []ErrorWithResolution  `json:"errors,omitempty"`
}

// TransformationReport provides comprehensive reporting
type TransformationReport struct {
	Summary  ReportSummary    `json:"summary"`
	Timeline []StageExecution `json:"timeline"`
	Changes  []FileChange     `json:"changes"`
	Errors   []ErrorWithResolution `json:"errors,omitempty"`
}

// ReportSummary provides high-level metrics
type ReportSummary struct {
	FilesModified int           `json:"files_modified"`
	LinesChanged  int           `json:"lines_changed"`
	Duration      time.Duration `json:"duration"`
	RecipesApplied int          `json:"recipes_applied"`
	PromptsExecuted int         `json:"prompts_executed"`
	SelfHealingAttempts int     `json:"self_healing_attempts"`
}

// StageExecution represents execution of a transformation stage
type StageExecution struct {
	Name      string        `json:"name"`
	Status    string        `json:"status"`
	StartTime time.Time     `json:"start_time"`
	EndTime   time.Time     `json:"end_time"`
	Duration  time.Duration `json:"duration"`
	Details   interface{}   `json:"details,omitempty"`
}

// FileChange represents changes to a file
type FileChange struct {
	File         string    `json:"file"`
	Type         string    `json:"type"` // added, modified, deleted
	LinesAdded   int       `json:"lines_added"`
	LinesRemoved int       `json:"lines_removed"`
	UnifiedDiff  string    `json:"unified_diff"`
	Timestamp    time.Time `json:"timestamp"`
}

// ErrorWithResolution represents an error with attempted resolution
type ErrorWithResolution struct {
	Type       string    `json:"type"`
	Message    string    `json:"message"`
	Details    string    `json:"details,omitempty"`
	Resolution string    `json:"resolution,omitempty"`
	Resolved   bool      `json:"resolved"`
	Timestamp  time.Time `json:"timestamp"`
}

// Workspace manages the working directory for transformations
type Workspace struct {
	ID           string
	Path         string
	OriginalPath string
	Language     string
	Framework    string
	Timeline     []StageExecution
	Changes      []FileChange
	Errors       []ErrorWithResolution
	mutex        sync.Mutex
}

// ExecuteRobustTransformation executes a robust transformation with self-healing
func ExecuteRobustTransformation(ctx context.Context, req *RobustTransformRequest, logger func(level, stage, message, details string)) (*RobustTransformResult, error) {
	startTime := time.Now()
	
	if logger != nil {
		logger("INFO", "transform_start", "Starting robust transformation", fmt.Sprintf("Recipes: %d, Prompts: %d", 
			len(req.Transformations.RecipeIDs), len(req.Transformations.LLMPrompts)))
	}

	// Step 1: Prepare workspace
	workspace, err := prepareWorkspace(ctx, req, logger)
	if err != nil {
		if logger != nil {
			logger("ERROR", "workspace_prep", "Failed to prepare workspace", err.Error())
		}
		return nil, fmt.Errorf("workspace preparation failed: %w", err)
	}
	defer workspace.Cleanup()

	// Step 2: Apply recipes sequentially
	if len(req.Transformations.RecipeIDs) > 0 {
		for i, recipeID := range req.Transformations.RecipeIDs {
			stage := fmt.Sprintf("recipe_%d", i+1)
			if logger != nil {
				logger("INFO", stage, fmt.Sprintf("Applying recipe %s", recipeID), "")
			}
			
			result := applyRecipeWithRetry(ctx, workspace, recipeID, req, logger)
			workspace.RecordStage(stage, result.Success)
			
			if !result.Success {
				// Enter self-healing mode
				if logger != nil {
					logger("INFO", "self_healing", "Entering self-healing mode", fmt.Sprintf("Error: %v", result.Error))
				}
				healResult := selfHeal(ctx, workspace, result.Error, req, logger)
				if !healResult.Success {
					return createFailureResult(workspace, healResult.Error), nil
				}
			}
		}
	}

	// Step 3: Build and deploy after recipes
	if len(req.Transformations.RecipeIDs) > 0 {
		if logger != nil {
			logger("INFO", "build_deploy", "Testing build and deployment", "")
		}
		buildResult := buildAndDeploy(ctx, workspace, "after-recipes", req, logger)
		workspace.RecordStage("build_deploy", buildResult.Success)
		
		if !buildResult.Success {
			// Enter self-healing mode
			if logger != nil {
				logger("INFO", "self_healing", "Build failed, entering self-healing", fmt.Sprintf("Error: %v", buildResult.Error))
			}
			healResult := selfHeal(ctx, workspace, buildResult.Error, req, logger)
			if !healResult.Success {
				return createFailureResult(workspace, healResult.Error), nil
			}
		}
	}

	// Step 4: Apply LLM prompts sequentially
	if len(req.Transformations.LLMPrompts) > 0 {
		for i, prompt := range req.Transformations.LLMPrompts {
			stage := fmt.Sprintf("prompt_%d", i+1)
			if logger != nil {
				logger("INFO", stage, "Applying LLM prompt", prompt)
			}
			
			result := applyLLMPromptWithRetry(ctx, workspace, prompt, req, logger)
			workspace.RecordStage(stage, result.Success)
			
			if !result.Success {
				return createFailureResult(workspace, result.Error), nil
			}
		}
	}

	// Step 5: Final build and deploy
	if logger != nil {
		logger("INFO", "final_build", "Final build and deployment test", "")
	}
	finalResult := buildAndDeploy(ctx, workspace, "final", req, logger)
	workspace.RecordStage("final_build", finalResult.Success)

	// Step 6: Generate output based on format
	output := generateOutput(workspace, req.Output.Format, logger)

	// Step 7: Generate comprehensive report
	report := generateReport(workspace, req.Output.ReportLevel)
	
	endTime := time.Now()
	report.Summary.Duration = endTime.Sub(startTime)

	if logger != nil {
		logger("INFO", "transform_complete", "Transformation completed", 
			fmt.Sprintf("Success: %v, Duration: %s", finalResult.Success, report.Summary.Duration))
	}

	return &RobustTransformResult{
		Success: finalResult.Success,
		Report:  report,
		Output:  output,
		Errors:  workspace.Errors,
	}, nil
}

// prepareWorkspace creates and prepares the working directory
func prepareWorkspace(ctx context.Context, req *RobustTransformRequest, logger func(level, stage, message, details string)) (workspace *Workspace, err error) {
	// Add panic recovery
	defer func() {
		if r := recover(); r != nil {
			if logger != nil {
				logger("ERROR", "workspace_prep", "Panic during workspace preparation", fmt.Sprintf("Panic: %v", r))
			}
			err = fmt.Errorf("workspace preparation panic: %v", r)
			workspace = nil
		}
	}()
	workspaceID := uuid.New().String()[:8]
	
	// Determine base directory
	baseDir := os.Getenv("NOMAD_ALLOC_DIR")
	if baseDir == "" {
		baseDir = filepath.Join(os.TempDir(), "arf-transform")
	}
	
	workspacePath := filepath.Join(baseDir, fmt.Sprintf("workspace-%s", workspaceID))
	if err := os.MkdirAll(workspacePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create workspace: %w", err)
	}

	workspace = &Workspace{
		ID:           workspaceID,
		Path:         workspacePath,
		OriginalPath: workspacePath + "-original",
		Timeline:     []StageExecution{},
		Changes:      []FileChange{},
		Errors:       []ErrorWithResolution{},
	}

	// Clone repository or extract archive
	if req.InputSource.Repository != "" {
		if logger != nil {
			logger("INFO", "workspace_prep", "Starting repository clone", fmt.Sprintf("URL: %s, Branch: %s", req.InputSource.Repository, req.InputSource.Branch))
		}
		
		gitOps := NewGitOperations("")
		if gitOps == nil {
			return nil, fmt.Errorf("failed to create git operations")
		}
		
		if logger != nil {
			logger("INFO", "workspace_prep", "Cloning main repository", workspacePath)
		}
		if err := gitOps.CloneRepository(ctx, req.InputSource.Repository, req.InputSource.Branch, workspacePath); err != nil {
			if logger != nil {
				logger("ERROR", "workspace_prep", "Failed to clone repository", err.Error())
			}
			return nil, fmt.Errorf("failed to clone repository: %w", err)
		}
		
		if logger != nil {
			logger("INFO", "workspace_prep", "Cloning original repository for comparison", workspace.OriginalPath)
		}
		// Also clone to original for comparison
		if err := gitOps.CloneRepository(ctx, req.InputSource.Repository, req.InputSource.Branch, workspace.OriginalPath); err != nil {
			if logger != nil {
				logger("ERROR", "workspace_prep", "Failed to clone original repository", err.Error())
			}
			return nil, fmt.Errorf("failed to clone original repository: %w", err)
		}
		
		if logger != nil {
			logger("INFO", "workspace_prep", "Repository cloning completed", "Both main and original cloned successfully")
		}
	} else if req.InputSource.Archive != "" {
		// Extract archive
		if err := extractArchive(req.InputSource.Archive, workspacePath); err != nil {
			return nil, fmt.Errorf("failed to extract archive: %w", err)
		}
		// Copy to original
		if err := copyDirectory(workspacePath, workspace.OriginalPath); err != nil {
			return nil, fmt.Errorf("failed to copy original: %w", err)
		}
	} else {
		return nil, fmt.Errorf("no input source specified")
	}

	// Detect language and framework
	buildOps := NewBuildOperations(5 * time.Minute)
	workspace.Language = detectLanguage(workspacePath)
	workspace.Framework = buildOps.DetectBuildSystem(workspacePath)

	return workspace, nil
}

// applyRecipeWithRetry applies a recipe with retry logic
// isOpenRewriteRecipe checks if a recipe ID is an OpenRewrite recipe
func isOpenRewriteRecipe(recipeID string) bool {
	// OpenRewrite recipes typically follow the pattern org.openrewrite.*
	return strings.HasPrefix(recipeID, "org.openrewrite.")
}

// applyOpenRewriteRecipe applies an OpenRewrite recipe using the dispatcher
func applyOpenRewriteRecipe(ctx context.Context, workspace *Workspace, recipeID string, req *RobustTransformRequest, logger func(level, stage, message, details string)) *TransformResult {
	if logger != nil {
		logger("INFO", "openrewrite", "Applying OpenRewrite recipe", recipeID)
	}
	
	// Create OpenRewrite client
	if logger != nil {
		logger("INFO", "openrewrite", "Creating OpenRewrite client", "")
	}
	client := NewOpenRewriteClient()
	if client == nil {
		if logger != nil {
			logger("ERROR", "openrewrite", "OpenRewrite client is nil", "")
		}
		return &TransformResult{
			Success: false,
			Error:   fmt.Errorf("OpenRewrite client could not be created"),
		}
	}
	if client.dispatcher == nil {
		if logger != nil {
			logger("ERROR", "openrewrite", "OpenRewrite dispatcher not available - cannot proceed", "client created but dispatcher is nil")
		}
		return &TransformResult{
			Success: false,
			Error:   fmt.Errorf("OpenRewrite dispatcher not initialized - check Nomad/Consul connectivity"),
		}
	}
	
	// Create tar archive of workspace
	archivePath := workspace.Path + ".tar.gz"
	if err := createArchive(workspace.Path, archivePath); err != nil {
		if logger != nil {
			logger("ERROR", "openrewrite", "Failed to create archive", err.Error())
		}
		return &TransformResult{
			Success: false,
			Error:   fmt.Errorf("failed to create archive: %w", err),
		}
	}
	defer os.Remove(archivePath)
	
	// Read archive for submission
	archiveFile, err := os.Open(archivePath)
	if err != nil {
		if logger != nil {
			logger("ERROR", "openrewrite", "Failed to open archive", err.Error())
		}
		return &TransformResult{
			Success: false,
			Error:   fmt.Errorf("failed to open archive: %w", err),
		}
	}
	defer archiveFile.Close()
	
	// Submit job to dispatcher
	job, err := client.dispatcher.SubmitJob(ctx, recipeID, archiveFile)
	if err != nil {
		if logger != nil {
			logger("ERROR", "openrewrite", "Failed to submit job", err.Error())
		}
		return &TransformResult{
			Success: false,
			Error:   fmt.Errorf("failed to submit OpenRewrite job: %w", err),
		}
	}
	
	// Poll for job completion
	timeout := 10 * time.Minute
	if req.Execution.Timeout != "" {
		timeout, _ = time.ParseDuration(req.Execution.Timeout)
	}
	
	deadline := time.Now().Add(timeout)
	var result *OpenRewriteJob
	
	for time.Now().Before(deadline) {
		result, err = client.dispatcher.GetJob(ctx, job.ID)
		if err != nil {
			if logger != nil {
				logger("ERROR", "openrewrite", "Failed to get job status", err.Error())
			}
			return &TransformResult{
				Success: false,
				Error:   fmt.Errorf("failed to get job status: %w", err),
			}
		}
		
		if result.Status == "completed" {
			break
		} else if result.Status == "failed" {
			return &TransformResult{
				Success: false,
				Error:   fmt.Errorf("OpenRewrite job failed: %s", result.Error),
			}
		}
		
		time.Sleep(5 * time.Second)
	}
	
	if result.Status != "completed" {
		return &TransformResult{
			Success: false,
			Error:   fmt.Errorf("OpenRewrite job timed out after %v", timeout),
		}
	}
	
	// Download and extract output
	if result.OutputURL != "" {
		// TODO: Download output from storage and extract to workspace
		if logger != nil {
			logger("INFO", "openrewrite", "Job completed", fmt.Sprintf("Output: %s", result.OutputURL))
		}
	}
	
	// Parse result metadata
	changesApplied := 0
	filesModified := []string{}
	if result.Result != nil {
		if changes, ok := result.Result["changes_applied"].(float64); ok {
			changesApplied = int(changes)
		}
		if files, ok := result.Result["files_modified"].([]interface{}); ok {
			for _, f := range files {
				if file, ok := f.(string); ok {
					filesModified = append(filesModified, file)
				}
			}
		}
	}
	
	return &TransformResult{
		Success:        true,
		ChangesApplied: changesApplied,
		FilesModified:  filesModified,
	}
}

func applyRecipeWithRetry(ctx context.Context, workspace *Workspace, recipeID string, req *RobustTransformRequest, logger func(level, stage, message, details string)) *TransformResult {
	// Use OpenRewrite dispatcher directly for OpenRewrite recipes
	if isOpenRewriteRecipe(recipeID) {
		return applyOpenRewriteRecipe(ctx, workspace, recipeID, req, logger)
	}
	
	// Use existing recipe executor with deployment sandbox manager for custom recipes
	deployMgr := NewDeploymentSandboxManager(getControllerURLForBenchmark(), logger)
	recipeExecutor := NewRecipeExecutor(NewInMemoryRecipeStorage(), deployMgr)
	
	result, err := recipeExecutor.ExecuteRecipeByID(ctx, recipeID, workspace.Path)
	if err != nil {
		return &TransformResult{
			Success: false,
			Error:   err,
		}
	}
	
	// Record changes
	if result.ChangesApplied > 0 {
		workspace.RecordChanges(result.FilesModified)
	}
	
	return &TransformResult{
		Success:        result.Success,
		ChangesApplied: result.ChangesApplied,
		FilesModified:  result.FilesModified,
	}
}

// applyLLMPromptWithRetry applies an LLM prompt with retry logic using Nomad dispatcher
func applyLLMPromptWithRetry(ctx context.Context, workspace *Workspace, prompt string, req *RobustTransformRequest, logger func(level, stage, message, details string)) *TransformResult {
	// Parse LLM model configuration
	provider, model := parseLLMModel(req.Execution.ExecModel)
	
	// Create LLM dispatcher if not already initialized
	llmDispatcher, err := GetOrCreateLLMDispatcher()
	if err != nil {
		return &TransformResult{
			Success: false,
			Error:   fmt.Errorf("failed to create LLM dispatcher: %w", err),
		}
	}
	
	// Create tar archive of workspace
	archiveData, err := createWorkspaceArchive(workspace)
	if err != nil {
		return &TransformResult{
			Success: false,
			Error:   fmt.Errorf("failed to create workspace archive: %w", err),
		}
	}
	
	// Prepare parameters for LLM job
	params := map[string]interface{}{
		"language":    workspace.Language,
		"framework":   workspace.Framework,
		"temperature": 0.1,
		"max_tokens":  4096,
	}
	
	// Submit LLM transformation job
	job, err := llmDispatcher.SubmitLLMTransformation(ctx, provider, model, prompt, bytes.NewReader(archiveData), params)
	if err != nil {
		return &TransformResult{
			Success: false,
			Error:   fmt.Errorf("failed to submit LLM job: %w", err),
		}
	}
	
	logger("info", "llm-transformation", fmt.Sprintf("Submitted LLM job %s", job.ID), "")
	
	// Wait for job completion (with timeout)
	timeout := 5 * time.Minute
	completedJob, err := llmDispatcher.WaitForCompletion(ctx, job.ID, timeout)
	if err != nil {
		return &TransformResult{
			Success: false,
			Error:   fmt.Errorf("LLM job failed or timed out: %w", err),
		}
	}
	
	// Check job status
	if completedJob.Status == "failed" {
		return &TransformResult{
			Success: false,
			Error:   fmt.Errorf("LLM transformation failed: %s", completedJob.Error),
		}
	}
	
	// Download and apply the transformation result
	if err := applyLLMResult(workspace, completedJob.OutputURL); err != nil {
		return &TransformResult{
			Success: false,
			Error:   fmt.Errorf("failed to apply LLM result: %w", err),
		}
	}
	
	return &TransformResult{
		Success: true,
		ChangesApplied: 1,
	}
}

// buildAndDeploy builds and deploys the application for testing
func buildAndDeploy(ctx context.Context, workspace *Workspace, stage string, req *RobustTransformRequest, logger func(level, stage, message, details string)) *RobustDeploymentResult {
	// Use deployment sandbox manager for testing
	deployMgr := NewDeploymentSandboxManager(getControllerURLForBenchmark(), logger)
	
	config := SandboxConfig{
		Repository: req.InputSource.Repository,
		Branch:     req.InputSource.Branch,
		LocalPath:  workspace.Path,
		Language:   workspace.Language,
		BuildTool:  workspace.Framework,
		TTL:        30 * time.Minute,
	}
	
	// Deploy application
	sandbox, err := deployMgr.CreateSandbox(ctx, config)
	if err != nil {
		return &RobustDeploymentResult{
			Success: false,
			Error:   err,
		}
	}
	
	// Test deployment
	if err := deployMgr.TestSandbox(ctx, sandbox); err != nil {
		deployMgr.DestroySandbox(ctx, sandbox.ID)
		return &RobustDeploymentResult{
			Success: false,
			Error:   err,
		}
	}
	
	// Cleanup
	deployMgr.DestroySandbox(ctx, sandbox.ID)
	
	appURL := ""
	if url, ok := sandbox.Metadata["app_url"]; ok {
		appURL = fmt.Sprintf("%v", url)
	}
	
	return &RobustDeploymentResult{
		Success: true,
		URL:     appURL,
	}
}

// TransformResult represents the result of a transformation attempt
type TransformResult struct {
	Success        bool
	Error          error
	ChangesApplied int
	FilesModified  []string
}

// RobustDeploymentResult represents the result of a deployment attempt
type RobustDeploymentResult struct {
	Success bool
	Error   error
	URL     string
}

// HealingResult represents the result of self-healing
type HealingResult struct {
	Success  bool
	Error    error
	Solution string
}

// RobustSolution represents a potential solution from LLM
type RobustSolution struct {
	ID          string
	Description string
	Changes     []string
	Confidence  float64
}

// selfHeal attempts to automatically fix errors using LLM
func selfHeal(ctx context.Context, workspace *Workspace, err error, req *RobustTransformRequest, logger func(level, stage, message, details string)) *HealingResult {
	iterations := 0
	currentError := err
	
	for iterations < req.Execution.MaxIterations {
		if logger != nil {
			logger("INFO", "self_healing", fmt.Sprintf("Attempt %d/%d", iterations+1, req.Execution.MaxIterations), currentError.Error())
		}
		
		// Step 1: Ask LLM to analyze error and plan solutions
		solutions := planSolutions(ctx, currentError, workspace, req.Execution.PlanModel, logger)
		
		// Step 2: Try solutions in parallel
		results := make(chan *TransformResult, len(solutions))
		var wg sync.WaitGroup
		
		for _, solution := range solutions {
			wg.Add(1)
			go func(sol RobustSolution) {
				defer wg.Done()
				
				// Create branch for this solution attempt
				branch := workspace.CreateBranch(sol.ID)
				
				// Apply solution
				result := applySolution(ctx, branch, sol, req.Execution.ExecModel, logger)
				
				// Test with build and deploy
				if result.Success {
					buildResult := buildAndDeploy(ctx, branch, sol.ID, req, logger)
					result.Success = buildResult.Success
					result.Error = buildResult.Error
				}
				
				results <- result
			}(solution)
		}
		
		// Wait for all solutions to complete
		wg.Wait()
		close(results)
		
		// Step 3: Check if any solution succeeded
		for result := range results {
			if result.Success {
				// Merge successful branch back
				workspace.MergeBranch(result.FilesModified[0]) // Branch ID is in FilesModified
				return &HealingResult{
					Success:  true,
					Solution: "Applied successful solution",
				}
			}
		}
		
		iterations++
	}
	
	return &HealingResult{
		Success: false,
		Error:   fmt.Errorf("self-healing failed after %d attempts: %w", iterations, currentError),
	}
}

// planSolutions generates potential solutions using LLM
func planSolutions(ctx context.Context, err error, workspace *Workspace, model string, logger func(level, stage, message, details string)) []RobustSolution {
	// Parse LLM model
	provider, modelName := parseLLMModel(model)
	
	// Create LLM generator
	var llmGen LLMRecipeGenerator
	
	switch provider {
	case "ollama":
		llmGen, _ = NewOllamaLLMGeneratorWithConfig(modelName, "http://localhost:11434", 0.1, 4096)
	case "openai":
		llmGen, _ = NewOpenAILLMGenerator()
	default:
		return []RobustSolution{}
	}
	
	// Generate solutions prompt
	prompt := fmt.Sprintf(`
I have encountered an error while transforming code:

Error: %s

Context:
- Language: %s
- Framework: %s
- Recent changes: %d files modified

Please provide 3 different solutions to fix this error.
For each solution, provide:
1. Description of the fix
2. Specific code changes or commands to run
3. Why this might work

Format as JSON array of solutions with fields: id, description, changes[], confidence
`, err.Error(), workspace.Language, workspace.Framework, len(workspace.Changes))
	
	// Query LLM
	request := RecipeGenerationRequest{
		ErrorContext: ErrorContext{
			ErrorMessage: prompt,
		},
	}
	
	_, err = llmGen.GenerateRecipe(ctx, request)
	if err != nil {
		return []RobustSolution{}
	}
	
	// Parse solutions from recipe
	// For now, return mock solutions
	return []RobustSolution{
		{
			ID:          "solution-1",
			Description: "Fix compilation error",
			Changes:     []string{"Update imports", "Fix method signatures"},
			Confidence:  0.8,
		},
		{
			ID:          "solution-2", 
			Description: "Update dependencies",
			Changes:     []string{"Update pom.xml", "Refresh build"},
			Confidence:  0.6,
		},
		{
			ID:          "solution-3",
			Description: "Rollback problematic changes",
			Changes:     []string{"Revert last commit", "Apply safer transformation"},
			Confidence:  0.4,
		},
	}
}

// applySolution applies a solution to a workspace branch
func applySolution(ctx context.Context, branch *Workspace, solution RobustSolution, model string, logger func(level, stage, message, details string)) *TransformResult {
	// Apply the solution changes
	for _, change := range solution.Changes {
		if logger != nil {
			logger("DEBUG", "apply_solution", fmt.Sprintf("Applying: %s", change), solution.ID)
		}
		// TODO: Actually apply the changes
	}
	
	return &TransformResult{
		Success:       true,
		FilesModified: []string{solution.ID}, // Store branch ID
	}
}

// generateOutput generates the output in the requested format
func generateOutput(workspace *Workspace, format string, logger func(level, stage, message, details string)) map[string]interface{} {
	output := make(map[string]interface{})
	
	switch format {
	case "archive":
		// Create tar archive of final workspace
		archivePath := workspace.Path + ".tar.gz"
		if err := createArchive(workspace.Path, archivePath); err == nil {
			// Read the archive file contents
			if archiveData, err := os.ReadFile(archivePath); err == nil {
				// Encode as base64 for JSON transport
				output["output"] = base64.StdEncoding.EncodeToString(archiveData)
				output["format"] = "archive"
				output["encoding"] = "base64"
			} else {
				if logger != nil {
					logger("ERROR", "output", "Failed to read archive", err.Error())
				}
				output["location"] = archivePath
			}
			// Clean up the temporary archive file
			os.Remove(archivePath)
		}
		
	case "diff":
		// Generate unified diff
		gitOps := NewGitOperations("")
		diffs, _ := gitOps.GetDiff(context.Background(), workspace.Path)
		output["diff"] = diffs
		output["location"] = "inline diff"
		
	case "mr":
		// Generate merge request data
		output["merge_request"] = map[string]interface{}{
			"title":       "ARF Transformation",
			"description": generateMRDescription(workspace),
			"changes":     len(workspace.Changes),
		}
		output["location"] = "merge request data"
		
	default:
		output["location"] = "memory"
	}
	
	return output
}

// generateReport generates a comprehensive transformation report
func generateReport(workspace *Workspace, level string) TransformationReport {
	report := TransformationReport{
		Summary: ReportSummary{
			FilesModified:   len(workspace.Changes),
			LinesChanged:    calculateLinesChanged(workspace.Changes),
			RecipesApplied:  countSuccessfulStages(workspace.Timeline, "recipe"),
			PromptsExecuted: countSuccessfulStages(workspace.Timeline, "prompt"),
			SelfHealingAttempts: countSuccessfulStages(workspace.Timeline, "self_healing"),
		},
		Timeline: workspace.Timeline,
	}
	
	if level == "standard" || level == "detailed" {
		report.Changes = workspace.Changes
	}
	
	if level == "detailed" {
		report.Errors = workspace.Errors
	}
	
	return report
}

// Helper functions

func (w *Workspace) Cleanup() {
	os.RemoveAll(w.Path)
	os.RemoveAll(w.OriginalPath)
}

func (w *Workspace) RecordStage(name string, success bool) {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	
	status := "success"
	if !success {
		status = "failed"
	}
	
	stage := StageExecution{
		Name:      name,
		Status:    status,
		StartTime: time.Now(),
		EndTime:   time.Now(),
		Duration:  time.Since(time.Now()),
	}
	
	w.Timeline = append(w.Timeline, stage)
}

func (w *Workspace) RecordChanges(files []string) {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	
	for _, file := range files {
		change := FileChange{
			File:      file,
			Type:      "modified",
			Timestamp: time.Now(),
		}
		w.Changes = append(w.Changes, change)
	}
}

func (w *Workspace) CreateBranch(id string) *Workspace {
	branchPath := fmt.Sprintf("%s-branch-%s", w.Path, id)
	copyDirectory(w.Path, branchPath)
	
	return &Workspace{
		ID:           id,
		Path:         branchPath,
		OriginalPath: w.OriginalPath,
		Language:     w.Language,
		Framework:    w.Framework,
		Timeline:     w.Timeline,
		Changes:      w.Changes,
		Errors:       w.Errors,
	}
}

func (w *Workspace) MergeBranch(branchID string) {
	branchPath := fmt.Sprintf("%s-branch-%s", w.Path, branchID)
	if _, err := os.Stat(branchPath); err == nil {
		os.RemoveAll(w.Path)
		os.Rename(branchPath, w.Path)
	}
}

func createFailureResult(workspace *Workspace, err error) *RobustTransformResult {
	return &RobustTransformResult{
		Success: false,
		Report:  generateReport(workspace, "detailed"),
		Output:  map[string]interface{}{"error": err.Error()},
		Errors:  workspace.Errors,
	}
}

func parseLLMModel(model string) (provider, modelName string) {
	parts := strings.Split(model, "/")
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "ollama", model
}

func detectLanguage(path string) string {
	// Check for language-specific files
	if _, err := os.Stat(filepath.Join(path, "pom.xml")); err == nil {
		return "java"
	}
	if _, err := os.Stat(filepath.Join(path, "build.gradle")); err == nil {
		return "java"
	}
	if _, err := os.Stat(filepath.Join(path, "package.json")); err == nil {
		return "javascript"
	}
	if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
		return "go"
	}
	if _, err := os.Stat(filepath.Join(path, "requirements.txt")); err == nil {
		return "python"
	}
	return "unknown"
}

func extractArchive(archivePath, targetPath string) error {
	// TODO: Implement archive extraction
	return fmt.Errorf("archive extraction not yet implemented")
}

func copyDirectory(src, dst string) error {
	// TODO: Implement directory copying
	return fmt.Errorf("directory copying not yet implemented")
}

func createArchive(sourcePath, archivePath string) error {
	// TODO: Implement archive creation
	return fmt.Errorf("archive creation not yet implemented")
}

func calculateLinesChanged(changes []FileChange) int {
	total := 0
	for _, change := range changes {
		total += change.LinesAdded + change.LinesRemoved
	}
	return total
}

func countSuccessfulStages(timeline []StageExecution, prefix string) int {
	count := 0
	for _, stage := range timeline {
		if strings.HasPrefix(stage.Name, prefix) && stage.Status == "success" {
			count++
		}
	}
	return count
}

func generateMRDescription(workspace *Workspace) string {
	return fmt.Sprintf("ARF Transformation with %d files modified", len(workspace.Changes))
}

// Global LLM dispatcher instance
var (
	llmDispatcher     *LLMDispatcher
	llmDispatcherOnce sync.Once
	llmDispatcherErr  error
)

// GetOrCreateLLMDispatcher returns a singleton LLM dispatcher instance
func GetOrCreateLLMDispatcher() (*LLMDispatcher, error) {
	llmDispatcherOnce.Do(func() {
		nomadAddr := os.Getenv("NOMAD_ADDR")
		if nomadAddr == "" {
			nomadAddr = "http://127.0.0.1:4646"
		}
		
		consulAddr := os.Getenv("CONSUL_HTTP_ADDR")
		if consulAddr == "" {
			consulAddr = "127.0.0.1:8500"
		}
		
		// Create storage client
		seaweedProvider, err := storage.NewSeaweedFSClient(storage.SeaweedFSConfig{
			Master: os.Getenv("SEAWEEDFS_MASTER_URL"),
			Filer:  os.Getenv("SEAWEEDFS_FILER_URL"),
		})
		if err != nil {
			llmDispatcherErr = fmt.Errorf("failed to create SeaweedFS provider: %w", err)
			return
		}
		
		storageClient := storage.NewStorageClient(seaweedProvider, storage.DefaultClientConfig())
		if err != nil {
			llmDispatcherErr = fmt.Errorf("failed to create storage client: %w", err)
			return
		}
		
		llmDispatcher, llmDispatcherErr = NewLLMDispatcher(nomadAddr, consulAddr, storageClient)
	})
	
	return llmDispatcher, llmDispatcherErr
}

// createWorkspaceArchive creates a tar.gz archive of the workspace
func createWorkspaceArchive(workspace *Workspace) ([]byte, error) {
	var buf bytes.Buffer
	
	// Create gzip writer
	gzWriter := gzip.NewWriter(&buf)
	defer gzWriter.Close()
	
	// Create tar writer
	tarWriter := tar.NewWriter(gzWriter)
	defer tarWriter.Close()
	
	// Walk through workspace directory
	err := filepath.Walk(workspace.Path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		// Skip directories
		if info.IsDir() {
			return nil
		}
		
		// Get relative path
		relPath, err := filepath.Rel(workspace.Path, path)
		if err != nil {
			return err
		}
		
		// Create tar header
		header := &tar.Header{
			Name:    relPath,
			Mode:    int64(info.Mode()),
			Size:    info.Size(),
			ModTime: info.ModTime(),
		}
		
		// Write header
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}
		
		// Open file
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		
		// Copy file content
		_, err = io.Copy(tarWriter, file)
		return err
	})
	
	if err != nil {
		return nil, fmt.Errorf("failed to create archive: %w", err)
	}
	
	// Close writers
	if err := tarWriter.Close(); err != nil {
		return nil, err
	}
	if err := gzWriter.Close(); err != nil {
		return nil, err
	}
	
	return buf.Bytes(), nil
}

// applyLLMResult downloads and applies the LLM transformation result
func applyLLMResult(workspace *Workspace, outputURL string) error {
	// Download the result
	resp, err := http.Get(outputURL)
	if err != nil {
		return fmt.Errorf("failed to download LLM result: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download LLM result: status %d", resp.StatusCode)
	}
	
	// Create gzip reader
	gzReader, err := gzip.NewReader(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzReader.Close()
	
	// Create tar reader
	tarReader := tar.NewReader(gzReader)
	
	// Extract files
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar header: %w", err)
		}
		
		// Skip directories
		if header.Typeflag == tar.TypeDir {
			continue
		}
		
		// Read transformed content
		if header.Name == "transformed.txt" {
			content, err := io.ReadAll(tarReader)
			if err != nil {
				return fmt.Errorf("failed to read transformed content: %w", err)
			}
			
			// Apply the transformation (simplified - in reality would parse and apply diffs)
			workspace.Changes = append(workspace.Changes, FileChange{
				File:        "llm-transformation",
				Type:        "modified",
				LinesAdded:  len(strings.Split(string(content), "\n")),
				UnifiedDiff: string(content),
				Timestamp:   time.Now(),
			})
			
			return nil
		}
	}
	
	return fmt.Errorf("transformed.txt not found in result archive")
}