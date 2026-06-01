package pull

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/iw2rmb/ploy/internal/cli/common"
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

func resolveHEADSHA(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=echo")

	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to resolve HEAD: %w", err)
	}
	sha := strings.TrimSpace(string(out))
	if sha == "" {
		return "", fmt.Errorf("HEAD resolved to empty sha")
	}
	return sha, nil
}

func ensureHEADMatchesSource(ctx context.Context, sourceCommit string) error {
	sourceCommit = strings.TrimSpace(sourceCommit)
	if sourceCommit == "" {
		return fmt.Errorf("source_commit_sha is required")
	}
	headSHA, err := resolveHEADSHA(ctx)
	if err != nil {
		return err
	}
	if !strings.EqualFold(headSHA, sourceCommit) {
		return fmt.Errorf("local HEAD %s does not match run source_commit_sha %s", headSHA, sourceCommit)
	}
	return nil
}

func ListRunDiffs(ctx context.Context, httpClient *http.Client, baseURL *url.URL, runID domaintypes.RunID, repoID domaintypes.RepoID) ([]migs.DiffEntry, error) {
	cmd := migs.ListRunDiffsCommand{
		Client:  httpClient,
		BaseURL: baseURL,
		RunID:   runID,
		RepoID:  repoID,
	}
	return cmd.Run(ctx)
}

// downloadAndApplyDiffs downloads and applies all diffs to the working tree.
// Returns the count of successfully applied diffs (excluding empty patches).
func downloadAndApplyDiffs(ctx context.Context, runID domaintypes.RunID, repoID domaintypes.RepoID, diffs []migs.DiffEntry, stderr io.Writer) (int, error) {
	if len(diffs) == 0 {
		return 0, nil
	}

	base, httpClient, err := common.ResolveControlPlaneHTTP(ctx)
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
