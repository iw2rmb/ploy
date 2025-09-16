package mods

import (
	"context"

	"github.com/iw2rmb/ploy/internal/cli/common"
	"github.com/iw2rmb/ploy/internal/git/provider"
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

// SetGitOperations sets the Git operations implementation (for dependency injection/testing)
func (r *ModRunner) SetGitOperations(gitOps GitOperationsInterface) {
	r.gitOps = gitOps
	if gitOps != nil {
		r.repoManager = NewRepoManagerAdapter(gitOps)
	} else {
		r.repoManager = nil
	}
}

// SetRecipeExecutor sets the recipe executor implementation (for dependency injection/testing)
func (r *ModRunner) SetRecipeExecutor(executor RecipeExecutorInterface) {
	r.recipeExecutor = executor
}

// SetTransformationExecutor sets the modular TransformationExecutor
func (r *ModRunner) SetTransformationExecutor(x TransformationExecutor) { r.transformExec = x }

// SetBuildChecker sets the build checker implementation (for dependency injection/testing)
func (r *ModRunner) SetBuildChecker(checker BuildCheckerInterface) {
	r.buildChecker = checker
	// Also expose through BuildGate adapter for modularization
	if checker != nil {
		r.buildGate = NewBuildGateAdapter(checker)
	} else {
		r.buildGate = nil
	}
}

// SetBuildGate sets the modular BuildGate; takes precedence over buildChecker when set.
func (r *ModRunner) SetBuildGate(g BuildGate) { r.buildGate = g }

// SetJobSubmitter sets the job submitter for healing workflows (for dependency injection/testing)
func (r *ModRunner) SetJobSubmitter(submitter JobSubmitter) {
	r.jobSubmitter = submitter
}

// SetGitProvider sets the Git provider implementation for MR creation (for dependency injection/testing)
func (r *ModRunner) SetGitProvider(p provider.GitProvider) {
	r.gitProvider = p
	if p != nil {
		r.mrManager = NewMRManagerAdapter(p)
	} else {
		r.mrManager = nil
	}
}

// SetEventReporter sets the reporter used for real-time observability
func (r *ModRunner) SetEventReporter(reporter EventReporter) {
	r.eventReporter = reporter
}

// SetHealingOrchestrator sets the modular healing orchestrator
func (r *ModRunner) SetHealingOrchestrator(h HealingOrchestrator) { r.healer = h }

// SetHCLSubmitter sets the indirection used for HCL validate/submit flows.
func (r *ModRunner) SetHCLSubmitter(h HCLSubmitter) { r.hcl = h }

// SetJobHelper allows injecting a planner/reducer submission helper for testing.
func (r *ModRunner) SetJobHelper(h JobSubmissionHelper) { r.jobHelper = h }

// GetHCLSubmitter exposes the HCLSubmitter for helpers that need it.
func (r *ModRunner) GetHCLSubmitter() HCLSubmitter { return r.hcl }

// GetGitProvider returns the Git provider for human-step branch operations
func (r *ModRunner) GetGitProvider() provider.GitProvider {
	return r.gitProvider
}

// GetBuildChecker returns the build checker for human-step branch operations
func (r *ModRunner) GetBuildChecker() BuildCheckerInterface {
	return r.buildChecker
}

// GetWorkspaceDir returns the workspace directory for human-step branch operations
func (r *ModRunner) GetWorkspaceDir() string {
	return r.workspaceDir
}

// GetTargetRepo returns the target repository URL for human-step branch operations
func (r *ModRunner) GetTargetRepo() string { return r.config.TargetRepo }
