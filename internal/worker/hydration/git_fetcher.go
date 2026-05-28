package hydration

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/gitauth"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// GitFetcherOptions holds configuration for the git fetcher.
type GitFetcherOptions struct {
	// CacheDir specifies the base directory for caching git clones.
	// When set (typically from PLOYD_CACHE_HOME), the fetcher reuses existing clones
	// for the same repo and resolved commit combination, avoiding repeated full clones.
	// When empty, caching is disabled and each fetch performs a fresh clone.
	CacheDir string
}

// GitFetcher fetches git repositories using shallow clones for base hydration.
//
// Base Hydration Strategy:
// The fetcher uses git shallow clones (--depth 1) to create the logical "base snapshot"
// for each run on each node. This strategy minimizes network transfer and disk usage
// while providing a consistent starting point for applying per-step diffs during
// multi-step Migs runs.
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
	Fetch(ctx context.Context, repo *contracts.RepoMaterialization, dest string, auth gitauth.Options) error
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
// base clones for the same repo and resolved commit combination. The cache path is
// derived from the normalized repo URL and full commit SHA. If a cached clone exists, it is
// copied to the destination workspace, avoiding repeated network fetches.
//
// The resulting clone serves as the logical "base snapshot" that nodes use
// for applying ordered per-step diffs during multi-step Migs runs. Each node
// performs this clone independently, ensuring consistent base states across
// distributed execution.
//
// The workspace remains on base_ref so subsequent diff application produces the
// correct final state for each step.
func (g *gitFetcher) Fetch(ctx context.Context, repo *contracts.RepoMaterialization, dest string, auth gitauth.Options) error {
	if repo == nil {
		return fmt.Errorf("repo materialization is required")
	}

	if err := repo.Validate(); err != nil {
		return fmt.Errorf("invalid repo: %w", err)
	}

	url := strings.TrimSpace(string(repo.URL))
	baseRef := strings.TrimSpace(string(repo.BaseRef))
	commitSHA := strings.TrimSpace(string(repo.Commit))

	// If destination already looks like a hydrated clone of this repo, skip re-clone.
	// This makes hydration idempotent when the orchestrator reuses the same workspace
	// path (for example, when a per-step workspace is a copy of an existing base clone).
	if dest != "" {
		if info, err := os.Stat(dest); err == nil && info.IsDir() {
			fullCommitSHA := normalizeFullCommitSHA(commitSHA)
			if fullCommitSHA != "" && validateCachedClone(ctx, dest, url, fullCommitSHA) {
				if err := sanitizeCloneOrigin(ctx, dest, url); err != nil {
					return fmt.Errorf("sanitize hydrated clone origin: %w", err)
				}
				return nil
			}
			if fullCommitSHA == "" && validateCloneOrigin(ctx, dest, url) {
				if err := sanitizeCloneOrigin(ctx, dest, url); err != nil {
					return fmt.Errorf("sanitize hydrated clone origin: %w", err)
				}
				return nil
			}
			if removeErr := os.RemoveAll(dest); removeErr != nil {
				return fmt.Errorf("remove stale hydrated clone destination %q: %w", dest, removeErr)
			}
		}
	}

	// Check if caching is enabled and we have a cached clone.
	if g.opts.CacheDir != "" {
		fullCommitSHA := normalizeFullCommitSHA(commitSHA)
		if fullCommitSHA != "" {
			cachedClonePath, err := cacheClonePath(g.opts.CacheDir, url, fullCommitSHA)
			if err != nil {
				return fmt.Errorf("build git clone cache path: %w", err)
			}

			// If cache exists, copy it to dest and validate before using.
			if _, err := os.Stat(cachedClonePath); err == nil {
				if validateCachedClone(ctx, cachedClonePath, url, fullCommitSHA) {
					if err := sanitizeCloneOrigin(ctx, cachedClonePath, url); err != nil {
						if removeErr := os.RemoveAll(cachedClonePath); removeErr != nil {
							return fmt.Errorf("remove unsanitized cached clone %q: %w", cachedClonePath, removeErr)
						}
					}
				}
				if err := copyGitClone(cachedClonePath, dest); err == nil {
					if validateCachedClone(ctx, dest, url, fullCommitSHA) {
						if err := sanitizeCloneOrigin(ctx, dest, url); err != nil {
							return fmt.Errorf("sanitize cached clone destination origin: %w", err)
						}
						return nil
					}
					if removeErr := os.RemoveAll(dest); removeErr != nil {
						return fmt.Errorf("remove invalid cached clone destination %q: %w", dest, removeErr)
					}
				}
				// Cache copy failed or invalid; remove stale cache and fall through to fresh clone.
				if removeErr := os.RemoveAll(cachedClonePath); removeErr != nil {
					return fmt.Errorf("remove invalid cached clone %q: %w", cachedClonePath, removeErr)
				}
			}
		}

		// Cache miss or copy failed: perform fresh clone and populate cache.
		if err := g.cloneAndCheckout(ctx, url, baseRef, commitSHA, dest, auth); err != nil {
			return err
		}

		resolvedCommitSHA, err := resolveCloneHEAD(ctx, dest)
		if err != nil {
			slog.Warn("failed to resolve hydrated clone head for cache population", "path", dest, "error", err)
			return nil
		}
		cachedClonePath, err := cacheClonePath(g.opts.CacheDir, url, resolvedCommitSHA)
		if err != nil {
			slog.Warn("failed to build git clone cache path", "repo_url", url, "commit_sha", resolvedCommitSHA, "error", err)
			return nil
		}
		if err := populateCachedClone(ctx, dest, cachedClonePath, url, resolvedCommitSHA); err != nil {
			slog.Warn("failed to populate git clone cache", "path", cachedClonePath, "error", err)
		}

		return nil
	}

	// No caching: perform a fresh clone.
	return g.cloneAndCheckout(ctx, url, baseRef, commitSHA, dest, auth)
}

// cloneAndCheckout performs a shallow clone and optional commit checkout.
// This is the core git fetch logic extracted for reuse between cached and non-cached flows.
func (g *gitFetcher) cloneAndCheckout(ctx context.Context, rawURL, baseRef, commitSHA, dest string, auth gitauth.Options) error {
	prepared := gitauth.PrepareURL(rawURL, auth)

	// Step 1: Create base snapshot via shallow clone.
	// --depth 1: Fetch only the latest commit to minimize transfer size.
	// --single-branch: Fetch only the specified branch to reduce clone time.
	// If base_ref is empty, git clones the repository's default branch.
	cloneArgs := []string{"clone", "--depth", "1"}
	if baseRef != "" {
		cloneArgs = append(cloneArgs, "--branch", baseRef, "--single-branch")
	}
	cloneArgs = append(cloneArgs, prepared.URL, dest)

	if err := runGitCommand(ctx, "", prepared.Env, cloneArgs...); err != nil {
		return fmt.Errorf("git clone failed: %w", err)
	}
	if err := sanitizeCloneOrigin(ctx, dest, rawURL); err != nil {
		return fmt.Errorf("sanitize cloned origin: %w", err)
	}

	// Step 2: Pin to specific commit if requested (optional).
	// When commit_sha is provided, fetch and checkout that exact commit.
	// This ensures deterministic base snapshots across nodes for the same run,
	// even if base_ref (e.g., 'main') has moved forward between node executions.
	if commitSHA != "" {
		fetchArgs := []string{"fetch", "origin", commitSHA, "--depth", "1"}
		if err := runGitCommand(ctx, dest, prepared.Env, fetchArgs...); err != nil {
			return fmt.Errorf("git fetch %s failed: %w", commitSHA, err)
		}
		checkoutArgs := []string{"checkout", "FETCH_HEAD"}
		if err := runGitCommand(ctx, dest, nil, checkoutArgs...); err != nil {
			return fmt.Errorf("git checkout %s failed: %w", commitSHA, err)
		}
	}

	return nil
}

func cacheClonePath(cacheDir, rawURL, commitSHA string) (string, error) {
	fullCommitSHA := normalizeFullCommitSHA(commitSHA)
	if fullCommitSHA == "" {
		return "", fmt.Errorf("commit sha must be a full 40-character hex sha")
	}

	components, err := cacheRepoPathComponents(rawURL)
	if err != nil {
		return "", err
	}
	components = append([]string{cacheDir, "git-clones"}, components...)
	components = append(components, fullCommitSHA)
	return filepath.Join(components...), nil
}

func cacheRepoPathComponents(rawURL string) ([]string, error) {
	normalized := domaintypes.NormalizeRepoURL(rawURL)
	if normalized == "" {
		return nil, fmt.Errorf("repo url is empty")
	}

	parsed, err := url.Parse(normalized)
	if err != nil {
		return nil, fmt.Errorf("parse repo url: %w", err)
	}

	scheme := strings.ToLower(parsed.Scheme)
	var components []string
	switch scheme {
	case "https", "http", "ssh":
		host := strings.ToLower(strings.TrimSpace(parsed.Host))
		if host == "" {
			return nil, fmt.Errorf("repo url host is empty")
		}
		components = append(components, host)
		components = append(components, splitRepoPath(parsed.Path)...)
	case "file":
		components = append(components, "_file")
		components = append(components, splitRepoPath(parsed.Path)...)
	default:
		return nil, fmt.Errorf("unsupported repo url scheme %q", parsed.Scheme)
	}

	if len(components) < 2 {
		return nil, fmt.Errorf("repo url path is empty")
	}
	for _, component := range components {
		if !safeCachePathComponent(component) {
			return nil, fmt.Errorf("unsafe repo url path component %q", component)
		}
	}
	return components, nil
}

func splitRepoPath(path string) []string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	components := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			components = append(components, part)
		}
	}
	return components
}

func safeCachePathComponent(component string) bool {
	return component != "" && component != "." && component != ".." && !strings.ContainsAny(component, `/\`)
}

func normalizeFullCommitSHA(commitSHA string) string {
	commitSHA = strings.ToLower(strings.TrimSpace(commitSHA))
	if len(commitSHA) != 40 {
		return ""
	}
	for _, r := range commitSHA {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return ""
		}
	}
	return commitSHA
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

func validateCachedClone(ctx context.Context, dest, expectedURL, expectedCommitSHA string) bool {
	if !validateCloneOrigin(ctx, dest, expectedURL) {
		return false
	}
	head, err := resolveCloneHEAD(ctx, dest)
	if err != nil {
		return false
	}
	return head == normalizeFullCommitSHA(expectedCommitSHA)
}

func resolveCloneHEAD(ctx context.Context, dest string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", dest, "rev-parse", "HEAD")
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=echo")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git rev-parse HEAD: %w (output: %s)", err, string(output))
	}
	commitSHA := normalizeFullCommitSHA(string(output))
	if commitSHA == "" {
		return "", fmt.Errorf("git rev-parse HEAD returned non-full sha %q", strings.TrimSpace(string(output)))
	}
	return commitSHA, nil
}

func sanitizeCloneOrigin(ctx context.Context, dest, expectedURL string) error {
	prepared := gitauth.PrepareURL(expectedURL, gitauth.Options{})
	if strings.TrimSpace(prepared.URL) == "" {
		return nil
	}
	return runGitCommand(ctx, dest, nil, "remote", "set-url", "origin", prepared.URL)
}

func populateCachedClone(ctx context.Context, src, dest, expectedURL, expectedCommitSHA string) error {
	if _, err := os.Stat(dest); err == nil {
		if validateCachedClone(ctx, dest, expectedURL, expectedCommitSHA) {
			return nil
		}
		if err := os.RemoveAll(dest); err != nil {
			return fmt.Errorf("remove invalid cached clone %q: %w", dest, err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect cached clone %q: %w", dest, err)
	}

	parent := filepath.Dir(dest)
	if err := os.MkdirAll(parent, 0o750); err != nil {
		return fmt.Errorf("create cache parent %q: %w", parent, err)
	}
	tmp, err := os.MkdirTemp(parent, "."+filepath.Base(dest)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp cache dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	if err := copyGitClone(src, tmp); err != nil {
		return err
	}
	if !validateCachedClone(ctx, tmp, expectedURL, expectedCommitSHA) {
		return fmt.Errorf("copied cache clone failed validation")
	}
	if err := os.Rename(tmp, dest); err != nil {
		if validateCachedClone(ctx, dest, expectedURL, expectedCommitSHA) {
			return nil
		}
		return fmt.Errorf("publish cached clone %q: %w", dest, err)
	}
	return nil
}

// copyGitClone creates a copy of a git repository from src to dest.
func copyGitClone(src, dest string) error {
	if _, err := os.Stat(filepath.Join(src, ".git")); err != nil {
		return fmt.Errorf("source is not a git repository: %w", err)
	}

	// Prefer rsync, fall back to cp.
	if _, err := exec.LookPath("rsync"); err == nil {
		out, err := exec.Command("rsync", "-a", src+"/", dest).CombinedOutput()
		if err != nil {
			return fmt.Errorf("rsync failed: %w (output: %s)", err, string(out))
		}
		return nil
	}

	out, err := exec.Command("cp", "-a", src+"/.", dest).CombinedOutput()
	if err != nil {
		return fmt.Errorf("cp failed: %w (output: %s)", err, string(out))
	}
	return nil
}

// runGitCommand executes a git command in the specified directory.
func runGitCommand(ctx context.Context, dir string, env []string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	// Disable any interactive credential prompts to avoid hanging in headless runs.
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=echo")
	cmd.Env = append(cmd.Env, env...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %w (output: %s)", strings.Join(args, " "), err, string(output))
	}

	return nil
}
