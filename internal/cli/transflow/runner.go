package transflow

import (
	"context"
	crand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
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
	eventReporter  EventReporter        // Optional real-time event reporter
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

// SetEventReporter sets the reporter used for real-time observability
func (r *TransflowRunner) SetEventReporter(reporter EventReporter) {
	r.eventReporter = reporter
}

func (r *TransflowRunner) emit(ctx context.Context, phase, step, level, message string) {
	if r.eventReporter != nil {
		_ = r.eventReporter.Report(ctx, Event{Phase: phase, Step: step, Level: level, Message: message, Time: time.Now()})
		return
	}
	// Fallback to local log output when no reporter is configured
	log.Printf("[Transflow][%s/%s][%s] %s", phase, step, level, message)
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

// downloadToFile fetches the url content and writes to dest path
func downloadToFile(url, dest string) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("http %d", resp.StatusCode)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

// test indirections
var downloadToFileFn = downloadToFile

// randomStepID returns s-<12 hex chars>
func randomStepID() string {
	var buf [6]byte
	_, _ = crand.Read(buf[:])
	return "s-" + hex.EncodeToString(buf[:])
}

// putFile uploads a local file to SeaweedFS artifacts namespace using PUT
func putFile(seaweedBase, key, srcPath, contentType string) error {
	url := strings.TrimRight(seaweedBase, "/") + "/artifacts/" + strings.TrimLeft(key, "/")
	f, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer f.Close()
	req, err := http.NewRequest(http.MethodPut, url, f)
	if err != nil {
		return err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("put %s: http %d: %s", key, resp.StatusCode, string(b))
	}
	return nil
}

// putJSON uploads JSON bytes to SeaweedFS
func putJSON(seaweedBase, key string, body []byte) error {
	url := strings.TrimRight(seaweedBase, "/") + "/artifacts/" + strings.TrimLeft(key, "/")
	req, err := http.NewRequest(http.MethodPut, url, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("put %s: http %d: %s", key, resp.StatusCode, string(b))
	}
	return nil
}

var putJSONFn = putJSON

// test indirection for file upload
var putFileFn = putFile

// uploadInputTar uploads input.tar to artifacts/transflow/<execID>/input.tar (best-effort)
func uploadInputTar(seaweedBase, execID, inputTarPath string) error {
	key := fmt.Sprintf("transflow/%s/input.tar", execID)
	return putFileFn(seaweedBase, key, inputTarPath, "application/octet-stream")
}

// getJSON fetches a JSON document from SeaweedFS
func getJSON(seaweedBase, key string) ([]byte, int, error) {
	url := strings.TrimRight(seaweedBase, "/") + "/artifacts/" + strings.TrimLeft(key, "/")
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return b, resp.StatusCode, nil
}

var getJSONFn = getJSON

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
	dir := filepath.Join(r.workspaceDir, "llm-exec", optionID)
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
	dir := filepath.Join(r.workspaceDir, "orw-apply", optionID)
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
	// Ensure build tar is created from the repository root without changing process cwd
	// SharedPush honors WorkingDir in config when set via build checker integration
	// Inject working dir via context: decorate DeployConfig with a metadata hint consumed by SharedPush
	// For minimal change, use WorkingDir when supported by build checker implementation.
	buildCfg.Metadata = map[string]string{"working_dir": repoPath}
	// If build checker supports WorkingDir in DeployConfig, it will honor it; otherwise default behavior applies.
	res, err := r.buildChecker.CheckBuild(ctx, buildCfg)
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
		log.Printf("[Transflow] Repo root after clone: %s | entries: %v", repoPath, names)
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

			// Guard: ensure repository contains a supported build file before creating input tar
			{
				p1 := filepath.Join(repoPath, "pom.xml")
				p2 := filepath.Join(repoPath, "build.gradle")
				p3 := filepath.Join(repoPath, "build.gradle.kts")
				_, e1 := os.Stat(p1)
				_, e2 := os.Stat(p2)
				_, e3 := os.Stat(p3)
				hasPom := e1 == nil
				hasGradle := e2 == nil
				hasKts := e3 == nil
				log.Printf("[Transflow] Build file check at %s: pom=%v gradle=%v gradle.kts=%v", repoPath, hasPom, hasGradle, hasKts)
				// Emit guard details to the controller event stream
				r.emit(ctx, "apply", "guard-build-file", "info", fmt.Sprintf("repo=%s pom=%v gradle=%v kts=%v", repoPath, hasPom, hasGradle, hasKts))
				if !hasPom && !hasGradle && !hasKts {
					r.emit(ctx, "apply", "orw-apply", "error", "no build file in repo (pom.xml/build.gradle)")
					result.StepResults = append(result.StepResults, StepResult{StepID: step.ID, Success: false, Message: "No build file found in repository"})
					result.ErrorMessage = "no build file found in repository"
					result.Duration = time.Since(startTime)
					return nil, fmt.Errorf("no build file found in repository")
				}
			}

			// Prepare input tar from repository
			inputTar := filepath.Join(filepath.Dir(renderedPath), "input.tar")
			if err := createTarFromDir(repoPath, inputTar); err != nil {
				result.StepResults = append(result.StepResults, StepResult{StepID: step.ID, Success: false, Message: fmt.Sprintf("Failed to create input tar: %v", err)})
				return nil, fmt.Errorf("failed to create input tar: %w", err)
			}
			// Log a brief preview of tar contents for diagnostics
			{
				// Best-effort: list a few entries
				cmd := exec.Command("tar", "-tf", inputTar)
				out, _ := cmd.CombinedOutput()
				lines := strings.Split(strings.TrimSpace(string(out)), "\n")
				max := 20
				if len(lines) < max {
					max = len(lines)
				}
				log.Printf("[Transflow] input.tar preview (%d entries):\n%s", max, strings.Join(lines[:max], "\n"))
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
			// Determine coordinates strictly from YAML (no discovery)
			rgroup, rartifact, rversion := step.RecipeGroup, step.RecipeArtifact, step.RecipeVersion
			if rgroup == "" || rartifact == "" || rversion == "" {
				result.StepResults = append(result.StepResults, StepResult{StepID: step.ID, Success: false, Message: "Missing recipe coordinates (recipe_group/artifact/version)"})
				result.ErrorMessage = "missing recipe coordinates in transflow step"
				result.Duration = time.Since(startTime)
				return nil, fmt.Errorf("missing recipe coordinates: please set recipe_group, recipe_artifact, recipe_version in step %s", step.ID)
			}
			// Optional Maven plugin version (prefer YAML, then env; runner defaults internally if unset)
			pluginVersion := step.MavenPluginVersion
			if pluginVersion == "" {
				pluginVersion = os.Getenv("TRANSFLOW_MAVEN_PLUGIN_VERSION")
			}
			// Create run ID for this submission and then substitute it
			runID := fmt.Sprintf("orw-apply-%s-%d", step.ID, time.Now().Unix())
			prePath := strings.ReplaceAll(renderedPath, ".rendered.hcl", ".pre.hcl")
			preContent := strings.ReplaceAll(string(hclBytes), "${RECIPE_CLASS}", rclass)
			preContent = strings.ReplaceAll(preContent, "${INPUT_TAR_HOST_PATH}", inputTar)
			preContent = strings.ReplaceAll(preContent, "${RUN_ID}", runID)
			preContent = strings.ReplaceAll(preContent, "${RECIPE_GROUP}", rgroup)
			preContent = strings.ReplaceAll(preContent, "${RECIPE_ARTIFACT}", rartifact)
			preContent = strings.ReplaceAll(preContent, "${RECIPE_VERSION}", rversion)
			preContent = strings.ReplaceAll(preContent, "${MAVEN_PLUGIN_VERSION}", pluginVersion)
			if err := os.WriteFile(prePath, []byte(preContent), 0644); err != nil {
				result.StepResults = append(result.StepResults, StepResult{StepID: step.ID, Success: false, Message: fmt.Sprintf("Failed to write pre-HCL: %v", err)})
				return nil, fmt.Errorf("failed to write pre-substituted HCL: %w", err)
			}

			// Prepare env and substitute final template
			baseDir := filepath.Dir(renderedPath)
			_ = os.MkdirAll(filepath.Join(baseDir, "out"), 0755)
			// Compute per-run OUT_DIR without mutating global env
			outDir := filepath.Join(baseDir, "out")

			// Prepare branch-scoped step id and DIFF_KEY so job uploads directly under branches/<branch>/steps/<step_id>
			execID := os.Getenv("PLOY_TRANSFLOW_EXECUTION_ID")
			branchID := step.ID
			curStepID := randomStepID()
			diffKey := fmt.Sprintf("transflow/%s/branches/%s/steps/%s/diff.patch", execID, branchID, curStepID)

			// Prepare input tar from the cloned repository and upload to SeaweedFS for task-side download
			execID = os.Getenv("PLOY_TRANSFLOW_EXECUTION_ID")
			seaweed := os.Getenv("PLOY_SEAWEEDFS_URL")
			if seaweed == "" {
				seaweed = "http://seaweedfs-filer.service.consul:8888"
			}
			// Upload best-effort to artifacts/transflow/<id>/input.tar using HTTP client
			if err := uploadInputTar(seaweed, execID, inputTar); err != nil {
				log.Printf("[Transflow] input.tar upload failed: %v", err)
			}
			// Substitute HCL with explicit variables to avoid global env writes
			vars := map[string]string{
				"TRANSFLOW_CONTEXT_DIR":       baseDir,
				"TRANSFLOW_OUT_DIR":           outDir,
				"PLOY_TRANSFLOW_EXECUTION_ID": execID,
				"TRANSFLOW_DIFF_KEY":          diffKey,
				"PLOY_CONTROLLER":             os.Getenv("PLOY_CONTROLLER"),
				"PLOY_SEAWEEDFS_URL":          seaweed,
				"TRANSFLOW_ORW_APPLY_IMAGE":   os.Getenv("TRANSFLOW_ORW_APPLY_IMAGE"),
				"TRANSFLOW_REGISTRY":          os.Getenv("TRANSFLOW_REGISTRY"),
				"NOMAD_DC":                    os.Getenv("NOMAD_DC"),
			}
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
					log.Printf("[Transflow] Saved submitted HCL to %s", dest)
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
						log.Printf("[Transflow] orw-apply env block (preview):\n%s", block)
					}
				}
			}
			// Prepare diff path for later fetch and processing
			diffPath := filepath.Join(baseDir, "out", "diff.patch")
			_ = os.MkdirAll(filepath.Dir(diffPath), 0755)
			// Preflight validate HCL, then submit job and wait terminal
			r.emit(ctx, "apply", "orw-apply", "info", "Submitting orw-apply job")
			log.Printf("[Transflow] Submitting orw-apply job runID=%s; hcl=%s", runID, submittedPath)
			if err := validateJob(submittedPath); err != nil {
				r.emit(ctx, "apply", "orw-apply", "error", fmt.Sprintf("HCL validation failed: %v", err))
				result.StepResults = append(result.StepResults, StepResult{StepID: step.ID, Success: false, Message: fmt.Sprintf("ORW HCL validation failed: %v", err)})
				result.ErrorMessage = fmt.Sprintf("orw-apply HCL validation failed: %v", err)
				result.Duration = time.Since(startTime)
				return nil, fmt.Errorf("orw-apply HCL validation failed: %w", err)
			}
			r.reportLastJobAsync(ctx, runID, "apply", "orw-apply")
			if err := submitAndWaitTerminal(submittedPath, 15*time.Minute); err != nil {
				// Best-effort: if diff.patch exists, proceed even if job failed (uploads/network can fail)
				if fi, statErr := os.Stat(diffPath); statErr == nil && fi.Size() > 0 {
					log.Printf("[Transflow] orw-apply wait failed (%v), but diff present (size=%d). Proceeding.", err, fi.Size())
				} else {
					r.emit(ctx, "apply", "orw-apply", "error", fmt.Sprintf("orw-apply failed: %v", err))
					result.StepResults = append(result.StepResults, StepResult{StepID: step.ID, Success: false, Message: fmt.Sprintf("ORW apply failed: %v", err)})
					result.ErrorMessage = fmt.Sprintf("orw-apply job failed: %v", err)
					result.Duration = time.Since(startTime)
					return nil, fmt.Errorf("orw-apply job failed: %w", err)
				}
			}
			// Successful wait implies job completed; emit explicit completion event
			r.emit(ctx, "apply", "orw-apply", "info", "orw-apply job completed")
			// Fetch diff from SeaweedFS now
			seaweed = os.Getenv("PLOY_SEAWEEDFS_URL")
			if seaweed == "" {
				seaweed = "http://seaweedfs-filer.service.consul:8888"
			}
			execID = os.Getenv("PLOY_TRANSFLOW_EXECUTION_ID")
			var fetchErr error
			if execID != "" {
				branchDiffKey := fmt.Sprintf("transflow/%s/branches/%s/steps/%s/diff.patch", execID, branchID, curStepID)
				url := strings.TrimRight(seaweed, "/") + "/artifacts/" + branchDiffKey
				fetchErr = downloadToFileFn(url, diffPath)
			} else {
				fetchErr = fmt.Errorf("missing execution id for diff fetch")
			}
			if fetchErr != nil {
				r.emit(ctx, "apply", "orw-apply", "error", fmt.Sprintf("no diff produced: %v", err))
				result.StepResults = append(result.StepResults, StepResult{StepID: step.ID, Success: false, Message: fmt.Sprintf("No diff.patch produced: %v", err)})
				result.ErrorMessage = "no diff produced by orw-apply"
				result.Duration = time.Since(startTime)
				return nil, fmt.Errorf("no diff produced by orw-apply: %w", fetchErr)
			}

			// Reconstruct branch state: apply all prior diffs from chain HEAD → root
			{
				branchID := step.ID
				// Read HEAD
				headKey := fmt.Sprintf("transflow/%s/branches/%s/HEAD.json", execID, branchID)
				if b, code, _ := getJSONFn(seaweed, headKey); code == 200 {
					var head map[string]string
					_ = json.Unmarshal(b, &head)
					cur := head["step_id"]
					// Collect step_ids from head back to root
					chain := []string{}
					for cur != "" {
						chain = append(chain, cur)
						metaKey := fmt.Sprintf("transflow/%s/branches/%s/steps/%s/meta.json", execID, branchID, cur)
						if mb, mc, _ := getJSONFn(seaweed, metaKey); mc == 200 {
							var meta struct {
								Prev string `json:"prev_step_id"`
							}
							_ = json.Unmarshal(mb, &meta)
							cur = meta.Prev
						} else {
							cur = ""
						}
					}
					// Reverse to root→head
					for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
						chain[i], chain[j] = chain[j], chain[i]
					}
					// Apply each recorded diff in order
					for _, sid := range chain {
						url := strings.TrimRight(seaweed, "/") + "/artifacts/" + fmt.Sprintf("transflow/%s/branches/%s/steps/%s/diff.patch", execID, branchID, sid)
						tmp := filepath.Join(baseDir, "out", "chain-"+sid+".patch")
						_ = downloadToFileFn(url, tmp)
						// Validate and apply
						allow := []string{"src/**", "pom.xml"}
						if v := os.Getenv("TRANSFLOW_ALLOWLIST"); v != "" {
							allow = strings.Split(v, ",")
						}
						if err := validateDiffPathsFn(tmp, allow); err == nil {
							_ = validateUnifiedDiffFn(ctx, repoPath, tmp)
							_ = applyUnifiedDiffFn(ctx, repoPath, tmp)
						}
					}
				}
			}

			if fi, err := os.Stat(diffPath); err == nil {
				log.Printf("[Transflow] Diff ready: path=%s size=%d bytes", diffPath, fi.Size())
				r.emit(ctx, "apply", "diff-found", "info", fmt.Sprintf("diff ready (%d bytes)", fi.Size()))
				if fi.Size() == 0 {
					// Treat empty diff as no-op: skip apply/build and continue pipeline
					msg := "No changes produced by orw-apply; skipping apply/build"
					log.Printf("[Transflow] %s", msg)
					r.emit(ctx, "apply", "diff-empty", "info", msg)
					result.StepResults = append(result.StepResults, StepResult{StepID: step.ID, Success: true, Message: msg, Duration: time.Since(stepStart)})
					// Continue with next steps
					continue
				}
			} else {
				log.Printf("[Transflow] Diff ready but stat failed: %v", err)
			}

			// Apply + build with a phase timeout
			applyTimeout := ResolveDefaultsFromEnv().BuildApplyTimeout
			applyCtx, cancelApply := context.WithTimeout(ctx, applyTimeout)
			defer cancelApply()
			log.Printf("[Transflow] Applying diff and running build gate (timeout=%s): repo=%s diff=%s", applyTimeout, repoPath, diffPath)
			r.emit(ctx, "apply", "diff-apply-started", "info", "Applying diff to repository")
			r.emit(ctx, "build", "build-gate-start", "info", "Running build gate")
			if err := r.ApplyDiffAndBuild(applyCtx, repoPath, diffPath); err != nil {
				r.emit(ctx, "build", "build-gate-failed", "error", fmt.Sprintf("apply/build failed: %v", err))
				result.StepResults = append(result.StepResults, StepResult{StepID: step.ID, Success: false, Message: fmt.Sprintf("Apply/build failed: %v", err)})
				result.ErrorMessage = fmt.Sprintf("apply/build failed: %v", err)
				result.Duration = time.Since(startTime)
				return nil, fmt.Errorf("apply/build failed: %w", err)
			}

			r.emit(ctx, "apply", "diff-applied", "info", "Diff applied and build gate passed")
			result.StepResults = append(result.StepResults, StepResult{
				StepID:   step.ID,
				Success:  true,
				Message:  "Applied ORW diff and passed build gate",
				Duration: time.Since(stepStart),
			})

			// Record chain metadata for this branch (option_id = step.ID)
			{
				branchID := step.ID
				// Read previous HEAD
				headKey := fmt.Sprintf("transflow/%s/branches/%s/HEAD.json", execID, branchID)
				prevID := ""
				if b, code, _ := getJSON(seaweed, headKey); code == 200 {
					var head map[string]string
					_ = json.Unmarshal(b, &head)
					prevID = head["step_id"]
				}
				// Diff already uploaded by task under DIFF_KEY; reference it directly
				branchDiffKey := fmt.Sprintf("transflow/%s/branches/%s/steps/%s/diff.patch", execID, branchID, curStepID)
				// Write meta.json
				meta := map[string]any{
					"step_id":      curStepID,
					"prev_step_id": prevID,
					"branch_id":    branchID,
					"diff_key":     branchDiffKey,
					"ts":           time.Now().UTC().Format(time.RFC3339),
				}
				if mb, e := json.Marshal(meta); e == nil {
					_ = putJSONFn(seaweed, fmt.Sprintf("transflow/%s/branches/%s/steps/%s/meta.json", execID, branchID, curStepID), mb)
					_ = putJSONFn(seaweed, headKey, []byte(fmt.Sprintf("{\"step_id\":\"%s\"}", curStepID)))
				}
			}

		case "recipe":
			// Deprecated: recipe step is no longer supported in main workflow
			return nil, fmt.Errorf("recipe step is no longer supported; use orw-apply")
		}
	}

	// Step 4: Commit changes (only if not already committed by an apply step)
	headBefore := initialHead
	// Re-check working tree status
	changed, _ := hasRepoChangesFn(repoPath)
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
		r.emit(ctx, "commit", "commit", "error", "no changes to commit")
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
		r.emit(ctx, "commit", "commit", "error", fmt.Sprintf("commit failed: %v", err))
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

	// Step 6: Push branch
	// Push branch with a phase timeout
	pushTimeout := 3 * time.Minute
	pushCtx, cancelPush := context.WithTimeout(ctx, pushTimeout)
	defer cancelPush()
	log.Printf("[Transflow] Pushing branch (timeout=%s): repo=%s branch=%s", pushTimeout, r.config.TargetRepo, branchName)
	r.emit(ctx, "push", "push", "info", "Pushing branch")
	if err := r.gitOps.PushBranch(pushCtx, repoPath, r.config.TargetRepo, branchName); err != nil {
		msg := fmt.Sprintf("push failed: %v", err)
		if strings.Contains(msg, "rc=128") || strings.Contains(msg, "exit status 128") {
			r.emit(ctx, "push", "push-failed-rc-128", "error", "push failed (rc=128)")
		}
		r.emit(ctx, "push", "push", "error", msg)
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
		r.emit(ctx, "mr", "mr", "info", "Creating merge request")
		if err := r.gitProvider.ValidateConfiguration(); err != nil {
			r.emit(ctx, "mr", "mr", "warn", "MR creation skipped - configuration invalid")
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

			// Create MR with a phase timeout
			mrTimeout := 3 * time.Minute
			mrCtx, cancelMR := context.WithTimeout(ctx, mrTimeout)
			defer cancelMR()
			log.Printf("[Transflow] Creating/updating MR (timeout=%s): source=%s target=%s", mrTimeout, mrConfig.SourceBranch, mrConfig.TargetBranch)
			mrResult, err := r.gitProvider.CreateOrUpdateMR(mrCtx, mrConfig)
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
