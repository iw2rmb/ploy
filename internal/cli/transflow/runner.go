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
		result.ErrorMessage = "build check failed"
		result.Duration = time.Since(startTime)
		return nil, fmt.Errorf("build check failed: %s", message)
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

// CleanupWorkspace removes the temporary workspace directory
func (r *TransflowRunner) CleanupWorkspace() error {
	if r.workspaceDir != "" {
		return os.RemoveAll(r.workspaceDir)
	}
	return nil
}
