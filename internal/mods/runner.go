package mods

import (
	"context"
	crand "crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/common"
	"github.com/iw2rmb/ploy/internal/git/provider"
	"github.com/iw2rmb/ploy/internal/orchestration"
	nomadtpl "github.com/iw2rmb/ploy/platform/nomad/transflow"
)

// submitAndWaitTerminal is a package-level indirection to allow test stubbing.
// By default it points to orchestration.SubmitAndWaitTerminal.
var submitAndWaitTerminal = orchestration.SubmitAndWaitTerminal

// validateJob is a package-level indirection for orchestration.ValidateJob to ease unit testing.
var validateJob = orchestration.ValidateJob
var validateDiffPathsFn = ValidateDiffPaths
var validateUnifiedDiffFn = ValidateUnifiedDiff
var applyUnifiedDiffFn = ApplyUnifiedDiff
var hasRepoChangesFn = hasRepoChanges

// ErrNoBuildFile indicates missing supported build files in repository.
var ErrNoBuildFile = errors.New("no build file found in repository")

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
	repoManager    RepoManager
	recipeExecutor RecipeExecutorInterface
	transformExec  TransformationExecutor
	buildChecker   BuildCheckerInterface
	buildGate      BuildGate
	jobSubmitter   JobSubmitter         // For healing workflows
	gitProvider    provider.GitProvider // For MR creation
	mrManager      MRManager
	eventReporter  EventReporter // Optional real-time event reporter
	healer         HealingOrchestrator
	hcl            HCLSubmitter
	jobHelper      JobSubmissionHelper
}

// NewTransflowRunner creates a new transflow runner with the given configuration
func NewTransflowRunner(config *TransflowConfig, workspaceDir string) (*TransflowRunner, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &TransflowRunner{
		config:       config,
		workspaceDir: workspaceDir,
		hcl:          DefaultHCLSubmitter{},
	}, nil
}

// SetGitOperations sets the Git operations implementation (for dependency injection/testing)
func (r *TransflowRunner) SetGitOperations(gitOps GitOperationsInterface) {
	r.gitOps = gitOps
	if gitOps != nil {
		r.repoManager = NewRepoManagerAdapter(gitOps)
	} else {
		r.repoManager = nil
	}
}

// SetRecipeExecutor sets the recipe executor implementation (for dependency injection/testing)
func (r *TransflowRunner) SetRecipeExecutor(executor RecipeExecutorInterface) {
	r.recipeExecutor = executor
}

// SetTransformationExecutor sets the modular TransformationExecutor
func (r *TransflowRunner) SetTransformationExecutor(x TransformationExecutor) { r.transformExec = x }

// SetBuildChecker sets the build checker implementation (for dependency injection/testing)
func (r *TransflowRunner) SetBuildChecker(checker BuildCheckerInterface) {
	r.buildChecker = checker
	// Also expose through BuildGate adapter for modularization
	if checker != nil {
		r.buildGate = NewBuildGateAdapter(checker)
	} else {
		r.buildGate = nil
	}
}

// SetBuildGate sets the modular BuildGate; takes precedence over buildChecker when set.
func (r *TransflowRunner) SetBuildGate(g BuildGate) { r.buildGate = g }

// SetJobSubmitter sets the job submitter for healing workflows (for dependency injection/testing)
func (r *TransflowRunner) SetJobSubmitter(submitter JobSubmitter) {
	r.jobSubmitter = submitter
}

// SetGitProvider sets the Git provider implementation for MR creation (for dependency injection/testing)
func (r *TransflowRunner) SetGitProvider(provider provider.GitProvider) {
	r.gitProvider = provider
	if provider != nil {
		r.mrManager = NewMRManagerAdapter(provider)
	} else {
		r.mrManager = nil
	}
}

// SetEventReporter sets the reporter used for real-time observability
func (r *TransflowRunner) SetEventReporter(reporter EventReporter) {
	r.eventReporter = reporter
}

// SetHealingOrchestrator sets the modular healing orchestrator
func (r *TransflowRunner) SetHealingOrchestrator(h HealingOrchestrator) { r.healer = h }

// SetHCLSubmitter sets the indirection used for HCL validate/submit flows.
func (r *TransflowRunner) SetHCLSubmitter(h HCLSubmitter) { r.hcl = h }

// SetJobHelper allows injecting a planner/reducer submission helper for testing.
func (r *TransflowRunner) SetJobHelper(h JobSubmissionHelper) { r.jobHelper = h }

// GetHCLSubmitter exposes the HCLSubmitter for helpers that need it.
func (r *TransflowRunner) GetHCLSubmitter() HCLSubmitter { return r.hcl }

func (r *TransflowRunner) emit(ctx context.Context, phase, step, level, message string) {
	if r.eventReporter != nil {
		_ = r.eventReporter.Report(ctx, Event{Phase: phase, Step: step, Level: level, Message: message, Time: time.Now()})
		return
	}
	// Fallback to local log output when no reporter is configured
    log.Printf("[Mods][%s/%s][%s] %s", phase, step, level, message)
}

// GetEventReporter exposes the reporter for orchestrators
func (r *TransflowRunner) GetEventReporter() EventReporter {
	return r.eventReporter
}

// reportLastJobAsync looks up allocation ID and reports job metadata once available
func (r *TransflowRunner) reportLastJobAsync(ctx context.Context, jobName, phase, step string) {
	if r.eventReporter == nil || jobName == "" {
		return
	}
	go func() {
		// brief delay to allow registration
		select {
		case <-time.After(1 * time.Second):
		case <-ctx.Done():
			return
		}
		deadline := time.Now().Add(1 * time.Minute)
		for time.Now().Before(deadline) {
			if id := findFirstAllocID(jobName); id != "" {
				_ = r.eventReporter.Report(ctx, Event{Phase: phase, Step: step, Level: "info", Message: "job submitted", JobName: jobName, AllocID: id, Time: time.Now()})
				return
			}
			select {
			case <-time.After(1 * time.Second):
			case <-ctx.Done():
				return
			}
		}
	}()
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

// test indirections moved to job_io.go

// randomStepID returns s-<12 hex chars>
func randomStepID() string {
	var buf [6]byte
	_, _ = crand.Read(buf[:])
	return "s-" + hex.EncodeToString(buf[:])
}

// IO helpers moved to job_io.go; keep indirection vars there

// uploadInputTar uploads input.tar to artifacts/transflow/<execID>/input.tar (best-effort)
func uploadInputTar(seaweedBase, execID, inputTarPath string) error {
	key := fmt.Sprintf("transflow/%s/input.tar", execID)
	return putFileFn(seaweedBase, key, inputTarPath, "application/octet-stream")
}

// JSON helpers moved to job_io.go; keep indirection vars there

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

	// Write embedded planner template into workspace
	hclPath := filepath.Join(r.workspaceDir, "planner", "planner.hcl")
	if err := os.WriteFile(hclPath, nomadtpl.GetPlannerTemplate(), 0644); err != nil {
		return nil, err
	}

	return &PlannerAssets{InputsPath: inputsPath, HCLPath: hclPath}, nil
}

// RenderLLMExecAssets writes a rendered llm_exec.hcl for the given option ID.
func (r *TransflowRunner) RenderLLMExecAssets(optionID string) (string, error) {
	dir := filepath.Join(r.workspaceDir, string(StepTypeLLMExec), optionID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	renderedPath := filepath.Join(dir, "llm_exec.rendered.hcl")
	// Defer env substitution to caller (same as planner/reducer), we just copy template here
	if err := os.WriteFile(renderedPath, nomadtpl.GetLLMExecTemplate(), 0644); err != nil {
		return "", err
	}
	return renderedPath, nil
}

// RenderORWApplyAssets writes a rendered orw_apply.hcl for the given option ID.
func (r *TransflowRunner) RenderORWApplyAssets(optionID string) (string, error) {
	dir := filepath.Join(r.workspaceDir, string(StepTypeORWApply), optionID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	renderedPath := filepath.Join(dir, "orw_apply.rendered.hcl")
	if err := os.WriteFile(renderedPath, nomadtpl.GetORWApplyTemplate(), 0644); err != nil {
		return "", err
	}
	return renderedPath, nil
}

// PrepareRepo clones the target repository and creates a workflow branch; returns the repo path and branch name.
func (r *TransflowRunner) PrepareRepo(ctx context.Context) (string, string, error) {
	repoPath := filepath.Join(r.workspaceDir, "repo-apply")
	if r.repoManager != nil {
		if err := r.repoManager.Clone(ctx, r.config.TargetRepo, r.config.BaseRef, repoPath); err != nil {
			return "", "", fmt.Errorf("clone failed: %w", err)
		}
	} else if err := r.gitOps.CloneRepository(ctx, r.config.TargetRepo, r.config.BaseRef, repoPath); err != nil {
		return "", "", fmt.Errorf("clone failed: %w", err)
	}
	branchName := GenerateBranchName(r.config.ID)
	if r.repoManager != nil {
		if err := r.repoManager.CreateBranch(ctx, repoPath, branchName); err != nil {
			return "", "", fmt.Errorf("branch failed: %w", err)
		}
	} else if err := r.gitOps.CreateBranchAndCheckout(ctx, repoPath, branchName); err != nil {
		return "", "", fmt.Errorf("branch failed: %w", err)
	}
	return repoPath, branchName, nil
}

// ApplyDiffAndBuild validates and applies a diff, commits changes, and runs a build gate.
func (r *TransflowRunner) ApplyDiffAndBuild(ctx context.Context, repoPath, diffPath string) error {
	// Validate paths first (allowlist)
	allow := ResolveDefaultsFromEnv().Allowlist
	if err := validateDiffPathsFn(diffPath, allow); err != nil {
		return err
	}
	if err := validateUnifiedDiffFn(ctx, repoPath, diffPath); err != nil {
		return err
	}
	if err := applyUnifiedDiffFn(ctx, repoPath, diffPath); err != nil {
		return err
	}
	if r.repoManager != nil {
		if err := r.repoManager.Commit(ctx, repoPath, "apply(diff): transflow branch patch"); err != nil {
			return fmt.Errorf("commit failed: %w", err)
		}
	} else if err := r.gitOps.CommitChanges(ctx, repoPath, "apply(diff): transflow branch patch"); err != nil {
		return fmt.Errorf("commit failed: %w", err)
	}
	// Build gate
	res, err := r.runBuildGate(ctx, repoPath)
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

	hclPath := filepath.Join(r.workspaceDir, "reducer", "reducer.hcl")
	if err := os.WriteFile(hclPath, nomadtpl.GetReducerTemplate(), 0644); err != nil {
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
	r.emit(ctx, "clone", "clone", "info", fmt.Sprintf("Cloning repository: repo=%s ref=%s", r.config.TargetRepo, r.config.BaseRef))
	repoPath := filepath.Join(r.workspaceDir, "repo")
	if err := r.gitOps.CloneRepository(ctx, r.config.TargetRepo, r.config.BaseRef, repoPath); err != nil {
		r.emit(ctx, "clone", "clone", "error", fmt.Sprintf("clone failed: %v", err))
		result.ErrorMessage = fmt.Sprintf("failed to clone repository: %v", err)
		result.Duration = time.Since(startTime)
		return nil, fmt.Errorf("failed to clone repository: %w", err)
	}
	// Post-clone verification and diagnostics
	if entries, err := os.ReadDir(repoPath); err == nil {
		max := len(entries)
		if max > 10 {
			max = 10
		}
		var names []string
		for i := 0; i < max; i++ {
			names = append(names, entries[i].Name())
		}
		// Emit as event for remote visibility
		r.emit(ctx, "clone", "clone-diagnostics", "info", fmt.Sprintf("repo=%s entries=%s", repoPath, strings.Join(names, ",")))
	}

	// If repository has no working tree files (besides .git), fail early
	{
		entries, _ := os.ReadDir(repoPath)
		nonMeta := 0
		for _, e := range entries {
			if e.Name() == ".git" {
				continue
			}
			nonMeta++
		}
		if nonMeta == 0 {
			msg := fmt.Sprintf("clone produced empty working tree: repo=%s ref=%s", r.config.TargetRepo, r.config.BaseRef)
			r.emit(ctx, "clone", "clone-failed", "error", msg)
			result.ErrorMessage = msg
			result.Duration = time.Since(startTime)
			return nil, errors.New(msg)
		}
	}

	result.StepResults = append(result.StepResults, StepResult{
		StepID:  "clone",
		Success: true,
		Message: fmt.Sprintf("Cloned %s at %s", r.config.TargetRepo, r.config.BaseRef),
	})

	// Step 2: Create and checkout workflow branch
	r.emit(ctx, "branch", "create-branch", "info", "Creating workflow branch")
	branchName := GenerateBranchName(r.config.ID)
	result.BranchName = branchName
	if err := r.gitOps.CreateBranchAndCheckout(ctx, repoPath, branchName); err != nil {
		r.emit(ctx, "branch", "create-branch", "error", fmt.Sprintf("branch failed: %v", err))
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
		case string(StepTypeORWApply):
			stepStart := time.Now()
			// Render ORW apply HCL assets (prefer transformation executor)
			var renderedPath string
			var err error
			if r.transformExec != nil {
				renderedPath, err = r.transformExec.RenderORWAssets(step.ID)
			} else {
				renderedPath, err = r.RenderORWApplyAssets(step.ID)
			}
			if err != nil {
				result.StepResults = append(result.StepResults, StepResult{StepID: step.ID, Success: false, Message: fmt.Sprintf("Failed to render ORW assets: %v", err)})
				result.ErrorMessage = fmt.Sprintf("failed to render orw-apply assets: %v", err)
				result.Duration = time.Since(startTime)
				return nil, fmt.Errorf("failed to render orw-apply assets: %w", err)
			}

			// Guard: ensure repository contains a supported build file before creating input tar
			{
				hasPom, hasGradle, hasKts := checkBuildFiles(repoPath)
				r.emit(ctx, "apply", "guard-build-file", "info", fmt.Sprintf("repo=%s pom=%v gradle=%v kts=%v", repoPath, hasPom, hasGradle, hasKts))
				if err := ensureBuildFile(repoPath); err != nil {
					r.emit(ctx, "apply", string(StepTypeORWApply), "error", "no build file in repo (pom.xml/build.gradle)")
					result.StepResults = append(result.StepResults, StepResult{StepID: step.ID, Success: false, Message: ErrNoBuildFile.Error()})
					result.ErrorMessage = ErrNoBuildFile.Error()
					result.Duration = time.Since(startTime)
					return nil, ErrNoBuildFile
				}
			}

			// Prepare input tar from repository
			inputTar := filepath.Join(filepath.Dir(renderedPath), "input.tar")
			if err := createTarFromDir(repoPath, inputTar); err != nil {
				result.StepResults = append(result.StepResults, StepResult{StepID: step.ID, Success: false, Message: fmt.Sprintf("Failed to create input tar: %v", err)})
				return nil, fmt.Errorf("failed to create input tar: %w", err)
			}
			// Preview tar contents for diagnostics via reporter
			if r.eventReporter != nil {
				logPreviewTarWithReporter(r.eventReporter, "apply", "input-preview", inputTar, 20)
			} else {
				logPreviewTar(inputTar, 20)
			}

			// Pre-substitute recipe class and input tar host path into template
			rclass := ""
			if len(step.Recipes) > 0 {
				rclass = step.Recipes[0]
			}
			// Determine coordinates strictly from YAML (no discovery)
			rgroup, rartifact, rversion := step.RecipeGroup, step.RecipeArtifact, step.RecipeVersion
			if err := validateRecipeCoords(rgroup, rartifact, rversion, step.ID); err != nil {
				result.StepResults = append(result.StepResults, StepResult{StepID: step.ID, Success: false, Message: err.Error()})
				result.ErrorMessage = err.Error()
				result.Duration = time.Since(startTime)
				return nil, err
			}
			// Optional Maven plugin version (prefer YAML, then env; runner defaults internally if unset)
			pluginVersion := step.MavenPluginVersion
			if pluginVersion == "" {
				pluginVersion = os.Getenv("TRANSFLOW_MAVEN_PLUGIN_VERSION")
			}
			// Create run ID for this submission and then substitute it
			runID := ORWRunID(step.ID)
			prePath, err := writeORWPreHCL(renderedPath, ORWRecipeParams{Class: rclass, Group: rgroup, Artifact: rartifact, Version: rversion, PluginVersion: pluginVersion}, inputTar, runID)
			if err != nil {
				result.StepResults = append(result.StepResults, StepResult{StepID: step.ID, Success: false, Message: fmt.Sprintf("Failed to write pre-HCL: %v", err)})
				return nil, fmt.Errorf("failed to write pre-substituted HCL: %w", err)
			}

			// Prepare env and substitute final template
			baseDir := filepath.Dir(renderedPath)

			// Prepare branch-scoped step id and DIFF_KEY so job uploads directly under branches/<branch>/steps/<step_id>
			execID := os.Getenv("PLOY_TRANSFLOW_EXECUTION_ID")
			branchID := step.ID
			bs := NewBranchStep(execID, branchID)
			curStepID := bs.ID
			diffKey := bs.DiffKey

			// Prepare input tar from the cloned repository and upload to SeaweedFS for task-side download
			execID = os.Getenv("PLOY_TRANSFLOW_EXECUTION_ID")
			seaweed := ResolveInfraFromEnv().SeaweedURL
			// Upload best-effort to artifacts/transflow/<id>/input.tar using HTTP client
			if err := uploadInputTar(seaweed, execID, inputTar); err != nil {
				r.emit(ctx, "apply", "input-upload", "warn", fmt.Sprintf("input.tar upload failed: %v", err))
			}
			// Substitute HCL with explicit variables to avoid global env writes
			vars := makeORWVars(baseDir, execID, diffKey, seaweed)
			submittedPath, err := substituteORWTemplateVars(prePath, runID, vars)
			if err != nil {
				result.StepResults = append(result.StepResults, StepResult{StepID: step.ID, Success: false, Message: fmt.Sprintf("Failed to substitute ORW HCL: %v", err)})
				return nil, fmt.Errorf("failed to substitute ORW HCL: %w", err)
			}

			// Persist a copy of the submitted HCL for post-mortem inspection
			if execID := os.Getenv("PLOY_TRANSFLOW_EXECUTION_ID"); execID != "" {
				persistDir := filepath.Join("/tmp/transflow-submitted", execID, step.ID)
				_ = os.MkdirAll(persistDir, 0755)
				dest := filepath.Join(persistDir, "orw_apply.submitted.hcl")
				if b, e := os.ReadFile(submittedPath); e == nil {
					_ = os.WriteFile(dest, b, 0644)
					r.emit(ctx, "apply", string(StepTypeORWApply), "info", fmt.Sprintf("Saved submitted HCL to %s", dest))
				}
			}

			// Debug: log env block from submitted HCL for verification (INPUT_URL, SEAWEEDFS_URL, etc.)
			if b, e := os.ReadFile(submittedPath); e == nil {
				s := string(b)
				start := strings.Index(s, "env = {")
				if start >= 0 {
					end := strings.Index(s[start:], "}")
					if end > 0 {
						block := s[start : start+end+1]
						// Avoid spamming controller with large blocks; only log locally
						_ = block
					}
				}
			}
			// Prepare diff path for later fetch and processing
			diffPath := filepath.Join(baseDir, "out", "diff.patch")
			_ = os.MkdirAll(filepath.Dir(diffPath), 0755)
			r.emit(ctx, "apply", string(StepTypeORWApply), "info", "Submitting orw-apply job")
			// Submit job and fetch diff via executor/helper
			orwTimeout := ResolveDefaultsFromEnv().ORWApplyTimeout
			if r.transformExec != nil {
				params := ORWSubmitParams{
					SeaweedURL:       seaweed,
					ExecID:           os.Getenv("PLOY_TRANSFLOW_EXECUTION_ID"),
					BranchID:         branchID,
					StepID:           curStepID,
					RunID:            runID,
					SubmittedHCLPath: submittedPath,
					DiffPath:         diffPath,
					Timeout:          orwTimeout,
				}
				if _, err := r.transformExec.SubmitORWAndFetchDiff(ctx, params); err != nil {
					r.emit(ctx, "apply", string(StepTypeORWApply), "error", err.Error())
					result.StepResults = append(result.StepResults, StepResult{StepID: step.ID, Success: false, Message: err.Error()})
					result.ErrorMessage = err.Error()
					result.Duration = time.Since(startTime)
					return nil, err
				}
			} else if err := submitORWJobAndFetchDiff(ctx,
				func(p string) error {
					if r.hcl != nil {
						return r.hcl.Validate(p)
					}
					return validateJob(p)
				},
				func(p string, t time.Duration) error {
					if r.hcl != nil {
						return r.hcl.Submit(p, t)
					}
					return submitAndWaitTerminal(p, t)
				},
				r.reportLastJobAsync,
				seaweed,
				os.Getenv("PLOY_TRANSFLOW_EXECUTION_ID"), branchID, curStepID, runID,
				submittedPath, diffPath, orwTimeout); err != nil {
				r.emit(ctx, "apply", string(StepTypeORWApply), "error", err.Error())
				result.StepResults = append(result.StepResults, StepResult{StepID: step.ID, Success: false, Message: err.Error()})
				result.ErrorMessage = err.Error()
				result.Duration = time.Since(startTime)
				return nil, err
			}
			// Successful wait and fetch implies job completed
			r.emit(ctx, "apply", string(StepTypeORWApply), "info", "orw-apply job completed")

			// Reconstruct branch state: apply all prior diffs from chain HEAD → root
			_ = r.reconstructBranchState(ctx, seaweed, execID, step.ID, baseDir, repoPath)

			if fi, err := os.Stat(diffPath); err == nil {
				r.emit(ctx, "apply", "diff-found", "info", fmt.Sprintf("diff ready (%d bytes)", fi.Size()))
				if fi.Size() == 0 {
					// Treat empty diff as no-op: skip apply/build and continue pipeline
					msg := "No changes produced by orw-apply; skipping apply/build"
					r.emit(ctx, "apply", "diff-empty", "info", msg)
					result.StepResults = append(result.StepResults, StepResult{StepID: step.ID, Success: true, Message: msg, Duration: time.Since(stepStart)})
					// Continue with next steps
					continue
				}
			} else {
				r.emit(ctx, "apply", "diff-stat", "warn", fmt.Sprintf("diff stat failed: %v", err))
			}

			// Apply + build via helper with events and timeout
			r.emit(ctx, "build", "build-gate-start", "info", fmt.Sprintf("Applying diff and running build gate: repo=%s diff=%s", repoPath, diffPath))
			sr, err := runApplyAndBuildWithEvents(ctx, r, repoPath, diffPath, step.ID, stepStart, r.ApplyDiffAndBuild)
			result.StepResults = append(result.StepResults, sr)
			if err != nil {
				result.ErrorMessage = sr.Message
				result.Duration = time.Since(startTime)
				return nil, err
			}

			// Record chain metadata for this branch (option_id = step.ID)
			{
				branchID := step.ID
				branchDiffKey := computeBranchDiffKey(execID, branchID, curStepID)
				_ = writeBranchChainStepMeta(seaweed, execID, branchID, curStepID, branchDiffKey)
			}

		case "recipe":
			// Deprecated: recipe step is no longer supported in main workflow
			return nil, fmt.Errorf("recipe step is no longer supported; use orw-apply")
		}
	}

	// Step 4: Commit changes (only if not already committed by an apply step)
	if committed, msg, err := r.runCommitStep(ctx, repoPath, initialHead); err != nil {
		r.emit(ctx, "commit", "commit", "error", msg)
		result.StepResults = append(result.StepResults, StepResult{StepID: "commit", Success: false, Message: msg})
		result.ErrorMessage = err.Error()
		result.Duration = time.Since(startTime)
		return nil, err
	} else if committed {
		result.StepResults = append(result.StepResults, StepResult{StepID: "commit", Success: true, Message: msg})
	} else {
		// committed=false implies already committed by apply step
		result.StepResults = append(result.StepResults, StepResult{StepID: "commit", Success: true, Message: msg})
		goto build_step
	}

build_step:
	// Step 5: Run build check
	buildStart := time.Now()
	buildResult, err := r.runBuildGate(ctx, repoPath)
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
				r.emit(ctx, "healing", "healing", "error", "build check failed and healing failed")
				// Healing also failed
				result.ErrorMessage = "build check failed and healing failed"
				result.Duration = time.Since(startTime)
				return nil, fmt.Errorf("build check failed: %s (healing also failed: %v)", message, healingErr)
			}
		} else {
			// No healing enabled, fail immediately
			r.emit(ctx, "build", "build", "error", message)
			result.ErrorMessage = "build check failed"
			result.Duration = time.Since(startTime)
			return nil, fmt.Errorf("build check failed: %s", message)
		}
	}

	if buildResult != nil {
		result.BuildVersion = buildResult.Version
	}
	r.emit(ctx, "build", "build-gate-succeeded", "info", fmt.Sprintf("Build version %s", result.BuildVersion))
	result.StepResults = append(result.StepResults, StepResult{
		StepID:   "build",
		Success:  true,
		Message:  "Build completed successfully",
		Duration: time.Since(buildStart),
	})

	// Step 6: Push branch (via helper)
	if sr, err := runPushWithEvents(r, ctx, repoPath, branchName); err != nil {
		result.StepResults = append(result.StepResults, sr)
		result.ErrorMessage = sr.Message
		result.Duration = time.Since(startTime)
		return nil, fmt.Errorf("failed to push branch: %w", err)
	} else {
		result.StepResults = append(result.StepResults, sr)
	}

	// Step 7: Create or update merge request (if provider is configured)
	if r.gitProvider != nil {
		r.createOrUpdateMR(ctx, result, branchName)
	}

	// Success!
	result.Success = true
	result.Duration = time.Since(startTime)
	return result, nil
}

// MR description rendering moved to mr_template.go

// attemptHealing orchestrates the healing workflow: planner → fanout → reducer
func (r *TransflowRunner) attemptHealing(ctx context.Context, repoPath string, buildError string) (*TransflowHealingSummary, error) {
	summary := &TransflowHealingSummary{
		Enabled:       true,
		AttemptsCount: 1,
	}

	// Step 1: Submit planner job to analyze the build error
	var jobHelper JobSubmissionHelper
	if r.jobHelper != nil {
		jobHelper = r.jobHelper
	} else {
		jobHelper = NewJobSubmissionHelperWithRunner(r.jobSubmitter, r)
	}
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

		// Default and normalize planner types to canonical values
		branchType := string(StepTypeLLMExec)
		if t, ok := option["type"].(string); ok {
			branchType = string(NormalizeStepType(t))
		}

		branches = append(branches, BranchSpec{
			ID:     branchID,
			Type:   branchType,
			Inputs: option,
		})
	}

	// Step 3: Execute fanout orchestration
	maxParallel := 3 // Default parallelism
	if r.config.SelfHeal.MaxRetries > 0 {
		maxParallel = r.config.SelfHeal.MaxRetries
	}

	var (
		winner     BranchResult
		allResults []BranchResult
		fanoutErr  error
	)
	if r.healer != nil {
		// Prefer modular HealingOrchestrator directly
		winner, allResults, fanoutErr = r.healer.RunFanout(ctx, nil, branches, maxParallel)
	} else {
		// Fallback to existing fanout orchestrator
		orchestrator := NewFanoutOrchestratorWithRunner(r.jobSubmitter, r)
		winner, allResults, fanoutErr = orchestrator.RunHealingFanout(ctx, nil, branches, maxParallel)
	}
	summary.AllResults = allResults

	if fanoutErr != nil {
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
// repo ops moved to repo_ops.go
