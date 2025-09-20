package mods

import (
	"context"
	"fmt"
)

// ApplyDiffAndBuild validates and applies a diff, commits changes, and runs a build gate.
func (r *ModRunner) ApplyDiffAndBuild(ctx context.Context, repoPath, diffPath string) error {
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
	// Stage only files referenced by the unified diff to avoid committing build artifacts
	if err := stagePathsFromDiff(ctx, repoPath, diffPath); err != nil {
		return err
	}
	pruneGeneratedDockerfiles(repoPath)
	if r.repoManager != nil {
		if err := r.repoManager.Commit(ctx, repoPath, "apply(diff): mods branch patch"); err != nil {
			return fmt.Errorf("commit failed: %w", err)
		}
	} else if err := r.gitOps.CommitChanges(ctx, repoPath, "apply(diff): mods branch patch"); err != nil {
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

// ApplyDiffOnly validates and applies a diff without invoking the build gate.
// The caller is responsible for committing and running subsequent build checks.
func (r *ModRunner) ApplyDiffOnly(ctx context.Context, repoPath, diffPath string) error {
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
	return stagePathsFromDiff(ctx, repoPath, diffPath)
}
