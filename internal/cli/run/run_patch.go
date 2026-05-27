package run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/common"
	"github.com/iw2rmb/ploy/internal/cli/migs"
	"github.com/iw2rmb/ploy/internal/cli/pull"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

type PatchOptions struct {
	RunID      string
	RepoID     string
	RepoURL    string
	DiffID     string
	OutputPath string
	Output     io.Writer
}

// RunPatch implements:
//
//	ploy run patch [--repo-id <id> | --repo-url <url>] [--diff-id <uuid>] [--output <path|->] <run-id>
//
// It is a read-only command: it downloads the stored patch artifact and does not apply it.
func RunPatch(ctx context.Context, opts PatchOptions) error {
	runIDValue := strings.TrimSpace(opts.RunID)
	if runIDValue == "" {
		return errors.New("run-id required")
	}
	out := opts.Output
	if out == nil {
		out = io.Discard
	}

	runID := domaintypes.RunID(runIDValue)

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	base, httpClient, err := common.ResolveControlPlaneHTTP(ctx)
	if err != nil {
		return fmt.Errorf("run patch: %w", err)
	}

	repoID, err := resolveRunPatchRepoID(ctx, httpClient, base, runID, strings.TrimSpace(opts.RepoID), strings.TrimSpace(opts.RepoURL))
	if err != nil {
		return fmt.Errorf("run patch: %w", err)
	}

	diffs, err := pull.ListRunRepoDiffs(ctx, httpClient, base, runID, repoID)
	if err != nil {
		return fmt.Errorf("run patch: list diffs: %w", err)
	}
	if len(diffs) == 0 {
		return errors.New("run patch: no diffs available for this repo execution")
	}

	selectedDiff, err := resolveRunPatchDiffID(diffs, strings.TrimSpace(opts.DiffID))
	if err != nil {
		return fmt.Errorf("run patch: %w", err)
	}

	downloadCmd := migs.DownloadDiffGzipCommand{
		Client:      httpClient,
		BaseURL:     base,
		RunID:       runID,
		RepoID:      repoID,
		DiffID:      selectedDiff,
		Accumulated: true,
	}
	patchGzip, err := downloadCmd.Run(ctx)
	if err != nil {
		return fmt.Errorf("run patch: download patch: %w", err)
	}

	if err := writeRunPatchOutput(strings.TrimSpace(opts.OutputPath), patchGzip, out); err != nil {
		return fmt.Errorf("run patch: write output: %w", err)
	}

	return nil
}

func resolveRunPatchRepoID(
	ctx context.Context,
	httpClient *http.Client,
	baseURL *url.URL,
	runID domaintypes.RunID,
	repoIDFlag string,
	repoURLFlag string,
) (domaintypes.MigRepoID, error) {
	if repoIDFlag != "" && repoURLFlag != "" {
		return "", errors.New("--repo-id and --repo-url are mutually exclusive")
	}

	if repoIDFlag != "" {
		var repoID domaintypes.MigRepoID
		if err := repoID.UnmarshalText([]byte(repoIDFlag)); err != nil {
			return "", errors.New("invalid --repo-id")
		}
		return repoID, nil
	}

	if repoURLFlag != "" {
		if err := domaintypes.RepoURL(repoURLFlag).Validate(); err != nil {
			return "", errors.New("invalid --repo-url")
		}

		pullCmd := migs.RunPullCommand{
			Client:  httpClient,
			BaseURL: baseURL,
			RunID:   runID,
			RepoURL: repoURLFlag,
		}
		resolution, err := pullCmd.Run(ctx)
		if err != nil {
			return "", fmt.Errorf("resolve repo: %w", err)
		}
		return resolution.RepoID, nil
	}

	repos, err := listRunPatchRepos(ctx, httpClient, baseURL, runID)
	if err != nil {
		return "", fmt.Errorf("list run repos: %w", err)
	}
	switch len(repos) {
	case 0:
		return "", errors.New("run has no repos")
	case 1:
		return repos[0].RepoID, nil
	default:
		return "", errors.New("multiple repos found in run; provide --repo-id or --repo-url")
	}
}

func resolveRunPatchDiffID(diffs []migs.DiffEntry, diffIDFlag string) (domaintypes.DiffID, error) {
	if diffIDFlag == "" {
		// API ordering is ascending by execution chain / created_at; last is newest.
		return diffs[len(diffs)-1].ID, nil
	}

	var diffID domaintypes.DiffID
	if err := diffID.UnmarshalText([]byte(diffIDFlag)); err != nil {
		return "", errors.New("invalid --diff-id")
	}

	for _, item := range diffs {
		if item.ID == diffID {
			return diffID, nil
		}
	}
	return "", fmt.Errorf("diff %s not found in run repo diff listing", diffID)
}

func writeRunPatchOutput(outputPath string, patchGzip []byte, out io.Writer) error {
	if outputPath == "" || outputPath == "-" {
		_, err := out.Write(patchGzip)
		return err
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0o750); err != nil {
		return err
	}
	return os.WriteFile(outputPath, patchGzip, 0o600)
}

type runPatchRepoEntry struct {
	RepoID  domaintypes.MigRepoID `json:"repo_id"`
	RepoURL string                `json:"repo_url"`
}

func listRunPatchRepos(
	ctx context.Context,
	httpClient *http.Client,
	baseURL *url.URL,
	runID domaintypes.RunID,
) ([]runPatchRepoEntry, error) {
	if baseURL == nil {
		return nil, errors.New("base url required")
	}

	endpoint := baseURL.JoinPath("v1", "runs", runID.String(), "repos")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var result struct {
		Repos []runPatchRepoEntry `json:"repos"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if result.Repos == nil {
		result.Repos = make([]runPatchRepoEntry, 0)
	}
	return result.Repos, nil
}
