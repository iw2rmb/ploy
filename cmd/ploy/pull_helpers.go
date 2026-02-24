package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"

	"github.com/iw2rmb/ploy/internal/cli/migs"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// ensureInsideGitWorktree verifies that the current working directory is inside
// a git repository worktree.
func ensureInsideGitWorktree(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--is-inside-work-tree")
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=echo")

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return errors.New("must be run inside a git repository")
	}
	if strings.TrimSpace(stdout.String()) != "true" {
		return errors.New("must be run inside a git repository")
	}
	return nil
}

// ensureCleanWorkingTree verifies that the git working tree has no staged or
// unstaged changes.
func ensureCleanWorkingTree(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain=v1")
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=echo")

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to check working tree status: %w", err)
	}
	if stdout.Len() > 0 {
		return errors.New("working tree must be clean (commit or stash changes first)")
	}
	return nil
}

// resolveGitRemoteURL retrieves the URL for the specified git remote.
func resolveGitRemoteURL(ctx context.Context, remoteName string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "remote", "get-url", remoteName)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=echo")

	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git remote %q not found", remoteName)
	}
	rawURL := strings.TrimSpace(string(out))
	if rawURL == "" {
		return "", fmt.Errorf("git remote %q has no URL configured", remoteName)
	}
	return rawURL, nil
}

// fetchRef fetches the given ref from the origin remote using a shallow fetch.
// The fetched commit is available as FETCH_HEAD.
func fetchRef(ctx context.Context, origin, ref string, stderr io.Writer, dryRun bool) error {
	_, _ = fmt.Fprintf(stderr, "  fetching %q from %s...\n", ref, origin)
	if dryRun {
		return nil
	}

	cmd := exec.CommandContext(ctx, "git", "fetch", origin, ref, "--depth=1")
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=echo")

	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	if err := cmd.Run(); err != nil {
		stderrStr := stderrBuf.String()
		if strings.Contains(stderrStr, "couldn't find remote ref") ||
			strings.Contains(stderrStr, "not found") ||
			strings.Contains(stderrStr, "invalid refspec") {
			return fmt.Errorf("ref %q not reachable from origin %q", ref, origin)
		}
		return fmt.Errorf("git fetch failed: %w (stderr: %s)", err, strings.TrimSpace(stderrStr))
	}

	return nil
}

func resolveFetchHeadSHA(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "FETCH_HEAD")
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=echo")

	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to resolve FETCH_HEAD: %w", err)
	}
	sha := strings.TrimSpace(string(out))
	if sha == "" {
		return "", fmt.Errorf("FETCH_HEAD resolved to empty sha")
	}
	return sha, nil
}

// checkBranchCollision checks if a branch with the given name already exists locally or remotely.
func checkBranchCollision(ctx context.Context, origin, targetRef string, stderr io.Writer) error {
	localCmd := exec.CommandContext(ctx, "git", "show-ref", "--verify", "refs/heads/"+targetRef)
	localCmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=echo")
	if err := localCmd.Run(); err == nil {
		return fmt.Errorf("branch %q already exists locally", targetRef)
	}

	remoteCmd := exec.CommandContext(ctx, "git", "ls-remote", "--heads", origin, targetRef)
	remoteCmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=echo")
	var remoteBuf bytes.Buffer
	remoteCmd.Stdout = &remoteBuf
	if err := remoteCmd.Run(); err == nil && strings.TrimSpace(remoteBuf.String()) != "" {
		return fmt.Errorf("branch %q already exists on remote %q", targetRef, origin)
	}

	return nil
}

// createAndCheckoutBranch creates a new branch at the given commit and checks it out.
func createAndCheckoutBranch(ctx context.Context, targetRef, commitSHA string, stderr io.Writer) error {
	_, _ = fmt.Fprintf(stderr, "  creating branch %q at %s...\n", targetRef, commitSHA)

	branchCmd := exec.CommandContext(ctx, "git", "branch", targetRef, commitSHA)
	branchCmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=echo")
	var branchStderr bytes.Buffer
	branchCmd.Stderr = &branchStderr
	if err := branchCmd.Run(); err != nil {
		return fmt.Errorf("failed to create branch %q: %w (stderr: %s)", targetRef, err, strings.TrimSpace(branchStderr.String()))
	}

	checkoutCmd := exec.CommandContext(ctx, "git", "checkout", targetRef)
	checkoutCmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=echo")
	var checkoutStderr bytes.Buffer
	checkoutCmd.Stderr = &checkoutStderr
	if err := checkoutCmd.Run(); err != nil {
		return fmt.Errorf("failed to checkout branch %q: %w (stderr: %s)", targetRef, err, strings.TrimSpace(checkoutStderr.String()))
	}

	_, _ = fmt.Fprintf(stderr, "  switched to branch %q\n", targetRef)
	return nil
}

func listRunRepoDiffs(ctx context.Context, httpClient *http.Client, baseURL *url.URL, runID domaintypes.RunID, repoID domaintypes.MigRepoID) ([]migs.DiffEntry, error) {
	cmd := migs.ListRunRepoDiffsCommand{
		Client:  httpClient,
		BaseURL: baseURL,
		RunID:   runID,
		RepoID:  repoID,
	}
	return cmd.Run(ctx)
}

// downloadAndApplyDiffs downloads and applies all diffs to the working tree.
// Returns the count of successfully applied diffs (excluding empty patches).
func downloadAndApplyDiffs(ctx context.Context, runID domaintypes.RunID, repoID domaintypes.MigRepoID, diffs []migs.DiffEntry, stderr io.Writer) (int, error) {
	if len(diffs) == 0 {
		return 0, nil
	}

	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return 0, err
	}

	appliedCount := 0
	for i, diff := range diffs {
		_, _ = fmt.Fprintf(stderr, "  applying diff %d/%d: %s...\n",
			i+1, len(diffs), diff.ID)

		downloadCmd := migs.DownloadDiffCommand{
			Client:  httpClient,
			BaseURL: base,
			RunID:   runID,
			RepoID:  repoID,
			DiffID:  diff.ID,
		}
		patch, err := downloadCmd.Run(ctx)
		if err != nil {
			return appliedCount, fmt.Errorf("failed to download diff %s: %w", diff.ID, err)
		}

		if len(bytes.TrimSpace(patch)) == 0 {
			_, _ = fmt.Fprintf(stderr, "    skipped (empty patch)\n")
			continue
		}

		if err := applyPatch(ctx, patch); err != nil {
			return appliedCount, fmt.Errorf("failed to apply diff %s: %w", diff.ID, err)
		}

		appliedCount++
		_, _ = fmt.Fprintf(stderr, "    applied (%d bytes)\n", len(patch))
	}

	return appliedCount, nil
}

// applyPatch applies a unified diff patch to the current working directory via `git apply`.
func applyPatch(ctx context.Context, patch []byte) error {
	cmd := exec.CommandContext(ctx, "git", "apply")
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=echo")
	cmd.Stdin = bytes.NewReader(patch)

	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git apply failed: %w (stderr: %s)", err, strings.TrimSpace(stderrBuf.String()))
	}
	return nil
}
