package run

import (
	"bytes"
	"context"
	"encoding/json"
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

type resolvedRunRepoForApply struct {
	RunID           domaintypes.RunID  `json:"run_id"`
	RepoID          domaintypes.RepoID `json:"repo_id"`
	RepoURL         string             `json:"repo_url"`
	SourceCommitSHA string             `json:"source_commit_sha"`
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

	resolved, err := resolveRunRepoForApply(ctx, httpClient, base, runID, local.RepoURL)
	if err != nil {
		return fmt.Errorf("run apply: %w", err)
	}
	sourceSHA := strings.TrimSpace(resolved.SourceCommitSHA)
	if sourceSHA == "" {
		return errors.New("run apply: source_commit_sha is not available for this run")
	}
	if !strings.EqualFold(local.CommitSHA, sourceSHA) && !opts.Force {
		return fmt.Errorf("run apply: local HEAD %s does not match run source_commit_sha %s; use --force to apply anyway", local.CommitSHA, sourceSHA)
	}

	repoID := domaintypes.MigRepoID(resolved.RepoID.String())
	diffs, err := migs.ListRunRepoDiffsCommand{
		Client:  httpClient,
		BaseURL: base,
		RunID:   runID,
		RepoID:  repoID,
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
		RepoID:      repoID,
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

func resolveRunRepoForApply(ctx context.Context, httpClient *http.Client, baseURL *url.URL, runID domaintypes.RunID, repoURL string) (resolvedRunRepoForApply, error) {
	if baseURL == nil {
		return resolvedRunRepoForApply{}, errors.New("base url required")
	}
	endpoint := baseURL.JoinPath("v1", "runs", runID.String(), "repos", "resolve")
	payload, err := json.Marshal(struct {
		RepoURL string `json:"repo_url"`
	}{RepoURL: repoURL})
	if err != nil {
		return resolvedRunRepoForApply{}, fmt.Errorf("resolve repo: marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(payload))
	if err != nil {
		return resolvedRunRepoForApply{}, fmt.Errorf("resolve repo: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return resolvedRunRepoForApply{}, fmt.Errorf("resolve repo: http request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		var apiErr struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(body, &apiErr); err == nil && strings.TrimSpace(apiErr.Error) != "" {
			return resolvedRunRepoForApply{}, fmt.Errorf("resolve repo: %s", strings.TrimSpace(apiErr.Error))
		}
		return resolvedRunRepoForApply{}, fmt.Errorf("resolve repo: %s", strings.TrimSpace(string(body)))
	}
	var result resolvedRunRepoForApply
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return resolvedRunRepoForApply{}, fmt.Errorf("resolve repo: decode response: %w", err)
	}
	if result.RepoID.IsZero() {
		return resolvedRunRepoForApply{}, errors.New("resolve repo: empty repo_id in response")
	}
	return result, nil
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
