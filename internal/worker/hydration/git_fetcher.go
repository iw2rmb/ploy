package hydration

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// GitFetcherOptions holds configuration for the git fetcher.
type GitFetcherOptions struct {
	Publisher       interface{}
	PublishSnapshot bool
}

// GitFetcher fetches git repositories.
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

// Fetch performs a shallow clone by repo URL, checking out base_ref then fetching target_ref or commit_sha.
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

	// Step 1: Shallow clone with base_ref (if provided) or default branch.
	// Use --depth 1 for shallow clone, --single-branch for efficiency.
	cloneArgs := []string{"clone", "--depth", "1"}
	if baseRef != "" {
		cloneArgs = append(cloneArgs, "--branch", baseRef, "--single-branch")
	}
	cloneArgs = append(cloneArgs, url, dest)

	if err := runGitCommand(ctx, "", cloneArgs...); err != nil {
		return fmt.Errorf("git clone failed: %w", err)
	}

	// Step 2: Stay on base_ref for modification runs; do not checkout target_ref here.
	// If a specific commit is requested (rare), checkout that commit.
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
