package hydration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// GitFetcherOptions holds configuration for the git fetcher.
type GitFetcherOptions struct {
	// PublishSnapshot indicates whether snapshots should be published during fetch.
	// Currently unused but reserved for future observability extensions.
	PublishSnapshot bool

	// CacheDir specifies the base directory for caching git clones.
	// When set (typically from PLOYD_CACHE_HOME), the fetcher reuses existing clones
	// for the same repo/ref/commit combination, avoiding repeated full clones.
	// When empty, caching is disabled and each fetch performs a fresh clone.
	CacheDir string
}

// GitFetcher fetches git repositories using shallow clones for base hydration.
//
// Base Hydration Strategy:
// The fetcher uses git shallow clones (--depth 1) to create the logical "base snapshot"
// for each run on each node. This strategy minimizes network transfer and disk usage
// while providing a consistent starting point for applying per-step diffs during
// multi-step Mods runs.
//
// The base snapshot is determined by:
//  1. base_ref (if provided): Clones the specified branch/tag as the base.
//  2. commit_sha (if provided): Fetches and checks out the specific commit after cloning.
//  3. Default branch: Used when base_ref is not specified.
//
// For multi-node execution, each node clones the same base_ref/commit_sha independently,
// ensuring identical base states across nodes before applying ordered diffs.
type GitFetcher interface {
	// Fetch performs shallow clone and checkout of the specified repository.
	Fetch(ctx context.Context, repo *contracts.RepoMaterialization, dest string) error
}

type gitFetcher struct {
	opts GitFetcherOptions
}

// NewGitFetcher creates a new git fetcher.
func NewGitFetcher(opts GitFetcherOptions) (GitFetcher, error) {
	return &gitFetcher{opts: opts}, nil
}

// Fetch performs a shallow clone to create the base snapshot for hydration.
//
// This method implements the base hydration strategy using git shallow clones:
//  1. Clones the repository with --depth 1 to minimize data transfer.
//  2. Uses base_ref (if provided) to determine the starting branch/tag.
//  3. Optionally fetches and checks out a specific commit_sha for pinned snapshots.
//
// When CacheDir is configured (via PLOYD_CACHE_HOME), the fetcher reuses existing
// base clones for the same repo/ref/commit combination. The cache key is derived from
// the normalized repo URL, base_ref, and commit_sha. If a cached clone exists, it is
// copied to the destination workspace, avoiding repeated network fetches.
//
// The resulting clone serves as the logical "base snapshot" that nodes use
// for applying ordered per-step diffs during multi-step Mods runs. Each node
// performs this clone independently, ensuring consistent base states across
// distributed execution.
//
// Note: target_ref is intentionally not checked out during hydration. The workspace
// remains on base_ref so that subsequent diff application produces the correct
// final state for each step.
func (g *gitFetcher) Fetch(ctx context.Context, repo *contracts.RepoMaterialization, dest string) error {
	if repo == nil {
		return fmt.Errorf("repo materialization is required")
	}

	if err := repo.Validate(); err != nil {
		return fmt.Errorf("invalid repo: %w", err)
	}

	url := strings.TrimSpace(string(repo.URL))
	baseRef := strings.TrimSpace(string(repo.BaseRef))
	_ = strings.TrimSpace(string(repo.TargetRef)) // targetRef is intentionally unused during hydration
	commitSHA := strings.TrimSpace(string(repo.Commit))

	// If destination already looks like a hydrated clone of this repo, skip re-clone.
	// This makes hydration idempotent when the orchestrator reuses the same workspace
	// path (for example, when a per-step workspace is a copy of an existing base clone).
	if dest != "" {
		if info, err := os.Stat(dest); err == nil && info.IsDir() {
			if validateCloneOrigin(ctx, dest, url) {
				return nil
			}
		}
	}

	// Check if caching is enabled and we have a cached clone.
	if g.opts.CacheDir != "" {
		cacheKey := computeCacheKey(url, baseRef, commitSHA)
		cachedClonePath := filepath.Join(g.opts.CacheDir, "git-clones", cacheKey)

		// If cache exists, copy it to dest and validate before using.
		if _, err := os.Stat(cachedClonePath); err == nil {
			if err := copyGitClone(cachedClonePath, dest); err == nil {
				// Validate the cached clone matches expected URL.
				if validateCloneOrigin(ctx, dest, url) {
					return nil
				}
				// Cache invalid or corrupted; remove and fall through to fresh clone.
				os.RemoveAll(dest)
			}
			// Cache copy failed or invalid; fall through to fresh clone.
		}

		// Cache miss or copy failed: perform fresh clone and populate cache.
		if err := g.cloneAndCheckout(ctx, url, baseRef, commitSHA, dest); err != nil {
			return err
		}

		// Populate cache: copy dest to cache directory.
		// Ignore errors here to avoid failing the overall fetch if cache write fails.
		// The clone succeeded, so the workspace is valid even if caching fails.
		if err := os.MkdirAll(filepath.Dir(cachedClonePath), 0o755); err == nil {
			_ = copyGitClone(dest, cachedClonePath)
		}

		return nil
	}

	// No caching: perform a fresh clone.
	return g.cloneAndCheckout(ctx, url, baseRef, commitSHA, dest)
}

// cloneAndCheckout performs a shallow clone and optional commit checkout.
// This is the core git fetch logic extracted for reuse between cached and non-cached flows.
func (g *gitFetcher) cloneAndCheckout(ctx context.Context, url, baseRef, commitSHA, dest string) error {
	// Step 1: Create base snapshot via shallow clone.
	// --depth 1: Fetch only the latest commit to minimize transfer size.
	// --single-branch: Fetch only the specified branch to reduce clone time.
	// If base_ref is empty, git clones the repository's default branch.
	cloneArgs := []string{"clone", "--depth", "1"}
	if baseRef != "" {
		cloneArgs = append(cloneArgs, "--branch", baseRef, "--single-branch")
	}
	cloneArgs = append(cloneArgs, url, dest)

	if err := runGitCommand(ctx, "", cloneArgs...); err != nil {
		return fmt.Errorf("git clone failed: %w", err)
	}

	// Step 2: Pin to specific commit if requested (optional).
	// When commit_sha is provided, fetch and checkout that exact commit.
	// This ensures deterministic base snapshots across nodes for the same run,
	// even if base_ref (e.g., 'main') has moved forward between node executions.
	if commitSHA != "" {
		fetchArgs := []string{"fetch", "origin", commitSHA, "--depth", "1"}
		if err := runGitCommand(ctx, dest, fetchArgs...); err != nil {
			return fmt.Errorf("git fetch %s failed: %w", commitSHA, err)
		}
		checkoutArgs := []string{"checkout", "FETCH_HEAD"}
		if err := runGitCommand(ctx, dest, checkoutArgs...); err != nil {
			return fmt.Errorf("git checkout %s failed: %w", commitSHA, err)
		}
	}

	return nil
}

// computeCacheKey generates a stable cache key for a repo/ref/commit combination.
// The key is a SHA256 hash of the normalized inputs to ensure filesystem-safe,
// collision-resistant identifiers that remain stable across runs.
func computeCacheKey(url, baseRef, commitSHA string) string {
	// Normalize URL: strip trailing slashes and .git suffix for consistent keys.
	// Uses the shared domaintypes.NormalizeRepoURL helper for canonical URL form.
	normalized := domaintypes.NormalizeRepoURL(url)

	// Include base_ref and commit_sha in the key for cache isolation.
	// Different base_ref or commit_sha values result in different cache entries.
	keyInput := fmt.Sprintf("%s|%s|%s", normalized, baseRef, commitSHA)

	hash := sha256.Sum256([]byte(keyInput))
	return hex.EncodeToString(hash[:])
}

// validateCloneOrigin checks if dest is a git repository with the expected origin URL.
// Returns true if the clone is valid and matches the expected URL.
func validateCloneOrigin(ctx context.Context, dest, expectedURL string) bool {
	gitDir := filepath.Join(dest, ".git")
	if _, err := os.Stat(gitDir); err != nil {
		return false
	}
	cmd := exec.CommandContext(ctx, "git", "-C", dest, "remote", "get-url", "origin")
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=echo")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	remoteURL := strings.TrimSpace(string(output))
	return domaintypes.NormalizeRepoURL(remoteURL) == domaintypes.NormalizeRepoURL(expectedURL)
}

// copyGitClone creates a copy of a git repository from src to dest.
// This uses rsync for efficient copying of git repositories, including the .git directory.
func copyGitClone(src, dest string) error {
	// Ensure src is a git repository.
	if _, err := os.Stat(filepath.Join(src, ".git")); err != nil {
		return fmt.Errorf("source is not a git repository: %w", err)
	}

	// Use rsync for efficient recursive copy. rsync is required; do not fall back
	// to cp to avoid diverging semantics between environments.
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

// runGitCommand executes a git command in the specified directory.
func runGitCommand(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	// Disable any interactive credential prompts to avoid hanging in headless runs.
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=echo")

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %w (output: %s)", strings.Join(args, " "), err, string(output))
	}

	return nil
}
