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
	return step.NewFilesystemDiffGenerator(step.FilesystemDiffGeneratorOptions{})
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
// Note: The caller is responsible for fetching diffs in the correct order (by step_index).
// Use the control plane API endpoint GET /v1/mods/{id}/diffs?step_index=... to retrieve
// diffs for steps 0 through k-1 when preparing to execute step k.
func RehydrateWorkspaceFromBaseAndDiffs(ctx context.Context, baseClonePath, destWorkspace string, diffs [][]byte) error {
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
// Falls back to cp if rsync is not available.
func copyGitClone(src, dest string) error {
	// Ensure src is a git repository.
	if _, err := os.Stat(filepath.Join(src, ".git")); err != nil {
		return fmt.Errorf("source is not a git repository: %w", err)
	}

	// Try rsync first (more efficient for git repos).
	if _, err := exec.LookPath("rsync"); err == nil {
		// rsync -a preserves permissions, timestamps, and copies recursively.
		// Trailing slash on src ensures contents are copied, not the directory itself.
		cmd := exec.Command("rsync", "-a", src+"/", dest)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("rsync failed: %w (output: %s)", err, string(output))
		}
		return nil
	}

	// Fallback: use cp -R (less efficient but more portable).
	cmd := exec.Command("cp", "-R", src, dest)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("cp failed: %w (output: %s)", err, string(output))
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
	defer reader.Close()

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, reader); err != nil {
		return nil, fmt.Errorf("gzip decompression failed: %w", err)
	}

	return buf.Bytes(), nil
}
