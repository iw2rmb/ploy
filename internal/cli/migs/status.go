// status.go provides CLI client implementations for fetching diffs and patches.
//
// This file implements helpers used by `ploy run pull` and `ploy mig pull`:
//   - ListRunRepoDiffsCommand: Fetches repo-scoped diffs via GET /v1/runs/{run_id}/repos/{repo_id}/diffs (v1).
//   - DownloadDiffCommand: Downloads a single diff via GET /v1/runs/{run_id}/repos/{repo_id}/diffs?download=true&diff_id=<uuid>.
//   - DownloadDiffGzipCommand: Downloads raw gzip bytes for a single diff from the same endpoint.
package migs

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/httpx"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// DiffEntry represents a single diff record from the list diffs response.
type DiffEntry struct {
	ID        domaintypes.DiffID      `json:"id"`
	JobID     domaintypes.JobID       `json:"job_id"`
	CreatedAt time.Time               `json:"created_at"`
	Size      int                     `json:"gzipped_size"`
	Summary   domaintypes.DiffSummary `json:"summary,omitempty"`
}

// ListRunRepoDiffsCommand fetches repo-scoped diffs via GET /v1/runs/{run_id}/repos/{repo_id}/diffs.
// This is the v1 repo-scoped diffs listing endpoint.
//
// Returns diffs filtered by repo_id via jobs.repo_id join.
// Diffs for repo A are excluded from repo B listing.
// Response shape is unchanged from legacy endpoint.
type ListRunRepoDiffsCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	RunID   domaintypes.RunID // Run ID (KSUID-backed domain type)
	RepoID  domaintypes.MigRepoID
}

// Run executes GET /v1/runs/{run_id}/repos/{repo_id}/diffs and returns all diff entries.
// Diffs are returned in server-provided order (ordered by next_id, then created_at).
func (c ListRunRepoDiffsCommand) Run(ctx context.Context) ([]DiffEntry, error) {
	if err := httpx.RequireClientAndURL(c.Client, c.BaseURL); err != nil {
		return nil, fmt.Errorf("list run repo diffs: %w", err)
	}
	if c.RunID.IsZero() {
		return nil, fmt.Errorf("list run repo diffs: run id required")
	}
	if c.RepoID.IsZero() {
		return nil, fmt.Errorf("list run repo diffs: repo id required")
	}

	// Build endpoint: /v1/runs/{run_id}/repos/{repo_id}/diffs
	endpoint := c.BaseURL.JoinPath("v1", "runs", c.RunID.String(), "repos", c.RepoID.String(), "diffs")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("list run repo diffs: build request: %w", err)
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list run repo diffs: http request failed: %w", err)
	}
	defer httpx.DrainAndClose(resp)

	if resp.StatusCode != http.StatusOK {
		return nil, httpx.WrapError("list run repo diffs", resp.Status, resp.Body)
	}

	// Response structure: {"diffs": [...]}
	var result struct {
		Diffs []DiffEntry `json:"diffs"`
	}
	if err := httpx.DecodeResponseJSON(resp.Body, &result, httpx.MaxJSONBodyBytes); err != nil {
		return nil, fmt.Errorf("list run repo diffs: decode response: %w", err)
	}

	return result.Diffs, nil
}

// DownloadDiffCommand downloads a single diff patch via:
// GET /v1/runs/{run_id}/repos/{repo_id}/diffs?download=true&diff_id=<uuid>
// Returns the decompressed patch bytes ready for application via `git apply`.
type DownloadDiffCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	RunID   domaintypes.RunID // Run ID (KSUID-backed domain type)
	RepoID  domaintypes.MigRepoID
	DiffID  domaintypes.DiffID
	// Accumulated requests cumulative patch content up to DiffID.
	Accumulated bool
}

// DownloadDiffGzipCommand downloads a single diff patch as raw gzip bytes via:
// GET /v1/runs/{run_id}/repos/{repo_id}/diffs?download=true&diff_id=<uuid>
type DownloadDiffGzipCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	RunID   domaintypes.RunID // Run ID (KSUID-backed domain type)
	RepoID  domaintypes.MigRepoID
	DiffID  domaintypes.DiffID
	// Accumulated requests cumulative patch content up to DiffID.
	Accumulated bool
}

// Run executes GET /v1/runs/{run_id}/repos/{repo_id}/diffs?download=true&diff_id=<uuid>
// and returns the decompressed patch.
func (c DownloadDiffCommand) Run(ctx context.Context) ([]byte, error) {
	if err := httpx.RequireClientAndURL(c.Client, c.BaseURL); err != nil {
		return nil, fmt.Errorf("download diff: %w", err)
	}
	if c.RunID.IsZero() {
		return nil, fmt.Errorf("download diff: run id required")
	}
	if c.RepoID.IsZero() {
		return nil, fmt.Errorf("download diff: repo id required")
	}
	if c.DiffID.IsZero() {
		return nil, fmt.Errorf("download diff: diff id required")
	}

	// Build endpoint: /v1/runs/{run_id}/repos/{repo_id}/diffs?download=true&diff_id=<uuid>
	endpoint := c.BaseURL.JoinPath("v1", "runs", c.RunID.String(), "repos", c.RepoID.String(), "diffs")
	q := endpoint.Query()
	q.Set("download", "true")
	q.Set("diff_id", c.DiffID.String())
	if c.Accumulated {
		q.Set("accumulated", "true")
	}
	endpoint.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("download diff: build request: %w", err)
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download diff: http request failed: %w", err)
	}
	defer httpx.DrainAndClose(resp)

	if resp.StatusCode != http.StatusOK {
		return nil, httpx.WrapError("download diff", resp.Status, resp.Body)
	}

	patch, err := httpx.GunzipToBytes(io.LimitReader(resp.Body, httpx.MaxDownloadBodyBytes), httpx.MaxGunzipOutputBytes)
	if err != nil {
		return nil, fmt.Errorf("download diff: gunzip: %w", err)
	}

	return patch, nil
}

// Run executes GET /v1/runs/{run_id}/repos/{repo_id}/diffs?download=true&diff_id=<uuid>
// and returns raw gzip-compressed patch bytes from the server.
func (c DownloadDiffGzipCommand) Run(ctx context.Context) ([]byte, error) {
	if err := httpx.RequireClientAndURL(c.Client, c.BaseURL); err != nil {
		return nil, fmt.Errorf("download diff gzip: %w", err)
	}
	if c.RunID.IsZero() {
		return nil, fmt.Errorf("download diff gzip: run id required")
	}
	if c.RepoID.IsZero() {
		return nil, fmt.Errorf("download diff gzip: repo id required")
	}
	if c.DiffID.IsZero() {
		return nil, fmt.Errorf("download diff gzip: diff id required")
	}

	endpoint := c.BaseURL.JoinPath("v1", "runs", c.RunID.String(), "repos", c.RepoID.String(), "diffs")
	q := endpoint.Query()
	q.Set("download", "true")
	q.Set("diff_id", c.DiffID.String())
	if c.Accumulated {
		q.Set("accumulated", "true")
	}
	endpoint.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("download diff gzip: build request: %w", err)
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download diff gzip: http request failed: %w", err)
	}
	defer httpx.DrainAndClose(resp)

	if resp.StatusCode != http.StatusOK {
		return nil, httpx.WrapError("download diff gzip", resp.Status, resp.Body)
	}

	patchGzip, err := io.ReadAll(io.LimitReader(resp.Body, httpx.MaxDownloadBodyBytes))
	if err != nil {
		return nil, fmt.Errorf("download diff gzip: read body: %w", err)
	}
	return patchGzip, nil
}
