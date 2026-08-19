package pull

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/iw2rmb/ploy/internal/cli/common"
	"github.com/iw2rmb/ploy/internal/cli/runs"
	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/gitexec"
)

// ensureInsideGitWorktree verifies that the current working directory is inside
// a git repository worktree.
func ensureInsideGitWorktree(ctx context.Context) error {
	result, err := gitexec.Execute(ctx, gitexec.Request{Args: []string{"rev-parse", "--is-inside-work-tree"}})
	if err != nil {
		return errors.New("must be run inside a git repository")
	}
	if strings.TrimSpace(string(result.Stdout)) != "true" {
		return errors.New("must be run inside a git repository")
	}
	return nil
}

// ensureCleanWorkingTree verifies that the git working tree has no staged or
// unstaged changes.
func ensureCleanWorkingTree(ctx context.Context) error {
	result, err := gitexec.Execute(ctx, gitexec.Request{Args: []string{"status", "--porcelain=v1"}})
	if err != nil {
		return fmt.Errorf("failed to check working tree status: %w", err)
	}
	if len(result.Stdout) > 0 {
		return errors.New("working tree must be clean (commit or stash changes first)")
	}
	return nil
}

// resolveGitRemoteURL retrieves the URL for the specified git remote.
func resolveGitRemoteURL(ctx context.Context, remoteName string) (string, error) {
	result, err := gitexec.Execute(ctx, gitexec.Request{Args: []string{"remote", "get-url", remoteName}})
	if err != nil {
		return "", fmt.Errorf("git remote %q not found", remoteName)
	}
	rawURL := strings.TrimSpace(string(result.Stdout))
	if rawURL == "" {
		return "", fmt.Errorf("git remote %q has no URL configured", remoteName)
	}
	return rawURL, nil
}

func resolveHEADSHA(ctx context.Context) (string, error) {
	result, err := gitexec.Execute(ctx, gitexec.Request{Args: []string{"rev-parse", "HEAD"}})
	if err != nil {
		return "", fmt.Errorf("failed to resolve HEAD: %w", err)
	}
	sha := strings.TrimSpace(string(result.Stdout))
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

// downloadAndApplyDiffs downloads and applies all diffs to the working tree.
// Returns the count of successfully applied diffs (excluding empty patches).
func downloadAndApplyDiffs(ctx context.Context, runID domaintypes.RunID, diffs []domainapi.DiffListItem, stderr io.Writer) (int, error) {
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

		downloadCmd := runs.DownloadDiffCommand{
			Client:  httpClient,
			BaseURL: base,
			RunID:   runID,
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
	result, err := gitexec.Execute(ctx, gitexec.Request{Args: []string{"apply"}, Stdin: patch})
	if err != nil {
		return fmt.Errorf("git apply failed: %w (stderr: %s)", err, strings.TrimSpace(string(result.Stderr)))
	}
	return nil
}
