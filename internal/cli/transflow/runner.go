package transflow

import (
    "context"
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
    "strings"
    "time"

    "github.com/iw2rmb/ploy/internal/cli/common"
    "github.com/iw2rmb/ploy/internal/git/provider"
    "github.com/iw2rmb/ploy/internal/orchestration"
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
	MRURL          string // GitLab merge request URL if created
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

	// Include MR URL if available
	if r.MRURL != "" {
		sb.WriteString(fmt.Sprintf("\nMerge Request: %s\n", r.MRURL))
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
	jobSubmitter   interface{}          // For healing workflows
	gitProvider    provider.GitProvider // For MR creation
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

// SetGitProvider sets the Git provider implementation for MR creation (for dependency injection/testing)
func (r *TransflowRunner) SetGitProvider(provider provider.GitProvider) {
	r.gitProvider = provider
}

// GetGitProvider returns the Git provider for human-step branch operations
func (r *TransflowRunner) GetGitProvider() provider.GitProvider {
	return r.gitProvider
}

// GetBuildChecker returns the build checker for human-step branch operations
func (r *TransflowRunner) GetBuildChecker() BuildCheckerInterface {
	return r.buildChecker
}

// GetWorkspaceDir returns the workspace directory for human-step branch operations
func (r *TransflowRunner) GetWorkspaceDir() string {
	return r.workspaceDir
}

// GetTargetRepo returns the target repository URL for human-step branch operations
func (r *TransflowRunner) GetTargetRepo() string {
	return r.config.TargetRepo
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

    // Read planner template from workspace (provided by server)
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
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
    hclTemplate := filepath.Join("roadmap", "transflow", "jobs", "llm_exec.hcl")
    hclBytes, err := os.ReadFile(hclTemplate)
    if err != nil {
        return "", fmt.Errorf("failed to read llm_exec.hcl template: %w", err)
    }
	renderedPath := filepath.Join(dir, "llm_exec.rendered.hcl")
	// Defer env substitution to caller (same as planner/reducer), we just copy template here
	if err := os.WriteFile(renderedPath, hclBytes, 0644); err != nil {
		return "", err
	}
	return renderedPath, nil
}

// RenderORWApplyAssets writes a rendered orw_apply.hcl for the given option ID.
func (r *TransflowRunner) RenderORWApplyAssets(optionID string) (string, error) {
    dir := filepath.Join(r.workspaceDir, "orw-apply", optionID)
    if err := os.MkdirAll(dir, 0755); err != nil {
        return "", err
    }
    hclTemplate := filepath.Join("roadmap", "transflow", "jobs", "orw_apply.hcl")
    hclBytes, err := os.ReadFile(hclTemplate)
    if err != nil {
        return "", fmt.Errorf("failed to read orw_apply.hcl template: %w", err)
    }
	renderedPath := filepath.Join(dir, "orw_apply.rendered.hcl")
	if err := os.WriteFile(renderedPath, hclBytes, 0644); err != nil {
		return "", err
	}
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
	if v := os.Getenv("TRANSFLOW_ALLOWLIST"); v != "" {
		allow = strings.Split(v, ",")
	}
	if err := ValidateDiffPaths(diffPath, allow); err != nil {
		return err
	}
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
	if err != nil {
		return err
	}
	appName := GenerateAppName(r.config.ID)
	buildCfg := common.DeployConfig{
		App:         appName,
		Lane:        r.config.Lane,
		Environment: "dev",
		Timeout:     timeout,
	}
	// Ensure build tar is created from the repository root
	cwd, _ := os.Getwd()
	_ = os.Chdir(repoPath)
	res, err := r.buildChecker.CheckBuild(ctx, buildCfg)
	_ = os.Chdir(cwd)
	if err != nil {
		return fmt.Errorf("build gate failed: %w", err)
	}
	if res != nil && !res.Success {
		return fmt.Errorf("build gate failed: %s", res.Message)
	}
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

    // Capture initial HEAD to detect later if steps produced a commit
    initialHead, _ := getHeadHash(repoPath)

    // Step 3: Execute transformation steps
    for _, step := range r.config.Steps {
        switch step.Type {
        case "orw-apply":
            stepStart := time.Now()
            // Render ORW apply HCL assets
            renderedPath, err := r.RenderORWApplyAssets(step.ID)
            if err != nil {
                result.StepResults = append(result.StepResults, StepResult{StepID: step.ID, Success: false, Message: fmt.Sprintf("Failed to render ORW assets: %v", err)})
                result.ErrorMessage = fmt.Sprintf("failed to render orw-apply assets: %v", err)
                result.Duration = time.Since(startTime)
                return nil, fmt.Errorf("failed to render orw-apply assets: %w", err)
            }

            // Prepare input tar from repository
            inputTar := filepath.Join(filepath.Dir(renderedPath), "input.tar")
            if err := createTarFromDir(repoPath, inputTar); err != nil {
                result.StepResults = append(result.StepResults, StepResult{StepID: step.ID, Success: false, Message: fmt.Sprintf("Failed to create input tar: %v", err)})
                return nil, fmt.Errorf("failed to create input tar: %w", err)
            }

            // Pre-substitute recipe class and input tar host path into template
            hclBytes, err := os.ReadFile(renderedPath)
            if err != nil {
                result.StepResults = append(result.StepResults, StepResult{StepID: step.ID, Success: false, Message: fmt.Sprintf("Failed to read HCL: %v", err)})
                return nil, fmt.Errorf("failed to read HCL: %w", err)
            }
            rclass := ""
            if len(step.Recipes) > 0 {
                rclass = step.Recipes[0]
            }
            // Determine coords and discovery flag
            discover := "true"
            rgroup, rartifact, rversion := "", "", ""
            if strings.HasPrefix(rclass, "org.openrewrite.java.migrate") {
                rgroup, rartifact, rversion = "org.openrewrite.recipe", "rewrite-migrate-java", "2.11.0"
                discover = "false"
            } else if strings.HasPrefix(rclass, "org.openrewrite.java.spring") {
                rgroup, rartifact, rversion = "org.openrewrite.recipe", "rewrite-spring", "5.7.0"
                discover = "false"
            } else if strings.HasPrefix(rclass, "org.openrewrite.java") {
                rgroup, rartifact, rversion = "org.openrewrite", "rewrite-java", "8.21.0"
                discover = "false"
            }
            // Create run ID for this submission and then substitute it
            runID := fmt.Sprintf("orw-apply-%s-%d", step.ID, time.Now().Unix())
            prePath := strings.ReplaceAll(renderedPath, ".rendered.hcl", ".pre.hcl")
            preContent := strings.ReplaceAll(string(hclBytes), "${RECIPE_CLASS}", rclass)
            preContent = strings.ReplaceAll(preContent, "${INPUT_TAR_HOST_PATH}", inputTar)
            preContent = strings.ReplaceAll(preContent, "${RUN_ID}", runID)
            preContent = strings.ReplaceAll(preContent, "${DISCOVER_RECIPE}", discover)
            preContent = strings.ReplaceAll(preContent, "${RECIPE_GROUP}", rgroup)
            preContent = strings.ReplaceAll(preContent, "${RECIPE_ARTIFACT}", rartifact)
            preContent = strings.ReplaceAll(preContent, "${RECIPE_VERSION}", rversion)
            if err := os.WriteFile(prePath, []byte(preContent), 0644); err != nil {
                result.StepResults = append(result.StepResults, StepResult{StepID: step.ID, Success: false, Message: fmt.Sprintf("Failed to write pre-HCL: %v", err)})
                return nil, fmt.Errorf("failed to write pre-substituted HCL: %w", err)
            }

            // Prepare env and substitute final template
            baseDir := filepath.Dir(renderedPath)
            _ = os.MkdirAll(filepath.Join(baseDir, "out"), 0755)
            os.Setenv("TRANSFLOW_CONTEXT_DIR", baseDir)
            os.Setenv("TRANSFLOW_OUT_DIR", filepath.Join(baseDir, "out"))
            submittedPath, err := substituteORWTemplate(prePath, runID)
            if err != nil {
                result.StepResults = append(result.StepResults, StepResult{StepID: step.ID, Success: false, Message: fmt.Sprintf("Failed to substitute ORW HCL: %v", err)})
                return nil, fmt.Errorf("failed to substitute ORW HCL: %w", err)
            }

            // Submit job and wait terminal
            if err := orchestration.SubmitAndWaitTerminal(submittedPath, 30*time.Minute); err != nil {
                result.StepResults = append(result.StepResults, StepResult{StepID: step.ID, Success: false, Message: fmt.Sprintf("ORW apply failed: %v", err)})
                result.ErrorMessage = fmt.Sprintf("orw-apply job failed: %v", err)
                result.Duration = time.Since(startTime)
                return nil, fmt.Errorf("orw-apply job failed: %w", err)
            }

            // Locate diff.patch and apply
            diffPath := filepath.Join(baseDir, "out", "diff.patch")
            if _, err := os.Stat(diffPath); err != nil {
                result.StepResults = append(result.StepResults, StepResult{StepID: step.ID, Success: false, Message: fmt.Sprintf("No diff.patch produced: %v", err)})
                result.ErrorMessage = "no diff produced by orw-apply"
                result.Duration = time.Since(startTime)
                return nil, fmt.Errorf("no diff produced by orw-apply: %w", err)
            }

            if err := r.ApplyDiffAndBuild(ctx, repoPath, diffPath); err != nil {
                result.StepResults = append(result.StepResults, StepResult{StepID: step.ID, Success: false, Message: fmt.Sprintf("Apply/build failed: %v", err)})
                result.ErrorMessage = fmt.Sprintf("apply/build failed: %v", err)
                result.Duration = time.Since(startTime)
                return nil, fmt.Errorf("apply/build failed: %w", err)
            }

            result.StepResults = append(result.StepResults, StepResult{
                StepID:   step.ID,
                Success:  true,
                Message:  "Applied ORW diff and passed build gate",
                Duration: time.Since(stepStart),
            })

        case "recipe":
            // Deprecated: recipe step is no longer supported in main workflow
            return nil, fmt.Errorf("recipe step is no longer supported; use orw-apply")
        }
    }

    // Step 4: Commit changes (only if not already committed by an apply step)
    headBefore := initialHead
    // Re-check working tree status
    changed, _ := hasRepoChanges(repoPath)
    commitMessage := fmt.Sprintf("Applied recipe transformations for workflow %s", r.config.ID)
    if !changed {
        // No staged/working changes; check if HEAD moved (apply step may have committed already)
        headAfter, _ := getHeadHash(repoPath)
        if headAfter != "" && headBefore != "" && headAfter != headBefore {
            // Consider commit step successful without creating a new commit
            result.StepResults = append(result.StepResults, StepResult{
                StepID:  "commit",
                Success: true,
                Message: "Changes already committed by apply step",
            })
            goto build_step
        }
        // No changes and HEAD same => fail to avoid empty MR
        result.StepResults = append(result.StepResults, StepResult{
            StepID:  "commit",
            Success: false,
            Message: "No changes to commit",
        })
        result.ErrorMessage = "no changes produced by transformation"
        result.Duration = time.Since(startTime)
        return nil, fmt.Errorf("no changes produced by transformation")
    }
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

build_step:
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

	// Ensure build tar is created from the repository root
	cwd2, _ := os.Getwd()
	_ = os.Chdir(repoPath)
	buildResult, err := r.buildChecker.CheckBuild(ctx, buildConfig)
	_ = os.Chdir(cwd2)
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

	// Step 7: Create or update GitLab merge request (if GitProvider is configured)
	if r.gitProvider != nil {
		mrStart := time.Now()
		if err := r.gitProvider.ValidateConfiguration(); err != nil {
			// MR creation is optional - log but don't fail the workflow
			result.StepResults = append(result.StepResults, StepResult{
				StepID:   "mr",
				Success:  false,
				Message:  fmt.Sprintf("MR creation skipped - configuration invalid: %v", err),
				Duration: time.Since(mrStart),
			})
		} else {
			mrConfig := provider.MRConfig{
				RepoURL:      r.config.TargetRepo,
				SourceBranch: branchName,
				TargetBranch: r.config.BaseRef,
				Title:        fmt.Sprintf("Transflow: %s", r.config.ID),
				Description:  r.generateMRDescription(result),
				Labels:       []string{"ploy", "tfl"},
			}

			mrResult, err := r.gitProvider.CreateOrUpdateMR(ctx, mrConfig)
			if err != nil {
				// MR creation is optional - log but don't fail the workflow
				result.StepResults = append(result.StepResults, StepResult{
					StepID:   "mr",
					Success:  false,
					Message:  fmt.Sprintf("MR creation failed: %v", err),
					Duration: time.Since(mrStart),
				})
			} else {
				action := "created"
				if !mrResult.Created {
					action = "updated"
				}
				result.StepResults = append(result.StepResults, StepResult{
					StepID:   "mr",
					Success:  true,
					Message:  fmt.Sprintf("MR %s: %s", action, mrResult.MRURL),
					Duration: time.Since(mrStart),
				})
				result.MRURL = mrResult.MRURL
			}
		}
	}

	// Success!
	result.Success = true
	result.Duration = time.Since(startTime)
	return result, nil
}

// generateMRDescription creates a descriptive merge request body based on workflow results
func (r *TransflowRunner) generateMRDescription(result *TransflowResult) string {
	var description strings.Builder

	description.WriteString(fmt.Sprintf("## Transflow Workflow: %s\n\n", r.config.ID))

	// Add basic workflow information
	description.WriteString(fmt.Sprintf("**Branch:** %s\n", result.BranchName))
	if result.BuildVersion != "" {
		description.WriteString(fmt.Sprintf("**Build Version:** %s\n", result.BuildVersion))
	}
	description.WriteString(fmt.Sprintf("**Duration:** %s\n\n", result.Duration.String()))

	// Add applied transformations
	description.WriteString("## Applied Transformations\n\n")
	for _, step := range result.StepResults {
		if step.StepID != "mr" && step.Success {
			description.WriteString(fmt.Sprintf("- ✅ **%s**: %s\n", strings.Title(step.StepID), step.Message))
		}
	}

	// Add healing information if present
	if result.HealingSummary != nil && result.HealingSummary.Winner != nil {
		description.WriteString("\n## Self-Healing Applied\n\n")
		description.WriteString(fmt.Sprintf("- **Plan ID:** %s\n", result.HealingSummary.PlanID))
		description.WriteString(fmt.Sprintf("- **Winning Strategy:** %s\n", result.HealingSummary.Winner.ID))
		description.WriteString(fmt.Sprintf("- **Attempts:** %d\n", result.HealingSummary.AttemptsCount))
	}

	// Add footer with automation info
	description.WriteString("\n---\n")
	description.WriteString("🤖 *This merge request was automatically created by Ploy Transflow*\n")
	description.WriteString("📝 *Labels: ploy, tfl*")

	return description.String()
}

// attemptHealing orchestrates the healing workflow: planner → fanout → reducer
func (r *TransflowRunner) attemptHealing(ctx context.Context, repoPath string, buildError string) (*TransflowHealingSummary, error) {
	summary := &TransflowHealingSummary{
		Enabled:       true,
		AttemptsCount: 1,
	}

	// Step 1: Submit planner job to analyze the build error
	jobHelper := NewJobSubmissionHelperWithRunner(r.jobSubmitter, r)
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
	orchestrator := NewFanoutOrchestratorWithRunner(r.jobSubmitter, r)
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
		summary.SetFinalResult(true)
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

// hasRepoChanges returns true if the working tree has any changes
func hasRepoChanges(repoPath string) (bool, error) {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = repoPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("git status failed: %v: %s", err, string(out))
	}
	return strings.TrimSpace(string(out)) != "", nil
}

// getHeadHash returns the current HEAD commit hash
func getHeadHash(repoPath string) (string, error) {
    cmd := exec.Command("git", "rev-parse", "HEAD")
    cmd.Dir = repoPath
    out, err := cmd.CombinedOutput()
    if err != nil {
        return "", fmt.Errorf("git rev-parse failed: %v: %s", err, string(out))
    }
    return strings.TrimSpace(string(out)), nil
}

// createTarFromDir creates a tar archive of a directory using system tar
func createTarFromDir(srcDir, dstTar string) error {
    // Remove existing tar if any
    _ = os.Remove(dstTar)
    cmd := exec.Command("tar", "-cf", dstTar, ".")
    cmd.Dir = srcDir
    if out, err := cmd.CombinedOutput(); err != nil {
        return fmt.Errorf("tar failed: %v: %s", err, string(out))
    }
    return nil
}
