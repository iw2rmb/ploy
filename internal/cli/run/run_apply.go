package run

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
	"time"

	"github.com/iw2rmb/ploy/internal/cli/common"
	"github.com/iw2rmb/ploy/internal/cli/migs"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

type ApplyOptions struct {
	RunID    string
	RepoPath string
	Force    bool
	Output   io.Writer
}

func RunApply(ctx context.Context, opts ApplyOptions) error {
	base, httpClient, err := common.ResolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}
	return runApply(ctx, opts, base, httpClient)
}

func runApply(ctx context.Context, opts ApplyOptions, base *url.URL, httpClient *http.Client) error {
	runIDValue := strings.TrimSpace(opts.RunID)
	if runIDValue == "" {
		return errors.New("run-id required")
	}
	runID := domaintypes.RunID(runIDValue)
	repoPath := strings.TrimSpace(opts.RepoPath)
	if repoPath == "" {
		repoPath = "."
	}
	out := opts.Output
	if out == nil {
		out = io.Discard
	}

	local, err := resolveLocalRunRepo(ctx, repoPath)
	if err != nil {
		return fmt.Errorf("run apply: %w", err)
	}
	if err := ensureNoGitDiff(ctx, local.Worktree); err != nil {
		return fmt.Errorf("run apply: %w", err)
	}

	resolved, err := migs.RunPullCommand{
		Client:  httpClient,
		BaseURL: base,
		RunID:   runID,
	}.Run(ctx)
	if err != nil {
		return fmt.Errorf("run apply: %w", err)
	}
	if domaintypes.NormalizeRepoURL(resolved.RepoURL) != domaintypes.NormalizeRepoURL(local.RepoURL) {
		return fmt.Errorf("run apply: local origin %s does not match run repo_url %s", local.RepoURL, resolved.RepoURL)
	}
	sourceSHA := strings.TrimSpace(resolved.SourceCommitSHA)
	if sourceSHA == "" {
		return errors.New("run apply: source_commit_sha is not available for this run")
	}
	if !strings.EqualFold(local.CommitSHA, sourceSHA) && !opts.Force {
		return fmt.Errorf("run apply: local HEAD %s does not match run source_commit_sha %s; use --force to apply anyway", local.CommitSHA, sourceSHA)
	}

	diffs, err := migs.ListRunDiffsCommand{
		Client:  httpClient,
		BaseURL: base,
		RunID:   runID,
		RepoID:  resolved.RepoID,
	}.Run(ctx)
	if err != nil {
		return fmt.Errorf("run apply: list diffs: %w", err)
	}
	if len(diffs) == 0 {
		_, _ = fmt.Fprintf(out, "No diffs available for run %s.\n", runID.String())
		return nil
	}

	latest := diffs[len(diffs)-1]
	patch, err := migs.DownloadDiffCommand{
		Client:      httpClient,
		BaseURL:     base,
		RunID:       runID,
		RepoID:      resolved.RepoID,
		DiffID:      latest.ID,
		Accumulated: true,
	}.Run(ctx)
	if err != nil {
		return fmt.Errorf("run apply: download patch: %w", err)
	}
	if len(bytes.TrimSpace(patch)) == 0 {
		_, _ = fmt.Fprintf(out, "Patch for run %s is empty.\n", runID.String())
		return nil
	}
	if err := gitApplyPatch(ctx, local.Worktree, patch); err != nil {
		return fmt.Errorf("run apply: %w", err)
	}
	_, _ = fmt.Fprintf(out, "Applied patch from run %s to %s\n", runID.String(), local.Worktree)
	return nil
}

func ensureNoGitDiff(ctx context.Context, worktree string) error {
	cmd := exec.CommandContext(ctx, "git", "-C", worktree, "diff", "--quiet", "HEAD", "--")
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=echo")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() == 1 {
			return errors.New("working tree must have no staged or unstaged diff")
		}
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return fmt.Errorf("check working tree diff: %w: %s", err, msg)
		}
		return fmt.Errorf("check working tree diff: %w", err)
	}
	return nil
}

func gitApplyPatch(ctx context.Context, worktree string, patch []byte) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "-C", worktree, "apply")
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=echo")
	cmd.Stdin = bytes.NewReader(patch)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git apply failed: %w (stderr: %s)", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}
