package nodeagent

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/worker/hydration"
	"github.com/iw2rmb/ploy/internal/workflow/runtime/step"
)

// Runtime component factory methods.
// These methods isolate component initialization logic from the orchestration flow.

// createGitFetcher initializes a git fetcher for repository operations.
// When PLOYD_CACHE_HOME is set, the fetcher uses it as the base directory for
// caching git clones, avoiding repeated network fetches for the same repo/ref/commit.
func (r *runController) createGitFetcher() (step.GitFetcher, error) {
	cacheDir := os.Getenv("PLOYD_CACHE_HOME")
	return hydration.NewGitFetcher(hydration.GitFetcherOptions{
		PublishSnapshot: false,
		CacheDir:        cacheDir,
	})
}

// createWorkspaceHydrator initializes a workspace hydrator with the provided repo fetcher.
func (r *runController) createWorkspaceHydrator(fetcher step.GitFetcher) (step.WorkspaceHydrator, error) {
	return step.NewFilesystemWorkspaceHydrator(step.FilesystemWorkspaceHydratorOptions{
		RepoFetcher: fetcher,
	})
}

// createContainerRuntime initializes a Docker container runtime with image pull enabled.
func (r *runController) createContainerRuntime() (step.ContainerRuntime, error) {
	network := os.Getenv("PLOY_DOCKER_NETWORK")
	return step.NewDockerContainerRuntime(step.DockerContainerRuntimeOptions{
		PullImage: true,
		Network:   network,
	})
}

// createDiffGenerator initializes a filesystem diff generator.
func (r *runController) createDiffGenerator() step.DiffGenerator {
	return step.NewFilesystemDiffGenerator()
}

// RehydrateWorkspaceFromBaseAndDiffs builds a fresh workspace by copying the base clone
// and applying ordered per-step diffs.
//
// This helper implements the core rehydration strategy for multi-step Mods runs:
//  1. Copy the base clone to the destination workspace (base snapshot).
//  2. Fetch and apply ordered diffs (step 0, 1, 2, ..., k-1) to reconstruct workspace state for step k.
//
// The resulting workspace contains all changes from prior steps, ready for execution of step k.
//
// Parameters:
//   - ctx: Context for cancellation and deadlines.
//   - baseClonePath: Path to the cached or freshly-cloned base repository snapshot.
//   - destWorkspace: Path where the rehydrated workspace will be created.
//   - diffs: Ordered list of diff patches (gzipped unified diffs) to apply sequentially.
//
// The function performs the following steps:
//   - Copy base clone to destination (ensures clean starting point).
//   - For each diff in order: decompress gzipped patch, apply via "git apply".
//   - Returns error if copy or any patch application fails.
//
// Note: Diffs must be applied in a deterministic order to ensure workspace rehydration
// produces identical results across nodes and retries. Callers should fetch diffs via
// DiffFetcher.FetchDiffsForStepRepo, which filters and sorts diffs by
// (summary.step_index, created_at, id) before downloading patches.
func RehydrateWorkspaceFromBaseAndDiffs(ctx context.Context, baseClonePath, destWorkspace string, diffs [][]byte) error {
	// Validate paths before proceeding.
	if strings.TrimSpace(baseClonePath) == "" {
		return fmt.Errorf("baseClonePath is empty")
	}
	if strings.TrimSpace(destWorkspace) == "" {
		return fmt.Errorf("destWorkspace is empty")
	}

	// Verify baseClonePath exists and is a directory.
	info, err := os.Stat(baseClonePath)
	if err != nil {
		return fmt.Errorf("baseClonePath validation failed: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("baseClonePath is not a directory: %s", baseClonePath)
	}

	// Step 1: Copy base clone to destination workspace.
	// This creates a fresh workspace starting from the base snapshot (base_ref + optional commit_sha).
	// Using the same copy logic as the git fetcher's cache reuse to ensure consistency.
	if err := copyGitClone(baseClonePath, destWorkspace); err != nil {
		return fmt.Errorf("failed to copy base clone: %w", err)
	}

	// Step 2: Apply each diff sequentially to reconstruct workspace state.
	// Diffs are expected to be gzipped unified patches (git diff output).
	// Apply them in order using "git apply" to replay changes from prior steps.
	for i, gzippedPatch := range diffs {
		if err := applyGzippedPatch(ctx, destWorkspace, gzippedPatch); err != nil {
			return fmt.Errorf("failed to apply diff at index %d: %w", i, err)
		}
	}

	return nil
}

// copyGitClone creates a copy of a git repository from src to dest.
// This is extracted from internal/worker/hydration for reuse in rehydration logic.
// It uses rsync for efficient copying of git repositories, including the .git directory.
func copyGitClone(src, dest string) error {
	// Ensure src is a git repository.
	if _, err := os.Stat(filepath.Join(src, ".git")); err != nil {
		return fmt.Errorf("source is not a git repository: %w", err)
	}

	// Use rsync for efficient recursive copy. rsync is required; do not fall back
	// to cp to keep behavior consistent with worker hydration.
	if _, err := exec.LookPath("rsync"); err != nil {
		return fmt.Errorf("rsync not available for git clone copy: %w", err)
	}

	// rsync -a preserves permissions, timestamps, and copies recursively.
	// Trailing slash on src ensures contents are copied, not the directory itself.
	cmd := exec.Command("rsync", "-a", src+"/", dest)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("rsync failed: %w (output: %s)", err, string(output))
	}

	return nil
}

// applyGzippedPatch decompresses a gzipped patch and applies it to the workspace using "git apply".
// This is the core patch application logic for rehydrating workspaces from stored diffs.
func applyGzippedPatch(ctx context.Context, workspace string, gzippedPatch []byte) error {
	// Step 1: Decompress the gzipped patch.
	patch, err := decompressPatch(gzippedPatch)
	if err != nil {
		return fmt.Errorf("failed to decompress patch: %w", err)
	}

	// Step 2: Handle empty patches (no changes in this step).
	// git apply rejects empty patches by default, so skip application for empty input.
	if len(bytes.TrimSpace(patch)) == 0 {
		return nil
	}

	// Step 3: Apply the patch using "git apply".
	// Use "git apply" without --index to avoid staging conflicts.
	// The patch is applied to the working tree, matching the behavior when diffs are generated.
	cmd := exec.CommandContext(ctx, "git", "apply")
	cmd.Dir = workspace
	cmd.Stdin = bytes.NewReader(patch)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Include stderr output for diagnostics when patch application fails.
		return fmt.Errorf("git apply failed: %w (stderr: %s)", err, stderr.String())
	}

	return nil
}

// decompressPatch decompresses a gzipped patch and returns the plaintext unified diff.
func decompressPatch(gzippedPatch []byte) ([]byte, error) {
	if len(gzippedPatch) == 0 {
		// Empty patch is valid (no changes in this step).
		return []byte{}, nil
	}

	reader, err := gzip.NewReader(bytes.NewReader(gzippedPatch))
	if err != nil {
		return nil, fmt.Errorf("gzip reader initialization failed: %w", err)
	}
	defer func() {
		_ = reader.Close()
	}()

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, reader); err != nil {
		return nil, fmt.Errorf("gzip decompression failed: %w", err)
	}

	return buf.Bytes(), nil
}

// ensureBaselineCommitForRehydration creates a git commit in the workspace after applying
// per-step diffs during rehydration. This commit establishes a baseline for incremental
// diff generation in the current step.
//
// Problem: Without a baseline commit, "git diff HEAD" generates a diff from the original
// base_ref to the current working tree, accumulating changes from all prior steps.
// This violates the incremental diff requirement: diff[k] should contain only changes
// from step k, not the cumulative changes from steps 0..k.
//
// Solution: After rehydrating workspace[step_k] by applying diffs[0..k-1], create a
// git commit. This commit becomes the new HEAD, so subsequent "git diff HEAD" in step k
// generates only the incremental changes introduced by step k's execution.
//
// Rehydration-safety: Given base clone + ordered diffs[0..k-1], we can reconstruct
// workspace[step_k] by:
//  1. Clone base_ref to temp workspace.
//  2. Apply diffs[0..k-1] sequentially using "git apply".
//  3. The resulting working tree matches the state before step k execution.
//
// The baseline commit ensures:
//   - diff[k] = git diff HEAD (after step k) = changes from step k only
//   - Replaying diffs[0..k] on base clone reconstructs workspace[step_k+1]
//
// Control plane persists these per-step diffs under the same step_index used for jobs.
//
// Parameters:
//   - ctx: Context for cancellation and deadlines.
//   - workspace: Path to the rehydrated workspace (after diff application).
//   - stepIndex: Job step index (for commit message).
//
// Returns:
//   - error: Non-nil if git commit fails (identity config, staging, or commit error).
func ensureBaselineCommitForRehydration(ctx context.Context, workspace string, stepIndex types.StepIndex) error {
	// Import the git helper package for commit operations.
	// Using internal/nodeagent/git.EnsureCommit to stage and commit rehydrated state.
	userName := "ploy-rehydrate"
	userEmail := "ploy-rehydrate@ploy.local"
	message := fmt.Sprintf("Ploy: rehydration baseline for step %.0f", stepIndex)

	// Import git package at the top of the file if not already imported.
	// For now, inline the commit logic to avoid circular dependency.
	// TODO: Consider extracting to git package if this grows.

	// Configure git identity (local repo config only).
	if err := runGitCommand(ctx, workspace, "config", "user.name", userName); err != nil {
		return fmt.Errorf("git config user.name: %w", err)
	}
	if err := runGitCommand(ctx, workspace, "config", "user.email", userEmail); err != nil {
		return fmt.Errorf("git config user.email: %w", err)
	}

	// Stage all changes (rehydrated diffs have been applied to working tree).
	// Use "git add -A" to stage modifications, additions, and deletions.
	// Exclude build outputs (Maven target/, etc.) to keep commits clean.
	if err := runGitCommand(ctx, workspace, "add", "-A", "--", ".",
		":(exclude)**/target/**", ":(exclude)target/"); err != nil {
		return fmt.Errorf("git add: %w", err)
	}

	// Create commit with the rehydrated baseline.
	// This commit becomes the new HEAD for incremental diff generation.
	if err := runGitCommand(ctx, workspace, "commit", "-m", message); err != nil {
		return fmt.Errorf("git commit: %w", err)
	}

	return nil
}

// runGitCommand is a helper to execute git commands in the specified directory.
// This is extracted for reuse in baseline commit creation.
func runGitCommand(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return fmt.Errorf("git %v failed: %w (stderr: %s)", args, err, stderr.String())
		}
		return fmt.Errorf("git %v failed: %w", args, err)
	}

	return nil
}
