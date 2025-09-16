package mods

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/git/provider"
	"github.com/iw2rmb/ploy/internal/orchestration"
	supply "github.com/iw2rmb/ploy/internal/supply"
	"github.com/iw2rmb/ploy/internal/utils"
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

// StepResult represents the result of executing a single step
// StepResult, ModResult and Summary moved to runner_results.go

// ModRunner orchestrates the execution of Mod steps
type ModRunner struct {
	config         *ModConfig
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

// NewModRunner creates a new Mod runner with the given configuration
func NewModRunner(config *ModConfig, workspaceDir string) (*ModRunner, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &ModRunner{
		config:       config,
		workspaceDir: workspaceDir,
		hcl:          DefaultHCLSubmitter{},
	}, nil
}

// SetGitOperations sets the Git operations implementation (for dependency injection/testing)
// Event emission helpers moved to events_emit.go

// test indirections moved to job_io.go

// IO helpers moved to job_io.go; keep indirection vars there

// JSON helpers moved to job_io.go; keep indirection vars there

// Planner/assets, repo ops, and apply/build helpers moved to dedicated files

// Run executes the complete Mods workflow
func (r *ModRunner) Run(ctx context.Context) (*ModResult, error) {
	startTime := time.Now()
	result := &ModResult{
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

	// Optional controller-side source SBOM generation (baseline)
	if r.sbomEnabled() {
		gen := supply.NewSBOMGenerator()
		opts := supply.DefaultSBOMOptions()
		opts.Lane = r.config.Lane
		opts.AppName = r.config.ID
		// Use branch name or base ref to label; SHA unknown here
		opts.SHA = r.config.BaseRef
		if err := gen.GenerateForSourceCode(repoPath, opts); err != nil {
			msg := fmt.Sprintf("source SBOM generation failed: %v", err)
			if r.sbomFailOnError() {
				r.emit(ctx, "sbom", "source", "error", msg)
				result.ErrorMessage = msg
				result.Duration = time.Since(startTime)
				return nil, errors.New(msg)
			}
			// Non-fatal: log and continue
			r.emit(ctx, "sbom", "source", "warn", msg)
		} else {
			// Emit success event
			if utils.FileExists(filepath.Join(repoPath, ".sbom.json")) {
				r.emit(ctx, "sbom", "source", "info", "Generated source SBOM (.sbom.json)")
			} else {
				r.emit(ctx, "sbom", "source", "info", "Generated source SBOM")
			}
		}
	}

	// Optional vulnerability scan (NVD) based on SBOM
	if r.vulnEnabled() {
		if err := r.runVulnerabilityGate(ctx, repoPath); err != nil {
			result.ErrorMessage = err.Error()
			result.Duration = time.Since(startTime)
			return nil, err
		}
	}

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
			sr, err := r.runORWApplyStep(ctx, repoPath, step, stepStart)
			result.StepResults = append(result.StepResults, sr)
			if err != nil {
				result.ErrorMessage = sr.Message
				result.Duration = time.Since(startTime)
				return nil, err
			}
			// proceed to next step
			continue
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

		// Emit a diagnostic event with a truncated build error message to aid planner/LLM debugging
		{
			const maxLen = 600
			msg := message
			if len(msg) > maxLen {
				msg = msg[:maxLen] + "…"
			}
			r.emit(ctx, "build", "build-gate-error", "info", msg)
		}

		result.StepResults = append(result.StepResults, StepResult{
			StepID:   "build",
			Success:  false,
			Message:  message,
			Duration: time.Since(buildStart),
		})

		// Check if self-healing is enabled
		if r.config.SelfHeal == nil || !r.config.SelfHeal.Enabled {
			r.emit(ctx, "build", "build", "error", message)
			result.ErrorMessage = "build check failed"
			result.Duration = time.Since(startTime)
			return nil, fmt.Errorf("build check failed: %s", message)
		}

		// Healing retry loop: planner → fanout → reducer; optionally apply winner and re-build
		maxRetries := r.config.SelfHeal.MaxRetries
		if maxRetries <= 0 {
			maxRetries = 1
		}
		var healingSuccess bool
		var lastErr error
		for attempt := 1; attempt <= maxRetries; attempt++ {
			r.emit(ctx, "healing", "healing", "info", fmt.Sprintf("attempt %d/%d", attempt, maxRetries))
			healingSummary, healingErr := r.attemptHealing(ctx, repoPath, message)
			result.HealingSummary = healingSummary
			if healingErr != nil {
				lastErr = healingErr
				if attempt == maxRetries {
					break
				}
				continue
			}
			// If reducer requested apply, ensure branch chain is replayed into working tree before rebuild.
			if strings.ToLower(healingSummary.NextAction.Action) == "apply" {
				if sid := healingSummary.NextAction.StepID; sid != "" {
					seaweed := ResolveInfraFromEnv().SeaweedURL
					if seaweed != "" {
						r.emit(ctx, "healing", "apply", "info", fmt.Sprintf("replay starting: branch_id=%s", sid))
						baseDir := filepath.Join(r.workspaceDir, "branch-apply")
						_ = os.MkdirAll(baseDir, 0755)
						_ = r.reconstructBranchState(ctx, seaweed, os.Getenv("MOD_ID"), sid, baseDir, repoPath)
                        if msg := buildFirstErrorSnippet(repoPath, message); strings.TrimSpace(msg) != "" {
                            r.emit(ctx, "healing", "apply", "info", msg)
                        }
                        if files := workingTreeDiffNames(ctx, repoPath, 8); len(files) > 0 {
                            joined := strings.Join(files, ", ")
                            if len(joined) > 400 {
                                joined = joined[:400] + "…"
                            }
                            r.emit(ctx, "healing", "apply", "info", "post-replay changed files: "+joined)
                        }
                        if head := firstErrorFileHead(repoPath, message, 10); strings.TrimSpace(head) != "" {
                            r.emit(ctx, "healing", "apply", "info", head)
                        }
					}
				}
                // Build check before committing healing changes
                r.emit(ctx, "build", "post-healing-build-start", "info", "Running post-healing build gate")
                buildStart2 := time.Now()
                if br2, err2 := r.runBuildGate(ctx, repoPath); err2 != nil || (br2 != nil && !br2.Success) {
                    // Revert working tree and retry if attempts remain
                    cmd := exec.CommandContext(ctx, "git", "reset", "--hard", "HEAD")
                    cmd.Dir = repoPath
                    _ = cmd.Run()
                    msg2 := "post-healing build failed"
                    if br2 != nil && br2.Message != "" {
                        msg2 = br2.Message
                    }
                    if err2 != nil {
                        msg2 = fmt.Sprintf("%s: %v", msg2, err2)
                    }
                    r.emit(ctx, "build", "post-healing-build-failed", "error", msg2)
                    result.StepResults = append(result.StepResults, StepResult{StepID: "build", Success: false, Message: msg2 + " (reverted healing patch)", Duration: time.Since(buildStart2)})
                    lastErr = fmt.Errorf("%s", msg2)
                    if attempt == maxRetries {
                        break
                    }
                    continue
                }
                // Commit healing changes and proceed
                if r.repoManager != nil {
                    _ = r.repoManager.Commit(ctx, repoPath, "apply(healing): reducer patch")
                } else {
                    _ = r.gitOps.CommitChanges(ctx, repoPath, "apply(healing): reducer patch")
                }
                result.StepResults = append(result.StepResults, StepResult{StepID: "build", Success: true, Message: "Build completed successfully (post-healing)", Duration: time.Since(buildStart2)})
                r.emit(ctx, "build", "post-healing-build-succeeded", "info", "Build completed successfully (post-healing)")
                r.emit(ctx, "build", "build-gate-succeeded", "info", fmt.Sprintf("Build version %s", ""))
                healingSuccess = true
                break
            }
			// If reducer chose stop, accept healing as succeeded (no additional apply)
			if strings.ToLower(healingSummary.NextAction.Action) == "stop" && healingSummary.Winner != nil {
				healingSuccess = true
				break
			}
			// Unknown or no action; prepare to retry or fail
			lastErr = fmt.Errorf("reducer returned no actionable next step")
			if attempt == maxRetries {
				break
			}
		}

		if !healingSuccess {
			r.emit(ctx, "healing", "healing", "error", "healing failed after retries")
			result.ErrorMessage = fmt.Sprintf("build check failed and healing failed: %v", lastErr)
			result.Duration = time.Since(startTime)
			return nil, fmt.Errorf("build check failed: %s (healing failed: %v)", message, lastErr)
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

	// Step 6: If healing requested applying additional diffs, commit them and re-run build gate
	if result.HealingSummary != nil && result.HealingSummary.FinalSuccess {
		if strings.ToLower(result.HealingSummary.NextAction.Action) == "apply" {
            // Build check before committing healing changes
            buildStart2 := time.Now()
            r.emit(ctx, "build", "post-healing-build-start", "info", "Running post-healing build gate")
            if br2, err := r.runBuildGate(ctx, repoPath); err != nil || (br2 != nil && !br2.Success) {
                msg := "Build check failed after healing apply"
                if br2 != nil && br2.Message != "" {
                    msg = br2.Message
                }
                if err != nil {
                    msg = fmt.Sprintf("%s: %v", msg, err)
                }
                // Revert working tree to pre-healing HEAD (discard uncommitted changes)
                // Discard uncommitted changes to return to pre-healing state
                {
                    cmd := exec.CommandContext(ctx, "git", "reset", "--hard", "HEAD")
                    cmd.Dir = repoPath
                    _ = cmd.Run()
                }
                // Record failed post-healing build but continue with normal flow (ORW-only)
                r.emit(ctx, "build", "post-healing-build-failed", "error", msg)
                result.StepResults = append(result.StepResults, StepResult{StepID: "build", Success: false, Message: msg + " (reverted healing patch)", Duration: time.Since(buildStart2)})
            } else {
                // Commit post-healing changes after successful build
                if r.repoManager != nil {
                    _ = r.repoManager.Commit(ctx, repoPath, "apply(healing): reducer patch")
                } else {
                    _ = r.gitOps.CommitChanges(ctx, repoPath, "apply(healing): reducer patch")
                }
                result.StepResults = append(result.StepResults, StepResult{StepID: "build", Success: true, Message: "Build completed successfully (post-healing)", Duration: time.Since(buildStart2)})
                r.emit(ctx, "build", "post-healing-build-succeeded", "info", "Build completed successfully (post-healing)")
                r.emit(ctx, "build", "build-gate-succeeded", "info", fmt.Sprintf("Build version %s", ""))
            }
		}
	}

	// Step 7: Push branch (via helper)
	// Apply MR auth/env selection from config before any Git operations that require credentials
	r.applyMRAuthFromConfig(ctx)
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

// SBOM helpers moved to vuln_gate.go

// applyMRAuthFromConfig resolves per-run Git provider environment from mods.yaml (mr.*)
// without embedding secrets in YAML. It reads the named env vars and maps them to
// the standard GITLAB_URL/GITLAB_TOKEN variables expected by provider and git ops.
// MR auth helper moved to mr_auth.go

// SBOM helpers moved to vuln_gate.go

// Vulnerability gate config helpers
// Vulnerability helpers moved to vuln_gate.go

// Vulnerability helpers moved to vuln_gate.go

// Vulnerability helpers moved to vuln_gate.go

// MR description rendering moved to mr_template.go

// hasRepoChanges returns true if the working tree has any changes
// repo ops moved to repo_ops.go
