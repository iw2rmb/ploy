package transflow

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/common"
)

// GitOperationsInterface defines the Git operations needed by the runner
type GitOperationsInterface interface {
	CloneRepository(ctx context.Context, repoURL, branch, targetPath string) error
	CreateBranchAndCheckout(ctx context.Context, repoPath, branchName string) error
	CommitChanges(ctx context.Context, repoPath, message string) error
	PushBranch(ctx context.Context, repoPath, remoteURL, branchName string) error
}

// RecipeExecutorInterface defines the recipe execution interface
type RecipeExecutorInterface interface {
	ExecuteRecipes(ctx context.Context, workspacePath string, recipeIDs []string) error
}

// BuildCheckerInterface defines the build check interface
type BuildCheckerInterface interface {
	CheckBuild(ctx context.Context, config common.DeployConfig) (*common.DeployResult, error)
}

// StepResult represents the result of executing a single step
type StepResult struct {
	StepID   string
	Success  bool
	Message  string
	Duration time.Duration
}

// TransflowResult represents the overall result of a transflow execution
type TransflowResult struct {
	Success        bool
	WorkflowID     string
	BranchName     string
	CommitSHA      string
	BuildVersion   string
	StepResults    []StepResult
	ErrorMessage   string
	Duration       time.Duration
	HealingSummary *TransflowHealingSummary
}

// Summary returns a human-readable summary of the transflow execution
func (r *TransflowResult) Summary() string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Workflow: %s\n", r.WorkflowID))

	if r.Success {
		sb.WriteString("Status: SUCCESS\n")
	} else {
		sb.WriteString("Status: FAILED\n")
	}

	if r.BranchName != "" {
		sb.WriteString(fmt.Sprintf("Branch: %s\n", r.BranchName))
	}

	if r.CommitSHA != "" {
		sb.WriteString(fmt.Sprintf("Commit: %s\n", r.CommitSHA))
	}

	if r.BuildVersion != "" {
		sb.WriteString(fmt.Sprintf("Build: %s\n", r.BuildVersion))
	}

	if !r.Success && r.ErrorMessage != "" {
		sb.WriteString(fmt.Sprintf("Error: %s\n", r.ErrorMessage))
	}

	sb.WriteString("Steps:\n")
	for _, step := range r.StepResults {
		status := "✓"
		if !step.Success {
			status = "✗"
		}
		sb.WriteString(fmt.Sprintf("  %s %s: %s\n", status, step.StepID, step.Message))
	}

	// Include healing summary if self-healing was enabled
	if r.HealingSummary != nil && r.HealingSummary.Enabled {
		sb.WriteString("\nSelf-Healing:\n")
		if r.HealingSummary.AttemptsCount > 0 {
			sb.WriteString(fmt.Sprintf("  Attempts: %d/%d\n", r.HealingSummary.AttemptsCount, r.HealingSummary.MaxRetries))
			sb.WriteString(fmt.Sprintf("  Successful fixes: %d\n", r.HealingSummary.TotalHealed))
			sb.WriteString(fmt.Sprintf("  Final result: %s\n", map[bool]string{true: "SUCCESS", false: "FAILED"}[r.HealingSummary.FinalSuccess]))
			
			for _, attempt := range r.HealingSummary.Attempts {
				status := "✗"
				if attempt.Success {
					status = "✓"
				}
				sb.WriteString(fmt.Sprintf("    %s Attempt %d: %s\n", status, attempt.AttemptNumber, 
					func() string {
						if attempt.Success {
							return fmt.Sprintf("Applied %d recipe(s)", len(attempt.AppliedRecipes))
						}
						return attempt.ErrorMessage
					}()))
			}
		} else {
			sb.WriteString("  No healing attempts made\n")
		}
	}

	return sb.String()
}

// TransflowRunner orchestrates the execution of transflow steps
type TransflowRunner struct {
    config         *TransflowConfig
    workspaceDir   string
    gitOps         GitOperationsInterface
    recipeExecutor RecipeExecutorInterface
    buildChecker   BuildCheckerInterface
    jobSubmitter   interface{} // For healing workflows
}

// NewTransflowRunner creates a new transflow runner with the given configuration
func NewTransflowRunner(config *TransflowConfig, workspaceDir string) (*TransflowRunner, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &TransflowRunner{
		config:       config,
		workspaceDir: workspaceDir,
	}, nil
}

// SetGitOperations sets the Git operations implementation (for dependency injection/testing)
func (r *TransflowRunner) SetGitOperations(gitOps GitOperationsInterface) {
	r.gitOps = gitOps
}

// SetRecipeExecutor sets the recipe executor implementation (for dependency injection/testing)
func (r *TransflowRunner) SetRecipeExecutor(executor RecipeExecutorInterface) {
	r.recipeExecutor = executor
}

// SetBuildChecker sets the build checker implementation (for dependency injection/testing)
func (r *TransflowRunner) SetBuildChecker(checker BuildCheckerInterface) {
    r.buildChecker = checker
}

// SetJobSubmitter sets the job submitter for healing workflows (for dependency injection/testing)
func (r *TransflowRunner) SetJobSubmitter(submitter interface{}) {
    r.jobSubmitter = submitter
}

// PlannerAssets holds file paths for rendered planner inputs and HCL
type PlannerAssets struct {
    InputsPath string
    HCLPath    string
}

// RenderPlannerAssets writes minimal inputs.json and a rendered planner.hcl (with placeholders) into the workspace.
// This is a dry-run helper to prepare artifacts for planner submission later.
func (r *TransflowRunner) RenderPlannerAssets() (*PlannerAssets, error) {
    inputsDir := filepath.Join(r.workspaceDir, "planner", "context")
    outDir := filepath.Join(r.workspaceDir, "planner", "out")
    if err := os.MkdirAll(inputsDir, 0755); err != nil {
        return nil, err
    }
    if err := os.MkdirAll(outDir, 0755); err != nil {
        return nil, err
    }
    // Minimal inputs.json
    inputsPath := filepath.Join(inputsDir, "inputs.json")
    inputs := fmt.Sprintf(`{
  "language": "java",
  "lane": %q,
  "last_error": {"stdout": "", "stderr": ""},
  "deps": {}
}`, r.config.Lane)
    
    if err := os.WriteFile(inputsPath, []byte(inputs), 0644); err != nil {
        return nil, err
    }
    
    // Copy HCL template
    hclTemplate := filepath.Join("roadmap", "transflow", "jobs", "planner.hcl")
    hclBytes, err := os.ReadFile(hclTemplate)
    if err != nil {
        return nil, fmt.Errorf("failed to read planner.hcl template: %w", err)
    }
    
    hclPath := filepath.Join(r.workspaceDir, "planner", "planner.hcl")
    if err := os.WriteFile(hclPath, hclBytes, 0644); err != nil {
        return nil, err
    }
    
    return &PlannerAssets{InputsPath: inputsPath, HCLPath: hclPath}, nil
}

// RenderLLMExecAssets writes a rendered llm_exec.hcl for the given option ID.
func (r *TransflowRunner) RenderLLMExecAssets(optionID string) (string, error) {
    dir := filepath.Join(r.workspaceDir, "llm-exec", optionID)
    if err := os.MkdirAll(dir, 0755); err != nil { return "", err }
    hclTemplate := filepath.Join("roadmap", "transflow", "jobs", "llm_exec.hcl")
    hclBytes, err := os.ReadFile(hclTemplate)
    if err != nil { return "", fmt.Errorf("failed to read llm_exec.hcl template: %w", err) }
    renderedPath := filepath.Join(dir, "llm_exec.rendered.hcl")
    // Defer env substitution to caller (same as planner/reducer), we just copy template here
    if err := os.WriteFile(renderedPath, hclBytes, 0644); err != nil { return "", err }
    return renderedPath, nil
}

// RenderORWApplyAssets writes a rendered orw_apply.hcl for the given option ID.
func (r *TransflowRunner) RenderORWApplyAssets(optionID string) (string, error) {
    dir := filepath.Join(r.workspaceDir, "orw-apply", optionID)
    if err := os.MkdirAll(dir, 0755); err != nil { return "", err }
    hclTemplate := filepath.Join("roadmap", "transflow", "jobs", "orw_apply.hcl")
    hclBytes, err := os.ReadFile(hclTemplate)
    if err != nil { return "", fmt.Errorf("failed to read orw_apply.hcl template: %w", err) }
    renderedPath := filepath.Join(dir, "orw_apply.rendered.hcl")
    if err := os.WriteFile(renderedPath, hclBytes, 0644); err != nil { return "", err }
    return renderedPath, nil
}

// PrepareRepo clones the target repository and creates a workflow branch; returns the repo path and branch name.
func (r *TransflowRunner) PrepareRepo(ctx context.Context) (string, string, error) {
    repoPath := filepath.Join(r.workspaceDir, "repo-apply")
    if err := r.gitOps.CloneRepository(ctx, r.config.TargetRepo, r.config.BaseRef, repoPath); err != nil {
        return "", "", fmt.Errorf("clone failed: %w", err)
    }
    branchName := GenerateBranchName(r.config.ID)
    if err := r.gitOps.CreateBranchAndCheckout(ctx, repoPath, branchName); err != nil {
        return "", "", fmt.Errorf("branch failed: %w", err)
    }
    return repoPath, branchName, nil
}

// ApplyDiffAndBuild validates and applies a diff, commits changes, and runs a build gate.
func (r *TransflowRunner) ApplyDiffAndBuild(ctx context.Context, repoPath, diffPath string) error {
    // Validate paths first (allowlist)
    allow := []string{"src/**", "pom.xml"}
    if v := os.Getenv("TRANSFLOW_ALLOWLIST"); v != "" { allow = strings.Split(v, ",") }
    if err := ValidateDiffPaths(diffPath, allow); err != nil { return err }
    if err := ValidateUnifiedDiff(ctx, repoPath, diffPath); err != nil {
        return err
    }
    if err := ApplyUnifiedDiff(ctx, repoPath, diffPath); err != nil {
        return err
    }
    if err := r.gitOps.CommitChanges(ctx, repoPath, "apply(diff): transflow branch patch"); err != nil {
        return fmt.Errorf("commit failed: %w", err)
    }
    // Build gate
    timeout, err := r.config.ParseBuildTimeout()
    if err != nil { return err }
    appName := GenerateAppName(r.config.ID)
    buildCfg := common.DeployConfig{
        App:         appName,
        Lane:        r.config.Lane,
        Environment: "dev",
        Timeout:     timeout,
    }
    res, err := r.buildChecker.CheckBuild(ctx, buildCfg)
    if err != nil { return fmt.Errorf("build gate failed: %w", err) }
    if res != nil && !res.Success { return fmt.Errorf("build gate failed: %s", res.Message) }
    return nil
}

// ReducerAssets holds file paths for rendered reducer inputs and HCL
type ReducerAssets struct {
    HistoryPath string
    HCLPath     string
}

// RenderReducerAssets writes a minimal history.json and a rendered reducer.hcl (with placeholders) into the workspace.
func (r *TransflowRunner) RenderReducerAssets() (*ReducerAssets, error) {
	ctxDir := filepath.Join(r.workspaceDir, "reducer", "context")
	if err := os.MkdirAll(ctxDir, 0755); err != nil {
		return nil, err
	}
	
	// Minimal history.json
	historyPath := filepath.Join(ctxDir, "history.json")
	history := "{\n  \"plan_id\": \"\",\n  \"branches\": [],\n  \"winner\": \"\"\n}"
	if err := os.WriteFile(historyPath, []byte(history), 0644); err != nil {
		return nil, err
	}
	
	// Copy HCL template
	hclTemplate := filepath.Join("roadmap", "transflow", "jobs", "reducer.hcl")
	hclBytes, err := os.ReadFile(hclTemplate)
	if err != nil {
		return nil, fmt.Errorf("failed to read reducer.hcl template: %w", err)
	}
	
	hclPath := filepath.Join(r.workspaceDir, "reducer", "reducer.hcl")
	if err := os.WriteFile(hclPath, hclBytes, 0644); err != nil {
		return nil, err
	}
	
	return &ReducerAssets{HistoryPath: historyPath, HCLPath: hclPath}, nil
}

// Run executes the complete transflow workflow
func (r *TransflowRunner) Run(ctx context.Context) (*TransflowResult, error) {
	startTime := time.Now()
	result := &TransflowResult{
		WorkflowID:  r.config.ID,
		StepResults: []StepResult{},
	}

	// Step 1: Clone repository
	repoPath := filepath.Join(r.workspaceDir, "repo")
	if err := r.gitOps.CloneRepository(ctx, r.config.TargetRepo, r.config.BaseRef, repoPath); err != nil {
		result.ErrorMessage = fmt.Sprintf("failed to clone repository: %v", err)
		result.Duration = time.Since(startTime)
		return nil, fmt.Errorf("failed to clone repository: %w", err)
	}
	result.StepResults = append(result.StepResults, StepResult{
		StepID:  "clone",
		Success: true,
		Message: fmt.Sprintf("Cloned %s at %s", r.config.TargetRepo, r.config.BaseRef),
	})

	// Step 2: Create and checkout workflow branch
	branchName := GenerateBranchName(r.config.ID)
	result.BranchName = branchName
	if err := r.gitOps.CreateBranchAndCheckout(ctx, repoPath, branchName); err != nil {
		result.ErrorMessage = fmt.Sprintf("failed to create branch: %v", err)
		result.Duration = time.Since(startTime)
		return nil, fmt.Errorf("failed to create branch: %w", err)
	}
	result.StepResults = append(result.StepResults, StepResult{
		StepID:  "create-branch",
		Success: true,
		Message: fmt.Sprintf("Created workflow branch: %s", branchName),
	})

	// Step 3: Execute recipe steps
	for _, step := range r.config.Steps {
		if step.Type == "recipe" {
			stepStart := time.Now()
			if err := r.recipeExecutor.ExecuteRecipes(ctx, repoPath, step.Recipes); err != nil {
				result.StepResults = append(result.StepResults, StepResult{
					StepID:   step.ID,
					Success:  false,
					Message:  fmt.Sprintf("Recipe execution failed: %v", err),
					Duration: time.Since(stepStart),
				})
				result.ErrorMessage = fmt.Sprintf("failed to execute recipes: %v", err)
				result.Duration = time.Since(startTime)
				return nil, fmt.Errorf("failed to execute recipes: %w", err)
			}

			result.StepResults = append(result.StepResults, StepResult{
				StepID:   step.ID,
				Success:  true,
				Message:  fmt.Sprintf("Applied recipe: %s", strings.Join(step.Recipes, ", ")),
				Duration: time.Since(stepStart),
			})
		}
	}

	// Step 4: Commit changes
	commitMessage := fmt.Sprintf("Applied recipe transformations for workflow %s", r.config.ID)
	if err := r.gitOps.CommitChanges(ctx, repoPath, commitMessage); err != nil {
		result.ErrorMessage = fmt.Sprintf("failed to commit changes: %v", err)
		result.Duration = time.Since(startTime)
		return nil, fmt.Errorf("failed to commit changes: %w", err)
	}
	result.StepResults = append(result.StepResults, StepResult{
		StepID:  "commit",
		Success: true,
		Message: "Committed changes",
	})

	// Step 5: Run build check
	buildStart := time.Now()
	appName := GenerateAppName(r.config.ID)
	timeout, err := r.config.ParseBuildTimeout()
	if err != nil {
		result.ErrorMessage = fmt.Sprintf("invalid build timeout: %v", err)
		result.Duration = time.Since(startTime)
		return nil, fmt.Errorf("invalid build timeout: %w", err)
	}

	buildConfig := common.DeployConfig{
		App:           appName,
		Lane:          r.config.Lane,
		Environment:   "dev",
		Timeout:       timeout,
		ControllerURL: "", // Will be set by the actual implementation
	}

	buildResult, err := r.buildChecker.CheckBuild(ctx, buildConfig)
	if err != nil || (buildResult != nil && !buildResult.Success) {
		message := "Build check failed"
		if buildResult != nil && buildResult.Message != "" {
			message = buildResult.Message
		}
		if err != nil {
			message = fmt.Sprintf("%s: %v", message, err)
		}

		result.StepResults = append(result.StepResults, StepResult{
			StepID:   "build",
			Success:  false,
			Message:  message,
			Duration: time.Since(buildStart),
		})

		// Check if self-healing is enabled
		if r.config.SelfHeal != nil && r.config.SelfHeal.Enabled && r.jobSubmitter != nil {
			// Attempt healing workflow
			healingSummary, healingErr := r.attemptHealing(ctx, repoPath, message)
			result.HealingSummary = healingSummary
			
			if healingErr == nil && healingSummary.Winner != nil {
				// Healing succeeded! Continue with the healed version
				result.StepResults = append(result.StepResults, StepResult{
					StepID:   "healing",
					Success:  true,
					Message:  fmt.Sprintf("Healing succeeded with plan %s", healingSummary.PlanID),
					Duration: time.Since(buildStart),
				})
				// Continue with normal workflow (push, etc.)
			} else {
				// Healing also failed
				result.ErrorMessage = "build check failed and healing failed"
				result.Duration = time.Since(startTime)
				return nil, fmt.Errorf("build check failed: %s (healing also failed: %v)", message, healingErr)
			}
		} else {
			// No healing enabled, fail immediately
			result.ErrorMessage = "build check failed"
			result.Duration = time.Since(startTime)
			return nil, fmt.Errorf("build check failed: %s", message)
		}
	}

	if buildResult != nil {
		result.BuildVersion = buildResult.Version
	}
	result.StepResults = append(result.StepResults, StepResult{
		StepID:   "build",
		Success:  true,
		Message:  "Build completed successfully",
		Duration: time.Since(buildStart),
	})

	// Step 6: Push branch
	if err := r.gitOps.PushBranch(ctx, repoPath, r.config.TargetRepo, branchName); err != nil {
		result.StepResults = append(result.StepResults, StepResult{
			StepID:  "push",
			Success: false,
			Message: fmt.Sprintf("Push failed: %v", err),
		})
		result.ErrorMessage = fmt.Sprintf("failed to push branch: %v", err)
		result.Duration = time.Since(startTime)
		return nil, fmt.Errorf("failed to push branch: %w", err)
	}
	result.StepResults = append(result.StepResults, StepResult{
		StepID:  "push",
		Success: true,
		Message: fmt.Sprintf("Pushed branch %s", branchName),
	})

	// Success!
	result.Success = true
	result.Duration = time.Since(startTime)
	return result, nil
}

// attemptHealing orchestrates the healing workflow: planner → fanout → reducer
func (r *TransflowRunner) attemptHealing(ctx context.Context, repoPath string, buildError string) (*TransflowHealingSummary, error) {
	summary := &TransflowHealingSummary{
		Enabled:       true,
		AttemptsCount: 1,
	}

	// Step 1: Submit planner job to analyze the build error
	jobHelper := NewJobSubmissionHelper(r.jobSubmitter)
	planResult, err := jobHelper.SubmitPlannerJob(ctx, r.config, buildError, r.workspaceDir)
	if err != nil {
		return summary, fmt.Errorf("planner job failed: %w", err)
	}
	
	summary.PlanID = planResult.PlanID

	// Step 2: Convert planner options to branch specs
	var branches []BranchSpec
	for i, option := range planResult.Options {
		branchID := fmt.Sprintf("option-%d", i)
		if id, ok := option["id"].(string); ok {
			branchID = id
		}
		
		branchType := "llm-exec" // default
		if t, ok := option["type"].(string); ok {
			branchType = t
		}

		branches = append(branches, BranchSpec{
			ID:     branchID,
			Type:   branchType,
			Inputs: option,
		})
	}

	// Step 3: Execute fanout orchestration
	orchestrator := NewFanoutOrchestrator(r.jobSubmitter)
	maxParallel := 3 // Default parallelism
	if r.config.SelfHeal.MaxRetries > 0 {
		maxParallel = r.config.SelfHeal.MaxRetries
	}

	winner, allResults, err := orchestrator.RunHealingFanout(ctx, nil, branches, maxParallel)
	summary.AllResults = allResults
	
	if err != nil {
		// Fanout failed, but continue to reducer anyway
		summary.Winner = nil
	} else {
		summary.Winner = &winner
	}

	// Step 4: Submit reducer job to determine next action
	nextAction, reducerErr := jobHelper.SubmitReducerJob(ctx, planResult.PlanID, allResults, summary.Winner, r.workspaceDir)
	if reducerErr != nil {
		return summary, fmt.Errorf("reducer job failed: %w", reducerErr)
	}

	// If reducer says to stop and we have a winner, healing succeeded
	if nextAction.Action == "stop" && summary.Winner != nil {
		return summary, nil
	}

	// Otherwise, healing failed
	return summary, fmt.Errorf("healing failed: %s", nextAction.Notes)
}

// CleanupWorkspace removes the temporary workspace directory
func (r *TransflowRunner) CleanupWorkspace() error {
	if r.workspaceDir != "" {
		return os.RemoveAll(r.workspaceDir)
	}
	return nil
}
