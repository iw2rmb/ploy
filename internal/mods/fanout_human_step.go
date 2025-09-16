package mods

import (
	"context"
	"fmt"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/common"
	"github.com/iw2rmb/ploy/internal/git/provider"
)

// executeHumanStepBranch handles human intervention branches with Git-based manual intervention workflow
func (o *fanoutOrchestrator) executeHumanStepBranch(ctx context.Context, branch BranchSpec, result BranchResult) BranchResult {
	// Parse timeout from branch inputs
	timeout := ResolveDefaultsFromEnv().BuildApplyTimeout // default timeout
	if timeoutStr, ok := branch.Inputs["timeout"].(string); ok {
		if parsedTimeout, err := time.ParseDuration(timeoutStr); err == nil {
			timeout = parsedTimeout
		}
	}

	// Create timeout context for this branch
	branchCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Get build error context
	buildError, _ := branch.Inputs["buildError"].(string)
	if buildError == "" {
		buildError = "Build failure - human intervention required"
	}

	// Check if runner is available for production mode
	if o.runner == nil {
		result.Status = "failed"
		result.Notes = "human-step branches requires production runner (not available in test mode)"
		result.FinishedAt = time.Now()
		result.Duration = time.Since(result.StartedAt)
		return result
	}

	gitProvider := o.runner.GetGitProvider()
	buildChecker := o.runner.GetBuildChecker()
	_ = o.runner.GetWorkspaceDir() // Available if needed for future enhancements

	if gitProvider == nil || buildChecker == nil {
		result.Status = "failed"
		result.Notes = "human-step branches require GitProvider and BuildChecker"
		result.FinishedAt = time.Now()
		result.Duration = time.Since(result.StartedAt)
		return result
	}

	// Step 1: Create intervention branch name
	interventionBranch := fmt.Sprintf("human-intervention-%s", branch.ID)

	// Step 2: Create MR for human intervention
	mrConfig := provider.MRConfig{
		RepoURL:      o.runner.GetTargetRepo(),
		SourceBranch: interventionBranch,
		TargetBranch: "main",
		Title:        fmt.Sprintf("Human Intervention Required: %s", branch.ID),
		Description:  fmt.Sprintf("Build Error:\n```\n%s\n```\n\nPlease fix the build failure and commit your changes to this branch for automated validation.", buildError),
		Labels:       []string{"ploy", "human-intervention"},
	}

	mrResult, err := gitProvider.CreateOrUpdateMR(branchCtx, mrConfig)
	if err != nil {
		result.Status = "failed"
		result.Notes = fmt.Sprintf("Failed to create human intervention MR: %v", err)
		result.FinishedAt = time.Now()
		result.Duration = time.Since(result.StartedAt)
		return result
	}

	// Step 3: Poll for manual commits and validate build
	pollInterval := 30 * time.Second
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-branchCtx.Done():
			// Timeout reached
			result.Status = "timeout"
			result.Notes = fmt.Sprintf("Human intervention timed out after %v", timeout)
			result.FinishedAt = time.Now()
			result.Duration = time.Since(result.StartedAt)
			return result

		case <-ticker.C:
			// Check if human made changes by attempting build validation
			buildConfig := common.DeployConfig{
				App:           branch.ID,
				Lane:          "A", // Simple build validation
				Environment:   "dev",
				ControllerURL: "", // Will be set by build checker if needed
				Timeout:       ResolveDefaultsFromEnv().BuildApplyTimeout,
			}

			buildResult, err := buildChecker.CheckBuild(branchCtx, buildConfig)
			if err != nil {
				continue // Build check failed, keep polling
			}

			if buildResult != nil && buildResult.Success {
				// Human fixed the build!
				result.Status = "completed"
				result.Notes = fmt.Sprintf("Human intervention successful via MR %s - build now passes", mrResult.MRURL)
				result.FinishedAt = time.Now()
				result.Duration = time.Since(result.StartedAt)
				return result
			}

			// Build still fails, continue polling
		}
	}
}
